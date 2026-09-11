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
			t.Errorf("%q: esperava %q, veio %q", in, want, got)
		}
	}
}

// Um segredo vem do ambiente e de nenhum outro lugar; o prefixo é a fronteira
// que impede um app de ler o DSN ou a senha do SMTP pelo mesmo caminho.
func TestSecretReadsOnlyItsOwnPrefix(t *testing.T) {
	e := &Engine{}
	t.Setenv("DDCORE_SECRET_STRIPE_KEY", "sk_live_x")
	t.Setenv("DDCORE_DSN", "postgres://nao-devia-vazar")

	if v, ok := e.Secret("stripe_key"); !ok || v != "sk_live_x" {
		t.Errorf("esperava o segredo, veio %q (%v)", v, ok)
	}
	if _, ok := e.Secret("DDCORE_DSN"); ok {
		t.Error("o prefixo devia impedir a leitura de uma variável que não é segredo de app")
	}
	if _, ok := e.Secret("nao_configurado"); ok {
		t.Error("um segredo ausente não pode parecer configurado")
	}
}

// A mensagem nomeia a variável para quem tem de configurá-la, e nunca o valor:
// ela acaba em Error Log e em toast.
func TestRequireSecretNamesTheVariableNotTheValue(t *testing.T) {
	e := &Engine{}
	t.Setenv("DDCORE_SECRET_PRESENTE", "valor-secreto")

	if _, err := e.RequireSecret("ausente"); err == nil {
		t.Fatal("esperava erro para segredo ausente")
	} else if msg := err.Error(); !strings.Contains(msg, "DDCORE_SECRET_AUSENTE") {
		t.Errorf("a mensagem devia nomear a variável, veio %q", msg)
	}
	v, err := e.RequireSecret("presente")
	if err != nil || v != "valor-secreto" {
		t.Errorf("esperava o valor, veio %q / %v", v, err)
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
			t.Errorf("SecretNames devolveu um valor: %q", n)
		}
	}
	if !seen["UM"] || !seen["DOIS"] {
		t.Errorf("esperava UM e DOIS, veio %v", names)
	}
}

// Um campo Password ainda é texto no banco — mas não sai mais por leitura.
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
			t.Errorf("%s devia ter sido apagado, veio %v", f, doc[f])
		}
	}
	if doc["endpoint"] != "https://x" || doc["name"] != "x" {
		t.Errorf("o que não é segredo tem de sobreviver: %v", doc)
	}
}
