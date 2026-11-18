package importer_test

import (
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/importer/pdftest"
)

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPDFLines(t *testing.T) {
	b := pdftest.PDF([]pdftest.Text{
		{X: 50, Y: 100, S: "Statement    of account"},
		{X: 300, Y: 100, S: "1.234,56"},
		{X: 50, Y: 120, S: "second line"},
	})
	if !importer.IsPDF(b) || importer.IsPDF([]byte("Fecha;Importe\n")) {
		t.Fatal("IsPDF")
	}
	lines, err := importer.PDFLines(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].Text() != "Statement of account 1.234,56" || lines[1].Text() != "second line" {
		t.Fatalf("lines = %+v", lines)
	}
	words := lines[0].Words
	if len(words) != 4 || words[0].X != 50 || words[3].X != 300 {
		t.Fatalf("words = %+v", words)
	}
	// 8 Courier characters at 7 pt are 0.6 em each.
	if w := words[3]; w.End < 300+8*4.2-0.01 || w.End > 300+8*4.2+0.01 {
		t.Fatalf("end = %v", w.End)
	}
	if _, err := importer.PDFLines([]byte("%PDF-1.4 not really")); err == nil {
		t.Fatal("a broken PDF was read")
	}
}

func TestStatementGrid(t *testing.T) {
	st := importer.Statement{
		Issued: date("2026-11-03"), Due: date("2026-11-16"), Decimal: ',',
		Lines: []importer.StatementLine{
			{Date: date("2026-10-19"), Detail: "PAGOS", Currency: "UYU", Amount: "-1.000,00"},
			{Date: date("2026-06-13"), Detail: "MUEBLERIA SUR", Instalment: "2/3", Currency: "UYU", Amount: "1500,00"},
			{Detail: "SEGURO", Currency: "UYU", Amount: "12,00"},
			{Date: date("2026-10-02"), Detail: "LIBRERIA NORTE", Currency: "USD", Amount: "20,00"},
		},
	}
	g, other := importer.StatementGrid(st, "UYU")
	if other != 1 || len(g) != 4 {
		t.Fatalf("grid = %v, other = %d", g, other)
	}
	m := importer.StatementMapping("bank", st.Decimal)
	if !importer.IsStatementMapping(m) || m.Validate(g.Columns(), len(g)) != nil {
		t.Fatal("statement mapping")
	}
	rows := importer.Apply(g, m, 2)
	want := []struct {
		on     string
		amount int64
		note   string
	}{
		{"2026-10-19", 1000_00, "PAGOS"},
		// An instalment is this statement's spending: dated on the issue date, numbered in the note.
		{"2026-11-03", -1500_00, "MUEBLERIA SUR 2/3 (13/06/26)"},
		{"2026-11-03", -12_00, "SEGURO"},
	}
	for i, w := range want {
		r := rows[i]
		if r.Err != "" || r.Date.Format(time.DateOnly) != w.on || r.Amount != w.amount || r.Note != w.note {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
}
