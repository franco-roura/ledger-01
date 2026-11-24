package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/google/uuid"
)

func (f importFixture) rule(pattern, rename string, cat uuid.UUID) (ledger.CategoryRule, int) {
	f.t.Helper()
	r, n, _, err := f.svc.CreateCategoryRule(context.Background(), f.a, ledger.CategoryRule{Pattern: pattern, CategoryID: cat, Rename: rename})
	if err != nil {
		f.t.Fatal(err)
	}
	return r, n
}

func (f importFixture) notes() map[string]bool {
	out := map[string]bool{}
	for _, tx := range f.transactions() {
		out[tx.Note] = true
	}
	return out
}

// A rule's rename is what matched rows import as; the bank text still finds them later.
func TestImportRuleRenames(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	f.rule("UBER", "Rides", f.expense)
	r, _ := f.rule("dlo*uber*rid", " Uber ", f.expense) // longer, so it wins
	if r.Rename != "Uber" {
		t.Fatalf("rename = %q", r.Rename)
	}
	f.rule("PAY*TRANS", "Bus", f.income) // income only

	body := "01/10/2026;COMPRA DLO*UBER*RID;-310,00\n02/10/2026;COMPRA UBER EATS;-500,00\n" +
		"03/10/2026;COMPRA PAY*TRANS*BUS;-7,40\n04/10/2026;COMPRA KIOSCO AZUL;-90,00\n"
	batch, m, rows := f.review(body)
	for i, want := range []string{"Uber", "Rides", "", ""} {
		if rows[i].Rename != want {
			t.Errorf("row %d rename = %q, want %q", i, rows[i].Rename, want)
		}
	}
	if rows[0].Note != "COMPRA DLO*UBER*RID" || rows[0].CategorySource != "rule" {
		t.Fatalf("row 0 = %+v", rows[0])
	}

	// A one-off name wins over the rule, for that row only; a long one is refused.
	long := strings.Repeat("x", 501)
	var inv *ledger.InvalidError
	if _, err := f.svc.CommitImport(ctx, f.a, batch, m, []ledger.ImportChoice{{Index: 3, CategoryID: f.expense, Note: &long}}, false); !errors.As(err, &inv) {
		t.Fatalf("long note: %v", err)
	}
	pizza := "  Pizza with Ana "
	var choices []ledger.ImportChoice
	for _, r := range rows {
		c := ledger.ImportChoice{Index: r.Index, CategoryID: f.expense}
		if r.Index == 3 {
			c.Note = &pizza
		}
		choices = append(choices, c)
	}
	if _, err := f.svc.CommitImport(ctx, f.a, batch, m, choices, true); err != nil {
		t.Fatal(err)
	}
	got := f.notes()
	for _, want := range []string{"Uber", "Rides", "COMPRA PAY*TRANS*BUS", "Pizza with Ana"} {
		if !got[want] {
			t.Errorf("no transaction named %q in %v", want, got)
		}
	}

	// The same file again: every row was imported already, renamed or not.
	_, _, rows = f.review(body)
	for i, r := range rows {
		if r.DupReason != ledger.DupImported {
			t.Errorf("row %d again = %q", i, r.DupReason)
		}
	}
	// The renamed purchase's discount still finds it by its bank text.
	_, _, rows = f.review("05/10/2026;REDIVA 19210DLO*UBER*RID;10,00\n")
	if rows[0].DiscountTarget == nil || rows[0].TargetNote != "Uber" {
		t.Fatalf("discount = %+v", rows[0])
	}
}

