package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOIDCFromEnv(t *testing.T) {
	if ps, err := oidcFromEnv(); err != nil || ps != nil {
		t.Fatalf("nothing configured: %v %v", ps, err)
	}

	t.Setenv("DDCORE_OIDC_PROVIDERS", "google, PocketID")
	if _, err := oidcFromEnv(); err == nil || !strings.Contains(err.Error(), "DDCORE_OIDC_GOOGLE_CLIENT_ID") {
		t.Fatalf("google without credentials: %v", err)
	}
	t.Setenv("DDCORE_OIDC_GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("DDCORE_OIDC_GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("DDCORE_OIDC_GOOGLE_ALLOWED_DOMAINS", "Example.com, @corp.example")
	if _, err := oidcFromEnv(); err == nil || !strings.Contains(err.Error(), "DDCORE_OIDC_POCKETID_ISSUER") {
		t.Fatalf("pocketid has no default issuer: %v", err)
	}
	t.Setenv("DDCORE_OIDC_POCKETID_ISSUER", "https://id.example.com/")
	t.Setenv("DDCORE_OIDC_POCKETID_CLIENT_ID", "p-id")
	t.Setenv("DDCORE_OIDC_POCKETID_CLIENT_SECRET", "p-secret")
	t.Setenv("DDCORE_OIDC_POCKETID_SCOPES", "email profile")
	ps, err := oidcFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	g, p := ps[0], ps[1]
	if g.Issuer != GoogleIssuer || g.Label != "Google" || strings.Join(g.AllowedDomains, ",") != "example.com,corp.example" {
		t.Errorf("google: %+v", g)
	}
	if p.ID != "pocketid" || p.Issuer != "https://id.example.com" || p.Label != "PocketID" || strings.Join(p.Scopes, " ") != "openid email profile" {
		t.Errorf("pocketid: %+v", p)
	}

	for k, v := range map[string]string{"DDCORE_OIDC_PROVIDERS": "google,google", "DDCORE_OIDC_POCKETID_ISSUER": "id.example.com"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := oidcFromEnv(); err == nil {
				t.Errorf("%s=%s accepted", k, v)
			}
		})
	}
	t.Run("bad id", func(t *testing.T) {
		t.Setenv("DDCORE_OIDC_PROVIDERS", "../x")
		if _, err := oidcFromEnv(); err == nil {
			t.Error("accepted")
		}
	})
}

func TestLoadValidatesOIDC(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, Name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"auth": {"passwordLogin": false}}`)
	if _, _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "passwordLogin") {
		t.Fatalf("password off with no provider: %v", err)
	}

	write(`{}`)
	t.Setenv("DDCORE_OIDC_PROVIDERS", "google")
	t.Setenv("DDCORE_OIDC_GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("DDCORE_OIDC_GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("DDCORE_URL", "")
	if _, _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "DDCORE_URL") {
		t.Fatalf("provider without a public URL: %v", err)
	}

	t.Setenv("DDCORE_URL", "https://erp.example.com")
	write(`{"auth": {"passwordLogin": false}}`)
	f, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Auth.AllowPasswordLogin() || len(f.OIDC) != 1 {
		t.Fatalf("loaded: %+v %+v", f.Auth, f.OIDC)
	}
}
