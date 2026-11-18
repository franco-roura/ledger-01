package uy

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
)

// ItauVisa is Itaú Uruguay's Visa card statement, a PDF. Its movements table is printed in a
// monospace font, one movement per line:
//
//	19 10 26        PAGOS                                          -5.000,00
//	13 06 26 1234   MUEBLERIA SUR                 2/ 3              1500,00
//	                SEGURO DE VIDA SOBRE SALDO                         12,00
//	                SALDO CONTADO                                   1.512,00      0,00
//
// with the purchase date (dd mm yy), the card's last digits, the detail, the instalment, then
// up to three amounts: in the original currency (a purchase abroad), in pesos, in dollars. The
// table's headings are drawn as an image, so the amount columns are found from the "SALDO
// CONTADO" line, which always prints the pesos and dollars totals. Above the table, the issue
// date stands alone in its box and the due dates follow the account number. A closing sentence
// says whether the bank debits the totals from an account ("PAGO TOTAL PESOS, PAGO TOTAL
// DÓLARES"); this reader adds those payments, dated on the due date, as the bill's payment.
var ItauVisa = importer.Bank{
	ID:        "uy-itau-visa",
	Name:      "Itaú Visa (PDF)",
	Country:   "UY",
	Patterns:  itauVisaPatterns,
	Statement: readItauVisa,
}

var itauVisaPatterns = []importer.Pattern{
	importer.NewPattern(importer.RoleCardPayment, `^\s*(?P<merchant>pagos?)\s*$`, importer.CardPaymentWindow),
	importer.NewPattern(importer.RoleCardPayment, `^\s*(?P<merchant>`+itauAutoDebitNote+`)\s*$`, importer.CardPaymentWindow),
}

// itauAutoDebitNote describes the payment this reader adds for an automatic debit of the total.
const itauAutoDebitNote = "PAGO TOTAL DEBITO AUTOMATICO"

var (
	itauDateRe   = regexp.MustCompile(`^\d{2}/\d{2}/\d{2}$`)
	itauAmountRe = regexp.MustCompile(`^-?\d{1,3}(?:\.\d{3})*,\d{2}$|^-?\d+,\d{2}$`)
	itauDayRe    = regexp.MustCompile(`^\d{2}$`)
	itauCardRe   = regexp.MustCompile(`^\d{4}$`)
	itauInstRe   = regexp.MustCompile(`^(\d{1,3})/(\d{1,3})?$`)
	itauNumRe    = regexp.MustCompile(`^\d{1,3}$`)
)

// itauSkipped are lines of the table that aren't movements.
var itauSkipped = []string{"SALDO DEL ESTADO DE CUENTA ANTERIOR", "SALDO CONTADO", "UD. HA GENERADO", "CUENTA ADHERIDA"}

