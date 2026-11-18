package ledger_test

import (
	"context"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/importer/pdftest"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

// A made-up card statement: an instalment, a fee, last month's payment, and this month's total,
// debited automatically from the bank account on the due date.
func cardStatement(payment string) []byte {
	return pdftest.ItauVisa("03/11/26", "16/11/26", "14/12/26", []pdftest.ItauVisaRow{
		{Date: "19 10 26", Detail: "PAGOS", Pesos: payment},
		{Date: "13 06 26", Card: "1234", Detail: "MUEBLERIA SUR", Instalment: "2/ 3", Pesos: "1500,00"},
		{Detail: "SEGURO DE VIDA SOBRE SALDO", Pesos: "12,00"},
	}, "1.512,00", "0,00", true)
}

type cardFixture struct {
	importFixture
	bank, card ledger.Wallet
}

func newCardFixture(t *testing.T) cardFixture {
	svc := ledger.New(testdb.Pool(t))
	a := newActor(t, svc)
	f := cardFixture{
		importFixture: importFixture{
			t: t, svc: svc, a: a,
			expense: categoryNamed(t, svc, a, ledger.CategoryExpense, "Groceries").ID,
			income:  categoryNamed(t, svc, a, ledger.CategoryIncome, "Other").ID,
		},
		bank: mustWallet(t, svc, a, "Southbank dollars", "USD", 0),
		card: mustWallet(t, svc, a, "Visa", "UYU", 0),
	}
	return f
}

// upload stages a file for a wallet and reads it with the suggested mapping, changed by edit.
func (f cardFixture) upload(wallet uuid.UUID, name string, content []byte, edit func(*importer.Mapping)) (ledger.ImportUpload, importer.Mapping, []ledger.ImportRow) {
	f.t.Helper()
	ctx := context.Background()
	up, err := f.svc.UploadImport(ctx, f.a, wallet, name, content)
	if err != nil {
		f.t.Fatal(err)
	}
	m := up.Mapping
	if edit != nil {
		edit(&m)
	}
	rows, _, err := f.svc.PreviewImport(ctx, f.a, up.BatchID, m, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	return up, m, rows
}

func (f cardFixture) commitAll(batch uuid.UUID, m importer.Mapping, rows []ledger.ImportRow) int64 {
	f.t.Helper()
	var choices []ledger.ImportChoice
	for _, r := range rows {
		if r.Err != "" || r.Duplicate() {
			continue
		}
		c := ledger.ImportChoice{Index: r.Index}
		switch r.Kind {
		case ledger.KindExpense:
			c.CategoryID = f.expense
		case ledger.KindIncome:
			c.CategoryID = f.income
		}
		choices = append(choices, c)
	}
	n, err := f.svc.CommitImport(context.Background(), f.a, batch, m, choices, true)
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f cardFixture) list(wallet uuid.UUID) []ledger.Transaction {
	f.t.Helper()
	txs, _, err := f.svc.ListTransactions(context.Background(), f.a, ledger.Filter{
		WalletID: wallet, From: on("2026-01-01"), To: on("2026-12-31"), Today: on("2026-12-31"),
	}, 100, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return txs
}

const bankCSV = statementHeader + "16/11/2026;DEB. VARIOS TARJ. VISA;-37,80\n10/11/2026;COMPRA KIOSCO AZUL;-5,00\n"

func TestCardStatementTurnsTheDebitIntoATransfer(t *testing.T) {
	f := newCardFixture(t)
	ctx := context.Background()

	// The bank account's statement, with the card's debit as an expense.
	up, m, rows := f.upload(f.bank.ID, "cuenta.csv", []byte(bankCSV), nil)
	if n := f.commitAll(up.BatchID, m, rows); n != 2 {
		t.Fatalf("bank import: %d", n)
	}

	// The card's statement, before choosing the wallet that pays it.
	up, m, rows = f.upload(f.card.ID, "V_22.pdf", cardStatement("-1.000,00"), nil)
	if up.Info.Format != "pdf" || up.Statement == nil || up.Statement.Due != on("2026-11-16") || m.PaidFrom != "" {
		t.Fatalf("upload = %+v", up)
	}
	if len(rows) != 4 || rows[0].Err == "" || rows[3].Err == "" {
		t.Fatalf("payments without a paying wallet: %+v", rows)
	}

	// Paid from the bank account: the total's payment finds the debit on the due date; last
	// month's payment has no debit to match (it was before this ledger).
	up, m, rows = f.upload(f.card.ID, "V_22.pdf", cardStatement("-1.000,00"), func(m *importer.Mapping) {
		m.PaidFrom = f.bank.ID.String()
	})
	if rows[0].Err == "" || rows[0].Kind != ledger.KindTransferIn {
		t.Fatalf("last month's payment = %+v", rows[0])
	}
	if rows[1].Note != "MUEBLERIA SUR 2/3 (13/06/26)" || rows[1].Date != on("2026-11-03") || rows[1].Kind != ledger.KindExpense {
		t.Fatalf("instalment = %+v", rows[1])
	}
	pay := rows[3]
	if pay.Err != "" || pay.Kind != ledger.KindTransferIn || pay.Payment == nil || pay.Payment.Amount != 37_80 ||
		pay.Payment.WalletID != f.bank.ID || pay.Amount != 1512_00 {
		t.Fatalf("payment = %+v (%+v)", pay, pay.Payment)
	}
	if n := f.commitAll(up.BatchID, m, rows); n != 3 {
		t.Fatalf("card import: %d", n)
	}

	// The debit is now a transfer: 37.80 USD out of the bank, 1,512.00 UYU into the card.
	var out *ledger.Transaction
	for _, tx := range f.list(f.bank.ID) {
		if tx.Note == "DEB. VARIOS TARJ. VISA" {
			out = &tx
		}
	}
	if out == nil || out.Kind != ledger.KindTransferOut || out.Amount != 37_80 || out.OccurredOn != on("2026-11-16") ||
		out.Counterpart == nil || out.Counterpart.Amount != 1512_00 || out.Counterpart.WalletID != f.card.ID {
		t.Fatalf("bank row = %+v", out)
	}
	if out.Counterpart.Rate != "40.0000000000" {
		t.Fatalf("rate = %s", out.Counterpart.Rate)
	}
	card, err := f.svc.GetWalletOn(ctx, f.a, f.card.ID, on("2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if card.Balance != 0 {
		t.Fatalf("card balance = %d", card.Balance)
	}

	// The next upload of either statement knows every row.
	_, _, rows = f.upload(f.bank.ID, "cuenta.csv", []byte(bankCSV), nil)
	for _, r := range rows {
		if r.DupReason != ledger.DupImported {
			t.Errorf("bank row %d = %q", r.Index, r.DupReason)
		}
	}
	up, _, rows = f.upload(f.card.ID, "V_22.pdf", cardStatement("-1.000,00"), nil)
	if up.Mapping.PaidFrom != f.bank.ID.String() || !up.SavedMapping {
		t.Fatalf("saved paying wallet = %+v", up.Mapping)
	}
	for _, r := range rows[1:] {
		if r.DupReason != ledger.DupImported {
			t.Errorf("card row %d = %q %s", r.Index, r.DupReason, r.Err)
		}
	}

	// Next month's statement lists that payment again, as "PAGOS": it's the same transfer.
	next := pdftest.ItauVisa("03/12/26", "14/12/26", "16/01/27", []pdftest.ItauVisaRow{
		{Date: "17 11 26", Detail: "PAGOS", Pesos: "-1.512,00"},
	}, "0,00", "0,00", true)
	_, _, rows = f.upload(f.card.ID, "V_23.pdf", next, nil)
	if len(rows) != 1 || rows[0].DupReason != ledger.DupSimilar || rows[0].MatchDate != on("2026-11-16") {
		t.Fatalf("next month's payment = %+v", rows)
	}
}

func TestCardStatementPayingWalletOfAnotherHousehold(t *testing.T) {
	f := newCardFixture(t)
	ctx := context.Background()
	other := newActor(t, f.svc)
	theirs := mustWallet(t, f.svc, other, "Their bank", "USD", 0)
	up, err := f.svc.UploadImport(ctx, f.a, f.card.ID, "V_22.pdf", cardStatement("-1.000,00"))
	if err != nil {
		t.Fatal(err)
	}
	m := up.Mapping
	m.PaidFrom = theirs.ID.String()
	if _, _, err := f.svc.PreviewImport(ctx, f.a, up.BatchID, m, 0); err == nil {
		t.Fatal("another household's wallet pays the card")
	}
}

func TestCardStatementInAnotherCurrency(t *testing.T) {
	f := newCardFixture(t)
	if _, err := f.svc.UploadImport(context.Background(), f.a, f.bank.ID, "V_22.pdf", cardStatement("-1.000,00")); err == nil {
		t.Fatal("a pesos statement imported into a dollar wallet")
	}
}
