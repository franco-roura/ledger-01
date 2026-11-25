package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

func TestBudgetRollover(t *testing.T) {
	svc := ledger.New(testdb.Pool(t))
	ctx := context.Background()
	a := newActor(t, svc)
	usd := mustWallet(t, svc, a, "Northbank", "USD", 1000_00)
	uyu := mustWallet(t, svc, a, "Northbank pesos", "UYU", 0)
	clothes := categoryNamed(t, svc, a, ledger.CategoryExpense, "Shopping")
	on := func(s string) time.Time { v, _ := time.Parse(time.DateOnly, s); return v }
	yes, no := true, false

	// 100 USD -> 4000 UYU: UYU expenses count at 1/40.
	if _, err := svc.CreateTransfer(ctx, a, ledger.TransferInput{
		FromWalletID: usd.ID, ToWalletID: uyu.ID, FromAmount: 100_00, ToAmount: 4000_00, Rate: "40", OccurredOn: on("2026-06-01"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []ledger.TransactionInput{
		{WalletID: usd.ID, CategoryID: clothes.ID, Amount: 60_00, OccurredOn: on("2026-07-12")},   // July: 60
		{WalletID: uyu.ID, CategoryID: clothes.ID, Amount: 4000_00, OccurredOn: on("2026-08-02")}, // August: 100 USD
		{WalletID: usd.ID, CategoryID: clothes.ID, Amount: 50_00, OccurredOn: on("2026-08-20")},   // August: 150 in all
		{WalletID: usd.ID, CategoryID: clothes.ID, Amount: 20_00, OccurredOn: on("2026-10-03")},   // October: 20
		{WalletID: usd.ID, CategoryID: clothes.ID, Amount: 500_00, OccurredOn: on("2026-10-20")},  // upcoming
	} {
		if _, err := svc.CreateTransaction(ctx, a, in); err != nil {
			t.Fatal(err)
		}
	}

	input := func(amount int64, leftover, overspend *bool, today string) ledger.BudgetInput {
		return ledger.BudgetInput{
			Name: "Clothes", Amount: amount, Currency: "USD", Repeat: ledger.RepeatMonthly, Start: on("2026-01-01"),
			CategoryIDs: []uuid.UUID{clothes.ID}, CarryLeftover: leftover, CarryOverspend: overspend, Today: on(today),
		}
	}
	// Rollover turned on in July starts the chain at July's period.
	b, err := svc.CreateBudget(ctx, a, input(100_00, &yes, &yes, "2026-07-10"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.CarryLeftover || !b.CarryOverspend || b.RolloverFrom == nil || !b.RolloverFrom.Equal(on("2026-07-01")) {
		t.Fatalf("created %+v", b)
	}

	today := on("2026-10-15")
	// check compares the carry into October, September, August, ... (newest first).
	check := func(step string, wantCarried ...int64) {
		t.Helper()
		list, _, err := svc.ListBudgets(ctx, a, today)
		if err != nil {
			t.Fatal(err)
		}
		cur := list[0].Current
		if cur.Carried != wantCarried[0] || cur.Limit != list[0].Budget.Amount+wantCarried[0] || cur.Spent != 20_00 ||
			cur.Remaining != cur.Limit-20_00 {
			t.Fatalf("%s: current %+v, want carried %d", step, cur, wantCarried[0])
		}
		d, err := svc.BudgetDetailFor(ctx, a, b.ID, today, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range wantCarried {
			if d.History[i].Carried != want {
				t.Fatalf("%s: history[%d] carried %d, want %d (%+v)", step, i, d.History[i].Carried, want, d.History[i])
			}
		}
		if d.Progress.Carried != wantCarried[0] {
			t.Fatalf("%s: detail progress %+v", step, d.Progress)
		}
	}
	// July 100-60 = +40; August 140-150 = -10; September 90-0 = +90; nothing before July.
	check("both", 90_00, -10_00, 40_00, 0, 0)

	aug := on("2026-08-10")
	d, err := svc.BudgetDetailFor(ctx, a, b.ID, today, &aug)
	if err != nil {
		t.Fatal(err)
	}
	if d.Progress.Limit != 140_00 || d.Progress.Spent != 150_00 || d.Progress.Remaining != -10_00 {
		t.Fatalf("august %+v", d.Progress)
	}

	// Editing the amount mid-chain recomputes every carry; omitted flags and the chain start stay.
	b, err = svc.UpdateBudget(ctx, a, b.ID, input(200_00, nil, nil, "2026-10-15"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.CarryLeftover || !b.CarryOverspend || !b.RolloverFrom.Equal(on("2026-07-01")) {
		t.Fatalf("after amount edit %+v", b)
	}
	// July +140; August 340-150 = +190; September 390-0 = +390.
	check("amount edited", 390_00, 190_00, 140_00, 0)

	// Overspend only: July's leftover is dropped, August 100-150 = -50, September's 50 left is dropped.
	b, err = svc.UpdateBudget(ctx, a, b.ID, input(100_00, &no, &yes, "2026-10-15"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.RolloverFrom.Equal(on("2026-07-01")) {
		t.Fatalf("one flag off moved the chain start: %+v", b)
	}
	check("overspend only", 0, -50_00, 0, 0)

	// Both off clears the chain; turning rollover on again starts it at today's period.
	b, err = svc.UpdateBudget(ctx, a, b.ID, input(100_00, &no, &no, "2026-10-15"))
	if err != nil {
		t.Fatal(err)
	}
	if b.RolloverFrom != nil || b.CarryLeftover || b.CarryOverspend {
		t.Fatalf("rollover off %+v", b)
	}
	check("off", 0, 0, 0)
	b, err = svc.UpdateBudget(ctx, a, b.ID, input(100_00, &yes, nil, "2026-10-15"))
	if err != nil {
		t.Fatal(err)
	}
	if b.RolloverFrom == nil || !b.RolloverFrom.Equal(on("2026-10-01")) || !b.CarryLeftover || b.CarryOverspend {
		t.Fatalf("rollover on again %+v", b)
	}
	check("on again", 0, 0, 0)

	// One-time budgets can't roll over; omitted flags on a new budget mean off.
	end := on("2026-12-31")
	once := ledger.BudgetInput{
		Name: "Trip", Amount: 100_00, Currency: "USD", Repeat: ledger.RepeatOnce, Start: on("2026-10-01"), End: &end, CarryLeftover: &yes,
	}
	var inv *ledger.InvalidError
	if _, err := svc.CreateBudget(ctx, a, once); !errors.As(err, &inv) || inv.Field != "carry_leftover" {
		t.Fatalf("once with carry: %v", err)
	}
	once.CarryLeftover = nil
	plain, err := svc.CreateBudget(ctx, a, once)
	if err != nil {
		t.Fatal(err)
	}
	if plain.CarryLeftover || plain.CarryOverspend || plain.RolloverFrom != nil {
		t.Fatalf("plain %+v", plain)
	}
}
