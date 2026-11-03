package server_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/apitest"
	"github.com/google/uuid"
)

type apiClient struct {
	t       *testing.T
	do      apitest.Driver
	session string
}

func managementClients(t *testing.T) (apiClient, apiClient) {
	t.Helper()
	e1, e2 := uuid.NewString()+"@example.com", uuid.NewString()+"@example.com"
	do := apitest.HTTPDriver(t, apitest.Handler(t, apitest.Config(t, map[string]string{"ALLOWED_EMAILS": e1 + "," + e2})))
	login := func(email string) apiClient {
		r := do(t, http.MethodPost, "/auth/dev/login", map[string]string{"email": email, "name": "Management"}, "")
		if r.Status != http.StatusNoContent || apitest.SessionCookie(r) == "" {
			t.Fatalf("login: %d %s", r.Status, r.Body)
		}
		return apiClient{t, do, apitest.SessionCookie(r)}
	}
	return login(e1), login(e2)
}

func (c apiClient) call(method string, body any, status int) map[string]any {
	c.t.Helper()
	r := c.do(c.t, http.MethodPost, "/ledger.v1."+method, body, c.session)
	if r.Status != status {
		c.t.Fatalf("%s: %d %s; want %d", method, r.Status, r.Body, status)
	}
	return r.JSON(c.t)
}

func object(m map[string]any, field string) map[string]any { return m[field].(map[string]any) }

func TestManagementLifecycleAndHouseholdIsolation(t *testing.T) {
	a, foreign := managementClients(t)
	w := object(a.call("WalletService/CreateWallet", map[string]any{"name": "Original", "currency": "USD", "initialBalanceMinor": "1200"}, 200), "wallet")
	wid := w["id"]
	label := object(a.call("LabelService/CreateLabel", map[string]any{"name": "Travel", "color": "cat-orange"}, 200), "label")
	lid := label["id"]
	cat := object(a.call("CategoryService/CreateCategory", map[string]any{"name": "Testing", "kind": "CATEGORY_KIND_EXPENSE", "icon": "coffee", "color": "cat-orange"}, 200), "category")
	cid := cat["id"]
	// Every mutation must scope its lookup to the authenticated household.
	for _, tc := range []struct {
		method string
		body   map[string]any
	}{
		{"WalletService/GetWallet", map[string]any{"id": wid}},
		{"WalletService/UpdateWallet", map[string]any{"id": wid, "name": "Stolen"}},
		{"WalletService/ArchiveWallet", map[string]any{"id": wid}},
		{"WalletService/DeleteWallet", map[string]any{"id": wid}},
		{"LabelService/UpdateLabel", map[string]any{"id": lid, "name": "Stolen"}},
		{"LabelService/DeleteLabel", map[string]any{"id": lid}},
		{"CategoryService/UpdateCategory", map[string]any{"id": cid, "name": "Stolen"}},
		{"CategoryService/DeleteCategory", map[string]any{"id": cid}},
	} {
		foreign.call(tc.method, tc.body, 404)
	}
	w = object(a.call("WalletService/UpdateWallet", map[string]any{"id": wid, "name": " Renamed ", "hidden": true, "initialBalanceMinor": "0", "position": 0}, 200), "wallet")
	if w["name"] != "Renamed" || w["hidden"] != true {
		t.Fatalf("wallet patch: %v", w)
	}
	if ws, _ := a.call("WalletService/ListWallets", map[string]any{}, 200)["wallets"].([]any); len(ws) != 0 {
		t.Fatal("hidden wallet listed")
	}
	w = object(a.call("WalletService/UpdateWallet", map[string]any{"id": wid, "hidden": false}, 200), "wallet")
	if w["hidden"] == true || w["name"] != "Renamed" {
		t.Fatal("optional false patch lost or reset name")
	}
	label = object(a.call("LabelService/UpdateLabel", map[string]any{"id": lid, "name": "Holiday"}, 200), "label")
	if label["name"] != "Holiday" || label["color"] != "cat-orange" {
		t.Fatalf("label patch: %v", label)
	}
	cat = object(a.call("CategoryService/UpdateCategory", map[string]any{"id": cid, "name": "Meals", "position": 0}, 200), "category")
	if cat["name"] != "Meals" || cat["kind"] != "CATEGORY_KIND_EXPENSE" {
		t.Fatalf("category patch: %v", cat)
	}
	a.call("CategoryService/SetCategoryHidden", map[string]any{"walletId": wid, "categoryId": cid, "hidden": true}, 200)
	a.call("CategoryService/SetCategoryHidden", map[string]any{"walletId": wid, "categoryId": cid, "hidden": false}, 200)
	a.call("LabelService/DeleteLabel", map[string]any{"id": lid}, 200)
	if ls, _ := a.call("LabelService/ListLabels", map[string]any{}, 200)["labels"].([]any); len(ls) != 0 {
		t.Fatal("label not deleted")
	}
	a.call("CategoryService/DeleteCategory", map[string]any{"id": cid}, 200)
	a.call("WalletService/DeleteWallet", map[string]any{"id": wid}, 200)
	a.call("WalletService/GetWallet", map[string]any{"id": wid}, 404)
	user := object(a.call("UserService/UpdateProfile", map[string]any{"name": " New Name "}, 200), "user")
	if user["name"] != "New Name" {
		t.Fatalf("profile: %v", user)
	}
	if object(a.call("UserService/GetMe", map[string]any{}, 200), "user")["name"] != "New Name" {
		t.Fatal("profile update not persisted")
	}
}

