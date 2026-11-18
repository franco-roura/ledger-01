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

// A made-up card statement in pesos: an instalment, a fee, last month's payment, and this
// month's total, debited automatically on the due date.
func cardStatement(payment string) []byte {
	return pdftest.ItauVisa("03/11/26", "16/11/26", "14/12/26", []pdftest.ItauVisaRow{
		{Date: "19 10 26", Detail: "PAGOS", Pesos: payment},
		{Date: "13 06 26", Card: "1234", Detail: "MUEBLERIA SUR", Instalment: "2/ 3", Pesos: "1500,00"},
		{Detail: "SEGURO DE VIDA SOBRE SALDO", Pesos: "12,00"},
	}, "1.512,00", "0,00", true)
}

// The same card spending in both currencies.
func twoCurrencyStatement() []byte {
	return pdftest.ItauVisa("03/11/26", "16/11/26", "14/12/26", []pdftest.ItauVisaRow{
		{Date: "13 06 26", Card: "1234", Detail: "MUEBLERIA SUR", Instalment: "2/ 3", Pesos: "1500,00"},
		{Detail: "SEGURO DE VIDA SOBRE SALDO", Pesos: "12,00"},
		{Date: "02 10 26", Card: "1234", Detail: "LIBRERIA NORTE", Dollars: "20,00"},
	}, "1.512,00", "20,00", true)
}

type cardFixture struct {
	importFixture
	// Two accounts that pay the card and its wallets in each currency.
	pesos, dollars   ledger.Wallet
	cardUYU, cardUSD ledger.Wallet
}

func newCardFixture(t *testing.T) cardFixture {
	svc := ledger.New(testdb.Pool(t))
	a := newActor(t, svc)
	return cardFixture{
		importFixture: importFixture{
			t: t, svc: svc, a: a,
			expense: categoryNamed(t, svc, a, ledger.CategoryExpense, "Groceries").ID,
			income:  categoryNamed(t, svc, a, ledger.CategoryIncome, "Other").ID,
		},
		pesos:   mustWallet(t, svc, a, "Southbank pesos", "UYU", 0),
		dollars: mustWallet(t, svc, a, "Southbank dollars", "USD", 0),
		cardUYU: mustWallet(t, svc, a, "Visa", "UYU", 0),
		cardUSD: mustWallet(t, svc, a, "Visa dollars", "USD", 0),
	}
}

// debit adds a card's debit to an account, as the account's own statement would.
func (f cardFixture) debit(w ledger.Wallet, amount int64) {
	f.t.Helper()
	if _, err := f.svc.CreateTransaction(context.Background(), f.a, ledger.TransactionInput{
		WalletID: w.ID, CategoryID: f.expense, Amount: amount, OccurredOn: on("2026-11-16"),
		Note: "DEB. VARIOS TARJ. VISA",
	}); err != nil {
		f.t.Fatal(err)
	}
}

// rate records 40 pesos per dollar, as an earlier exchange between the accounts.
func (f cardFixture) rate() {
	f.t.Helper()
	if _, err := f.svc.CreateTransfer(context.Background(), f.a, ledger.TransferInput{
		FromWalletID: f.dollars.ID, ToWalletID: f.pesos.ID, FromAmount: 100_00, ToAmount: 4000_00, Rate: "40",
		OccurredOn: on("2026-11-01"),
	}); err != nil {
		f.t.Fatal(err)
	}
}

