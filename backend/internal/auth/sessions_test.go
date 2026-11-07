package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

func TestSessionCookies(t *testing.T) {
	for _, secure := range []bool{false, true} {
		w := httptest.NewRecorder()
		setSessionCookie(w, "opaque-token", time.Now().Add(time.Hour), secure)
		c := w.Result().Cookies()[0]
		if c.Name != CookieName || c.Value != "opaque-token" || !c.HttpOnly || c.Secure != secure || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge < 3590 {
			t.Fatalf("cookie: %+v", c)
		}
		h := http.Header{"Cookie": {"unrelated=x; " + c.Name + "=" + c.Value}}
		if TokenFromHeader(h) != c.Value || TokenFromHeader(http.Header{}) != "" {
			t.Fatal("cookie extraction")
		}
		w = httptest.NewRecorder()
		clearSessionCookie(w, secure)
		c = w.Result().Cookies()[0]
		if c.Value != "" || c.MaxAge != -1 || c.Secure != secure || !c.HttpOnly || c.Path != "/" {
			t.Fatalf("clear cookie: %+v", c)
		}
	}
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	email := uuid.NewString() + "@example.com"
	a, err := ledger.New(pool).EnsureUser(ctx, ledger.Profile{Subject: email, Email: email, Name: "Sessions"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewSessions(dbq.New(pool), 24*time.Hour)
	token, expiry, err := s.Create(ctx, a.UserID, strings.Repeat("a", 400))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || time.Until(expiry) < 23*time.Hour {
		t.Fatal("token entropy or expiry")
	}
	var stored []byte
	var ua string
	if err := pool.QueryRow(ctx, "SELECT token_hash, user_agent FROM sessions WHERE user_id=$1", a.UserID).Scan(&stored, &ua); err != nil {
		t.Fatal(err)
	}
	if string(stored) == token || string(stored) != string(hashToken(token)) || len(ua) != 300 {
		t.Fatal("token hashing or user-agent cap")
	}
	got, err := s.Resolve(ctx, token)
	if err != nil || got != a {
		t.Fatalf("Resolve = %+v, %v; want %+v", got, err, a)
	}
	for _, bad := range []string{"", "unknown", token + "x"} {
		if _, err := s.Resolve(ctx, bad); !errors.Is(err, ErrNoSession) {
			t.Fatalf("bad token: %v", err)
		}
	}
	// An active session is touched only after the refresh interval.
	var before, after time.Time
	if err := pool.QueryRow(ctx, "SELECT expires_at FROM sessions WHERE token_hash=$1", hashToken(token)).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT expires_at FROM sessions WHERE token_hash=$1", hashToken(token)).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("recent session was refreshed before the write throttle elapsed")
	}
	if _, err := pool.Exec(ctx, "UPDATE sessions SET last_seen_at=now()-interval '2 hours', expires_at=now()+interval '1 hour' WHERE token_hash=$1", hashToken(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT expires_at FROM sessions WHERE token_hash=$1", hashToken(token)).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after.Before(before) {
		t.Fatalf("sliding expiry did not refresh: %s < %s", after, before)
	}
	if err := s.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("revoked token: %v", err)
	}
	token, _, err = s.Create(ctx, a.UserID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", hashToken(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired token: %v", err)
	}
}