func TestTransactionsReportsAndBudgetLifecycle(t *testing.T) {
	a, _ := managementClients(t)
	wid := object(a.call("WalletService/CreateWallet", map[string]any{"name": "Cash", "currency": "USD", "initialBalanceMinor": "10000"}, 200), "wallet")["id"]
	cid := object(a.call("CategoryService/CreateCategory", map[string]any{"name": "Meals", "kind": "CATEGORY_KIND_EXPENSE", "icon": "coffee", "color": "cat-orange"}, 200), "category")["id"]
	tx := object(a.call("TransactionService/CreateTransaction", map[string]any{"walletId": wid, "categoryId": cid, "amountMinor": "1500", "occurredOn": "2026-10-12", "today": "2026-10-15", "note": "Lunch"}, 200), "transaction")
	tid := tx["id"]
	txFilter := map[string]any{"walletId": wid, "period": map[string]any{"from": "2026-10-01", "to": "2026-10-31"}, "today": "2026-10-15", "noteQuery": "Lunch"}
	listed := a.call("TransactionService/ListTransactions", map[string]any{"filter": txFilter, "pageSize": 1}, 200)
	if rows := listed["transactions"].([]any); len(rows) != 1 || rows[0].(map[string]any)["id"] != tid {
		t.Fatalf("filtered transactions: %v", listed)
	}
	summary := a.call("TransactionService/GetPeriodSummary", map[string]any{"filter": txFilter}, 200)
	if summary["currency"] != "USD" || summary["balanceMinor"] != "8500" || summary["expensesMinor"] != "1500" || summary["changeMinor"] != "-1500" || summary["transactionCount"] != "1" {
		t.Fatalf("period summary: %v", summary)
	}
	a.call("WalletService/DeleteWallet", map[string]any{"id": wid}, 400)
	filter := map[string]any{"period": map[string]any{"from": "2026-10-01", "to": "2026-10-31"}, "walletIds": []any{wid}}
	dash := a.call("DashboardService/GetDashboard", map[string]any{"filter": filter, "currency": "USD", "today": "2026-10-15"}, 200)
	if dash["expensesMinor"] != "1500" || dash["balanceMinor"] != "8500" {
		t.Fatalf("dashboard totals: %v", dash)
	}
	budget := map[string]any{"name": "Food", "amountMinor": "10000", "currency": "USD", "repeat": "BUDGET_REPEAT_MONTHLY", "startDate": "2026-10-01", "walletIds": []any{wid}, "categoryIds": []any{cid}}
	bid := object(a.call("BudgetService/CreateBudget", map[string]any{"budget": budget}, 200), "budget")["id"]
	detail := a.call("BudgetService/GetBudget", map[string]any{"id": bid, "today": "2026-10-15"}, 200)
	if object(detail, "progress")["spentMinor"] != "1500" || object(detail, "progress")["remainingMinor"] != "8500" {
		t.Fatalf("budget progress: %v", detail)
	}
	budget["amountMinor"] = "20000"
	a.call("BudgetService/UpdateBudget", map[string]any{"id": bid, "budget": budget}, 200)
	detail = a.call("BudgetService/GetBudget", map[string]any{"id": bid, "today": "2026-10-15", "periodDate": "2026-10-01"}, 200)
	if object(detail, "progress")["remainingMinor"] != "18500" {
		t.Fatalf("budget update: %v", detail)
	}
	a.call("BudgetService/ListBudgets", map[string]any{"today": "2026-10-15"}, 200)
	report := a.call("DashboardService/GetCategoryReport", map[string]any{"categoryId": cid, "filter": filter, "currency": "USD", "today": "2026-10-15"}, 200)
	if report["totalMinor"] != "1500" || report["transactionCount"] != "1" {
		t.Fatalf("category report: %v", report)
	}
	client := apitest.RegisterClient(t, a.do, "Reporting assistant")
	access, _ := apitest.TokenFor(t, a.do, a.session, client)
	profile := apitest.CallTool(t, a.do, access, "get_profile", map[string]any{})
	if profile["base_currency"] != "USD" {
		t.Fatalf("MCP profile: %v", profile)
	}
	if cs := apitest.CallTool(t, a.do, access, "list_categories", map[string]any{})["categories"].([]any); len(cs) == 0 {
		t.Fatal("MCP categories empty")
	}
	apitest.CallTool(t, a.do, access, "list_labels", map[string]any{})
	upcoming := apitest.CallTool(t, a.do, access, "list_upcoming", map[string]any{"today": "2026-10-15", "wallet_id": wid, "limit": 1})
	if len(upcoming["transactions"].([]any)) != 0 {
		t.Fatalf("past transaction was upcoming: %v", upcoming)
	}
	mcpReport := apitest.CallTool(t, a.do, access, "category_report", map[string]any{"category_id": cid, "from": "2026-10-01", "to": "2026-10-31", "today": "2026-10-15"})
	if object(mcpReport, "total")["minor"] != float64(1500) {
		t.Fatalf("MCP category report: %v", mcpReport)
	}
	for _, args := range []map[string]any{{"today": "2026-10-15"}, {"today": "2026-10-15", "budget_id": bid, "period_date": "2026-10-01"}} {
		bs := apitest.CallTool(t, a.do, access, "budget_status", args)["budgets"].([]any)
		if len(bs) != 1 {
			t.Fatalf("MCP budgets: %v", bs)
		}
	}
	a.call("TransactionService/UpdateTransaction", map[string]any{"id": tid, "categoryId": cid, "amountMinor": "2000", "occurredOn": "2026-10-12", "today": "2026-10-15", "note": "Dinner"}, 200)
	w := object(a.call("WalletService/GetWallet", map[string]any{"id": wid, "today": "2026-10-15"}, 200), "wallet")
	if w["balanceMinor"] != "8000" {
		t.Fatalf("edited balance: %v", w)
	}
	a.call("BudgetService/DeleteBudget", map[string]any{"id": bid}, 200)
	a.call("BudgetService/GetBudget", map[string]any{"id": bid}, 404)
	a.call("TransactionService/DeleteTransactions", map[string]any{"ids": []any{tid}}, 200)
	w = object(a.call("WalletService/GetWallet", map[string]any{"id": wid, "today": "2026-10-15"}, 200), "wallet")
	if w["balanceMinor"] != "10000" {
		t.Fatalf("delete did not restore balance: %v", w)
	}
	a.call("WalletService/ArchiveWallet", map[string]any{"id": wid}, 200)
	if ws, _ := a.call("WalletService/ListWallets", map[string]any{}, 200)["wallets"].([]any); len(ws) != 0 {
		t.Fatal("archived wallet listed")
	}
	if ws, _ := a.call("WalletService/ListWallets", map[string]any{"includeArchived": true}, 200)["wallets"].([]any); len(ws) != 1 {
		t.Fatal("archive did not preserve wallet")
	}
}

