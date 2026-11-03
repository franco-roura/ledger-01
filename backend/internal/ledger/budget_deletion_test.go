package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

func TestDeleteBudgetCategoryPreservesScope(t *testing.T) {
	svc := ledger.New(testdb.Pool(t))
	ctx := context.Background()
	a := newActor(t, svc)
	w := mustWallet(t, svc, a, "Daily", "USD", 0)
	unused := categoryNamed(t, svc, a, ledger.CategoryExpense, "Groceries")
	food := categoryNamed(t, svc, a, ledger.CategoryExpense, "Food & Drink")
	salary := categoryNamed(t, svc, a, ledger.CategoryIncome, "Salary")
	if _, err := svc.CreateTransaction(ctx, a, ledger.TransactionInput{
		WalletID: w.ID, CategoryID: food.ID, Amount: 50_00, OccurredOn: day,
	}); err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateBudget(ctx, a, ledger.BudgetInput{
		Name: "Groceries", Amount: 100_00, Currency: "USD", Repeat: ledger.RepeatMonthly,
		Start: day, CategoryIDs: []uuid.UUID{unused.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteCategory(ctx, a, unused.ID, nil); err == nil {
		t.Fatal("deleted a category still selected by a budget")
	} else {
		var conflict *ledger.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("expected a budget conflict, got %v", err)
		}
	}
	d, err := svc.BudgetDetailFor(ctx, a, b.ID, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Budget.CategoryIDs) != 1 || d.Budget.CategoryIDs[0] != unused.ID || d.Progress.Spent != 0 {
		t.Fatalf("budget scope changed after rejected deletion: %+v", d)
	}

	// A replacement is validated even when the source has no transactions or rules.
	for _, target := range []uuid.UUID{unused.ID, salary.ID, uuid.New()} {
		if _, err := svc.DeleteCategory(ctx, a, unused.ID, &target); err == nil {
			t.Fatalf("accepted invalid replacement %s", target)
		}
	}
	if _, err := svc.DeleteCategory(ctx, a, unused.ID, &food.ID); err != nil {
		t.Fatal(err)
	}
	d, err = svc.BudgetDetailFor(ctx, a, b.ID, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Budget.CategoryIDs) != 1 || d.Budget.CategoryIDs[0] != food.ID || d.Progress.Spent != 50_00 {
		t.Fatalf("replacement did not preserve the explicit selection: %+v", d)
	}
}

func TestDeleteBudgetWalletPreservesScope(t *testing.T) {
	svc := ledger.New(testdb.Pool(t))
	ctx := context.Background()
	a := newActor(t, svc)
	empty := mustWallet(t, svc, a, "Empty", "USD", 0)
	other := mustWallet(t, svc, a, "Daily", "USD", 0)
	food := categoryNamed(t, svc, a, ledger.CategoryExpense, "Food & Drink")
	if _, err := svc.CreateTransaction(ctx, a, ledger.TransactionInput{
		WalletID: other.ID, CategoryID: food.ID, Amount: 50_00, OccurredOn: day,
	}); err != nil {
		t.Fatal(err)
	}
	in := ledger.BudgetInput{
		Name: "Empty wallet", Amount: 100_00, Currency: "USD", Repeat: ledger.RepeatMonthly,
		Start: day, WalletIDs: []uuid.UUID{empty.ID},
	}
	b, err := svc.CreateBudget(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteWallet(ctx, a, empty.ID); err == nil {
		t.Fatal("deleted a wallet still selected by a budget")
	} else {
		var conflict *ledger.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("expected a budget conflict, got %v", err)
		}
	}
	d, err := svc.BudgetDetailFor(ctx, a, b.ID, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Budget.WalletIDs) != 1 || d.Budget.WalletIDs[0] != empty.ID || d.Progress.Spent != 0 {
		t.Fatalf("budget scope changed after rejected deletion: %+v", d)
	}

	// Once the budget is explicitly changed, normal empty-wallet deletion still works.
	in.WalletIDs = []uuid.UUID{other.ID}
	if _, err := svc.UpdateBudget(ctx, a, b.ID, in); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteWallet(ctx, a, empty.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetWallet(ctx, a, empty.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("wallet still exists after deletion: %v", err)
	}
}
