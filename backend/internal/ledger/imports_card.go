package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/importer/banks"
	"github.com/franco-roura/ledger-01/backend/internal/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Card statements (PDF). A card is one or more ordinary wallets, one per currency it spends in
// (a Uruguayan card has a pesos and a dollars balance): the statement imports each line into its
// currency's wallet, purchases as expenses and the bill's payments as transfers from the
// accounts that pay the card. The bank debit that paid a total is usually in one of those
// accounts already (an expense from the account's own statement, "DEB. VARIOS TARJ. VISA"), not
// necessarily in the total's currency: importing the payment turns that expense into the
// transfer, so the debit isn't counted as spending twice.

// rateTolerance is how far a debit's implied rate may be from the household's own rate for it
// to count as the payment of a total in another currency.
const rateTolerance = 0.15

// StatementInfo describes an uploaded card statement.
type StatementInfo struct {
	Bank       string
	Issued     time.Time
	Due        time.Time
	Currencies []StatementCurrency
}

// StatementCurrency is what a statement holds in one currency: how many lines, and how much was
// spent (purchases and fees, positive, in minor units).
type StatementCurrency struct {
	Code  string
	Lines int
	Spent int64
}

// CardDebit is a bank debit that paid (part of) a card statement's payment row, in one of the
// accounts that pay the card, as it is now.
type CardDebit struct {
	ID         uuid.UUID
	WalletID   uuid.UUID
	WalletName string
	Currency   string
	Amount     int64 // positive, in the paying wallet's currency
	Date       time.Time
	Note       string
	// The part of the payment it covers, in the card wallet's currency: the whole payment, or
	// for a bill paid from two accounts, each debit's share.
	Part int64
	// A debit in another currency matched without a household rate to compare it with.
	RateUnchecked bool
}

// readStatement reads a card statement PDF into the table of its lines.
func (s *Service) readStatement(ctx context.Context, content []byte) (importer.Grid, importer.Bank, StatementInfo, importer.Statement, error) {
	lines, err := importer.PDFLines(content)
	if err != nil {
		return nil, importer.Bank{}, StatementInfo{}, importer.Statement{}, err
	}
	for _, b := range banks.All() {
		if b.Statement == nil {
			continue
		}
		st, ok, err := b.Statement(lines)
		if err != nil {
			return nil, b, StatementInfo{}, st, err
		}
		if !ok {
			continue
		}
		g := importer.StatementGrid(st)
		if len(g) < 2 {
			return nil, b, StatementInfo{}, st, errors.New("this statement has no movements")
		}
		info := StatementInfo{Bank: b.Name, Issued: st.Issued, Due: st.Due}
		units, err := s.minorUnits(ctx)
		if err != nil {
			return nil, b, info, st, err
		}
		index := map[string]int{}
		for _, l := range st.Lines {
			i, ok := index[l.Currency]
			if !ok {
				i = len(info.Currencies)
				index[l.Currency] = i
				info.Currencies = append(info.Currencies, StatementCurrency{Code: l.Currency})
			}
			c := &info.Currencies[i]
			c.Lines++
			if u, ok := units[l.Currency]; ok {
				if v, err := importer.ParseAmount(l.Amount, st.Decimal, u); err == nil && v > 0 {
					c.Spent += v
				}
			}
		}
		return g, b, info, st, nil
	}
	return nil, importer.Bank{}, StatementInfo{}, importer.Statement{}, errors.New("this PDF isn't a card statement the import can read")
}

func (s *Service) minorUnits(ctx context.Context) (map[string]int, error) {
	cs, err := s.Currencies(ctx)
	if err != nil {
		return nil, err
	}
	units := make(map[string]int, len(cs))
	for _, c := range cs {
		units[c.Code] = c.MinorUnits
	}
	return units, nil
}

// isStatement says whether a mapping reads a card statement batch.
func isStatement(m importer.Mapping) bool {
	return importer.IsStatementMapping(m) && banks.ByID(m.Bank).Statement != nil
}

