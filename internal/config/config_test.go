package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearMailEnv isolates a test from variables that the host machine might
// have. Without this, a developer's `.env` would leak into the test.
func clearMailEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DDCORE_MAIL_TRANSPORT", "DDCORE_MAIL_FROM", "DDCORE_MAIL_METHOD", "DDCORE_MAIL_DEBUG",
		"DDCORE_SMTP_HOST", "DDCORE_SMTP_PORT", "DDCORE_SMTP_USERNAME",
		"DDCORE_SMTP_PASSWORD", "DDCORE_SMTP_TLS", "DDCORE_URL", "DDCORE_TRUST_PROXY",
		"DDCORE_DSN", "DDCORE_PORT", "DATABASE_URL", "PORT",
		"DDCORE_LOGIN_NOTICE", "DDCORE_LOGIN_DEMO_USER", "DDCORE_LOGIN_DEMO_PASSWORD",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// site writes a minimal ddcore.json and returns the directory.
func site(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, Name), body)
	return dir
}

func TestLoadDefaults(t *testing.T) {
	clearMailEnv(t)
	// `site` was removed: a config still carrying it has to load, not be
	// refused, because every ddcore.json written before the change has one
	f, _, err := Load(site(t, `{"site":"x"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Auth.SessionDays != 30 || f.Auth.MinPasswordLength != 8 || f.Auth.MaxLoginAttempts != 5 {
		t.Errorf("default policy not applied: %+v", f.Auth)
	}
	if f.Mail.Transport != MailLog {
		t.Errorf("without configuration, transport is log, got %q", f.Mail.Transport)
	}
	if !f.Auth.AllowSelfServiceAPIKeys() {
		t.Error("key self-service is allowed by default")
	}
	if f.HasPublicURL() {
		t.Error("no public URL was declared")
	}
	if got := f.PublicURL(); !strings.HasPrefix(got, "http://localhost:") {
		t.Errorf("URL fallback: got %q", got)
	}
	if f.Auth.SessionTTL().Hours() != 24*30 {
		t.Errorf("SessionTTL: got %v", f.Auth.SessionTTL())
	}
}

// The auth block in ddcore.json only overrides what it declares — the rest remains
// default, otherwise declaring one key would erase the others.
func TestAuthPolicyMergesOverDefaults(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{"auth":{"sessionDays":7,"maxLoginAttempts":3}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Auth.SessionDays != 7 || f.Auth.MaxLoginAttempts != 3 {
		t.Errorf("declared value did not take effect: %+v", f.Auth)
	}
	if f.Auth.MinPasswordLength != 8 || f.Auth.InviteHours != 72 {
		t.Errorf("undeclared value should stay default: %+v", f.Auth)
	}
}

func TestAuthPolicyRefusesNonsense(t *testing.T) {
	for _, tc := range []struct{ name, json string }{
		{"sessionDays zero", `{"auth":{"sessionDays":0}}`},
		{"negative sessionDays", `{"auth":{"sessionDays":-1}}`},
		{"maxLoginAttempts zero", `{"auth":{"maxLoginAttempts":0}}`},
		{"lockoutMinutes zero", `{"auth":{"lockoutMinutes":0}}`},
		{"negative apiKeyDays", `{"auth":{"apiKeyDays":-1}}`},
		{"password too short", `{"auth":{"minPasswordLength":4}}`},
		{"password too long", `{"auth":{"minPasswordLength":500}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearMailEnv(t)
			if _, _, err := Load(site(t, tc.json)); err == nil {
				t.Fatal("expected load error, got nil")
			}
		})
	}
}

func TestMailFromEnvironment(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_MAIL_TRANSPORT", "smtp")
	t.Setenv("DDCORE_MAIL_FROM", "ddcore <no-reply@x.com>")
	t.Setenv("DDCORE_SMTP_HOST", "smtp.x.com")
	t.Setenv("DDCORE_SMTP_PORT", "2525")
	t.Setenv("DDCORE_SMTP_USERNAME", "u")
	t.Setenv("DDCORE_SMTP_PASSWORD", "p")

	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Mail.Transport != MailSMTP || f.Mail.Host != "smtp.x.com" || f.Mail.Port != 2525 {
		t.Errorf("email configuration did not come from environment: %+v", f.Mail)
	}
	if f.Mail.TLS != TLSStartTLS {
		t.Errorf("default TLS is starttls, got %q", f.Mail.TLS)
	}
}

func TestMailRefusesIncompleteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"unknown transport", map[string]string{"DDCORE_MAIL_TRANSPORT": "carta"}},
		{"smtp without host", map[string]string{"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_MAIL_FROM": "a@b.c"}},
		{"smtp without sender", map[string]string{"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h"}},
		{"user without password", map[string]string{
			"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h",
			"DDCORE_MAIL_FROM": "a@b.c", "DDCORE_SMTP_USERNAME": "u",
		}},
		{"unknown TLS", map[string]string{
			"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h",
			"DDCORE_MAIL_FROM": "a@b.c", "DDCORE_SMTP_TLS": "ssl3",
		}},
		{"invalid port", map[string]string{"DDCORE_SMTP_PORT": "zero"}},
		{"method without path", map[string]string{"DDCORE_MAIL_TRANSPORT": "method"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearMailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, _, err := Load(site(t, `{}`)); err == nil {
				t.Fatal("expected load error, got nil")
			}
		})
	}
}

