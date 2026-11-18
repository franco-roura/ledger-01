package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/importer/banks"
	"github.com/franco-roura/ledger-01/backend/internal/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Card statements (PDF). A card is an ordinary wallet in the card's currency: its statement
// imports the purchases as expenses, and the bill's payment as a transfer from the wallet that
// pays the card. The bank debit that paid it is usually in that wallet already (an expense from
// the account's own statement, "DEB. VARIOS TARJ. VISA"); importing the payment turns that
// expense into the transfer, in both currencies, so the debit isn't counted as spending twice.

// StatementInfo describes an uploaded card statement.
type StatementInfo struct {
	Bank   string
	Issued time.Time
	Due    time.Time
	// Lines in another currency than the wallet's, which a wallet in that currency imports.
	OtherCurrency int
}

// CardDebit is the bank debit that paid a card statement's payment row, in the wallet that pays
// the card, as it is now.
type CardDebit struct {
	ID         uuid.UUID
	WalletID   uuid.UUID
	WalletName string
	Currency   string
	Amount     int64 // positive, in the paying wallet's currency
	Date       time.Time
	Note       string
}

// readStatement reads a card statement PDF into the table of its lines in the wallet's currency.
func readStatement(content []byte, w Wallet) (importer.Grid, importer.Bank, StatementInfo, importer.Statement, error) {
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
		g, other := importer.StatementGrid(st, w.Currency)
		if len(g) < 2 {
			return nil, b, StatementInfo{}, st, fmt.Errorf("this statement has nothing in %s; import it into a wallet in its currency", w.Currency)
		}
		return g, b, StatementInfo{Bank: b.Name, Issued: st.Issued, Due: st.Due, OtherCurrency: other}, st, nil
	}
	return nil, importer.Bank{}, StatementInfo{}, importer.Statement{}, errors.New("this PDF isn't a card statement the import can read")
}

