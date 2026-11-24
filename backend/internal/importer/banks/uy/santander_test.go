package uy

import (
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/importer"
)

func TestSantanderClassify(t *testing.T) {
	for _, c := range []struct {
		note     string
		role     importer.Role
		merchant string
	}{
		{"Descuento Oriundo", importer.RoleDiscount, "oriundo"},
		{"Desc Boutique Ejemplo", importer.RoleDiscount, "boutique ejemplo"},
		{"Desc. Parrilla Sur", importer.RoleDiscount, "parrilla sur"},
		{"Pago Automatico Cargo Cta.Cte.", importer.RoleCardPayment, "pago automatico cargo cta.cte."},
		{"Pago Supernet", importer.RoleCardPayment, "pago supernet"},
		{"PAGO SERVICIO VISA ", importer.RoleCardDebit, "servicio visa"},
		{"PAGO ELECTRONICO TARJETA CREDITO", importer.RoleCardDebit, "electronico tarjeta credito"},
		// Purchases have no prefix: plain rows, the whole description is the merchant.
		{"Oriundo", importer.RolePlain, "oriundo"},
		{"Descartes Cafe", importer.RolePlain, "descartes cafe"},
		{"Merpago Tienda Cuota 01 02", importer.RolePlain, "merpago tienda cuota 01 02"},
		// A reversed payment and the account's generic credit aren't recognised.
		{"Anulacion De Pago Tf", importer.RolePlain, "anulacion de pago tf"},
		{"CREDITO POR DIVERSOS MOTIVOS ", importer.RolePlain, "credito por diversos motivos"},
		{"PAGO DE SERVICIO POR BANRED SERVICIO DE PAGOS BANRED , UTE TARJ: ############0000", importer.RolePlain, "pago de servicio por banred servicio de pagos banred , ute tarj: ############0000"},
	} {
		role, merchant, _ := Santander.Classify(c.note)
		if role != c.role || merchant != c.merchant {
			t.Errorf("Classify(%q) = %v %q", c.note, role, merchant)
		}
	}
	if !importer.CardDebit("PAGO SERVICIO VISA", []importer.Bank{importer.Generic, Itau, Santander}) {
		t.Error("CardDebit(PAGO SERVICIO VISA) = false")
	}
}

// A made-up card export, as mapped: charges negative, payments and credits positive.
func santanderCardRows() []importer.Row {
	day := func(s string) time.Time {
		d, _ := time.Parse("02/01/2006", s)
		return d
	}
	return []importer.Row{
		{Date: day("05/11/2026"), Amount: -120000, Note: "Dlo Pedidosya Burger"},
		{Date: day("06/11/2026"), Amount: -200000, Note: "Oriundo"},
		{Date: day("06/11/2026"), Amount: 50000, Note: "Descuento Oriundo"},
		{Date: day("08/11/2026"), Amount: -35050, Note: "Tata 123 Visa Cred"},
		{Date: day("09/11/2026"), Amount: -60000, Note: "Merpago Tienda Cuota 01 02"},
		{Date: day("11/11/2026"), Amount: 1234567, Note: "Pago Supernet"},
		{Date: day("12/11/2026"), Amount: 1234567, Note: "Pago Automatico Cargo Cta.Cte."},
		{Date: day("14/11/2026"), Amount: -100000, Note: "Meatme Ejemplo"},
		{Date: day("14/11/2026"), Amount: 25000, Note: "Desc Boutique Ejemplo"},
		{Date: day("15/11/2026"), Amount: -50000, Note: "Oriundo"},
		{Date: day("15/11/2026"), Amount: -50000, Note: "Pizzeria Ejemplo"},
	}
}

// A discount pairs with the same-named purchase on its day; one named after someone else stays
// on its own, and a later purchase at the same place doesn't take it.
func TestSantanderPairs(t *testing.T) {
	got := importer.PairAdjustments(santanderCardRows(), Santander, func(int) bool { return false })
	if len(got.Merged) != 1 || got.Merged[2] != 1 || len(got.Refunded) != 0 {
		t.Fatalf("pairs = %+v", got)
	}
}

func TestDetectSantander(t *testing.T) {
	all := []importer.Bank{importer.Generic, Itau, Santander}
	if b := importer.DetectBank(santanderCardRows(), all); b.ID != Santander.ID {
		t.Errorf("card export: detected %s", b.ID)
	}
	for _, f := range []string{"rediva.csv", "refund.csv", "northbank.csv"} {
		if b := importer.DetectBank(rowsOf(t, f), []importer.Bank{importer.Generic, Santander}); b.ID != importer.Generic.ID {
			t.Errorf("%s: detected %s", f, b.ID)
		}
	}
}