// The entire precedence, in a single test: real environment > .env > ddcore.json.
func TestPrecedenceEnvBeatsDotenvBeatsFile(t *testing.T) {
	clearMailEnv(t)
	dir := site(t, `{"url":"https://do-json.example","port":1111}`)
	writeFile(t, filepath.Join(dir, DotenvName),
		"DDCORE_URL=https://do-dotenv.example\nDDCORE_PORT=2222\nDDCORE_TRUST_PROXY=true\n")
	t.Setenv("DDCORE_URL", "https://do-ambiente.example")

	f, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.URL != "https://do-ambiente.example" {
		t.Errorf("real environment should take precedence over everything, got %q", f.URL)
	}
	if f.Port != 2222 {
		t.Errorf(".env should take precedence over ddcore.json, got %d", f.Port)
	}
	if !f.TrustProxy {
		t.Error("trustProxy should have come from .env")
	}
}

// A URL with trailing slash would cause double-slash links.
func TestPublicURLLosesTheTrailingSlash(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_URL", "https://erp.example.com/")
	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.PublicURL() != "https://erp.example.com" {
		t.Errorf("got %q", f.PublicURL())
	}
}

// DATABASE_URL must override ddcore.json's dsn, and DDCORE_DSN wins over both.
func TestDatabaseURLOverridesJSONAndDDCOREDSNWins(t *testing.T) {
	clearMailEnv(t)
	dir := site(t, `{"dsn":"postgres://local:5432/app"}`)

	// Without env, ddcore.json value is kept
	f, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.DSN != "postgres://local:5432/app" {
		t.Errorf("expected DSN from json, got %q", f.DSN)
	}

	// Platform's DATABASE_URL (Railway) overrides ddcore.json
	t.Setenv("DATABASE_URL", "postgres://railway:5432/prod")
	f, _, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.DSN != "postgres://railway:5432/prod" {
		t.Errorf("DATABASE_URL should override json, got %q", f.DSN)
	}

	// Explicit DDCORE_DSN overrides even DATABASE_URL
	t.Setenv("DDCORE_DSN", "postgres://custom:5432/override")
	f, _, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.DSN != "postgres://custom:5432/override" {
		t.Errorf("DDCORE_DSN should override DATABASE_URL, got %q", f.DSN)
	}
}

// Platform PORT (Railway/Heroku/Cloud Run) overrides ddcore.json, and DDCORE_PORT wins.
func TestPlatformPortOverridesJSONAndDDCOREPortWins(t *testing.T) {
	clearMailEnv(t)
	dir := site(t, `{"port":8091}`)

	// Without env, ddcore.json port is kept
	f, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Port != 8091 {
		t.Errorf("expected port 8091, got %d", f.Port)
	}

	// Platform PORT overrides ddcore.json
	t.Setenv("PORT", "65432")
	f, _, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Port != 65432 {
		t.Errorf("platform PORT should override json, got %d", f.Port)
	}

	// Explicit DDCORE_PORT overrides platform PORT
	t.Setenv("DDCORE_PORT", "9999")
	f, _, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Port != 9999 {
		t.Errorf("DDCORE_PORT should override PORT, got %d", f.Port)
	}
}

func TestLoginPage(t *testing.T) {
	clearMailEnv(t)
	dir := site(t, `{"login":{"notice":"From the file","demoUser":"visitor@example.com","demoPassword":"file-pw"}}`)
	f, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Login != (LoginPage{Notice: "From the file", DemoUser: "visitor@example.com", DemoPassword: "file-pw"}) {
		t.Errorf("ddcore.json: got %+v", f.Login)
	}

	t.Setenv("DDCORE_LOGIN_NOTICE", `Public demo\nData resets`)
	t.Setenv("DDCORE_LOGIN_DEMO_PASSWORD", "env-pw")
	if f, _, err = Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Login.Notice != "Public demo\nData resets" {
		t.Errorf("a literal \\n in the variable is a line break, got %q", f.Login.Notice)
	}
	if f.Login.DemoUser != "visitor@example.com" || f.Login.DemoPassword != "env-pw" {
		t.Errorf("the environment overrides the file field by field, got %+v", f.Login)
	}
}

func TestLoginDemoAccountNeedsBoth(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_LOGIN_DEMO_USER", "visitor@example.com")
	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Login.DemoUser != "" || f.Login.DemoPassword != "" {
		t.Errorf("a user without a password is dropped, got %+v", f.Login)
	}
}
