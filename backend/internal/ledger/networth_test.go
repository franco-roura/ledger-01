package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
)

func TestWalletTypeDefaultsAndUpdates(t *testing.T) {
	svc := ledger.New(testdb.Pool(t))
	ctx := context.Background()
	a := newActor(t, svc)
	w := mustWallet(t, svc, a, "Northbank", "USD", 0)
	if w.Type != ledger.WalletBank {
		t.Fatalf("default type = %q, want bank", w.Type)
	}
	card := ledger.WalletCreditCard
	got, err := svc.UpdateWallet(ctx, a, w.ID, ledger.WalletPatch{Type: &card})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != ledger.WalletCreditCard {
		t.Fatalf("updated type = %q", got.Type)
	}
	if _, err := svc.CreateWallet(ctx, a, "Odd", "USD", "spaceship", 0); err == nil {
		t.Fatal("unknown type accepted")
	}
	// Only investments and property take valuations.
	if _, err := svc.SaveValuation(ctx, a, w.ID, date(t, "2026-10-01"), 10_00); err == nil {
		t.Fatal("valuation on a card accepted")
	}
}

func date(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNetWorth(t *testing.T) {
	svc := ledger.New(testdb.Pool(t))
	ctx := context.Background()
	a := newActor(t, svc) // base currency USD
	today := date(t, "2026-10-20")
	food := categoryNamed(t, svc, a, ledger.CategoryExpense, "Food & Drink")
	groceries := categoryNamed(t, svc, a, ledger.CategoryExpense, "Groceries")
	salary := categoryNamed(t, svc, a, ledger.CategoryIncome, "Salary")

	bank := mustWallet(t, svc, a, "Northbank", "USD", 1000_00)
	pesos := mustWallet(t, svc, a, "Northbank pesos", "UYU", 0)
	broker := mustTypedWallet(t, svc, a, "Brokerage", "USD", ledger.WalletInvestment, 0)
	card := mustTypedWallet(t, svc, a, "Card UYU", "UYU", ledger.WalletCreditCard, 0)
	loan := mustTypedWallet(t, svc, a, "Old loan", "USD", ledger.WalletLoan, -5000_00)
	hidden := true
	if _, err := svc.UpdateWallet(ctx, a, loan.ID, ledger.WalletPatch{Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}

	transfer := func(from, to ledger.Wallet, fromAmt, toAmt int64, rate, on string) {
		t.Helper()
		if _, err := svc.CreateTransfer(ctx, a, ledger.TransferInput{
			FromWalletID: from.ID, ToWalletID: to.ID, FromAmount: fromAmt, ToAmount: toAmt, Rate: rate,
			OccurredOn: date(t, on), Today: today,
		}); err != nil {
			t.Fatal(err)
		}
	}
	tx := func(in ledger.TransactionInput) {
		t.Helper()
		in.Today = today
		if _, err := svc.CreateTransaction(ctx, a, in); err != nil {
			t.Fatal(err)
		}
	}

	transfer(bank, pesos, 100_00, 4000_00, "40", "2026-10-02") // rate 40 UYU per USD
	transfer(bank, broker, 200_00, 200_00, "1", "2026-10-03")
	if _, err := svc.SaveValuation(ctx, a, broker.ID, date(t, "2026-10-10"), 250_00); err != nil {
		t.Fatal(err)
	}
	transfer(bank, broker, 50_00, 50_00, "1", "2026-10-15") // added after the valuation: value 300
	tx(ledger.TransactionInput{WalletID: card.ID, CategoryID: food.ID, Amount: 400_00, OccurredOn: date(t, "2026-10-05")})
	// 12 installments of 100 UYU: one is due, three more exist (3-month horizon), eight are projected.
	tx(ledger.TransactionInput{WalletID: card.ID, CategoryID: food.ID, Amount: 100_00, OccurredOn: date(t, "2026-10-12"),
		Repeat: ledger.Recurrence{Frequency: ledger.FreqMonthly, Count: 12}})
	// An open-ended subscription only counts the rows that exist: Oct 25, Nov 25, Dec 25.
	tx(ledger.TransactionInput{WalletID: card.ID, CategoryID: groceries.ID, Amount: 50_00, OccurredOn: date(t, "2026-10-25"),
		Repeat: ledger.Recurrence{Frequency: ledger.FreqMonthly}})
	// A refund still to come lowers the debt; a payment still to come (a transfer) doesn't.
	tx(ledger.TransactionInput{WalletID: card.ID, CategoryID: salary.ID, Amount: 30_00, OccurredOn: date(t, "2026-11-01")})
	transfer(pesos, card, 200_00, 200_00, "1", "2026-11-05")

	f := ledger.ReportFilter{From: date(t, "2026-09-01"), To: date(t, "2026-10-31"), Today: today}
	got, err := svc.NetWorthFor(ctx, a, f, ledger.BucketMonth)
	if err != nil {
		t.Fatal(err)
	}

	// Assets: bank 1000-100-200-50 = 650, pesos 4000 UYU = 100, brokerage valued 250 + 50 = 300.
	// Liabilities: the card owes 400 + 100 UYU = 12.50 USD; the hidden loan is left out.
	if got.Assets != 1050_00 || got.Liabilities != 12_50 || got.NetWorth != 1037_50 {
		t.Fatalf("assets=%d liabilities=%d net=%d", got.Assets, got.Liabilities, got.NetWorth)
	}
	// Upcoming card debt: 11 installments + 3 subscription rows - refund = 1100 + 150 - 30 UYU.
	if got.UpcomingDebt != 30_50 {
		t.Fatalf("upcoming debt = %d, want 3050", got.UpcomingDebt)
	}
	var cardOut *ledger.NetWorthWallet
	for i, w := range got.Wallets {
		if w.WalletID == loan.ID {
			t.Fatal("hidden loan counted")
		}
		if w.WalletID == card.ID {
			cardOut = &got.Wallets[i]
		}
		if w.WalletID == broker.ID && w.Amount != 300_00 {
			t.Fatalf("brokerage value = %d", w.Amount)
		}
	}
	if cardOut == nil || cardOut.Amount != 500_00 || cardOut.UpcomingDebt != 1220_00 {
		t.Fatalf("card = %+v", cardOut)
	}
	if last := got.Wallets[len(got.Wallets)-1]; !ledger.IsLiability(last.Type) {
		t.Fatalf("liabilities should come last: %+v", got.Wallets)
	}
	// By category, in USD: food 1100 UYU (11 installments) = 27.50, groceries 150 UYU = 3.75. The
	// refund's income category nets negative and is left out.
	if len(got.UpcomingDebtCategories) != 2 ||
		got.UpcomingDebtCategories[0].CategoryID != food.ID || got.UpcomingDebtCategories[0].Amount != 27_50 ||
		got.UpcomingDebtCategories[0].Count != 11 ||
		got.UpcomingDebtCategories[1].CategoryID != groceries.ID || got.UpcomingDebtCategories[1].Amount != 3_75 {
		t.Fatalf("debt by category = %+v", got.UpcomingDebtCategories)
	}

	// History: September is the opening balances only (bank 1000, nothing owed); October ends as today.
	if len(got.Series) != 2 {
		t.Fatalf("series = %+v", got.Series)
	}
	if sep := got.Series[0]; sep.Assets != 1000_00 || sep.Liabilities != 0 {
		t.Fatalf("september = %+v", sep)
	}
	if oct := got.Series[1]; oct.Assets != 1050_00 || oct.Liabilities != 12_50 || oct.NetWorth != 1037_50 {
		t.Fatalf("october = %+v", oct)
	}

	// The wallet itself reports the valued figure next to its ledger balance.
	ws, err := svc.ListWallets(ctx, a, false, false, today)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		if w.ID == broker.ID && (w.Balance != 250_00 || w.Value != 300_00 || w.ValuedOn == nil) {
			t.Fatalf("brokerage wallet = %+v", w)
		}
	}

	// Upcoming card rows are debt, never spending: October's expenses are the 400 + 100 UYU due.
	d, err := svc.Dashboard(ctx, a, ledger.ReportFilter{From: date(t, "2026-10-01"), To: date(t, "2026-12-31"), Today: today}, ledger.BucketMonth)
	if err != nil {
		t.Fatal(err)
	}
	if d.Expenses != 12_50 {
		t.Fatalf("dashboard expenses = %d, want 1250", d.Expenses)
	}
}
