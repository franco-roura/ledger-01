package ledger

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/google/uuid"
)

type BudgetRepeat = dbq.BudgetRepeat

const (
	RepeatOnce     = dbq.BudgetRepeatOnce
	RepeatDaily    = dbq.BudgetRepeatDaily
	RepeatWeekly   = dbq.BudgetRepeatWeekly
	RepeatBiweekly = dbq.BudgetRepeatBiweekly
	RepeatMonthly  = dbq.BudgetRepeatMonthly
	RepeatYearly   = dbq.BudgetRepeatYearly
)

// maxBudgetHistory caps how many periods a budget detail returns.
const maxBudgetHistory = 60

type Budget struct {
	ID            uuid.UUID
	Name          string
	Amount        int64
	Currency      string
	Repeat        BudgetRepeat
	Start         time.Time
	End           *time.Time // only for RepeatOnce
	WalletIDs     []uuid.UUID
	CategoryIDs   []uuid.UUID
	AllWallets    bool
	AllCategories bool
	// Rollover: the leftover and/or the overspend of each period carries into the next, chaining
	// from the period starting at RolloverFrom (set while either flag is on).
	CarryLeftover  bool
	CarryOverspend bool
	RolloverFrom   *time.Time
}

type BudgetInput struct {
	Name          string
	Amount        int64
	Currency      string
	Repeat        BudgetRepeat
	Start         time.Time
	End           *time.Time
	WalletIDs     []uuid.UUID
	CategoryIDs   []uuid.UUID
	AllWallets    *bool
	AllCategories *bool
	// nil keeps the current value (false for a new budget).
	CarryLeftover  *bool
	CarryOverspend *bool
	// Turning rollover on starts the chain at the period containing Today (zero means UTC today).
	Today time.Time
}

type BudgetState int

const (
	BudgetUpcoming BudgetState = iota + 1
	BudgetActive
	BudgetEnded
)

// BudgetProgress is the spending in one period, in the budget's currency.
type BudgetProgress struct {
	From, To       time.Time
	State          BudgetState
	Spent          int64
	Carried        int64 // carried in from the previous period: + leftover, - overspend
	Limit          int64 // Amount + Carried; can be zero or negative
	Remaining      int64 // Limit - Spent
	DailyAllowance int64
	DaysTotal      int
	DaysLeft       int
}

type BudgetSummary struct {
	Budget  Budget
	Current BudgetProgress
}

type WalletTotal struct {
	WalletID uuid.UUID
	Amount   int64
	Count    int64
}

type BudgetDetail struct {
	Budget       Budget
	Progress     BudgetProgress
	History      []BudgetProgress // newest first
	Categories   []CategoryTotal
	Wallets      []WalletTotal
	Transactions []Transaction
	Unconverted  []string
}

// --- periods -----------------------------------------------------------------------------------

func daysBetween(from, to time.Time) int { return int(to.Sub(from).Hours() / 24) }

// addMonthsClamped moves d by n months, keeping its day of the month but clamping it to the
// target month's last day (Jan 31 + 1 month = Feb 28).
func addMonthsClamped(d time.Time, n int) time.Time {
	m := int(d.Month()) - 1 + n
	y := d.Year() + m/12
	if m %= 12; m < 0 {
		m += 12
		y--
	}
	first := time.Date(y, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(d.Day(), last)-1)
}

// periodStart is the first day of the budget's i-th period (0-based).
func (b Budget) periodStart(i int) time.Time {
	switch b.Repeat {
	case RepeatDaily:
		return b.Start.AddDate(0, 0, i)
	case RepeatWeekly:
		return b.Start.AddDate(0, 0, 7*i)
	case RepeatBiweekly:
		return b.Start.AddDate(0, 0, 14*i)
	case RepeatMonthly:
		return addMonthsClamped(b.Start, i)
	case RepeatYearly:
		return addMonthsClamped(b.Start, 12*i)
	}
	return b.Start
}

// period returns the first and last day of the i-th period.
func (b Budget) period(i int) (time.Time, time.Time) {
	if b.Repeat == RepeatOnce && b.End != nil {
		return b.Start, *b.End
	}
	return b.periodStart(i), b.periodStart(i+1).AddDate(0, 0, -1)
}