// statementMapping is the mapping for a statement uploaded to wallet w: the card's wallets and
// paying accounts saved for w (links that no longer hold are dropped), else w for its own
// currency and no paying accounts listed (every other wallet may pay the card). saved is true
// when nothing is left to choose: every currency of the statement has its wallet or is left out.
func (s *Service) statementMapping(ctx context.Context, a Actor, w Wallet, bank importer.Bank, st importer.Statement, info StatementInfo) (importer.Mapping, bool, error) {
	m := importer.StatementMapping(bank.ID, st.Decimal)
	fromProfile := false
	p, err := s.q.GetImportProfile(ctx, dbq.GetImportProfileParams{WalletID: w.ID, HouseholdID: a.HouseholdID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return m, false, err
	default:
		var saved importer.Mapping
		if json.Unmarshal(p.Mapping, &saved) == nil && importer.IsStatementMapping(saved) && saved.Bank == bank.ID {
			fromProfile = true
			m.CardWallets, m.PayingWallets = map[string]string{}, nil
			for cur, id := range saved.CardWallets {
				if id == "" {
					m.CardWallets[cur] = ""
				} else if cw, err := s.walletByID(ctx, a, id); err == nil && !cw.Archived && cw.Currency == cur {
					m.CardWallets[cur] = id
				}
			}
			for _, id := range saved.PayingWallets {
				if id == "" || slices.Contains(m.PayingWallets, id) || slices.Contains(mapsValues(m.CardWallets), id) || id == w.ID.String() {
					continue
				}
				if pw, err := s.walletByID(ctx, a, id); err == nil && !pw.Archived {
					m.PayingWallets = append(m.PayingWallets, id)
				}
			}
		}
	}
	if len(m.CardWallets) == 0 {
		m.CardWallets = map[string]string{w.Currency: w.ID.String()}
	}
	saved := fromProfile
	for _, c := range info.Currencies {
		if _, ok := m.CardWallets[c.Code]; !ok {
			saved = false
		}
	}
	return m, saved, nil
}

func mapsValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (s *Service) walletByID(ctx context.Context, a Actor, id string) (Wallet, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return Wallet{}, ErrNotFound
	}
	return s.GetWallet(ctx, a, u)
}

// cardWallets checks a statement mapping's card wallets and paying accounts: the household's,
// not archived, each card wallet in its currency and used once. A card wallet listed as a payer
// is skipped, as are closed accounts. With no paying accounts listed, every wallet that isn't
// the card's may pay it (a profile saved before the list had one PaidFrom wallet). A currency mapped to "" is left out (nil); with no card wallets, the
// uploading wallet is the card in its currency.
func (s *Service) cardWallets(ctx context.Context, a Actor, w Wallet, m importer.Mapping) (map[string]*Wallet, []Wallet, error) {
	cards := map[string]*Wallet{}
	if len(m.CardWallets) == 0 {
		cards[w.Currency] = &w
	}
	seen := map[uuid.UUID]bool{}
	for cur, id := range m.CardWallets {
		if id == "" {
			cards[cur] = nil
			continue
		}
		cw, err := s.walletByID(ctx, a, id)
		switch {
		case err != nil:
			return nil, nil, invalid("mapping.card_wallet_ids", "the %s wallet doesn't exist", cur)
		case cw.Archived:
			return nil, nil, invalid("mapping.card_wallet_ids", "the %s wallet is archived", cur)
		case cw.Currency != cur:
			return nil, nil, invalid("mapping.card_wallet_ids", "%s is in %s, not %s", cw.Name, cw.Currency, cur)
		case seen[cw.ID]:
			return nil, nil, invalid("mapping.card_wallet_ids", "%s is chosen for two currencies", cw.Name)
		}
		seen[cw.ID] = true
		cards[cur] = &cw
	}
	ids := m.PayingWallets
	if len(ids) == 0 && m.PaidFrom != "" {
		ids = []string{m.PaidFrom}
	}
	if len(ids) == 0 {
		ws, err := s.ListWallets(ctx, a, true, false, Today())
		if err != nil {
			return nil, nil, err
		}
		for _, o := range ws {
			if !seen[o.ID] && o.ID != w.ID {
				ids = append(ids, o.ID.String())
			}
		}
	}
	var payers []Wallet
	for _, id := range ids {
		if id == "" {
			continue
		}
		pw, err := s.walletByID(ctx, a, id)
		switch {
		case err != nil:
			return nil, nil, invalid("mapping.paying_wallet_ids", "an account that pays the card doesn't exist")
		case seen[pw.ID]:
			continue // listed twice, or one of the card's own wallets (linked after the list was made)
		case pw.Archived:
			continue // an account closed since; the others still pay the card
		}
		seen[pw.ID] = true
		payers = append(payers, pw)
	}
	return cards, payers, nil
}

