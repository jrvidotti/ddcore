package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestSecretEnvName(t *testing.T) {
	for in, want := range map[string]string{
		"stripe_key":     "DDCORE_SECRET_STRIPE_KEY",
		"StripeKey":      "DDCORE_SECRET_STRIPEKEY",
		"whatsapp-token": "DDCORE_SECRET_WHATSAPP_TOKEN",
		"a.b c":          "DDCORE_SECRET_A_B_C",
	} {
		if got := SecretEnvName(in); got != want {
			t.Errorf("%q: expected %q, got %q", in, want, got)
		}
	}
}

// A secret comes from the environment and nowhere else; the prefix is the boundary
// that prevents an app from reading the DSN or SMTP password via the same path.
func TestSecretReadsOnlyItsOwnPrefix(t *testing.T) {
	e := &Engine{}
	t.Setenv("DDCORE_SECRET_STRIPE_KEY", "sk_live_x")
	t.Setenv("DDCORE_DSN", "postgres://should-not-leak")

	if v, ok := e.Secret("stripe_key"); !ok || v != "sk_live_x" {
		t.Errorf("expected secret, got %q (%v)", v, ok)
	}
	if _, ok := e.Secret("DDCORE_DSN"); ok {
		t.Error("prefix should prevent reading a variable that is not an app secret")
	}
	if _, ok := e.Secret("nao_configurado"); ok {
		t.Error("a missing secret must not appear configured")
	}
}

// The message names the variable for whoever has to configure it, never the value:
// it ends up in Error Log and toast notifications.
func TestRequireSecretNamesTheVariableNotTheValue(t *testing.T) {
	e := &Engine{}
	t.Setenv("DDCORE_SECRET_PRESENTE", "valor-secreto")

	if _, err := e.RequireSecret("ausente"); err == nil {
		t.Fatal("expected error for missing secret")
	} else if msg := err.Error(); !strings.Contains(msg, "DDCORE_SECRET_AUSENTE") {
		t.Errorf("message should name the variable, got %q", msg)
	}
	v, err := e.RequireSecret("presente")
	if err != nil || v != "valor-secreto" {
		t.Errorf("expected value, got %q / %v", v, err)
	}
}

// The vault master key lives under the same DDCORE_SECRET_ prefix as an app
// secret (DDCORE_SECRET_KEY), but it is not an app secret: an app that asked
// for ddcore.secret("key") must not get it back.
func TestSecretRefusesTheVaultMasterKey(t *testing.T) {
	e := &Engine{}
	t.Setenv(MasterKeyEnvName, "super-secret-master-key")

	if v, ok := e.Secret("key"); ok {
		t.Errorf("expected the master key to be refused, got %q", v)
	}
	if _, err := e.RequireSecret("key"); err == nil {
		t.Error("expected RequireSecret to refuse the master key")
	}
	for _, n := range e.SecretNames() {
		if n == "KEY" {
			t.Error("SecretNames must not list the vault master key")
		}
	}
}

func TestSecretNamesOmitsValues(t *testing.T) {
	e := &Engine{}
	t.Setenv("DDCORE_SECRET_UM", "valor-um")
	t.Setenv("DDCORE_SECRET_DOIS", "valor-dois")
	names := e.SecretNames()
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if strings.Contains(n, "valor") {
			t.Errorf("SecretNames returned a value: %q", n)
		}
	}
	if !seen["UM"] || !seen["DOIS"] {
		t.Errorf("expected UM and DOIS, got %v", names)
	}
}

// A Password field is still text in the database — but is no longer exposed on read.
func TestRedactPasswordBlanksSecretsOnTheWayOut(t *testing.T) {
	d := &meta.DocType{Name: "Integration", Fields: []*meta.Field{
		{Fieldname: "name", Fieldtype: "Data"},
		{Fieldname: "token", Fieldtype: "Password"},
		{Fieldname: "api_secret", Fieldtype: "Data"},
		{Fieldname: "webhook_password", Fieldtype: "Data"},
		{Fieldname: "endpoint", Fieldtype: "Data"},
	}}
	doc := Doc{"name": "x", "token": "tok", "api_secret": "s", "webhook_password": "p", "endpoint": "https://x"}
	RedactPassword(d, doc)

	for _, f := range []string{"token", "api_secret", "webhook_password"} {
		if doc[f] != nil {
			t.Errorf("%s should have been redacted, got %v", f, doc[f])
		}
	}
	if doc["endpoint"] != "https://x" || doc["name"] != "x" {
		t.Errorf("non-secret fields must survive: %v", doc)
	}
}