// periodIndex returns the period containing d: 0 before the start, and 0 for one-time budgets.
func (b Budget) periodIndex(d time.Time) int {
	if b.Repeat == RepeatOnce || d.Before(b.Start) {
		return 0
	}
	var i int
	switch b.Repeat {
	case RepeatDaily:
		return daysBetween(b.Start, d)
	case RepeatWeekly:
		return daysBetween(b.Start, d) / 7
	case RepeatBiweekly:
		return daysBetween(b.Start, d) / 14
	case RepeatMonthly:
		i = (d.Year()-b.Start.Year())*12 + int(d.Month()) - int(b.Start.Month())
	case RepeatYearly:
		i = d.Year() - b.Start.Year()
	}
	for i > 0 && b.periodStart(i).After(d) {
		i--
	}
	for !b.periodStart(i + 1).After(d) {
		i++
	}
	return i
}

// rolls reports whether the budget carries anything between periods.
func (b Budget) rolls() bool {
	return b.Repeat != RepeatOnce && b.RolloverFrom != nil && (b.CarryLeftover || b.CarryOverspend)
}

// chainStart is the first period whose spending the carry into period cur depends on (cur itself
// without rollover).
func (b Budget) chainStart(cur int) int {
	if !b.rolls() {
		return cur
	}
	return min(cur, b.periodIndex(*b.RolloverFrom))
}

// carries returns what each period up to cur carries in from the one before, given the spending of
// every period from chainStart(cur). A period's result is its limit (amount plus its own carry)
// minus its spending; a positive result carries on with CarryLeftover, a negative one with
// CarryOverspend. There is no floor: a big overspend can push later limits to zero or below until
// cheaper periods pay it back. Periods missing from the map carry nothing.
func (b Budget) carries(spent map[int]int64, cur int) map[int]int64 {
	out := map[int]int64{}
	if !b.rolls() {
		return out
	}
	for i := b.chainStart(cur); i < cur; i++ {
		res := b.Amount + out[i] - spent[i]
		switch {
		case res > 0 && b.CarryLeftover:
			out[i+1] = res
		case res < 0 && b.CarryOverspend:
			out[i+1] = res
		}
	}
	return out
}

// progress measures one period's spending as of today, against its amount plus what it carried in.
func (b Budget) progress(i int, spent, carried int64, today time.Time) BudgetProgress {
	from, to := b.period(i)
	limit := b.Amount + carried
	p := BudgetProgress{
		From: from, To: to, Spent: spent, Carried: carried, Limit: limit, Remaining: limit - spent,
		DaysTotal: daysBetween(from, to) + 1,
	}
	switch {
	case today.Before(from):
		p.State, p.DaysLeft = BudgetUpcoming, p.DaysTotal
	case today.After(to):
		p.State = BudgetEnded
	default:
		p.State, p.DaysLeft = BudgetActive, daysBetween(today, to)+1
	}
	if p.Remaining > 0 && p.DaysLeft > 0 {
		p.DailyAllowance = p.Remaining / int64(p.DaysLeft)
	}
	return p
}

// --- reads -------------------------------------------------------------------------------------

func budgetFrom(r dbq.Budget, walletIDs, categoryIDs []uuid.UUID) Budget {
	return Budget{
		ID: r.ID, Name: r.Name, Amount: r.AmountMinor, Currency: strings.TrimSpace(r.Currency), Repeat: r.Repeat,
		Start: r.StartDate, End: r.EndDate, WalletIDs: walletIDs, CategoryIDs: categoryIDs,
		AllWallets: r.AllWallets, AllCategories: r.AllCategories,
		CarryLeftover: r.CarryLeftover, CarryOverspend: r.CarryOverspend, RolloverFrom: r.RolloverFrom,
	}
}

func (s *Service) getBudget(ctx context.Context, a Actor, id uuid.UUID) (Budget, error) {
	r, err := s.q.GetBudget(ctx, dbq.GetBudgetParams{ID: id, HouseholdID: a.HouseholdID})
	if err != nil {
		return Budget{}, dbErr(err, "budget")
	}
	return budgetFrom(r.Budget, r.WalletIds, r.CategoryIds), nil
}

// budgetExpenses returns the expenses a budget counts in [from, to], leaving out upcoming ones
// (dated after today).
func (s *Service) budgetExpenses(ctx context.Context, a Actor, b Budget, from, to, today time.Time) ([]dbq.BudgetExpensesRow, error) {
	return s.q.BudgetExpenses(ctx, dbq.BudgetExpensesParams{
		HouseholdID: a.HouseholdID, FromDate: from, ToDate: to, Today: today, WalletIds: b.WalletIDs, CategoryIds: b.CategoryIDs,
		AllWallets: b.AllWallets, AllCategories: b.AllCategories,
	})
}