func TestInvalidAPIMutationsDoNotChangeWallet(t *testing.T) {
	a, _ := managementClients(t)
	wid := object(a.call("WalletService/CreateWallet", map[string]any{"name": "Validation", "currency": "USD", "initialBalanceMinor": "10000"}, 200), "wallet")["id"]
	for _, tc := range []struct {
		name, method string
		body         map[string]any
	}{
		{"empty wallet name", "WalletService/UpdateWallet", map[string]any{"id": wid, "name": " "}},
		{"unknown currency", "WalletService/CreateWallet", map[string]any{"name": "Invalid", "currency": "ZZZ"}},
		{"empty profile", "UserService/UpdateProfile", map[string]any{"name": " "}},
		{"invalid label color", "LabelService/CreateLabel", map[string]any{"name": "Invalid", "color": "#123456"}},
		{"invalid label ID", "LabelService/DeleteLabel", map[string]any{"id": "bad"}},
		{"invalid category ID", "CategoryService/UpdateCategory", map[string]any{"id": "bad"}},
		{"invalid merge target", "CategoryService/MergeCategories", map[string]any{"targetId": "bad"}},
		{"missing budget", "BudgetService/CreateBudget", map[string]any{}},
		{"invalid budget ID", "BudgetService/UpdateBudget", map[string]any{"id": "bad"}},
		{"invalid budget date", "BudgetService/ListBudgets", map[string]any{"today": "2026-02-29"}},
		{"missing report filter", "DashboardService/GetDashboard", map[string]any{}},
		{"invalid report category", "DashboardService/GetCategoryReport", map[string]any{"categoryId": "bad"}},
		{"missing transaction filter", "TransactionService/ListTransactions", map[string]any{}},
		{"invalid transaction ID", "TransactionService/UpdateTransaction", map[string]any{"id": "bad"}},
		{"invalid deletion ID", "TransactionService/DeleteTransactions", map[string]any{"ids": []string{"bad"}}},
		{"invalid import ID", "ImportService/PreviewImport", map[string]any{"batchId": "bad"}},
		{"invalid rule category", "ImportService/CreateCategoryRule", map[string]any{"pattern": "test", "categoryId": "bad"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := a
			c.t = t
			if code := c.call(tc.method, tc.body, 400)["code"]; code != "invalid_argument" {
				t.Fatalf("error code = %v", code)
			}
		})
	}
	wallet := object(a.call("WalletService/GetWallet", map[string]any{"id": wid}, 200), "wallet")
	if wallet["name"] != "Validation" || wallet["balanceMinor"] != "10000" {
		t.Fatalf("failed mutations changed wallet: %v", wallet)
	}
	ws := a.call("WalletService/ListWallets", map[string]any{}, 200)["wallets"].([]any)
	if len(ws) != 1 {
		t.Fatal("failed wallet creation persisted")
	}
}

