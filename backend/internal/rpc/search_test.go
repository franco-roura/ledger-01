package rpc

import (
	"errors"
	"testing"

	v1 "github.com/franco-roura/ledger-01/backend/gen/ledger/v1"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/google/uuid"
)

func TestSearchFromProto(t *testing.T) {
	wallet, category, label := uuid.New(), uuid.New(), uuid.New()
	m := &v1.SearchTransactionsRequest{
		Period: &v1.DateRange{From: "2026-10-01", To: "2026-10-31"}, Today: "2026-10-15",
		WalletIds: []string{wallet.String()}, CategoryIds: []string{category.String()}, LabelIds: []string{label.String()},
		NoteQuery: "  Market  ", Kinds: []v1.TransactionKind{v1.TransactionKind_TRANSACTION_KIND_TRANSFER_IN},
	}
	got, err := searchFromProto(m)
	if err != nil {
		t.Fatal(err)
	}
	if got.NoteQuery != "Market" || got.WalletIDs[0] != wallet || got.CategoryIDs[0] != category || got.LabelIDs[0] != label || got.Kinds[0] != ledger.KindTransferIn || got.Today.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("wrong search conversion: %+v", got)
	}
}

func TestSearchFromProtoRejectsMalformedInputs(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*v1.SearchTransactionsRequest)
	}{
		{"missing period", "period", func(m *v1.SearchTransactionsRequest) { m.Period = nil }},
		{"invalid date", "period.from", func(m *v1.SearchTransactionsRequest) { m.Period.From = "2026-02-30" }},
		{"invalid today", "today", func(m *v1.SearchTransactionsRequest) { m.Today = "tomorrow" }},
		{"wallet", "wallet_ids[0]", func(m *v1.SearchTransactionsRequest) { m.WalletIds = []string{"invalid"} }},
		{"category", "category_ids[0]", func(m *v1.SearchTransactionsRequest) { m.CategoryIds = []string{"invalid"} }},
		{"label", "label_ids[0]", func(m *v1.SearchTransactionsRequest) { m.LabelIds = []string{"invalid"} }},
		{"unknown kind", "kinds", func(m *v1.SearchTransactionsRequest) { m.Kinds = []v1.TransactionKind{99} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &v1.SearchTransactionsRequest{Period: &v1.DateRange{From: "2026-10-01", To: "2026-10-31"}, Today: "2026-10-15"}
			tc.change(m)
			_, err := searchFromProto(m)
			var inv *ledger.InvalidError
			if !errors.As(err, &inv) || inv.Field != tc.field {
				t.Fatalf("expected %s validation error, got %v", tc.field, err)
			}
		})
	}
}
