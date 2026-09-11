package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearMailEnv isola um teste das variáveis que a máquina de quem roda possa
// ter. Sem isso um `.env` do desenvolvedor entraria no teste.
func clearMailEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DDCORE_MAIL_TRANSPORT", "DDCORE_MAIL_FROM", "DDCORE_MAIL_METHOD",
		"DDCORE_SMTP_HOST", "DDCORE_SMTP_PORT", "DDCORE_SMTP_USERNAME",
		"DDCORE_SMTP_PASSWORD", "DDCORE_SMTP_TLS", "DDCORE_URL", "DDCORE_TRUST_PROXY",
		"DDCORE_DSN", "DDCORE_PORT", "DATABASE_URL", "PORT",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// site escreve um ddcore.json mínimo e devolve o diretório.
func site(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, Name), body)
	return dir
}

func TestLoadDefaults(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{"site":"x"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Auth.SessionDays != 30 || f.Auth.MinPasswordLength != 8 || f.Auth.MaxLoginAttempts != 5 {
		t.Errorf("política padrão não aplicada: %+v", f.Auth)
	}
	if f.Mail.Transport != MailLog {
		t.Errorf("sem configuração, o transporte é o log, veio %q", f.Mail.Transport)
	}
	if !f.Auth.AllowSelfServiceAPIKeys() {
		t.Error("autosserviço de chaves é permitido por padrão")
	}
	if f.HasPublicURL() {
		t.Error("nenhuma URL pública foi declarada")
	}
	if got := f.PublicURL(); !strings.HasPrefix(got, "http://localhost:") {
		t.Errorf("fallback de URL: veio %q", got)
	}
	if f.Auth.SessionTTL().Hours() != 24*30 {
		t.Errorf("SessionTTL: veio %v", f.Auth.SessionTTL())
	}
}

// O bloco auth do ddcore.json sobrepõe só o que declara — o resto continua no
// padrão, senão declarar uma chave apagaria as outras.
func TestAuthPolicyMergesOverDefaults(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{"auth":{"sessionDays":7,"maxLoginAttempts":3}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Auth.SessionDays != 7 || f.Auth.MaxLoginAttempts != 3 {
		t.Errorf("o declarado não venceu: %+v", f.Auth)
	}
	if f.Auth.MinPasswordLength != 8 || f.Auth.InviteHours != 72 {
		t.Errorf("o não declarado devia ficar no padrão: %+v", f.Auth)
	}
}

func TestAuthPolicyRefusesNonsense(t *testing.T) {
	for _, tc := range []struct{ name, json string }{
		{"sessionDays zero", `{"auth":{"sessionDays":0}}`},
		{"sessionDays negativo", `{"auth":{"sessionDays":-1}}`},
		{"maxLoginAttempts zero", `{"auth":{"maxLoginAttempts":0}}`},
		{"lockoutMinutes zero", `{"auth":{"lockoutMinutes":0}}`},
		{"apiKeyDays negativo", `{"auth":{"apiKeyDays":-1}}`},
		{"senha curta demais", `{"auth":{"minPasswordLength":4}}`},
		{"senha longa demais", `{"auth":{"minPasswordLength":500}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearMailEnv(t)
			if _, _, err := Load(site(t, tc.json)); err == nil {
				t.Fatal("esperava erro de carga, veio nil")
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
		t.Errorf("configuração de e-mail não veio do ambiente: %+v", f.Mail)
	}
	if f.Mail.TLS != TLSStartTLS {
		t.Errorf("TLS padrão é starttls, veio %q", f.Mail.TLS)
	}
}

func TestMailRefusesIncompleteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"transporte desconhecido", map[string]string{"DDCORE_MAIL_TRANSPORT": "carta"}},
		{"smtp sem host", map[string]string{"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_MAIL_FROM": "a@b.c"}},
		{"smtp sem remetente", map[string]string{"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h"}},
		{"usuário sem senha", map[string]string{
			"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h",
			"DDCORE_MAIL_FROM": "a@b.c", "DDCORE_SMTP_USERNAME": "u",
		}},
		{"TLS desconhecido", map[string]string{
			"DDCORE_MAIL_TRANSPORT": "smtp", "DDCORE_SMTP_HOST": "h",
			"DDCORE_MAIL_FROM": "a@b.c", "DDCORE_SMTP_TLS": "ssl3",
		}},
		{"porta inválida", map[string]string{"DDCORE_SMTP_PORT": "zero"}},
		{"method sem caminho", map[string]string{"DDCORE_MAIL_TRANSPORT": "method"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearMailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, _, err := Load(site(t, `{}`)); err == nil {
				t.Fatal("esperava erro de carga, veio nil")
			}
		})
	}
}

// A precedência inteira, num teste só: ambiente real > .env > ddcore.json.
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
		t.Errorf("o ambiente real devia vencer tudo, veio %q", f.URL)
	}
	if f.Port != 2222 {
		t.Errorf("o .env devia vencer o ddcore.json, veio %d", f.Port)
	}
	if !f.TrustProxy {
		t.Error("trustProxy devia ter vindo do .env")
	}
}

// Uma URL com barra no fim viraria links com barra dupla.
func TestPublicURLLosesTheTrailingSlash(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_URL", "https://erp.example.com/")
	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.PublicURL() != "https://erp.example.com" {
		t.Errorf("veio %q", f.PublicURL())
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

