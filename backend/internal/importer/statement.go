package importer

import (
	"time"
)

// A card statement is a bill, not a table of movements: purchases (some of them one instalment
// of a larger purchase), fees, the payment of the previous bill, and the total the bank will
// debit from an account. A bank's statement reader (Bank.Statement) reads its layout into a
// Statement; StatementGrid turns the lines in one currency into a plain table, so the rest of
// the import (fingerprints, duplicates, category rules, the review) works as for a CSV.

// StatementLine is one movement of a card statement.
type StatementLine struct {
	// As printed; zero when the statement prints none (a monthly fee, say).
	Date   time.Time
	Detail string
	// "5/6" for the fifth of six instalments, else empty.
	Instalment string
	// ISO 4217 code of the column the amount is in.
	Currency string
	// As printed (with the statement's decimal separator); positive is money spent, negative is
	// money paid into the card. The bank's RoleCardPayment patterns tell the bill's payments.
	Amount string
}

// Statement is a card statement as its bank's reader understood it.
type Statement struct {
	// When it was issued, and when its total is due.
	Issued time.Time
	Due    time.Time
	// The statement's decimal separator, '.' or ','.
	Decimal byte
	Lines   []StatementLine
}

// Statement table columns, as StatementGrid lays them out.
const (
	stmtDate = iota
	stmtNote
	stmtAmount
	stmtCurrency
)

// StatementCurrency is the currency of a row of a StatementGrid, as Apply read it (the cells are
// kept on the row).
func StatementCurrency(r Row) string {
	if stmtCurrency < len(r.Cells) {
		return r.Cells[stmtCurrency]
	}
	return ""
}

// StatementMapping is the mapping that reads a StatementGrid: a card statement's purchases are
// positive.
func StatementMapping(bankID string, decimal byte) Mapping {
	return Mapping{
		HasHeader: true, Date: stmtDate, DateFormat: "dd/mm/yyyy", Amount: stmtAmount, Debit: -1, Credit: -1,
		Sign: PositiveIsExpense, Decimal: decimal, Note: stmtNote, Category: -1, Bank: bankID,
	}
}

// IsStatementMapping says whether m reads a StatementGrid (as opposed to a mapping someone
// saved for CSV files).
func IsStatementMapping(m Mapping) bool {
	s := StatementMapping(m.Bank, m.Decimal)
	return m.Skip == 0 && m.HasHeader && m.Date == s.Date && m.DateFormat == s.DateFormat && m.Amount == s.Amount &&
		m.Debit == -1 && m.Credit == -1 && m.Sign == s.Sign && m.Note == s.Note && m.Category == -1
}

// StatementGrid lays out a statement's lines as a table with a header row (date, description,
// amount, currency). A card can spend in several currencies; each one imports into its own
// wallet.
//
// An instalment is dated on the statement's issue date (it's this month's spending), with its
// number and purchase date in the note: the same instalment of the same purchase has the same
// date and amount every month, so the number is what tells them apart. Lines with no date are
// dated on the issue date too.
func StatementGrid(st Statement) Grid {
	g := Grid{{"Date", "Description", "Amount", "Currency"}}
	for _, l := range st.Lines {
		on, note := l.Date, l.Detail
		if l.Instalment != "" {
			note += " " + l.Instalment
			if !l.Date.IsZero() {
				note += " (" + l.Date.Format("02/01/06") + ")"
			}
			on = st.Issued
		}
		if on.IsZero() {
			on = st.Issued
		}
		g = append(g, []string{on.Format("02/01/2006"), note, l.Amount, l.Currency})
	}
	return g
}