// spendConverter converts expenses into a budget's currency and remembers what it couldn't.
type spendConverter struct {
	conv    *Converter
	missing map[string]bool
}

func (c *spendConverter) convert(row dbq.BudgetExpensesRow, to string) (int64, bool) {
	cur := strings.TrimSpace(row.Currency)
	v, ok := c.conv.Convert(row.AmountMinor, cur, to, row.OccurredOn)
	if !ok {
		c.missing[cur] = true
	}
	return v, ok
}

func (c *spendConverter) unconverted() []string {
	out := make([]string, 0, len(c.missing))
	for cur := range c.missing {
		out = append(out, cur)
	}
	slices.Sort(out)
	return out
}

func (s *Service) spendConverter(ctx context.Context, a Actor, today time.Time) (*spendConverter, error) {
	conv, err := s.converter(ctx, a, today)
	if err != nil {
		return nil, err
	}
	return &spendConverter{conv: conv, missing: map[string]bool{}}, nil
}

// ListBudgets returns every budget with the progress of the period containing today.
func (s *Service) ListBudgets(ctx context.Context, a Actor, today time.Time) ([]BudgetSummary, []string, error) {
	rows, err := s.q.ListBudgets(ctx, a.HouseholdID)
	if err != nil {
		return nil, nil, err
	}
	sc, err := s.spendConverter(ctx, a, today)
	if err != nil {
		return nil, nil, err
	}
	out := make([]BudgetSummary, len(rows))
	for n, r := range rows {
		b := budgetFrom(r.Budget, r.WalletIds, r.CategoryIds)
		i := b.periodIndex(today)
		from, _ := b.period(b.chainStart(i))
		_, to := b.period(i)
		exp, err := s.budgetExpenses(ctx, a, b, from, to, today)
		if err != nil {
			return nil, nil, err
		}
		spent := map[int]int64{}
		for _, e := range exp {
			if v, ok := sc.convert(e, b.Currency); ok {
				spent[b.periodIndex(e.OccurredOn)] += v
			}
		}
		out[n] = BudgetSummary{Budget: b, Current: b.progress(i, spent[i], b.carries(spent, i)[i], today)}
	}
	return out, sc.unconverted(), nil
}

// BudgetDetailFor returns one period of a budget (the one containing periodDate, or today's)
// with its breakdowns, plus the history of every period up to today's.
func (s *Service) BudgetDetailFor(ctx context.Context, a Actor, id uuid.UUID, today time.Time, periodDate *time.Time) (BudgetDetail, error) {
	b, err := s.getBudget(ctx, a, id)
	if err != nil {
		return BudgetDetail{}, err
	}
	cur := b.periodIndex(today)
	sel := cur
	if periodDate != nil {
		sel = min(b.periodIndex(*periodDate), cur)
	}
	lo := max(0, cur-maxBudgetHistory+1)
	from, _ := b.period(min(lo, sel, b.chainStart(cur)))
	_, to := b.period(cur)
	exp, err := s.budgetExpenses(ctx, a, b, from, to, today)
	if err != nil {
		return BudgetDetail{}, err
	}
	sc, err := s.spendConverter(ctx, a, today)
	if err != nil {
		return BudgetDetail{}, err
	}

	spent := map[int]int64{}
	cats := map[uuid.UUID]*CategoryTotal{}
	wallets := map[uuid.UUID]*WalletTotal{}
	out := BudgetDetail{Budget: b}
	for _, e := range exp {
		i := b.periodIndex(e.OccurredOn)
		if i == sel && len(out.Transactions) < 200 {
			out.Transactions = append(out.Transactions, Transaction{
				ID: e.ID, WalletID: e.WalletID, Kind: e.Kind, CategoryID: e.CategoryID, Amount: e.AmountMinor,
				Currency: strings.TrimSpace(e.Currency), OccurredOn: e.OccurredOn, Note: e.Note, LabelIDs: e.LabelIds,
				CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
			})
		}
		v, ok := sc.convert(e, b.Currency)
		if !ok {
			continue
		}
		spent[i] += v
		if i != sel {
			continue
		}
		if e.CategoryID != nil {
			ct := cats[*e.CategoryID]
			if ct == nil {
				ct = &CategoryTotal{CategoryID: *e.CategoryID}
				cats[*e.CategoryID] = ct
			}
			ct.Amount += v
			ct.Count++
		}
		wt := wallets[e.WalletID]
		if wt == nil {
			wt = &WalletTotal{WalletID: e.WalletID}
			wallets[e.WalletID] = wt
		}
		wt.Amount += v
		wt.Count++
	}

	carried := b.carries(spent, cur)
	out.Progress = b.progress(sel, spent[sel], carried[sel], today)
	for i := cur; i >= lo; i-- {
		out.History = append(out.History, b.progress(i, spent[i], carried[i], today))
	}
	out.Categories = sortedTotals(cats)
	for _, wt := range wallets {
		out.Wallets = append(out.Wallets, *wt)
	}
	slices.SortFunc(out.Wallets, func(x, y WalletTotal) int {
		if x.Amount != y.Amount {
			if x.Amount > y.Amount {
				return -1
			}
			return 1
		}
		return strings.Compare(x.WalletID.String(), y.WalletID.String())
	})
	out.Unconverted = sc.unconverted()
	return out, nil
}

