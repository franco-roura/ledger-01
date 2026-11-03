package config

import (
	"strings"
	"testing"
	"time"
)

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestDevelopmentDefaults(t *testing.T) {
	c, err := FromLookup(lookup(map[string]string{"DATABASE_URL": "postgres://db"}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsDev() || c.DevLoginEnabled() || c.SecureCookies() || c.DBMaxConns != 4 || c.SessionTTL != 720*time.Hour || c.Addr != "127.0.0.1:8080" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.OAuthRedirectURL() != "http://localhost:3000/auth/google/callback" {
		t.Fatal(c.OAuthRedirectURL())
	}
}

func TestConfigurationRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value, message string }{
		{"DATABASE_URL", "", "DATABASE_URL is required"},
		{"APP_ENV", "staging", "APP_ENV must"},
		{"PUBLIC_URL", "/relative", "absolute URL"},
		{"PUBLIC_URL", "http://%", "absolute URL"},
		{"DB_MAX_CONNS", "0", "positive integer"},
		{"DB_MAX_CONNS", "-1", "positive integer"},
		{"DB_MAX_CONNS", "many", "positive integer"},
		{"SESSION_TTL", "59m", "at least 1h"},
		{"SESSION_TTL", "tomorrow", "at least 1h"},
		{"LOG_LEVEL", "verbose", "LOG_LEVEL"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			env := map[string]string{"DATABASE_URL": "postgres://db", tc.key: tc.value}
			_, err := FromLookup(lookup(env))
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v, want %q", err, tc.message)
			}
		})
	}
}

func TestProductionSafetyAndEmailNormalization(t *testing.T) {
	env := map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://db", "AUTH_DEV_LOGIN": "1"}
	_, err := FromLookup(lookup(env))
	for _, want := range []string{"https", "GOOGLE_CLIENT_ID", "ALLOWED_EMAILS"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q: %v", want, err)
		}
	}
	env["PUBLIC_URL"] = "https://ledger.example/"
	env["GOOGLE_CLIENT_ID"], env["GOOGLE_CLIENT_SECRET"] = "client", "secret"
	env["ALLOWED_EMAILS"] = " Owner@Example.com, ,owner@example.com, Partner@example.com "
	env["API_BASE_PATH"], env["SESSION_TTL"], env["DB_MAX_CONNS"] = "/api///", "1h", "12"
	c, err := FromLookup(lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	if c.DevLoginEnabled() || !c.SecureCookies() || len(c.AllowedEmails) != 2 || !c.EmailAllowed(" OWNER@example.COM ") || c.EmailAllowed("stranger@example.com") || c.APIBasePath != "/api" || c.DBMaxConns != 12 || c.SessionTTL != time.Hour {
		t.Fatalf("unsafe or incorrectly normalized config: %+v", c)
	}
	if c.OAuthRedirectURL() != "https://ledger.example/auth/google/callback" {
		t.Fatal(c.OAuthRedirectURL())
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://environment")
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_URL", "http://localhost:3000")
	t.Setenv("DB_MAX_CONNS", "4")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("LOG_LEVEL", "debug")
	c, err := Load()
	if err != nil || c.DatabaseURL != "postgres://environment" || c.SessionTTL != 2*time.Hour {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}
