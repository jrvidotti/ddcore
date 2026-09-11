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
	b, err := s.render(Message{To: []string{"ana@x.com"}, Subject: "Oi", Text: "linha 1\nlinha 2"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)

	for _, want := range []string{
		"From: ddcore <no-reply@x.com>\r\n",
		"To: ana@x.com\r\n",
		"Subject: Oi\r\n",
		"MIME-Version: 1.0\r\n",
		`Content-Type: text/plain; charset="utf-8"`,
		"linha 1\r\nlinha 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("faltou %q em:\n%s", want, out)
		}
	}
	// headers e corpo separados por uma linha em branco
	if !strings.Contains(out, "\r\n\r\n") {
		t.Error("faltou a linha em branco entre headers e corpo")
	}
	// nenhum \n solto: SMTP exige CRLF
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("sobrou um \\n sem \\r")
	}
}

func TestRenderMultipart(t *testing.T) {
	s := testSender("no-reply@x.com")
	b, err := s.render(Message{To: []string{"a@x.com"}, Subject: "S", Text: "texto", HTML: "<p>html</p>"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "multipart/alternative; boundary=") {
		t.Fatal("esperava multipart/alternative")
	}
	i := strings.Index(out, `boundary="`)
	boundary := out[i+10 : i+10+strings.Index(out[i+10:], `"`)]
	if strings.Count(out, "--"+boundary) != 3 { // duas partes + o fechamento
		t.Errorf("esperava duas partes e o fechamento, veio %d marcadores", strings.Count(out, "--"+boundary))
	}
	if !strings.HasSuffix(out, "--"+boundary+"--\r\n") {
		t.Error("a mensagem tem de terminar no boundary de fechamento")
	}
}

// Um ponto no começo da linha encerraria o DATA e truncaria a mensagem.
func TestDotStuffing(t *testing.T) {
	for in, want := range map[string]string{
		".oculto": "..oculto",
		"a\n.b":   "a\r\n..b",
		"normal":  "normal",
		"a\r\nb":  "a\r\nb",
		"fim.":    "fim.",
		"a\n.\nb": "a\r\n..\r\nb",
	} {
		if got := dotStuff(in); got != want {
			t.Errorf("dotStuff(%q) = %q, esperava %q", in, got, want)
		}
	}
}

// Um assunto com acento não pode chegar como mojibake.
func TestHeaderEncoding(t *testing.T) {
	if got := encodeHeader("Redefina sua senha"); got != "Redefina sua senha" {
		t.Errorf("ASCII puro não devia ser codificado: %q", got)
	}
	got := encodeHeader("Redefinição de senha")
	if !strings.HasPrefix(got, "=?utf-8?") {
		t.Errorf("esperava RFC 2047, veio %q", got)
	}
}

func TestAddressOf(t *testing.T) {
	for in, want := range map[string]string{
		"ddcore <no-reply@x.com>": "no-reply@x.com",
		"no-reply@x.com":          "no-reply@x.com",
		"  a@b.c  ":               "a@b.c",
	} {
		if got := addressOf(in); got != want {
			t.Errorf("addressOf(%q) = %q, esperava %q", in, got, want)
		}
	}
}

// Mandar a senha do SMTP em claro para um relay remoto é erro de configuração,
// não preferência — falhar alto é mais barato que descobrir num tcpdump.
func TestRefusesCleartextCredentialsToARemoteHost(t *testing.T) {
	s := &smtpSender{cfg: config.Mail{
		From: "a@b.c", Host: "smtp.remoto.com", Port: 25,
		TLS: config.TLSNone, Username: "u", Password: "p",
	}, log: slog.Default()}

	err := s.Send(context.Background(), Message{To: []string{"x@y.z"}, Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "in the clear") {
		t.Fatalf("esperava recusa de credencial em claro, veio %v", err)
	}
}

// Em loopback é um relay local de desenvolvimento: aí não há rede para espiar.
func TestAllowsCleartextToLoopback(t *testing.T) {
	s := &smtpSender{cfg: config.Mail{
		From: "a@b.c", Host: "127.0.0.1", Port: 1, TLS: config.TLSNone, Username: "u", Password: "p",
	}, log: slog.Default()}

	err := s.Send(context.Background(), Message{To: []string{"x@y.z"}, Subject: "s", Text: "t"})
	if err != nil && strings.Contains(err.Error(), "in the clear") {
		t.Fatal("loopback não devia ser recusado por credencial em claro")
	}
}

func TestLogTransportDoesNotClaimDelivery(t *testing.T) {
	s, err := New(config.Mail{Transport: config.MailLog}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if s.Delivers() {
		t.Error("o transporte de log não entrega nada e não pode dizer que entrega")
	}
	if err := s.Send(context.Background(), Message{To: []string{"a@b.c"}, Subject: "s", Text: "t"}); err != nil {
		t.Errorf("o transporte de log não falha: %v", err)
	}
}
