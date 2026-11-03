package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

func TestReportsRejectConversionOverflow(t *testing.T) {
	ctx := context.Background()
	svc := ledger.New(testdb.Pool(t))
	a := newActor(t, svc)
	usd := mustWallet(t, svc, a, "Source", "USD", 0)
	uyu := mustWallet(t, svc, a, "Destination", "UYU", 0)
	cat := categoryNamed(t, svc, a, ledger.CategoryExpense, "Groceries")
	if _, err := svc.CreateTransfer(ctx, a, ledger.TransferInput{
		FromWalletID: usd.ID, ToWalletID: uyu.ID, FromAmount: 100,
		ToAmount: 1000000, Rate: "10000", OccurredOn: day,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTransaction(ctx, a, ledger.TransactionInput{
		WalletID: usd.ID, CategoryID: cat.ID, Amount: ledger.MaxAmount, OccurredOn: day,
	}); err != nil {
		t.Fatal(err)
	}
	budget, err := svc.CreateBudget(ctx, a, ledger.BudgetInput{
		Name: "Spending", Amount: 100_00, Currency: "UYU", Repeat: ledger.RepeatMonthly,
		Start: day, WalletIDs: []uuid.UUID{usd.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := ledger.ReportFilter{From: day, To: day, Today: day, Currency: "UYU", WalletIDs: []uuid.UUID{usd.ID}}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"dashboard", func() error { _, err := svc.Dashboard(ctx, a, f, ledger.BucketDay); return err }},
		{"balance only", func() error {
			future := f
			future.From, future.To = day.AddDate(0, 0, 1), day.AddDate(0, 0, 1)
			_, err := svc.Dashboard(ctx, a, future, ledger.BucketDay)
			return err
		}},
		{"category", func() error { _, err := svc.CategoryReportFor(ctx, a, cat.ID, f, ledger.BucketDay); return err }},
		{"budget list", func() error { _, _, err := svc.ListBudgets(ctx, a, day); return err }},
		{"budget detail", func() error { _, err := svc.BudgetDetailFor(ctx, a, budget.ID, day, nil); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conflict *ledger.ConflictError
			if err := tc.run(); !errors.As(err, &conflict) {
				t.Fatalf("expected explicit conversion overflow error, got %v", err)
			}
		})
	}
}
