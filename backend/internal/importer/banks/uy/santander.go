package uy

import (
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
)

// Santander is Santander Uruguay's CSV exports from home banking, the credit card's and the
// accounts'.
//
// Card purchases carry no prefix ("Oriundo", "Merpago Tienda Cuota 01 02"), so every plain row is
// a purchase. The VAT refund is never listed: the rows already come net of it. Card-benefit
// discounts follow their purchase on the same day as "Descuento <name>" or "Desc <name>"; the name
// is usually the merchant's, but not always (those stay as their own rows). The bill's payment is
// "Pago Automatico Cargo Cta.Cte." (an automatic debit) or "Pago Supernet" (paid in home
// banking), one row per currency; the account pays it as "PAGO SERVICIO VISA" or "PAGO
// ELECTRONICO TARJETA CREDITO", in either currency.
//
// Not covered by these patterns: a merchant refund repeats the purchase's description as a credit
// (no "DEVOLUCION"), and a reversed payment ("Anulacion De Pago Tf") is exported negative although
// it puts the debt back.
var Santander = importer.Bank{
	ID:      "uy-santander",
	Name:    "Santander",
	Country: "UY",
	Patterns: []importer.Pattern{
		importer.NewPattern(importer.RoleDiscount, `^\s*desc(?:uento)?\.?\s+(?P<merchant>.+?)\s*$`, santanderDiscountWindow),
		importer.NewPattern(importer.RoleCardPayment, `^\s*(?P<merchant>pago\s+(?:automatico\s+cargo\s+cta\.?\s*cte\.?|supernet))\s*$`, importer.CardPaymentWindow),
		importer.NewPattern(importer.RoleCardDebit, `^\s*pago\s+(?P<merchant>servicio\s+visa|electr[oó]nico\s+tarjeta\s+cr[eé]dito)\b`, 0),
	},
}

// santanderDiscountWindow: card-benefit discounts are listed on the purchase's day.
const santanderDiscountWindow = 24 * time.Hour