// upload stages a file for a wallet and reads it with the suggested mapping, changed by edit.
func (f cardFixture) upload(wallet uuid.UUID, content []byte, edit func(*importer.Mapping)) (ledger.ImportUpload, importer.Mapping, []ledger.ImportRow) {
	f.t.Helper()
	ctx := context.Background()
	up, err := f.svc.UploadImport(ctx, f.a, wallet, "V_22.pdf", content)
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

// link makes cardUYU and cardUSD the card's wallets.
func (f cardFixture) link(m *importer.Mapping) {
	m.CardWallets = map[string]string{"UYU": f.cardUYU.ID.String(), "USD": f.cardUSD.ID.String()}
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

func (f cardFixture) balance(w ledger.Wallet) int64 {
	f.t.Helper()
	got, err := f.svc.GetWalletOn(context.Background(), f.a, w.ID, on("2026-12-31"))
	if err != nil {
		f.t.Fatal(err)
	}
	return got.Balance
}

const bankCSV = statementHeader + "16/11/2026;DEB. VARIOS TARJ. VISA;-37,80\n10/11/2026;COMPRA KIOSCO AZUL;-5,00\n"

func TestCardStatementTurnsTheDebitIntoATransfer(t *testing.T) {
	f := newCardFixture(t)

	// The dollar account's statement, with the card's debit as an expense.
	up, err := f.svc.UploadImport(context.Background(), f.a, f.dollars.ID, "cuenta.csv", []byte(bankCSV))
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := f.svc.PreviewImport(context.Background(), f.a, up.BatchID, up.Mapping, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := f.commitAll(up.BatchID, up.Mapping, rows); n != 2 {
		t.Fatalf("bank import: %d", n)
	}

	// The card's statement: the card is the wallet it's dropped on, and every other wallet may
	// pay it (no paying accounts listed). With no exchange in the household yet, the only debit in dollars is taken
	// unchecked.
	up, m, rows := f.upload(f.cardUYU.ID, cardStatement("-1.000,00"), nil)
	if up.Info.Format != "pdf" || up.Statement == nil || up.Statement.Due != on("2026-11-16") || up.SavedMapping {
		t.Fatalf("upload = %+v", up)
	}
	if len(up.Statement.Currencies) != 1 || up.Statement.Currencies[0] != (ledger.StatementCurrency{Code: "UYU", Lines: 4, Spent: 1512_00}) {
		t.Fatalf("currencies = %+v", up.Statement.Currencies)
	}
	if m.CardWallets["UYU"] != f.cardUYU.ID.String() || len(m.PayingWallets) != 0 {
		t.Fatalf("mapping = %+v", m)
	}
	if rows[0].Err == "" || rows[0].Kind != ledger.KindTransferIn {
		t.Fatalf("last month's payment = %+v", rows[0])
	}
	if rows[1].Note != "MUEBLERIA SUR 2/3 (13/06/26)" || rows[1].Date != on("2026-11-03") || rows[1].WalletID != f.cardUYU.ID {
		t.Fatalf("instalment = %+v", rows[1])
	}
	pay := rows[3]
	if pay.Err != "" || len(pay.Payments) != 1 || pay.Payments[0].Amount != 37_80 || pay.Payments[0].WalletID != f.dollars.ID ||
		pay.Payments[0].Part != 1512_00 || !pay.Payments[0].RateUnchecked {
		t.Fatalf("payment = %+v (%+v)", pay, pay.Payments)
	}
	if n := f.commitAll(up.BatchID, m, rows); n != 3 {
		t.Fatalf("card import: %d", n)
	}

	// The debit is now a transfer: 37.80 USD out of the account, 1,512.00 UYU into the card.
	var out *ledger.Transaction
	for _, tx := range f.list(f.dollars.ID) {
		if tx.Note == "DEB. VARIOS TARJ. VISA" {
			out = &tx
		}
	}
	if out == nil || out.Kind != ledger.KindTransferOut || out.Amount != 37_80 || out.Counterpart == nil ||
		out.Counterpart.Amount != 1512_00 || out.Counterpart.WalletID != f.cardUYU.ID || out.Counterpart.Rate != "40.0000000000" {
		t.Fatalf("bank row = %+v (%+v)", out, out.Counterpart)
	}
	if b := f.balance(f.cardUYU); b != 0 {
		t.Fatalf("card balance = %d", b)
	}

	// The next upload of either statement knows every row, and the card needs no questions.
	_, _, rows = f.upload(f.dollars.ID, []byte(bankCSV), nil)
	for _, r := range rows {
		if r.DupReason != ledger.DupImported {
			t.Errorf("bank row %d = %q", r.Index, r.DupReason)
		}
	}
	up, _, rows = f.upload(f.cardUYU.ID, cardStatement("-1.000,00"), nil)
	if !up.SavedMapping {
		t.Fatalf("saved mapping = %+v", up.Mapping)
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
	_, _, rows = f.upload(f.cardUYU.ID, next, nil)
	if len(rows) != 1 || rows[0].DupReason != ledger.DupSimilar || rows[0].MatchDate != on("2026-11-16") {
		t.Fatalf("next month's payment = %+v", rows)
	}
}

func TestCardStatementDroppedOnTheOtherCurrencysWallet(t *testing.T) {
	f := newCardFixture(t)
	f.debit(f.pesos, 1512_00)

	// A pesos-only month dropped on the card's dollar wallet: nothing fails, the pesos lines wait
	// for a pesos wallet.
	up, m, rows := f.upload(f.cardUSD.ID, cardStatement("-1.000,00"), nil)
	if up.SavedMapping || len(up.Statement.Currencies) != 1 || up.Statement.Currencies[0].Code != "UYU" {
		t.Fatalf("upload = %+v", up)
	}
	for _, r := range rows {
		if r.Err != "choose a UYU wallet for this card" {
			t.Fatalf("row %d = %q", r.Index, r.Err)
		}
	}

	// Linked to the pesos wallet, they import there; the payment's debit is in the pesos account
	// (found among every account that isn't the card's).
	up, m, rows = f.upload(f.cardUSD.ID, cardStatement("-1.000,00"), f.link)
	for _, r := range rows[1:] {
		if r.Err != "" || r.WalletID != f.cardUYU.ID || r.Currency != "UYU" {
			t.Fatalf("row %d = %+v", r.Index, r)
		}
	}
	if p := rows[3].Payments; len(p) != 1 || p[0].WalletID != f.pesos.ID || p[0].RateUnchecked {
		t.Fatalf("payment = %+v", p)
	}
	if n := f.commitAll(up.BatchID, m, rows); n != 3 {
		t.Fatalf("imported %d", n)
	}
	if b := f.balance(f.cardUYU); b != 0 {
		t.Fatalf("pesos card balance = %d", b)
	}
	if n := len(f.list(f.cardUSD.ID)); n != 0 {
		t.Fatalf("dollar card has %d rows", n)
	}

	// The link is saved on both card wallets: the next statement goes straight to the review,
	// from either.
	for _, w := range []ledger.Wallet{f.cardUYU, f.cardUSD} {
		up, m, _ := f.upload(w.ID, twoCurrencyStatement(), nil)
		if !up.SavedMapping || m.CardWallets["UYU"] != f.cardUYU.ID.String() || m.CardWallets["USD"] != f.cardUSD.ID.String() {
			t.Fatalf("%s: saved = %v, mapping = %+v", w.Name, up.SavedMapping, m)
		}
	}
}

func TestCardStatementPaymentsAcrossAccounts(t *testing.T) {
	f := newCardFixture(t)
	f.rate()
	// Both totals were taken from the dollar account: the pesos one at about 40 per dollar.
	f.debit(f.dollars, 20_00)
	f.debit(f.dollars, 37_80)

	_, _, rows := f.upload(f.cardUYU.ID, twoCurrencyStatement(), f.link)
	got := map[string][]ledger.CardDebit{}
	for _, r := range rows {
		if r.Kind == ledger.KindTransferIn {
			if r.Err != "" {
				t.Fatalf("payment %s: %s", r.Currency, r.Err)
			}
			got[r.Currency] = r.Payments
		}
	}
	// The dollar total takes the debit of the same amount; the pesos one the other, by rate.
	if usd := got["USD"]; len(usd) != 1 || usd[0].Amount != 20_00 || usd[0].Part != 20_00 {
		t.Fatalf("USD payment = %+v", usd)
	}
	if uyu := got["UYU"]; len(uyu) != 1 || uyu[0].Amount != 37_80 || uyu[0].Part != 1512_00 || uyu[0].RateUnchecked {
		t.Fatalf("UYU payment = %+v", uyu)
	}
}

func TestCardStatementPaidFromTwoAccounts(t *testing.T) {
	f := newCardFixture(t)
	f.rate()
	// The pesos account had 1,000.00; the bank took the other 512.00 as 12.80 dollars.
	f.debit(f.pesos, 1000_00)
	f.debit(f.dollars, 12_80)

	up, m, rows := f.upload(f.cardUYU.ID, cardStatement("-1.000,00"), nil)
	p := rows[3].Payments
	if rows[3].Err != "" || len(p) != 2 || p[0].WalletID != f.pesos.ID || p[0].Part != 1000_00 ||
		p[1].WalletID != f.dollars.ID || p[1].Part != 512_00 {
		t.Fatalf("payment = %+v (%s)", p, rows[3].Err)
	}
	if n := f.commitAll(up.BatchID, m, rows); n != 3 {
		t.Fatalf("imported %d", n)
	}
	if b := f.balance(f.cardUYU); b != 0 {
		t.Fatalf("card balance = %d", b)
	}
	for _, w := range []ledger.Wallet{f.pesos, f.dollars} {
		for _, tx := range f.list(w.ID) {
			if tx.Note == "DEB. VARIOS TARJ. VISA" && tx.Kind != ledger.KindTransferOut {
				t.Errorf("%s still has the debit as %s", w.Name, tx.Kind)
			}
		}
	}
}

func TestCardStatementWalletsOfAnotherHousehold(t *testing.T) {
	f := newCardFixture(t)
	ctx := context.Background()
	other := newActor(t, f.svc)
	theirs := mustWallet(t, f.svc, other, "Their bank", "UYU", 0)
	up, err := f.svc.UploadImport(ctx, f.a, f.cardUSD.ID, "V_22.pdf", cardStatement("-1.000,00"))
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*importer.Mapping){
		"card wallet":    func(m *importer.Mapping) { m.CardWallets = map[string]string{"UYU": theirs.ID.String()} },
		"paying account": func(m *importer.Mapping) { m.PayingWallets = []string{theirs.ID.String()} },
	} {
		m := up.Mapping
		edit(&m)
		if _, _, err := f.svc.PreviewImport(ctx, f.a, up.BatchID, m, 0); err == nil {
			t.Errorf("another household's wallet as the %s", name)
		}
	}
}
