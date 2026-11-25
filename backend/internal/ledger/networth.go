package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/google/uuid"
)

type NetWorthPoint struct {
	Date        time.Time
	Assets      int64
	Liabilities int64 // owed, positive
	NetWorth    int64
}

type NetWorthWallet struct {
	WalletID uuid.UUID
	Type     WalletType
	Currency string
	// In the wallet's currency: the value of an asset, or what a liability owes (positive).
	Amount int64
	// Amount in the report currency; 0 when the currency has no rate.
	Converted int64
	// Liabilities only, in the wallet's currency: expenses minus income still to come.
	UpcomingDebt int64
}

type NetWorth struct {
	Currency    string
	Assets      int64
	Liabilities int64
	NetWorth    int64
	// Liabilities' upcoming debt in the report currency, at today's rate.
	UpcomingDebt           int64
	Series                 []NetWorthPoint
	Wallets                []NetWorthWallet
	UpcomingDebtCategories []CategoryTotal
	Unconverted            []string
}

// NetWorthFor adds up every visible wallet: asset values minus what liabilities owe, as of today,
// plus a history series over [f.From, f.To]. Only f.From, f.To, f.Today and f.Currency are used.
//
// Liabilities also report their upcoming debt: expenses minus income dated after today, plus the
// occurrences recurring rules that end have not created yet (the rest of an installment plan).
// It is debt still to come, never spending, so no other report counts it.
func (s *Service) NetWorthFor(ctx context.Context, a Actor, f ReportFilter, b Bucket) (NetWorth, error) {
	f = ReportFilter{From: f.From, To: f.To, Today: orToday(f.Today), Currency: f.Currency}
	r, err := s.loadReport(ctx, a, f, b)
	if err != nil {
		return NetWorth{}, err
	}
	today := f.Today
	vals, err := s.valuations(ctx, a, today)
	if err != nil {
		return NetWorth{}, err
	}
	out := NetWorth{Currency: r.base}

	// Totals as of today, over the same wallets as the series.
	ws, err := s.ListWallets(ctx, a, false, false, today)
	if err != nil {
		return NetWorth{}, err
	}
	var assets, liabilities []NetWorthWallet
	var liabilityIDs []uuid.UUID
	for _, w := range ws {
		nw := NetWorthWallet{WalletID: w.ID, Type: w.Type, Currency: w.Currency}
		if IsLiability(w.Type) {
			nw.Amount = -w.Balance
			nw.Converted, _ = r.convert(nw.Amount, w.Currency, today)
			out.Liabilities += nw.Converted
			liabilityIDs = append(liabilityIDs, w.ID)
			liabilities = append(liabilities, nw)
			continue
		}
		nw.Amount = w.Value
		nw.Converted, _ = r.convert(nw.Amount, w.Currency, today)
		out.Assets += nw.Converted
		assets = append(assets, nw)
	}
	out.NetWorth = out.Assets - out.Liabilities

	upcoming, cats, err := s.upcomingDebt(ctx, a, liabilityIDs, today)
	if err != nil {
		return NetWorth{}, err
	}
	for i := range liabilities {
		liabilities[i].UpcomingDebt = upcoming[liabilities[i].WalletID]
		v, _ := r.convert(liabilities[i].UpcomingDebt, liabilities[i].Currency, today)
		out.UpcomingDebt += v
	}
	out.Wallets = append(assets, liabilities...)
	catTotals := map[uuid.UUID]*CategoryTotal{}
	for _, c := range cats {
		v, ok := r.convert(c.amount, c.currency, today)
		if !ok {
			continue
		}
		ct := catTotals[c.categoryID]
		if ct == nil {
			ct = &CategoryTotal{CategoryID: c.categoryID}
			catTotals[c.categoryID] = ct
		}
		ct.Amount += v
		ct.Count += c.count
	}
	for id, ct := range catTotals {
		if ct.Amount <= 0 {
			delete(catTotals, id)
		}
	}
	out.UpcomingDebtCategories = sortedTotals(catTotals)

	// History: each wallet's balance at every bucket end, valued and converted on that date.
	running := map[uuid.UUID]int64{}
	types := map[uuid.UUID]WalletType{}
	for id, w := range r.wallets {
		running[id] = w.OpeningMinor
		types[id] = w.Type
	}
	out.Series = make([]NetWorthPoint, len(r.starts))
	ti := 0
	for i, st := range r.starts {
		end := f.To
		if i+1 < len(r.starts) && r.starts[i+1].AddDate(0, 0, -1).Before(end) {
			end = r.starts[i+1].AddDate(0, 0, -1)
		}
		for ; ti < len(r.txs) && !r.txs[ti].OccurredOn.After(end); ti++ {
			t := r.txs[ti]
			running[t.WalletID] += signed(t.Kind, t.AmountMinor)
		}
		p := NetWorthPoint{Date: st}
		for id, bal := range running {
			cur := r.wallets[id].Currency
			if IsLiability(types[id]) {
				v, _ := r.convert(-bal, cur, end)
				p.Liabilities += v
				continue
			}
			v, _ := r.convert(vals.valueOn(id, types[id], bal, end), cur, end)
			p.Assets += v
		}
		p.NetWorth = p.Assets - p.Liabilities
		out.Series[i] = p
	}
	out.Unconverted = r.unconverted()
	return out, nil
}