func readItauVisa(lines []importer.TextLine) (importer.Statement, bool, error) {
	st := importer.Statement{Decimal: ','}
	start, end := -1, -1
	var pesosX, dollarsX float64
	autoDebit := map[string]bool{}
	for i, l := range lines {
		text := l.Text()
		switch {
		case strings.HasPrefix(text, "SALDO DEL ESTADO DE CUENTA ANTERIOR") && start < 0:
			start = i + 1
		case strings.HasPrefix(text, "SALDO CONTADO") && end < 0:
			end = i
			var amounts []importer.Word
			for _, w := range l.Words {
				if itauAmountRe.MatchString(w.Text) {
					amounts = append(amounts, w)
				}
			}
			if len(amounts) != 2 {
				return st, false, nil
			}
			pesosX, dollarsX = rightEdge(amounts[0]), rightEdge(amounts[1])
		case strings.HasPrefix(text, "CUENTA ADHERIDA A DÉBITO AUTOMÁTICO"):
			autoDebit["UYU"] = strings.Contains(text, "PAGO TOTAL PESOS")
			autoDebit["USD"] = strings.Contains(text, "PAGO TOTAL DÓLARES")
		}
		if st.Issued.IsZero() && start < 0 && len(l.Words) == 1 && itauDateRe.MatchString(l.Words[0].Text) {
			st.Issued = itauDate(l.Words[0].Text)
		}
		// "16/11/26 14/12/26 …": the current and next due dates, then the holder's address.
		if st.Due.IsZero() && start < 0 && len(l.Words) >= 2 && itauDateRe.MatchString(l.Words[0].Text) &&
			itauDateRe.MatchString(l.Words[1].Text) {
			st.Due = itauDate(l.Words[0].Text)
		}
	}
	if start < 0 || end < start || st.Issued.IsZero() || st.Due.IsZero() {
		return st, false, nil
	}
	// Each amount belongs to the column whose right edge it shares; the original-currency column
	// is to the left of the pesos one and isn't imported.
	column := func(w importer.Word) string {
		x, tolerance := rightEdge(w), (dollarsX-pesosX)/3
		switch {
		case abs(x-pesosX) <= tolerance:
			return "UYU"
		case abs(x-dollarsX) <= tolerance:
			return "USD"
		}
		return ""
	}
	totals := map[string]string{}
	for _, w := range lines[end].Words {
		if c := column(w); c != "" && itauAmountRe.MatchString(w.Text) {
			totals[c] = w.Text
		}
	}

	for _, l := range lines[start:end] {
		text := l.Text()
		if skipped(text) {
			continue
		}
		line, amounts, ok := itauMovement(l.Words, column)
		if !ok {
			continue
		}
		for cur, amount := range amounts {
			m := line
			m.Currency, m.Amount = cur, amount
			st.Lines = append(st.Lines, m)
		}
	}
	for _, cur := range []string{"UYU", "USD"} {
		if total := totals[cur]; autoDebit[cur] && total != "" && !zero(total) && !strings.HasPrefix(total, "-") {
			st.Lines = append(st.Lines, importer.StatementLine{
				Date: st.Due, Detail: itauAutoDebitNote, Currency: cur, Amount: "-" + total,
			})
		}
	}
	return st, true, nil
}

// itauMovement reads one movement line: its date, detail and instalment, and its amounts by
// currency. ok is false for lines without an amount in pesos or dollars.
func itauMovement(words []importer.Word, column func(importer.Word) string) (importer.StatementLine, map[string]string, bool) {
	var l importer.StatementLine
	amounts := map[string]string{}
	// Amounts are the trailing words.
	n := len(words)
	for n > 0 && itauAmountRe.MatchString(words[n-1].Text) {
		if c := column(words[n-1]); c != "" {
			amounts[c] = words[n-1].Text
		}
		n--
	}
	words = words[:n]
	if len(amounts) == 0 {
		return l, nil, false
	}
	for c, a := range amounts {
		if zero(a) {
			delete(amounts, c)
		}
	}
	// A date as three words: "13 06 26".
	if len(words) >= 3 && itauDayRe.MatchString(words[0].Text) && itauDayRe.MatchString(words[1].Text) &&
		itauDayRe.MatchString(words[2].Text) {
		l.Date = itauDate(words[0].Text + "/" + words[1].Text + "/" + words[2].Text)
		words = words[3:]
		// The card's last digits.
		if len(words) > 1 && itauCardRe.MatchString(words[0].Text) {
			words = words[1:]
		}
	}
	// The instalment, "5/ 6" (two words) or "5/6", ends the detail.
	if k := len(words); k >= 2 && itauNumRe.MatchString(words[k-1].Text) && strings.HasSuffix(words[k-2].Text, "/") {
		if m := itauInstRe.FindStringSubmatch(words[k-2].Text); m != nil {
			l.Instalment = m[1] + "/" + words[k-1].Text
			words = words[:k-2]
		}
	} else if k >= 1 {
		if m := itauInstRe.FindStringSubmatch(words[k-1].Text); m != nil && m[2] != "" {
			l.Instalment = m[1] + "/" + m[2]
			words = words[:k-1]
		}
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.Text
	}
	l.Detail = strings.Join(parts, " ")
	if l.Detail == "" || len(amounts) == 0 {
		return l, nil, false
	}
	return l, amounts, true
}

func skipped(text string) bool {
	for _, s := range itauSkipped {
		if strings.HasPrefix(text, s) {
			return true
		}
	}
	return false
}

// itauDate reads "dd/mm/yy"; zero when it isn't a date.
func itauDate(s string) time.Time {
	t, err := time.Parse("02/01/06", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// rightEdge is where a word ends: amounts are right-aligned.
func rightEdge(w importer.Word) float64 { return w.End }

func zero(amount string) bool {
	v, err := strconv.ParseFloat(strings.NewReplacer(".", "", ",", ".", "-", "").Replace(amount), 64)
	return err == nil && v == 0
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
