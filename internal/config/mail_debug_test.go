package config

import (
	"testing"
)

func TestRedirectDebugEmail(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recipient string
		debug     string
		want      string
	}{
		{
			name:      "simple email",
			recipient: "locatario@teste.com",
			debug:     "admin@email.com",
			want:      "admin+locatario_teste_com@email.com",
		},
		{
			name:      "formatted name with email",
			recipient: "Locatario Silva <locatario.silva@teste.com.br>",
			debug:     "admin@email.com",
			want:      "admin+locatario_silva_teste_com_br@email.com",
		},
		{
			name:      "recipient with plus tag",
			recipient: "locatario+urgente@teste.com",
			debug:     "admin@email.com",
			want:      "admin+locatario_urgente_teste_com@email.com",
		},
		{
			name:      "debug email already has plus tag",
			recipient: "locatario@teste.com",
			debug:     "admin+dev@email.com",
			want:      "admin+dev+locatario_teste_com@email.com",
		},
		{
			name:      "debug email with braces",
			recipient: "locatario@teste.com",
			debug:     "{admin@email.com}",
			want:      "admin+locatario_teste_com@email.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RedirectDebugEmail(tc.recipient, tc.debug)
			if got != tc.want {
				t.Errorf("RedirectDebugEmail(%q, %q) = %q, want %q", tc.recipient, tc.debug, got, tc.want)
			}
		})
	}
}

func TestMailDebugFromEnv(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_MAIL_TRANSPORT", "smtp")
	t.Setenv("DDCORE_MAIL_FROM", "ddcore <no-reply@x.com>")
	t.Setenv("DDCORE_SMTP_HOST", "smtp.x.com")
	t.Setenv("DDCORE_MAIL_DEBUG", "{admin@email.com}")

	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Mail.Debug != "admin@email.com" {
		t.Errorf("expected Mail.Debug to be %q, got %q", "admin@email.com", f.Mail.Debug)
	}
}

func TestMailDebugInvalidFormat(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_MAIL_TRANSPORT", "smtp")
	t.Setenv("DDCORE_MAIL_FROM", "ddcore <no-reply@x.com>")
	t.Setenv("DDCORE_SMTP_HOST", "smtp.x.com")
	t.Setenv("DDCORE_MAIL_DEBUG", "invalid-email-without-at")

	if _, _, err := Load(site(t, `{}`)); err == nil {
		t.Fatal("expected error for invalid DDCORE_MAIL_DEBUG, got nil")
	}
}