// --- writes ------------------------------------------------------------------------------------

func (s *Service) checkBudget(ctx context.Context, q *dbq.Queries, a Actor, in *BudgetInput) error {
	name, err := cleanName("name", in.Name, 60)
	if err != nil {
		return err
	}
	in.Name = name
	if err := checkAmount("amount_minor", in.Amount, false); err != nil {
		return err
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if _, err := q.GetCurrency(ctx, in.Currency); err != nil {
		return invalid("currency", "unknown currency %q", in.Currency)
	}
	switch in.Repeat {
	case RepeatOnce, RepeatDaily, RepeatWeekly, RepeatBiweekly, RepeatMonthly, RepeatYearly:
	default:
		return invalid("repeat", "choose how often the budget repeats")
	}
	if in.Start.IsZero() {
		return invalid("start_date", "is required")
	}
	if in.Repeat == RepeatOnce {
		if in.End == nil {
			return invalid("end_date", "is required for a one-time budget")
		}
		if in.End.Before(in.Start) {
			return invalid("end_date", "must not be before the start date")
		}
	} else {
		in.End = nil
	}
	if in.Repeat == RepeatOnce {
		if in.CarryLeftover != nil && *in.CarryLeftover {
			return invalid("carry_leftover", "only repeating budgets can carry over")
		}
		if in.CarryOverspend != nil && *in.CarryOverspend {
			return invalid("carry_overspend", "only repeating budgets can carry over")
		}
	}

	if budgetAll(in.AllWallets, in.WalletIDs) && len(in.WalletIDs) > 0 {
		return invalid("wallet_ids", "choose all wallets or specific wallets")
	}
	if budgetAll(in.AllCategories, in.CategoryIDs) && len(in.CategoryIDs) > 0 {
		return invalid("category_ids", "choose all categories or specific categories")
	}
	in.WalletIDs = uniqueIDs(in.WalletIDs)
	if len(in.WalletIDs) > 100 {
		return invalid("wallet_ids", "at most 100 wallets")
	}
	if len(in.WalletIDs) > 0 {
		n, err := q.CountWalletsByIDs(ctx, dbq.CountWalletsByIDsParams{HouseholdID: a.HouseholdID, Ids: in.WalletIDs})
		if err != nil {
			return err
		}
		if int(n) != len(in.WalletIDs) {
			return invalid("wallet_ids", "some wallets don't exist")
		}
	}
	in.CategoryIDs = uniqueIDs(in.CategoryIDs)
	if len(in.CategoryIDs) > 200 {
		return invalid("category_ids", "at most 200 categories")
	}
	if len(in.CategoryIDs) > 0 {
		n, err := q.CountExpenseCategoriesByIDs(ctx, dbq.CountExpenseCategoriesByIDsParams{HouseholdID: a.HouseholdID, Ids: in.CategoryIDs})
		if err != nil {
			return err
		}
		if int(n) != len(in.CategoryIDs) {
			return invalid("category_ids", "only expense categories can be budgeted")
		}
	}
	return nil
}

func setBudgetLinks(ctx context.Context, q *dbq.Queries, id uuid.UUID, in BudgetInput) error {
	if err := q.ClearBudgetWallets(ctx, id); err != nil {
		return err
	}
	if err := q.AddBudgetWallets(ctx, dbq.AddBudgetWalletsParams{BudgetID: id, WalletIds: in.WalletIDs}); err != nil {
		return err
	}
	if err := q.ClearBudgetCategories(ctx, id); err != nil {
		return err
	}
	return q.AddBudgetCategories(ctx, dbq.AddBudgetCategoriesParams{BudgetID: id, CategoryIds: in.CategoryIDs})
}

// rollover resolves a write's carry flags against the stored budget (nil for a new one) and returns
// them with the chain's start: kept while rollover stays on, the period containing today when it
// is turned on, nil when it is off. One-time budgets never roll over.
func rollover(in BudgetInput, current *dbq.Budget) (leftover, overspend bool, from *time.Time) {
	pick := func(flag *bool, cur bool) bool {
		if flag != nil {
			return *flag
		}
		return cur
	}
	var wasOn bool
	if current != nil {
		leftover, overspend = current.CarryLeftover, current.CarryOverspend
		wasOn = (leftover || overspend) && current.RolloverFrom != nil
	}
	leftover, overspend = pick(in.CarryLeftover, leftover), pick(in.CarryOverspend, overspend)
	if in.Repeat == RepeatOnce || !(leftover || overspend) {
		return false, false, nil
	}
	if wasOn {
		return leftover, overspend, current.RolloverFrom
	}
	today := in.Today
	if today.IsZero() {
		today = Today()
	}
	b := Budget{Repeat: in.Repeat, Start: in.Start}
	start := b.periodStart(b.periodIndex(today))
	return leftover, overspend, &start
}

// budgetAll uses the legacy default for old clients and backups that omit the flag.
func budgetAll(flag *bool, ids []uuid.UUID) bool {
	if flag != nil {
		return *flag
	}
	return len(ids) == 0
}

func (s *Service) CreateBudget(ctx context.Context, a Actor, in BudgetInput) (Budget, error) {
	id := newID()
	err := s.inTx(ctx, func(q *dbq.Queries) error {
		if err := s.checkBudget(ctx, q, a, &in); err != nil {
			return err
		}
		leftover, overspend, from := rollover(in, nil)
		if _, err := q.CreateBudget(ctx, dbq.CreateBudgetParams{
			ID: id, HouseholdID: a.HouseholdID, Name: in.Name, AmountMinor: in.Amount, Currency: in.Currency,
			Repeat: in.Repeat, StartDate: in.Start, EndDate: in.End,
			AllWallets: budgetAll(in.AllWallets, in.WalletIDs), AllCategories: budgetAll(in.AllCategories, in.CategoryIDs),
			CarryLeftover: leftover, CarryOverspend: overspend, RolloverFrom: from,
		}); err != nil {
			return dbErr(err, "budget")
		}
		return setBudgetLinks(ctx, q, id, in)
	})
	if err != nil {
		return Budget{}, err
	}
	return s.getBudget(ctx, a, id)
}

func (s *Service) UpdateBudget(ctx context.Context, a Actor, id uuid.UUID, in BudgetInput) (Budget, error) {
	err := s.inTx(ctx, func(q *dbq.Queries) error {
		if err := s.checkBudget(ctx, q, a, &in); err != nil {
			return err
		}
		current, err := q.GetBudget(ctx, dbq.GetBudgetParams{ID: id, HouseholdID: a.HouseholdID})
		if err != nil {
			return dbErr(err, "budget")
		}
		// An older client editing an exhausted scope must not turn it into "all".
		if in.AllWallets == nil && len(current.WalletIds) == 0 && len(in.WalletIDs) == 0 {
			in.AllWallets = &current.Budget.AllWallets
		}
		if in.AllCategories == nil && len(current.CategoryIds) == 0 && len(in.CategoryIDs) == 0 {
			in.AllCategories = &current.Budget.AllCategories
		}
		leftover, overspend, from := rollover(in, &current.Budget)
		n, err := q.UpdateBudget(ctx, dbq.UpdateBudgetParams{
			ID: id, HouseholdID: a.HouseholdID, Name: in.Name, AmountMinor: in.Amount, Currency: in.Currency,
			Repeat: in.Repeat, StartDate: in.Start, EndDate: in.End,
			AllWallets: budgetAll(in.AllWallets, in.WalletIDs), AllCategories: budgetAll(in.AllCategories, in.CategoryIDs),
			CarryLeftover: leftover, CarryOverspend: overspend, RolloverFrom: from,
		})
		if err != nil {
			return dbErr(err, "budget")
		}
		if n == 0 {
			return ErrNotFound
		}
		return setBudgetLinks(ctx, q, id, in)
	})
	if err != nil {
		return Budget{}, err
	}
	return s.getBudget(ctx, a, id)
}

func (s *Service) DeleteBudget(ctx context.Context, a Actor, id uuid.UUID) error {
	n, err := s.q.DeleteBudget(ctx, dbq.DeleteBudgetParams{ID: id, HouseholdID: a.HouseholdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
