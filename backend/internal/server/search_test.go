package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/apitest"
	"github.com/google/uuid"
)

func TestTransactionSearchOverHTTP(t *testing.T) {
	owner, other := uuid.NewString()+"@example.com", uuid.NewString()+"@example.com"
	do := apitest.HTTPDriver(t, apitest.Handler(t, apitest.Config(t, map[string]string{"ALLOWED_EMAILS": owner + "," + other})))
	login := func(email string) string {
		r := do(t, http.MethodPost, "/auth/dev/login", map[string]string{"email": email}, "")
		if r.Status != http.StatusNoContent {
			t.Fatalf("login: %d %s", r.Status, r.Body)
		}
		return apitest.SessionCookie(r)
	}
	session := login(owner)
	rpc := func(method string, body any) map[string]any {
		r := do(t, http.MethodPost, "/ledger.v1."+method, body, session)
		if r.Status != http.StatusOK {
			t.Fatalf("%s: %d %s", method, r.Status, r.Body)
		}
		return r.JSON(t)
	}
	makeWallet := func(name, currency string) string {
		return rpc("WalletService/CreateWallet", map[string]string{"name": name, "currency": currency})["wallet"].(map[string]any)["id"].(string)
	}
	usd, uyu := makeWallet("Daily", "USD"), makeWallet("Pesos", "UYU")
	var expense, income string
	for _, value := range rpc("CategoryService/ListCategories", map[string]any{})["categories"].([]any) {
		c := value.(map[string]any)
		if c["name"] == "Groceries" {
			expense = c["id"].(string)
		}
		if c["name"] == "Salary" {
			income = c["id"].(string)
		}
	}
	label := rpc("LabelService/CreateLabel", map[string]string{"name": "Shared", "color": "cat-teal"})["label"].(map[string]any)["id"].(string)
	makeTx := func(wallet, category, on, note string, labels []string) string {
		return rpc("TransactionService/CreateTransaction", map[string]any{"walletId": wallet, "categoryId": category, "amountMinor": "1250", "occurredOn": on, "note": note, "labelIds": labels, "today": "2026-10-15"})["transaction"].(map[string]any)["id"].(string)
	}
	first := makeTx(usd, expense, "2026-10-13", "Market one", []string{label})
	second := makeTx(uyu, expense, "2026-10-14", "MARKET two", nil)
	makeTx(usd, income, "2026-10-15", "Market salary", nil)
	makeTx(usd, expense, "2026-10-16", "Market future", nil)
	type result struct {
		Transactions []struct {
			ID       string   `json:"id"`
			Currency string   `json:"currency"`
			LabelIDs []string `json:"labelIds"`
		} `json:"transactions"`
		Next string `json:"nextPageToken"`
	}
	search := func(body map[string]any, token string) (result, int) {
		r := do(t, http.MethodPost, "/ledger.v1.TransactionService/SearchTransactions", body, token)
		var out result
		if r.Status == http.StatusOK {
			if err := json.Unmarshal(r.Body, &out); err != nil {
				t.Fatal(err)
			}
		}
		return out, r.Status
	}
	base := func() map[string]any {
		return map[string]any{"period": map[string]string{"from": "2026-10-01", "to": "2026-10-31"}, "today": "2026-10-15", "noteQuery": " market ", "kinds": []string{"TRANSACTION_KIND_EXPENSE"}}
	}
	request := base()
	request["pageSize"] = 1
	page1, status := search(request, session)
	if status != http.StatusOK || len(page1.Transactions) != 1 || page1.Next == "" || page1.Transactions[0].ID != second {
		t.Fatalf("first page: %+v status=%d", page1, status)
	}
	request["pageToken"] = page1.Next
	page2, status := search(request, session)
	if status != http.StatusOK || len(page2.Transactions) != 1 || page2.Next != "" || page2.Transactions[0].ID != first || page1.Transactions[0].Currency != "UYU" || page2.Transactions[0].Currency != "USD" {
		t.Fatalf("second page: %+v status=%d", page2, status)
	}
	request = base()
	request["walletIds"], request["categoryIds"], request["labelIds"] = []string{usd}, []string{expense}, []string{label}
	filtered, status := search(request, session)
	if status != http.StatusOK || len(filtered.Transactions) != 1 || filtered.Transactions[0].ID != first || len(filtered.Transactions[0].LabelIDs) != 1 {
		t.Fatalf("combined filters: %+v status=%d", filtered, status)
	}
	foreign, status := search(request, login(other))
	if status != http.StatusOK || len(foreign.Transactions) != 0 {
		t.Fatalf("household isolation: %+v status=%d", foreign, status)
	}
	if _, status := search(base(), ""); status != http.StatusUnauthorized {
		t.Fatalf("anonymous search: %d", status)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing period", func(m map[string]any) { delete(m, "period") }},
		{"reversed range", func(m map[string]any) { m["period"] = map[string]string{"from": "2026-10-31", "to": "2026-10-01"} }},
		{"bad wallet", func(m map[string]any) { m["walletIds"] = []string{"bad"} }},
		{"unknown kind", func(m map[string]any) { m["kinds"] = []int{99} }},
		{"bad cursor", func(m map[string]any) { m["pageToken"] = "invalid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := base()
			tc.change(request)
			if _, status := search(request, session); status != http.StatusBadRequest {
				t.Fatalf("expected validation failure, got %d", status)
			}
		})
	}
}