// A rename applies even when the category column settled the category, and a hand-entered row
// with the renamed note is the same transaction.
func TestImportRuleRenameMatchesHandEntered(t *testing.T) {
	f := newImportFixture(t)
	f.rule("KIOSCO AZUL", "Kiosk", f.expense)
	if _, err := f.svc.CreateTransaction(context.Background(), f.a, ledger.TransactionInput{
		WalletID: f.wallet, CategoryID: f.expense, Amount: 90_00, OccurredOn: on("2026-10-04"), Note: "kiosk",
	}); err != nil {
		t.Fatal(err)
	}
	_, _, rows := f.review("04/10/2026;COMPRA KIOSCO AZUL;-90,00\n")
	if rows[0].Rename != "Kiosk" || rows[0].DupReason != ledger.DupSame {
		t.Fatalf("row = %q %q", rows[0].Rename, rows[0].DupReason)
	}
}

// Renaming what was imported before: only rows the rule owns, and never a note edited by hand.
func TestRenameRuleTransactions(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	batch, m, rows := f.review("01/10/2026;COMPRA ANCEL-IAJ290;-1200,00\n02/10/2026;COMPRA ANCEL-IAJ290;-1200,00\n" +
		"03/10/2026;COMPRA ANCEL FIBRA;-900,00\n04/10/2026;COMPRA ANCEL-IAJ290;-50,00\n")
	f.commit(batch, m, rows)
	// One edited by hand.
	for _, tx := range f.transactions() {
		if tx.Amount == 50_00 {
			if _, err := f.svc.UpdateTransaction(ctx, f.a, tx.ID, ledger.TransactionInput{
				WalletID: f.wallet, CategoryID: f.expense, Amount: tx.Amount, OccurredOn: tx.OccurredOn, Note: "Top-up for Ana",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// "ANCEL FIBRA" belongs to a longer rule.
	if _, n := f.rule("ancel fibra", "Home internet", f.expense); n != 1 {
		t.Fatalf("fibra renamable = %d", n)
	}
	r, n := f.rule("ANCEL", "Phone", f.expense)
	if n != 2 {
		t.Fatalf("renamable = %d", n)
	}
	got, err := f.svc.RenameRuleTransactions(ctx, f.a, r.ID, "")
	if err != nil || got != 2 {
		t.Fatalf("renamed = %d, %v", got, err)
	}
	notes := f.notes()
	if !notes["Phone"] || !notes["Top-up for Ana"] || !notes["COMPRA ANCEL FIBRA"] {
		t.Fatalf("notes = %v", notes)
	}

	// Changing the rename follows the earlier one, still leaving the hand edit alone.
	r, n, err = f.svc.UpdateCategoryRule(ctx, f.a, ledger.CategoryRule{ID: r.ID, Pattern: "ANCEL-IAJ", CategoryID: f.expense, Rename: "Antel Celu"})
	if err != nil || n != 2 || r.Pattern != "ANCEL-IAJ" {
		t.Fatalf("update = %+v %d %v", r, n, err)
	}
	if got, err := f.svc.RenameRuleTransactions(ctx, f.a, r.ID, "Phone"); err != nil || got != 2 {
		t.Fatalf("renamed again = %d, %v", got, err)
	}
	notes = f.notes()
	if !notes["Antel Celu"] || notes["Phone"] || !notes["Top-up for Ana"] {
		t.Fatalf("notes = %v", notes)
	}
	// Nothing left to do.
	if got, _ := f.svc.RenameRuleTransactions(ctx, f.a, r.ID, "Phone"); got != 0 {
		t.Fatalf("renamed a third time = %d", got)
	}

	// Another household's rule isn't reachable, and renames are validated.
	other := newImportFixture(t)
	if _, err := other.svc.RenameRuleTransactions(ctx, other.a, r.ID, ""); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("other household: %v", err)
	}
	var inv *ledger.InvalidError
	if _, _, err := f.svc.UpdateCategoryRule(ctx, f.a, ledger.CategoryRule{ID: r.ID, Pattern: "ANCEL", CategoryID: f.expense, Rename: strings.Repeat("x", 101)}); !errors.As(err, &inv) || inv.Field != "rename" {
		t.Fatalf("long rename: %v", err)
	}
}