func TestImportOverHTTPPreservesPreviewAndHouseholdScope(t *testing.T) {
	a, foreign := managementClients(t)
	wid := object(a.call("WalletService/CreateWallet", map[string]any{"name": "Import", "currency": "USD"}, 200), "wallet")["id"]
	cid := object(a.call("CategoryService/CreateCategory", map[string]any{"name": "Food", "kind": "CATEGORY_KIND_EXPENSE", "icon": "coffee", "color": "cat-orange"}, 200), "category")["id"]
	rule := object(a.call("ImportService/CreateCategoryRule", map[string]any{"pattern": "Lunch", "categoryId": cid}, 200), "rule")
	a.call("ImportService/ListCategoryRules", map[string]any{}, 200)
	content := base64.StdEncoding.EncodeToString([]byte("date,amount,note\n2026-10-12,-12.50,Lunch\n2026-10-13,-2.25,Coffee\n"))
	upload := a.call("ImportService/UploadImportFile", map[string]any{"walletId": wid, "fileName": "statement.csv", "content": content}, 200)
	batch := upload["batchId"]
	mapping := map[string]any{"hasHeader": true, "dateColumn": 0, "dateFormat": "yyyy-mm-dd", "amountColumn": 1, "noteColumn": 2, "decimalSeparator": "DECIMAL_SEPARATOR_DOT", "sign": "IMPORT_SIGN_NEGATIVE_IS_EXPENSE", "bank": "generic"}
	foreign.call("ImportService/PreviewImport", map[string]any{"batchId": batch, "mapping": mapping}, 400)
	foreign.call("ImportService/CommitImport", map[string]any{"batchId": batch, "mapping": mapping, "rows": []any{map[string]any{"index": 0, "categoryId": cid}}}, 400)
	preview := a.call("ImportService/PreviewImport", map[string]any{"batchId": batch, "mapping": mapping}, 200)
	rows := preview["rows"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["amountMinor"] != "1250" || rows[0].(map[string]any)["categoryId"] != cid {
		t.Fatalf("preview: %v", preview)
	}
	before := object(a.call("WalletService/GetWallet", map[string]any{"id": wid, "today": "2026-10-15"}, 200), "wallet")
	if before["balanceMinor"] != nil && before["balanceMinor"] != "0" {
		t.Fatal("preview changed wallet balance")
	}
	committed := a.call("ImportService/CommitImport", map[string]any{"batchId": batch, "mapping": mapping, "saveMapping": true, "rows": []any{map[string]any{"index": 0, "categoryId": cid}, map[string]any{"index": 1, "categoryId": cid}}}, 200)
	if committed["importedCount"] != "2" {
		t.Fatalf("commit: %v", committed)
	}
	wallet := object(a.call("WalletService/GetWallet", map[string]any{"id": wid, "today": "2026-10-15"}, 200), "wallet")
	if wallet["balanceMinor"] != "-1475" || wallet["transactionCount"] != "2" {
		t.Fatalf("import balance: %v", wallet)
	}
	a.call("ImportService/DeleteCategoryRule", map[string]any{"id": rule["id"]}, 200)
	if rs, _ := a.call("ImportService/ListCategoryRules", map[string]any{}, 200)["rules"].([]any); len(rs) != 0 {
		t.Fatal("rule was not deleted")
	}
}
