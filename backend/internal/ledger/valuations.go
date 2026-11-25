package ledger

import (
	"context"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/google/uuid"
)

// WalletType is what a wallet is. Cash, bank, investment and property are assets; credit cards
// and loans are liabilities, whose (usually negative) balance is what is owed.
type WalletType = dbq.WalletType

const (
	WalletCash       = dbq.WalletTypeCash
	WalletBank       = dbq.WalletTypeBank
	WalletInvestment = dbq.WalletTypeInvestment
	WalletProperty   = dbq.WalletTypeProperty
	WalletCreditCard = dbq.WalletTypeCreditCard
	WalletLoan       = dbq.WalletTypeLoan
)

func validWalletType(t WalletType) bool {
	switch t {
	case WalletCash, WalletBank, WalletInvestment, WalletProperty, WalletCreditCard, WalletLoan:
		return true
	}
	return false
}

// IsLiability reports whether a wallet of this type holds debt (credit cards and loans).
func IsLiability(t WalletType) bool { return t == WalletCreditCard || t == WalletLoan }

// TakesValuations reports whether a wallet of this type is valued with valuation entries
// (investments and property). Valuations on other types are kept but ignored.
func TakesValuations(t WalletType) bool { return t == WalletInvestment || t == WalletProperty }

// Valuation is what an investment or property wallet is worth on a date, in its currency.
type Valuation struct {
	ID       uuid.UUID
	WalletID uuid.UUID
	ValuedOn time.Time
	Value    int64
}

// valuationPoint is a valuation with the difference between it and the wallet's ledger balance
// on its date. From that date on, value = ledger balance + offset.
type valuationPoint struct {
	on     time.Time
	offset int64
}

// valuationOffsets holds every wallet's valuations up to today, oldest first.
type valuationOffsets map[uuid.UUID][]valuationPoint

func (s *Service) valuations(ctx context.Context, a Actor, today time.Time) (valuationOffsets, error) {
	rows, err := s.q.ValuationOffsets(ctx, dbq.ValuationOffsetsParams{HouseholdID: a.HouseholdID, Today: today})
	if err != nil {
		return nil, err
	}
	out := valuationOffsets{}
	for _, r := range rows {
		out[r.WalletID] = append(out[r.WalletID], valuationPoint{on: r.ValuedOn, offset: r.ValueMinor - r.LedgerBalanceMinor})
	}
	return out, nil
}

// at returns the latest valuation of a wallet on or before d.
func (v valuationOffsets) at(walletID uuid.UUID, d time.Time) (valuationPoint, bool) {
	pts := v[walletID]
	for i := len(pts) - 1; i >= 0; i-- {
		if !pts[i].on.After(d) {
			return pts[i], true
		}
	}
	return valuationPoint{}, false
}

// valueOn turns a wallet's ledger balance on d into its value on d.
func (v valuationOffsets) valueOn(walletID uuid.UUID, typ WalletType, balance int64, d time.Time) int64 {
	if !TakesValuations(typ) {
		return balance
	}
	if p, ok := v.at(walletID, d); ok {
		return balance + p.offset
	}
	return balance
}

// apply sets a wallet's Value from its latest valuation (the valuations were loaded up to the
// same today as the wallet's balance).
func (v valuationOffsets) apply(w *Wallet) {
	w.Value = w.Balance
	w.ValuedOn = nil
	if !TakesValuations(w.Type) {
		return
	}
	if pts := v[w.ID]; len(pts) > 0 {
		p := pts[len(pts)-1]
		w.Value = w.Balance + p.offset
		on := p.on
		w.ValuedOn = &on
	}
}

// ListValuations returns a wallet's valuations, newest first.
func (s *Service) ListValuations(ctx context.Context, a Actor, walletID uuid.UUID) ([]Valuation, error) {
	if _, err := s.GetWallet(ctx, a, walletID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListWalletValuations(ctx, dbq.ListWalletValuationsParams{HouseholdID: a.HouseholdID, WalletID: walletID})
	if err != nil {
		return nil, err
	}
	out := make([]Valuation, len(rows))
	for i, r := range rows {
		out[i] = Valuation{ID: r.ID, WalletID: r.WalletID, ValuedOn: r.ValuedOn, Value: r.ValueMinor}
	}
	return out, nil
}

// SaveValuation records what an investment or property wallet is worth on a date, replacing an
// earlier valuation on the same date.
func (s *Service) SaveValuation(ctx context.Context, a Actor, walletID uuid.UUID, on time.Time, value int64) (Valuation, error) {
	w, err := s.GetWallet(ctx, a, walletID)
	if err != nil {
		return Valuation{}, err
	}
	if !TakesValuations(w.Type) {
		return Valuation{}, invalid("wallet_id", "only investment and property wallets take valuations")
	}
	if on.IsZero() {
		return Valuation{}, invalid("valued_on", "is required")
	}
	if err := checkAmount("value_minor", abs(value), true); err != nil {
		return Valuation{}, err
	}
	r, err := s.q.SaveWalletValuation(ctx, dbq.SaveWalletValuationParams{
		ID: newID(), HouseholdID: a.HouseholdID, WalletID: walletID, ValuedOn: on, ValueMinor: value,
	})
	if err != nil {
		return Valuation{}, dbErr(err, "valuation")
	}
	return Valuation{ID: r.ID, WalletID: r.WalletID, ValuedOn: r.ValuedOn, Value: r.ValueMinor}, nil
}

func (s *Service) DeleteValuation(ctx context.Context, a Actor, id uuid.UUID) error {
	n, err := s.q.DeleteWalletValuation(ctx, dbq.DeleteWalletValuationParams{ID: id, HouseholdID: a.HouseholdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