// markCardPayments marks a card wallet's payment rows (as the statement's bank words them,
// money into the card) as transfers in. One that matches a transfer into the card for the same
// amount close by was imported before.
func markCardPayments(bank importer.Bank, rows []ImportRow, cands []dbq.ImportCandidatesRow, used map[uuid.UUID]bool) {
	window := importer.CardPaymentWindow
	for i := range rows {
		r := &rows[i]
		if r.Err != "" || r.Amount <= 0 || !bank.CardPayment(r.Note) {
			continue
		}
		r.Kind, r.CategoryID, r.CategorySource = KindTransferIn, nil, ""
		if r.DupReason == DupSimilar || r.DupReason == DupSame {
			// The heuristics may have matched an income; only a transfer counts for a payment.
			r.DupReason, r.MatchNote, r.MatchDate = "", "", time.Time{}
		}
		if r.DupReason != "" {
			continue
		}
		var best *dbq.ImportCandidatesRow
		for j := range cands {
			c := &cands[j]
			gap := c.OccurredOn.Sub(r.Date).Abs()
			if used[c.ID] || c.Kind != KindTransferIn || c.SignedMinor != r.Amount || gap > window {
				continue
			}
			if best == nil || gap < best.OccurredOn.Sub(r.Date).Abs() {
				best = c
			}
		}
		if best != nil {
			r.DupReason, r.MatchNote, r.MatchDate = DupSimilar, best.Note, best.OccurredOn
			used[best.ID] = true
		}
	}
}

// debitCandidate is an expense in a paying account that reads as a card debit.
type debitCandidate struct {
	dbq.CardDebitCandidatesRow
	wallet Wallet
}

