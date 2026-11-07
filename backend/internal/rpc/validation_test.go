package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/franco-roura/ledger-01/backend/gen/ledger/v1"
	"github.com/franco-roura/ledger-01/backend/internal/importer"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/google/uuid"
)

func TestErrorContract(t *testing.T) {
	s := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	for _, tc := range []struct {
		err  error
		code connect.Code
	}{
		{&ledger.InvalidError{Field: "amount", Msg: "invalid"}, connect.CodeInvalidArgument},
		{&ledger.ConflictError{Msg: "conflict"}, connect.CodeFailedPrecondition},
		{fmt.Errorf("wrapped: %w", ledger.ErrNotFound), connect.CodeNotFound},
		{ledger.ErrForbidden, connect.CodePermissionDenied},
		{context.Canceled, connect.CodeCanceled},
		{errors.New("password=secret database internals"), connect.CodeInternal},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			err := s.fail(context.Background(), tc.err)
			if connect.CodeOf(err) != tc.code {
				t.Fatalf("%v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("internal information leaked")
			}
		})
	}
	ce := connect.NewError(connect.CodeResourceExhausted, errors.New("quota"))
	if s.fail(context.Background(), fmt.Errorf("wrapped: %w", ce)) != ce {
		t.Fatal("lost Connect error identity")
	}
}

func TestImportMappingPreservesColumnZeroAndAbsentColumns(t *testing.T) {
	for _, decimal := range []byte{'.', ','} {
		for _, sign := range []importer.Sign{importer.NegativeIsExpense, importer.PositiveIsExpense} {
			want := importer.Mapping{Skip: 2, HasHeader: true, Date: 1, DateFormat: "yyyy-mm-dd", Amount: 0, Debit: -1, Credit: -1, Note: 3, Category: -1, Sign: sign, Decimal: decimal, Bank: "generic"}
			encoded := mappingToProto(want)
			if encoded.AmountColumn == nil || *encoded.AmountColumn != 0 || encoded.DebitColumn != nil {
				t.Fatal("optional zero column lost")
			}
			got, err := mappingFromProto(encoded)
			if err != nil || got != want {
				t.Fatalf("mapping round trip: %+v, %v; want %+v", got, err, want)
			}
		}
	}
	if _, err := mappingFromProto(nil); err == nil {
		t.Fatal("missing import mapping accepted")
	}
}

func TestOptionalIDsAndIndexedValidation(t *testing.T) {
	empty := ""
	for _, s := range []*string{nil, &empty} {
		if id, err := parseOptID("id", s); id != nil || err != nil {
			t.Fatalf("optional id = %v, %v", id, err)
		}
	}
	id := uuid.NewString()
	parsed, err := parseOptID("id", &id)
	if err != nil || parsed.String() != id {
		t.Fatalf("%v %v", parsed, err)
	}
	_, err = parseIDs("label_ids", []string{id, "invalid"})
	var inv *ledger.InvalidError
	if !errors.As(err, &inv) || inv.Field != "label_ids[1]" {
		t.Fatalf("%v", err)
	}
}

func TestDateValidation(t *testing.T) {
	for _, date := range []string{"2026-02-29", "2026-13-01", "2026-1-01", "2026-01-01T00:00:00Z", ""} {
		if _, err := parseDate("date", date); err == nil {
			t.Fatalf("accepted %q", date)
		}
	}
	date, err := parseToday("2024-02-29")
	if err != nil || date.Format("2006-01-02") != "2024-02-29" {
		t.Fatalf("%v %v", date, err)
	}
}

func TestReportAndBudgetRequiredInput(t *testing.T) {
	for _, filter := range []*v1.DashboardFilter{nil, {}} {
		if _, err := reportFilterFromProto(filter); err == nil {
			t.Fatal("missing period accepted")
		}
	}
	if _, err := budgetInputFromProto(nil); err == nil {
		t.Fatal("missing budget accepted")
	}
	for _, tc := range []struct {
		from, to            string
		wallets, categories []string
	}{
		{"bad", "2026-01-31", nil, nil},
		{"2026-01-01", "bad", nil, nil},
		{"2026-01-01", "2026-01-31", []string{"bad"}, nil},
		{"2026-01-01", "2026-01-31", nil, []string{"bad"}},
	} {
		_, err := reportFilterFromProto(&v1.DashboardFilter{Period: &v1.DateRange{From: tc.from, To: tc.to}, WalletIds: tc.wallets, CategoryIds: tc.categories})
		if err == nil {
			t.Fatalf("invalid filter accepted: %+v", tc)
		}
	}
	if _, err := recurrenceFromProto(&v1.Recurrence{Frequency: v1.RecurringFrequency(999)}); err == nil {
		t.Fatal("unknown recurrence accepted")
	}
	if _, err := recurrenceFromProto(&v1.Recurrence{Frequency: v1.RecurringFrequency_RECURRING_FREQUENCY_MONTHLY, EndDate: "bad"}); err == nil {
		t.Fatal("bad recurrence end accepted")
	}
}
