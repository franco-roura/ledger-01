package importer

import (
	"fmt"
	"regexp"
	"time"
)

// Banks word the same things differently: "COMPRA <merchant>" in one, "POS <merchant>" in
// another; a VAT refund is "REDIVA 19210<merchant>" at one Uruguayan bank and something else at
// the next. The matching in this package only knows roles (a purchase, a discount on it, a
// refund of it) and merchants; a Bank says how its statements spell them. Banks live in
// internal/importer/banks, one file each.

// Role is what a statement row means for pairing.
type Role int

const (
	// RolePlain: a row with no special meaning. With a bank that has no purchase pattern, plain
	// money-out rows are the purchases (their merchant is the whole description).
	RolePlain Role = iota
	// RolePurchase: a card purchase or payment that a discount or refund can refer to.
	RolePurchase
	// RoleDiscount: money back on a purchase (e.g. a VAT refund), always smaller than it.
	RoleDiscount
	// RoleRefund: a purchase given back, in full or in part.
	RoleRefund
	// RoleCardDebit: in a bank account, the debit that pays a card's statement. Importing that
	// statement into the card's wallet turns it into a transfer to the card.
	RoleCardDebit
	// RoleCardPayment: in a card statement, the payment of the bill (money into the card).
	RoleCardPayment
)

// CardPaymentWindow is how far a card's payment and the bank debit that paid it may be apart:
// banks debit on the due date or the next working day, and list the payment within days.
const CardPaymentWindow = 5 * 24 * time.Hour

// Default windows: how long after its purchase an adjustment may be listed.
const (
	DefaultDiscountWindow = 7 * 24 * time.Hour
	DefaultRefundWindow   = 30 * 24 * time.Hour
)

// Pattern recognises one kind of row in a bank's descriptions. Re must have a "merchant" group.
type Pattern struct {
	Role   Role
	Re     *regexp.Regexp
	Window time.Duration // for discounts and refunds; zero means the role's default
}

// NewPattern compiles a case-insensitive pattern and checks it has a "merchant" group. It panics
// on a bad pattern: banks are defined in code and checked by their tests.
func NewPattern(role Role, expr string, window time.Duration) Pattern {
	re := regexp.MustCompile(`(?i)` + expr)
	if re.SubexpIndex("merchant") < 0 {
		panic(fmt.Sprintf("importer: pattern %q has no (?P<merchant>…) group", expr))
	}
	return Pattern{Role: role, Re: re, Window: window}
}

// Bank is how one bank words its statements.
type Bank struct {
	ID       string // "<country>-<bank>", e.g. "uy-itau"; "generic" for no bank
	Name     string
	Country  string // ISO 3166-1 alpha-2, empty for Generic
	Patterns []Pattern
	// Statement reads the bank's card statement PDFs; nil for banks that only export tables.
	// ok is false when the lines aren't one of its statements.
	Statement func(lines []TextLine) (st Statement, ok bool, err error)
}

// CardDebit says whether a bank account's row is a card statement's debit, as any of banks
// words it.
func CardDebit(note string, banks []Bank) bool {
	for _, b := range banks {
		if role, _, _ := b.Classify(note); role == RoleCardDebit {
			return true
		}
	}
	return false
}

// CardPayment says whether a card statement's row is the payment of a bill.
func (b Bank) CardPayment(note string) bool {
	role, _, _ := b.Classify(note)
	return role == RoleCardPayment
}

// Generic is the bank with no wording rules: rows are still fingerprinted and checked for
// duplicates, but no discount or refund is recognised.
var Generic = Bank{ID: "generic", Name: "Other bank"}

// Classify reads a description: the first matching pattern's role and merchant, else RolePlain
// with the whole description as the merchant. window is set for discounts and refunds.
func (b Bank) Classify(note string) (role Role, merchant string, window time.Duration) {
	for _, p := range b.Patterns {
		if m := p.Re.FindStringSubmatch(note); m != nil {
			window = p.Window
			if window == 0 {
				window = defaultWindow(p.Role)
			}
			return p.Role, NormNote(m[p.Re.SubexpIndex("merchant")]), window
		}
	}
	return RolePlain, NormNote(note), 0
}

func defaultWindow(r Role) time.Duration {
	switch r {
	case RoleDiscount:
		return DefaultDiscountWindow
	case RoleRefund:
		return DefaultRefundWindow
	case RoleCardDebit, RoleCardPayment:
		return CardPaymentWindow
	}
	return 0
}

// Adjustment reads a discount or refund row: its role, merchant and window.
func (b Bank) Adjustment(note string) (role Role, merchant string, window time.Duration, ok bool) {
	role, merchant, window = b.Classify(note)
	return role, merchant, window, role == RoleDiscount || role == RoleRefund
}

// Purchase returns the merchant of a row that adjustments can refer to: a RolePurchase row, or
// any plain row when the bank has no purchase pattern.
func (b Bank) Purchase(note string) (string, bool) {
	role, merchant, _ := b.Classify(note)
	switch role {
	case RolePurchase:
		return merchant, true
	case RolePlain:
		return merchant, !b.hasRole(RolePurchase)
	}
	return "", false
}

func (b Bank) hasRole(r Role) bool {
	for _, p := range b.Patterns {
		if p.Role == r {
			return true
		}
	}
	return false
}

// MaxWindow is the longest any of the bank's adjustments may follow its purchase.
func (b Bank) MaxWindow() time.Duration {
	longest := time.Duration(0)
	for _, p := range b.Patterns {
		w := p.Window
		if w == 0 {
			w = defaultWindow(p.Role)
		}
		longest = max(longest, w)
	}
	return longest
}

// DetectBank picks the bank whose patterns recognise the most descriptions of a file, at least
// two and at least 2% of the rows; otherwise Generic. Ties go to the earlier bank in banks.
func DetectBank(rows []Row, banks []Bank) Bank {
	readable := 0
	for _, r := range rows {
		if r.Err == "" {
			readable++
		}
	}
	best, bestN := Generic, 0
	for _, b := range banks {
		// Statement banks are chosen by their reader, never from a table's descriptions.
		if len(b.Patterns) == 0 || b.Statement != nil {
			continue
		}
		n := 0
		for _, r := range rows {
			if r.Err != "" {
				continue
			}
			if role, _, _ := b.Classify(r.Note); role != RolePlain {
				n++
			}
		}
		if n > bestN && n >= 2 && n*50 >= readable {
			best, bestN = b, n
		}
	}
	return best
}
