package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/config"
)

func TestCallbackRejectsMissingCorruptAndMismatchedState(t *testing.T) {
	h := &Handler{cfg: config.Config{PublicURL: &url.URL{Scheme: "https", Host: "ledger.example"}}}
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	flow := func(state string) string {
		b, err := json.Marshal(flowState{State: state})
		if err != nil {
			t.Fatal(err)
		}
		return encode(string(b))
	}
	for _, tc := range []struct {
		name, cookie, query string
		status              int
	}{
		{"missing cookie", "", "state=expected", 400},
		{"invalid base64", "!", "state=expected", 400},
		{"invalid JSON", encode("{"), "state=expected", 400},
		{"empty state", flow(""), "state=", 400},
		{"mismatched state", flow("expected"), "state=wrong", 400},
		{"cancelled with matching state", flow("expected"), "state=expected&error=access_denied", 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?"+tc.query, nil)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: flowCookie, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			h.callback(w, r)
			res := w.Result()
			if res.StatusCode != tc.status {
				t.Fatalf("%d: %s", res.StatusCode, w.Body.String())
			}
			if tc.status == 302 && res.Header.Get("Location") != "/login?error=cancelled" {
				t.Fatal(res.Header)
			}
			for _, c := range res.Cookies() {
				if c.Name == CookieName {
					t.Fatal("invalid callback created session")
				}
				if c.Name == flowCookie && (c.MaxAge != -1 || !c.HttpOnly || !c.Secure || c.Path != "/auth") {
					t.Fatalf("flow cookie not cleared securely: %+v", c)
				}
			}
		})
	}
}

func TestSafeNext(t *testing.T) {
	for next, want := range map[string]string{
		"":                     "/dashboard",
		"/wallets/abc?from=x":  "/wallets/abc?from=x",
		"/oauth/authorize?a=b": "/oauth/authorize?a=b",
		"https://evil.example": "/dashboard",
		"//evil.example":       "/dashboard",
		`/\evil.example`:       "/dashboard",
		`/a\b`:                 "/dashboard",
		"/\t/evil.example":     "/dashboard",
		"/\n/evil.example":     "/dashboard",
		"/\r\n/evil.example":   "/dashboard",
		"/\x00x":               "/dashboard",
		"relative":             "/dashboard",
		// Still percent-encoded when it reaches the browser, so it stays a path.
		"/%09/evil.example": "/%09/evil.example",
	} {
		if got := safeNext(next); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}
}