// assignCardDebits finds the bank debits that paid a statement's open payments, in any of the
// accounts that pay the card (the bank may take a total from the account in the other
// currency, or part from each). Pairs are taken best first, each debit paying one payment:
//  1. a debit in the payment's currency for the same amount;
//  2. a debit in another currency whose implied rate is within rateTolerance of the household's
//     rate on that day (closest first);
//  3. with no household rate for the pair, the only debit left in another currency;
//  4. a debit in the payment's currency for part of it, plus one in another currency for the
//     rest (at a rate within tolerance).
//
// A payment none of these settle can't be imported.
func (s *Service) assignCardDebits(ctx context.Context, q *dbq.Queries, a Actor, rows []ImportRow, cards map[string]*Wallet, payers []Wallet) error {
	var open []int
	for i, r := range rows {
		if r.Err == "" && r.DupReason == "" && r.Kind == KindTransferIn {
			open = append(open, i)
		}
	}
	if len(open) == 0 {
		return nil
	}
	if len(payers) == 0 {
		for _, i := range open {
			rows[i].Err = "choose the accounts that pay this card to import its payment"
		}
		return nil
	}
	window := importer.CardPaymentWindow
	from, to := rows[open[0]].Date, rows[open[0]].Date
	for _, i := range open {
		from, to = minTime(from, rows[i].Date), maxTime(to, rows[i].Date)
	}
	ids := make([]uuid.UUID, len(payers))
	byID := map[uuid.UUID]Wallet{}
	for i, p := range payers {
		ids[i], byID[p.ID] = p.ID, p
	}
	found, err := q.CardDebitCandidates(ctx, dbq.CardDebitCandidatesParams{
		HouseholdID: a.HouseholdID, WalletIds: ids, FromDate: from.Add(-window), ToDate: to.Add(window),
	})
	if err != nil {
		return err
	}
	all := banks.All()
	var debits []debitCandidate
	for _, d := range found {
		for _, n := range append([]string{d.Note}, d.SourceNotes...) {
			if importer.CardDebit(n, all) {
				debits = append(debits, debitCandidate{CardDebitCandidatesRow: d, wallet: byID[d.WalletID]})
				break
			}
		}
	}
	conv, err := s.converter(ctx, a, Today())
	if err != nil {
		return err
	}
	card := func(i int) Wallet { return *cards[rows[i].Currency] }
	// deviation is how far a debit's implied rate for amount (in the card's currency) is from
	// the household's rate; ok is false when there's no rate to compare with.
	deviation := func(i int, d debitCandidate, amount int64) (float64, bool) {
		cw := card(i)
		ref, ok := conv.Rate(d.wallet.Currency, cw.Currency, rows[i].Date)
		if !ok || d.AmountMinor <= 0 || amount <= 0 {
			return 0, false
		}
		implied, _ := new(big.Rat).SetString(impliedRate(d.AmountMinor, d.wallet.MinorUnits, amount, cw.MinorUnits))
		f, _ := new(big.Rat).Quo(implied, ref).Float64()
		return math.Abs(f - 1), true
	}
	near := func(i int, d debitCandidate) bool { return d.OccurredOn.Sub(rows[i].Date).Abs() <= window }
	taken := map[uuid.UUID]bool{}
	assign := func(i int, d debitCandidate, part int64, unchecked bool) {
		taken[d.ID] = true
		rows[i].Payments = append(rows[i].Payments, CardDebit{
			ID: d.ID, WalletID: d.wallet.ID, WalletName: d.wallet.Name, Currency: d.wallet.Currency, Amount: d.AmountMinor,
			Date: d.OccurredOn, Note: d.Note, Part: part, RateUnchecked: unchecked,
		})
	}
	paid := func(i int) bool { return len(rows[i].Payments) > 0 }

	// 1 and 2, best score first.
	type pair struct {
		i     int
		d     debitCandidate
		score float64
		gap   time.Duration
	}
	var pairs []pair
	for _, i := range open {
		cur := rows[i].Currency
		for _, d := range debits {
			if !near(i, d) {
				continue
			}
			gap := d.OccurredOn.Sub(rows[i].Date).Abs()
			switch {
			case d.wallet.Currency == cur && d.AmountMinor == rows[i].Amount:
				pairs = append(pairs, pair{i, d, 0, gap})
			case d.wallet.Currency != cur:
				if dev, ok := deviation(i, d, rows[i].Amount); ok && dev <= rateTolerance {
					pairs = append(pairs, pair{i, d, 1 + dev, gap})
				}
			}
		}
	}
	sort.SliceStable(pairs, func(x, y int) bool {
		if pairs[x].score != pairs[y].score {
			return pairs[x].score < pairs[y].score
		}
		return pairs[x].gap < pairs[y].gap
	})
	for _, p := range pairs {
		if !paid(p.i) && !taken[p.d.ID] {
			assign(p.i, p.d, rows[p.i].Amount, false)
		}
	}

	// 3. No rate to compare: only when one debit in another currency is left.
	for _, i := range open {
		if paid(i) {
			continue
		}
		var only []debitCandidate
		for _, d := range debits {
			if !taken[d.ID] && near(i, d) && d.wallet.Currency != rows[i].Currency {
				if _, ok := deviation(i, d, rows[i].Amount); !ok {
					only = append(only, d)
				}
			}
		}
		if len(only) == 1 {
			assign(i, only[0], rows[i].Amount, true)
		}
	}

	// 4. Part from each account.
	for _, i := range open {
		if paid(i) {
			continue
		}
		var bestSame, bestOther *debitCandidate
		bestDev := math.Inf(1)
		for x := range debits {
			same := &debits[x]
			if taken[same.ID] || !near(i, *same) || same.wallet.Currency != rows[i].Currency || same.AmountMinor >= rows[i].Amount {
				continue
			}
			rest := rows[i].Amount - same.AmountMinor
			for y := range debits {
				other := &debits[y]
				if taken[other.ID] || !near(i, *other) || other.wallet.Currency == rows[i].Currency {
					continue
				}
				if dev, ok := deviation(i, *other, rest); ok && dev <= rateTolerance && dev < bestDev {
					bestSame, bestOther, bestDev = same, other, dev
				}
			}
		}
		if bestSame != nil {
			assign(i, *bestSame, bestSame.AmountMinor, false)
			assign(i, *bestOther, rows[i].Amount-bestSame.AmountMinor, false)
		}
	}

	names := make([]string, 0, len(payers))
	for _, p := range payers {
		names = append(names, p.Name)
	}
	where := strings.Join(names, ", ")
	if len(names) > 3 {
		where = strings.Join(names[:3], ", ") + fmt.Sprintf(" or %d other accounts", len(names)-3)
	}
	for _, i := range open {
		if !paid(i) {
			rows[i].Err = fmt.Sprintf("no card debit in %s within %d days of this payment", where, int(window.Hours()/24))
		}
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// convertCardDebit turns a debit that paid (part of) a card statement's payment row into a
// transfer from its account to the card's wallet, with the debit's date, note, labels and
// author, and the rate the two amounts imply. The debit's bank rows now point at the transfer's
// outgoing leg, so importing the account's statement again still finds them. Returns the
// incoming leg.
func (s *Service) convertCardDebit(ctx context.Context, q *dbq.Queries, a Actor, card Wallet, r ImportRow, debit CardDebit, field string) (uuid.UUID, error) {
	changed := invalid(field, "row %d: a debit it pays has changed; upload the file again", r.Index+1)
	d, err := q.LockCardDebit(ctx, dbq.LockCardDebitParams{ID: debit.ID, HouseholdID: a.HouseholdID, WalletID: debit.WalletID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, changed
	}
	if err != nil {
		return uuid.Nil, err
	}
	if d.AmountMinor != debit.Amount {
		return uuid.Nil, changed
	}
	payer, err := s.GetWallet(ctx, a, debit.WalletID)
	if err != nil {
		return uuid.Nil, err
	}
	labels, err := q.ListTransactionLabelIDs(ctx, d.ID)
	if err != nil {
		return uuid.Nil, err
	}
	in := TransferInput{
		FromWalletID: payer.ID, ToWalletID: card.ID, FromAmount: d.AmountMinor, ToAmount: debit.Part,
		Rate: impliedRate(d.AmountMinor, payer.MinorUnits, debit.Part, card.MinorUnits), OccurredOn: d.OccurredOn,
		Note: d.Note, LabelIDs: labels,
	}
	if err := s.prepareTransfer(ctx, q, a, &in); err != nil {
		var ie *InvalidError
		if errors.As(err, &ie) {
			return uuid.Nil, invalid(field, "row %d: %s", r.Index+1, ie.Msg)
		}
		return uuid.Nil, err
	}
	id, err := insertTransfer(ctx, q, a.HouseholdID, d.CreatedBy, in, recurringLink{})
	if err != nil {
		return uuid.Nil, err
	}
	out, err := q.TransferLeg(ctx, dbq.TransferLegParams{TransferID: &id, HouseholdID: a.HouseholdID, Kind: KindTransferOut})
	if err != nil {
		return uuid.Nil, err
	}
	inLeg, err := q.TransferLeg(ctx, dbq.TransferLegParams{TransferID: &id, HouseholdID: a.HouseholdID, Kind: KindTransferIn})
	if err != nil {
		return uuid.Nil, err
	}
	if err := q.MoveImportRefs(ctx, dbq.MoveImportRefsParams{HouseholdID: a.HouseholdID, FromID: d.ID, ToID: out}); err != nil {
		return uuid.Nil, err
	}
	n, err := q.DeletePlainTransactions(ctx, dbq.DeletePlainTransactionsParams{HouseholdID: a.HouseholdID, Ids: []uuid.UUID{d.ID}})
	if err != nil {
		return uuid.Nil, err
	}
	if n != 1 {
		return uuid.Nil, changed
	}
	return inLeg, nil
}

// impliedRate is to ÷ from in whole units, with money.RateScale decimals.
func impliedRate(from int64, fromUnits int, to int64, toUnits int) string {
	r := new(big.Rat).SetFrac(big.NewInt(to), big.NewInt(from))
	r.Mul(r, pow10(fromUnits-toUnits))
	return money.FormatRate(r)
}
