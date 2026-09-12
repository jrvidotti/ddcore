package mail

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
)

func testSender(from string) *smtpSender {
	return &smtpSender{cfg: config.Mail{From: from, Host: "smtp.x.com", Port: 587, TLS: config.TLSStartTLS}}
}

func TestRenderPlainMessage(t *testing.T) {
	s := testSender("ddcore <no-reply@x.com>")
	b, err := s.render(Message{To: []string{"ana@x.com"}, Subject: "Hello", Text: "line 1\nline 2"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)

	for _, want := range []string{
		"From: ddcore <no-reply@x.com>\r\n",
		"To: ana@x.com\r\n",
		"Subject: Hello\r\n",
		"MIME-Version: 1.0\r\n",
		`Content-Type: text/plain; charset="utf-8"`,
		"line 1\r\nline 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// headers and body separated by a blank line
	if !strings.Contains(out, "\r\n\r\n") {
		t.Error("missing blank line between headers and body")
	}
	// no bare \n: SMTP requires CRLF
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("leftover \\n without \\r")
	}
}

func TestRenderMultipart(t *testing.T) {
	s := testSender("no-reply@x.com")
	b, err := s.render(Message{To: []string{"a@x.com"}, Subject: "S", Text: "text", HTML: "<p>html</p>"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "multipart/alternative; boundary=") {
		t.Fatal("expected multipart/alternative")
	}
	i := strings.Index(out, `boundary="`)
	boundary := out[i+10 : i+10+strings.Index(out[i+10:], `"`)]
	if strings.Count(out, "--"+boundary) != 3 { // two parts + closing boundary
		t.Errorf("expected two parts and closing boundary, got %d markers", strings.Count(out, "--"+boundary))
	}
	if !strings.HasSuffix(out, "--"+boundary+"--\r\n") {
		t.Error("message must end with closing boundary")
	}
}

// A dot at the start of a line would terminate DATA and truncate the message.
func TestDotStuffing(t *testing.T) {
	for in, want := range map[string]string{
		".hidden": "..hidden",
		"a\n.b":   "a\r\n..b",
		"normal":  "normal",
		"a\r\nb":  "a\r\nb",
		"end.":    "end.",
		"a\n.\nb": "a\r\n..\r\nb",
	} {
		if got := dotStuff(in); got != want {
			t.Errorf("dotStuff(%q) = %q, expected %q", in, got, want)
		}
	}
}

// An accented subject must not arrive as mojibake.
func TestHeaderEncoding(t *testing.T) {
	if got := encodeHeader("Reset your password"); got != "Reset your password" {
		t.Errorf("pure ASCII should not be encoded: %q", got)
	}
	got := encodeHeader("Redefinição de senha")
	if !strings.HasPrefix(got, "=?utf-8?") {
		t.Errorf("expected RFC 2047, got %q", got)
	}
}

func TestAddressOf(t *testing.T) {
	for in, want := range map[string]string{
		"ddcore <no-reply@x.com>": "no-reply@x.com",
		"no-reply@x.com":          "no-reply@x.com",
		"  a@b.c  ":               "a@b.c",
	} {
		if got := addressOf(in); got != want {
			t.Errorf("addressOf(%q) = %q, expected %q", in, got, want)
		}
	}
}

// Sending SMTP credentials in cleartext to a remote relay is a configuration error,
// not a preference — failing loudly is cheaper than finding out in a tcpdump.
func TestRefusesCleartextCredentialsToARemoteHost(t *testing.T) {
	s := &smtpSender{cfg: config.Mail{
		From: "a@b.c", Host: "smtp.remote.com", Port: 25,
		TLS: config.TLSNone, Username: "u", Password: "p",
	}, log: slog.Default()}

	err := s.Send(context.Background(), Message{To: []string{"x@y.z"}, Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "in the clear") {
		t.Fatalf("expected cleartext credential refusal, got %v", err)
	}
}

// On loopback it is a local development relay: there is no network to snoop.
func TestAllowsCleartextToLoopback(t *testing.T) {
	s := &smtpSender{cfg: config.Mail{
		From: "a@b.c", Host: "127.0.0.1", Port: 1, TLS: config.TLSNone, Username: "u", Password: "p",
	}, log: slog.Default()}

	err := s.Send(context.Background(), Message{To: []string{"x@y.z"}, Subject: "s", Text: "t"})
	if err != nil && strings.Contains(err.Error(), "in the clear") {
		t.Fatal("loopback should not be rejected for cleartext credentials")
	}
}

func TestLogTransportDoesNotClaimDelivery(t *testing.T) {
	s, err := New(config.Mail{Transport: config.MailLog}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if s.Delivers() {
		t.Error("log transport does not deliver anything and must not report that it delivers")
	}
	if err := s.Send(context.Background(), Message{To: []string{"a@b.c"}, Subject: "s", Text: "t"}); err != nil {
		t.Errorf("log transport should not fail: %v", err)
	}
}

func TestDebugRedirect(t *testing.T) {
	t.Run("redirects in dev mode when debug is set", func(t *testing.T) {
		s := &smtpSender{
			cfg: config.Mail{
				From:  "no-reply@x.com",
				Dev:   true,
				Debug: "admin@email.com",
			},
			log: slog.Default(),
		}

		in := Message{
			To:      []string{"locatario@teste.com"},
			Subject: "Contrato",
			Text:    "texto do contrato",
		}
		prepared := s.applyDebugRedirect(in)
		if len(prepared.To) != 1 || prepared.To[0] != "admin+locatario_teste_com@email.com" {
			t.Fatalf("expected redirected to, got %v", prepared.To)
		}
		if len(prepared.OriginalTo) != 1 || prepared.OriginalTo[0] != "locatario@teste.com" {
			t.Fatalf("expected original to preserved, got %v", prepared.OriginalTo)
		}

		b, err := s.render(prepared)
		if err != nil {
			t.Fatal(err)
		}
		out := string(b)
		if !strings.Contains(out, "To: admin+locatario_teste_com@email.com\r\n") {
			t.Errorf("rendered message missing redirected To header: %s", out)
		}
		if !strings.Contains(out, "X-Original-To: locatario@teste.com\r\n") {
			t.Errorf("rendered message missing X-Original-To header: %s", out)
		}
	})

	t.Run("does not redirect when not in dev mode", func(t *testing.T) {
		s := &smtpSender{
			cfg: config.Mail{
				From:  "no-reply@x.com",
				Dev:   false,
				Debug: "admin@email.com",
			},
			log: slog.Default(),
		}

		in := Message{
			To:      []string{"locatario@teste.com"},
			Subject: "Contrato",
			Text:    "texto do contrato",
		}
		prepared := s.applyDebugRedirect(in)
		if len(prepared.To) != 1 || prepared.To[0] != "locatario@teste.com" {
			t.Fatalf("expected original to preserved in prod, got %v", prepared.To)
		}
		if len(prepared.OriginalTo) != 0 {
			t.Fatalf("expected no OriginalTo in prod, got %v", prepared.OriginalTo)
		}

		b, err := s.render(prepared)
		if err != nil {
			t.Fatal(err)
		}
		out := string(b)
		if strings.Contains(out, "X-Original-To") {
			t.Errorf("rendered message should not have X-Original-To in prod: %s", out)
		}
	})

	t.Run("does not redirect when debug email is empty", func(t *testing.T) {
		s := &smtpSender{
			cfg: config.Mail{
				From:  "no-reply@x.com",
				Dev:   true,
				Debug: "",
			},
			log: slog.Default(),
		}

		in := Message{
			To:      []string{"locatario@teste.com"},
			Subject: "Contrato",
			Text:    "texto do contrato",
		}
		prepared := s.applyDebugRedirect(in)
		if len(prepared.To) != 1 || prepared.To[0] != "locatario@teste.com" {
			t.Fatalf("expected original to preserved, got %v", prepared.To)
		}

		b, err := s.render(prepared)
		if err != nil {
			t.Fatal(err)
		}
		out := string(b)
		if strings.Contains(out, "X-Original-To") {
			t.Errorf("rendered message should not have X-Original-To: %s", out)
		}
	})
}