// statementMapping is the mapping for a statement, with the wallet's saved paying wallet.
func (s *Service) statementMapping(ctx context.Context, a Actor, walletID uuid.UUID, bank importer.Bank, st importer.Statement) (importer.Mapping, bool, error) {
	m := importer.StatementMapping(bank.ID, st.Decimal)
	p, err := s.q.GetImportProfile(ctx, dbq.GetImportProfileParams{WalletID: walletID, HouseholdID: a.HouseholdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	var saved importer.Mapping
	if json.Unmarshal(p.Mapping, &saved) != nil {
		return m, false, nil
	}
	if id, err := uuid.Parse(saved.PaidFrom); err == nil {
		if payer, err := s.GetWallet(ctx, a, id); err == nil && !payer.Archived && payer.ID != walletID {
			m.PaidFrom = saved.PaidFrom
		}
	}
	return m, importer.IsStatementMapping(saved) && saved.Bank == bank.ID && m.PaidFrom != "", nil
}

// payingWallet is the mapping's paying wallet, or nil when it has none.
func (s *Service) payingWallet(ctx context.Context, a Actor, w Wallet, m importer.Mapping) (*Wallet, error) {
	if m.PaidFrom == "" {
		return nil, nil
	}
	id, err := uuid.Parse(m.PaidFrom)
	if err != nil {
		return nil, invalid("mapping.paid_from_wallet_id", "isn't a wallet")
	}
	if id == w.ID {
		return nil, invalid("mapping.paid_from_wallet_id", "choose another wallet than the card's")
	}
	payer, err := s.GetWallet(ctx, a, id)
	if err != nil {
		return nil, invalid("mapping.paid_from_wallet_id", "wallet doesn't exist")
	}
	if payer.Archived {
		return nil, invalid("mapping.paid_from_wallet_id", "wallet is archived")
	}
	return &payer, nil
}

// matchCardPayments reads a card statement's payment rows (as its bank words them, money into
// the card) as transfers from the paying wallet: a transfer into the card for the same amount
// close by means it was imported before; else the closest card debit in the paying wallet (as
// any bank words it) is the one that paid it. A payment with neither can't be imported.
func (s *Service) matchCardPayments(ctx context.Context, q *dbq.Queries, a Actor, card Wallet, payer *Wallet, bank importer.Bank,
	rows []ImportRow, cands []dbq.ImportCandidatesRow, used map[uuid.UUID]bool,
) error {
	var payments []int
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
		if r.DupReason == "" {
			payments = append(payments, i)
		}
	}
	if len(payments) == 0 {
		return nil
	}
	window := importer.CardPaymentWindow
	open := payments[:0]
	for _, i := range payments {
		r := &rows[i]
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
			continue
		}
		open = append(open, i)
	}
	if len(open) == 0 {
		return nil
	}
	if payer == nil {
		for _, i := range open {
			rows[i].Err = "choose the wallet that pays this card to import its payment"
		}
		return nil
	}
	from, to := rows[open[0]].Date, rows[open[0]].Date
	for _, i := range open {
		from, to = minTime(from, rows[i].Date), maxTime(to, rows[i].Date)
	}
	debits, err := q.CardDebitCandidates(ctx, dbq.CardDebitCandidatesParams{
		HouseholdID: a.HouseholdID, WalletID: payer.ID, FromDate: from.Add(-window), ToDate: to.Add(window),
	})
	if err != nil {
		return err
	}
	all := banks.All()
	taken := map[uuid.UUID]bool{}
	for _, i := range open {
		r := &rows[i]
		var best *dbq.CardDebitCandidatesRow
		for j := range debits {
			d := &debits[j]
			gap := d.OccurredOn.Sub(r.Date).Abs()
			if taken[d.ID] || gap > window || (payer.Currency == card.Currency && d.AmountMinor != r.Amount) {
				continue
			}
			isDebit := false
			for _, n := range append([]string{d.Note}, d.SourceNotes...) {
				isDebit = isDebit || importer.CardDebit(n, all)
			}
			if isDebit && (best == nil || gap < best.OccurredOn.Sub(r.Date).Abs()) {
				best = d
			}
		}
		if best == nil {
			r.Err = fmt.Sprintf("no card debit in %s within %d days of this payment", payer.Name, int(window.Hours()/24))
			continue
		}
		taken[best.ID] = true
		r.Payment = &CardDebit{
			ID: best.ID, WalletID: payer.ID, WalletName: payer.Name, Currency: payer.Currency, Amount: best.AmountMinor,
			Date: best.OccurredOn, Note: best.Note,
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

// convertCardDebit turns the debit that paid a card statement's payment row into a transfer
// from the paying wallet to the card, with the debit's date, note, labels and author, and the
// rate the two amounts imply. The debit's bank rows now point at the transfer's outgoing leg,
// so importing the account's statement again still finds them. Returns the incoming leg.
func (s *Service) convertCardDebit(ctx context.Context, q *dbq.Queries, a Actor, card Wallet, r ImportRow, field string) (uuid.UUID, error) {
	changed := invalid(field, "row %d: the debit it pays has changed; upload the file again", r.Index+1)
	d, err := q.LockCardDebit(ctx, dbq.LockCardDebitParams{ID: r.Payment.ID, HouseholdID: a.HouseholdID, WalletID: r.Payment.WalletID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, changed
	}
	if err != nil {
		return uuid.Nil, err
	}
	if d.AmountMinor != r.Payment.Amount {
		return uuid.Nil, changed
	}
	payer, err := s.GetWallet(ctx, a, r.Payment.WalletID)
	if err != nil {
		return uuid.Nil, err
	}
	labels, err := q.ListTransactionLabelIDs(ctx, d.ID)
	if err != nil {
		return uuid.Nil, err
	}
	in := TransferInput{
		FromWalletID: payer.ID, ToWalletID: card.ID, FromAmount: d.AmountMinor, ToAmount: r.Amount,
		Rate: impliedRate(d.AmountMinor, payer.MinorUnits, r.Amount, card.MinorUnits), OccurredOn: d.OccurredOn,
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
