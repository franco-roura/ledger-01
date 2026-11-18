package uy

import (
	"os"
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/importer/pdftest"
)

// A made-up statement laid out like Itaú's Visa PDF.
func fakeItauVisa(autoDebit bool, dollars string) []importer.TextLine {
	b := pdftest.ItauVisa("03/11/26", "16/11/26", "14/12/26", []pdftest.ItauVisaRow{
		{Date: "19 10 26", Detail: "PAGOS", Pesos: "-1.000,00"},
		{Date: "13 06 26", Card: "1234", Detail: "MUEBLERIA SUR", Instalment: "2/ 3", Pesos: "1500,00"},
		{Detail: "SEGURO DE VIDA SOBRE SALDO", Pesos: "12,00"},
		{Date: "02 10 26", Card: "1234", Detail: "LIBRERIA NORTE", Dollars: dollars},
	}, "1.512,00", dollars, autoDebit)
	lines, err := importer.PDFLines(b)
	if err != nil {
		panic(err)
	}
	return lines
}

func TestReadItauVisa(t *testing.T) {
	st, ok, err := ItauVisa.Statement(fakeItauVisa(true, "20,00"))
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if st.Issued.Format(time.DateOnly) != "2026-11-03" || st.Due.Format(time.DateOnly) != "2026-11-16" || st.Decimal != ',' {
		t.Fatalf("statement = %+v", st)
	}
	want := []importer.StatementLine{
		{Date: day("2026-10-19"), Detail: "PAGOS", Currency: "UYU", Amount: "-1.000,00"},
		{Date: day("2026-06-13"), Detail: "MUEBLERIA SUR", Instalment: "2/3", Currency: "UYU", Amount: "1500,00"},
		{Detail: "SEGURO DE VIDA SOBRE SALDO", Currency: "UYU", Amount: "12,00"},
		{Date: day("2026-10-02"), Detail: "LIBRERIA NORTE", Currency: "USD", Amount: "20,00"},
		// The automatic debits of both totals, on the due date.
		{Date: day("2026-11-16"), Detail: itauAutoDebitNote, Currency: "UYU", Amount: "-1.512,00"},
		{Date: day("2026-11-16"), Detail: itauAutoDebitNote, Currency: "USD", Amount: "-20,00"},
	}
	if len(st.Lines) != len(want) {
		t.Fatalf("lines = %+v", st.Lines)
	}
	for i, w := range want {
		if st.Lines[i] != w {
			t.Errorf("line %d = %+v, want %+v", i, st.Lines[i], w)
		}
	}
	for _, note := range []string{"PAGOS", itauAutoDebitNote} {
		if !ItauVisa.CardPayment(note) {
			t.Errorf("%q isn't a payment", note)
		}
	}
	if ItauVisa.CardPayment("MUEBLERIA SUR") {
		t.Error("a purchase is a payment")
	}
}

func TestReadItauVisaWithoutAutoDebit(t *testing.T) {
	st, ok, err := ItauVisa.Statement(fakeItauVisa(false, "0,00"))
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	// No debit sentence: no payment added; a zero amount isn't a movement.
	for _, l := range st.Lines {
		if l.Detail == itauAutoDebitNote || l.Currency == "USD" {
			t.Errorf("unexpected line %+v", l)
		}
	}
}

func TestReadItauVisaOtherPDF(t *testing.T) {
	lines, err := importer.PDFLines(pdftest.PDF([]pdftest.Text{{X: 50, Y: 100, S: "An invoice"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ItauVisa.Statement(lines); ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
}

func TestItauCardDebit(t *testing.T) {
	for _, note := range []string{"DEB. VARIOS TARJ. VISA", "deb varios tarj visa"} {
		if role, _, _ := Itau.Classify(note); role != importer.RoleCardDebit {
			t.Errorf("Classify(%q) = %v", note, role)
		}
	}
	if !importer.CardDebit("DEB. VARIOS TARJ. VISA", []importer.Bank{importer.Generic, Itau}) {
		t.Error("CardDebit")
	}
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// testdata/card-statement.pdf (used by the end-to-end tests) is a made-up statement written by
// pdftest.ItauVisa.
func TestItauVisaFixture(t *testing.T) {
	b, err := os.ReadFile("../../testdata/card-statement.pdf")
	if err != nil {
		t.Fatal(err)
	}
	lines, err := importer.PDFLines(b)
	if err != nil {
		t.Fatal(err)
	}
	st, ok, err := ItauVisa.Statement(lines)
	if err != nil || !ok || len(st.Lines) != 6 || st.Lines[3].Currency != "USD" || st.Lines[5].Amount != "-20,00" {
		t.Fatalf("ok = %v, err = %v, lines = %+v", ok, err, st.Lines)
	}
}