type upcomingCategory struct {
	categoryID uuid.UUID
	currency   string
	amount     int64
	count      int64
}

// upcomingDebt returns each liability's upcoming debt in its own currency (expenses minus income
// after today, created or still to be created by rules that end) and the same split by category.
func (s *Service) upcomingDebt(ctx context.Context, a Actor, walletIDs []uuid.UUID, today time.Time) (map[uuid.UUID]int64, []upcomingCategory, error) {
	debt := map[uuid.UUID]int64{}
	if len(walletIDs) == 0 {
		return debt, nil, nil
	}
	currencies := map[uuid.UUID]string{}
	ws, err := s.q.GetWalletsByIDs(ctx, dbq.GetWalletsByIDsParams{HouseholdID: a.HouseholdID, Ids: walletIDs})
	if err != nil {
		return nil, nil, err
	}
	for _, w := range ws {
		currencies[w.ID] = strings.TrimSpace(w.Currency)
	}
	type key struct {
		wallet, category uuid.UUID
	}
	byCat := map[key]*upcomingCategory{}
	add := func(walletID uuid.UUID, categoryID *uuid.UUID, amount, count int64) {
		debt[walletID] += amount
		if categoryID == nil {
			return
		}
		k := key{walletID, *categoryID}
		c := byCat[k]
		if c == nil {
			c = &upcomingCategory{categoryID: *categoryID, currency: currencies[walletID]}
			byCat[k] = c
		}
		c.amount += amount
		c.count += count
	}

	rows, err := s.q.UpcomingLiabilityRows(ctx, dbq.UpcomingLiabilityRowsParams{
		HouseholdID: a.HouseholdID, WalletIds: walletIDs, Today: today,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		amount := r.AmountMinor
		if r.Kind == KindIncome {
			amount = -amount
		}
		add(r.WalletID, r.CategoryID, amount, r.RowCount)
	}

	rules, err := s.q.FiniteLiabilityRules(ctx, dbq.FiniteLiabilityRulesParams{HouseholdID: a.HouseholdID, WalletIds: walletIDs})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rules {
		// Occurrences up to generated_through exist as rows (counted above when upcoming).
		after := maxDate(r.GeneratedThrough, today)
		dates, _ := schedule{freq: r.Frequency, start: r.StartDate, end: r.EndDate}.between(after, *r.EndDate, 0)
		if len(dates) == 0 {
			continue
		}
		amount := r.AmountMinor * int64(len(dates))
		if r.Kind == dbq.RecurringKindIncome {
			amount = -amount
		}
		add(r.WalletID, r.CategoryID, amount, int64(len(dates)))
	}

	cats := make([]upcomingCategory, 0, len(byCat))
	for _, c := range byCat {
		cats = append(cats, *c)
	}
	return debt, cats, nil
}
