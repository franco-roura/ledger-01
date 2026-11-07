package oauth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
)

// RFC 7636 appendix B: an independently specified S256 test vector.
const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

func TestPKCE(t *testing.T) {
	if !pkceMatches(challenge, verifier) {
		t.Fatal("RFC 7636 vector rejected")
	}
	for _, bad := range []string{"", "short", verifier + "x", strings.Repeat("x", 129)} {
		if pkceMatches(challenge, bad) {
			t.Fatalf("accepted verifier %q", bad)
		}
	}
	if pkceMatches("tampered", verifier) {
		t.Fatal("accepted wrong challenge")
	}
}

func TestConsentAndTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	s := NewStore(pool)
	email := uuid.NewString() + "@example.com"
	a, err := ledger.New(pool).EnsureUser(ctx, ledger.Profile{Subject: email, Email: email, Name: "OAuth"})
	if err != nil {
		t.Fatal(err)
	}
	const redirect = "https://client.example/callback?existing=keep"
	const issuer = "https://ledger.example"
	c, err := s.RegisterClient(ctx, "Assistant", []string{redirect})
	if err != nil {
		t.Fatal(err)
	}
	create := func() uuid.UUID {
		t.Helper()
		id, err := s.CreateRequest(ctx, Request{ClientID: c.ID, RedirectURI: redirect, CodeChallenge: challenge, Scope: ScopeRead, Resource: issuer + "/mcp", State: "opaque state + &"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := create()
	pending, err := s.PendingRequest(ctx, id)
	if err != nil || pending.ClientName != "Assistant" || pending.RedirectHost != "client.example" || pending.Scope != ScopeRead {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	denied, err := s.Deny(ctx, id, issuer)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(denied)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("error") != "access_denied" || u.Query().Get("existing") != "keep" || u.Query().Get("state") != "opaque state + &" || u.Query().Get("iss") != issuer {
		t.Fatal(denied)
	}
	if _, err := s.Approve(ctx, a.UserID, id, issuer); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("denied consent reused: %v", err)
	}
	code := func() string {
		t.Helper()
		id := create()
		redirected, err := s.Approve(ctx, a.UserID, id, issuer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.PendingRequest(ctx, id); !errors.Is(err, ledger.ErrNotFound) {
			t.Fatalf("approved consent still pending: %v", err)
		}
		u, err := url.Parse(redirected)
		if err != nil {
			t.Fatal(err)
		}
		return u.Query().Get("code")
	}
	badCode := code()
	if _, err := s.ExchangeCode(ctx, c.ID, badCode, redirect, verifier+"x"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("wrong PKCE: %v", err)
	}
	if _, err := s.ExchangeCode(ctx, c.ID, badCode, redirect, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("failed code replay: %v", err)
	}
	validCode := code()
	tokens, err := s.ExchangeCode(ctx, c.ID, validCode, redirect, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if tokens.Access == tokens.Refresh || len(tokens.Access) != 43 || tokens.Scope != ScopeRead {
		t.Fatalf("invalid tokens: %+v", tokens)
	}
	if _, err := s.ExchangeCode(ctx, c.ID, validCode, redirect, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("code replay: %v", err)
	}
	access, err := s.Resolve(ctx, tokens.Access)
	if err != nil || access.Actor != a || access.ClientID != c.ID {
		t.Fatalf("access: %+v %v", access, err)
	}
	if err := s.RevokeToken(ctx, uuid.New(), tokens.Access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, tokens.Access); err != nil {
		t.Fatalf("foreign client revoked token: %v", err)
	}
	rotated, err := s.Refresh(ctx, c.ID, tokens.Refresh)
	if err != nil || rotated.Access == tokens.Access || rotated.Refresh == tokens.Refresh {
		t.Fatalf("rotation: %v", err)
	}
	if _, err := s.Refresh(ctx, c.ID, tokens.Refresh); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh replay: %v", err)
	}
	if _, err := s.Resolve(ctx, rotated.Access); !errors.Is(err, ErrNoToken) {
		t.Fatalf("replayed grant not revoked: %v", err)
	}
	// A fresh grant can also be explicitly revoked with its refresh token.
	tokens, err = s.ExchangeCode(ctx, c.ID, code(), redirect, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, c.ID, tokens.Refresh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, tokens.Access); !errors.Is(err, ErrNoToken) {
		t.Fatalf("revoked grant: %v", err)
	}
	if err := s.RevokeToken(ctx, c.ID, "unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
}
