// Package pdftest writes small text PDFs for tests: statements laid out like a bank's, with
// made-up data.
package pdftest

import (
	"bytes"

	"github.com/go-pdf/fpdf"
)

// Text is a run of text at (X, Y) points from the page's top-left corner.
type Text struct {
	X, Y float64
	S    string
}

// PDF writes an A4 page per slice of texts, in 7 pt Courier (a standard font, as banks use).
func PDF(pages ...[]Text) []byte {
	f := fpdf.New("P", "pt", "A4", "")
	f.SetCompression(false)
	f.SetFont("Courier", "", 7)
	tr := f.UnicodeTranslatorFromDescriptor("") // the standard fonts are Windows-1252
	for _, page := range pages {
		f.AddPage()
		for _, t := range page {
			f.Text(t.X, t.Y, tr(t.S))
		}
	}
	var b bytes.Buffer
	if err := f.Output(&b); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// ItauVisaRow is one movement line of an Itaú Visa statement's table.
type ItauVisaRow struct {
	Date       string // "dd mm yy" or ""
	Card       string // last four digits or ""
	Detail     string
	Instalment string // "2/ 3" or ""
	Pesos      string // as printed, "1.500,00"
	Dollars    string
}

// ItauVisa lays out a statement like Itaú Uruguay's Visa PDF: the issue date alone in its box,
// the due dates, then the movements table between the previous balance and "SALDO CONTADO",
// and the automatic debit sentence when autoDebit.
func ItauVisa(issued, due, nextDue string, rows []ItauVisaRow, pesos, dollars string, autoDebit bool) []byte {
	page := []Text{
		{491, 115, issued},
		{275, 136, "ANA PEREZ"},
		{132, 152, due}, {206, 152, nextDue}, {275, 152, "CALLE FALSA 123"},
		{65, 182, "100200-3"}, {124, 182, "$"}, {145, 182, "50.000,00"},
		{24.5, 250, line(-1, "", "", "SALDO DEL ESTADO DE CUENTA ANTERIOR", "", "1.000,00", "0,00")},
	}
	y := 257.0
	for _, r := range rows {
		page = append(page, Text{24.5, y, line(0, r.Date, r.Card, r.Detail, r.Instalment, r.Pesos, r.Dollars)})
		y += 7
	}
	page = append(page, Text{24.5, y + 7, line(1, "", "", "SALDO CONTADO", "", pesos, dollars)})
	page = append(page, Text{24.5, y + 21, pad("", 19) + "UD. HA GENERADO          5,0 MILLAS ITAU"})
	if autoDebit {
		page = append(page, Text{24.5, y + 42, pad("", 16) +
			"CUENTA ADHERIDA A DÉBITO AUTOMÁTICO EN BANCO ITAÚ: PAGO TOTAL PESOS, PAGO TOTAL DÓLARES."})
	}
	return PDF(page)
}

// line lays out a table line on the bank's character grid: date at column 10, card at 20, the
// detail at 28 (21 for payments and totals), the instalment at 64, pesos ending at column 107
// and dollars at 121.
func line(kind int, date, card, detail, inst, pesos, dollars string) string {
	s := ""
	if date != "" {
		s = pad("", 10) + date
	}
	switch kind {
	case -1:
		s = pad("", 26) + detail
		s = pad(s, 85-len([]rune("1.000,00"))) + pesos
	default:
		if card != "" {
			s = pad(s+pad("", max(0, 20-len([]rune(s))))+card, 28)
			s += detail
		} else if date != "" || kind == 1 {
			s = pad(s+pad("", max(0, 21-len([]rune(s)))), 21) + detail
		} else {
			s = pad("", 28) + detail
		}
		if inst != "" {
			s = pad(s, 64) + inst
		}
	}
	if kind != -1 {
		if pesos != "" {
			s = pad(s, 107-len([]rune(pesos))) + pesos
		}
	}
	if dollars != "" && kind != -1 {
		s = pad(s, 121-len([]rune(dollars))) + dollars
	}
	return s
}

// pad right-pads s with spaces to n characters.
func pad(s string, n int) string {
	for len([]rune(s)) < n {
		s += " "
	}
	return s
}
