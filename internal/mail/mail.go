// Package mail delivers the few messages the framework itself sends: a
// password recovery link and an invitation. It is not an e-mail framework —
// no templates, no queue of its own, no delivery log. Durable retries come
// from ddcore_job, which the caller enqueues onto, and anything richer belongs
// to OPS-02 rather than here.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
)

// Message is one outgoing e-mail.
type Message struct {
	To          []string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
	OriginalTo  []string
}

// Attachment is a file already read into memory. The bytes are resolved from a
// File document at send time and never stored with the delivery record, so an
// attachment is authorized once, when the message is queued, and read once,
// when it goes out.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// ErrUncertain marks the one outcome a sender cannot resolve: the connection
// failed while waiting for the answer to the final dot, so the relay may or may
// not have accepted the message. Retrying is how the same message is delivered
// twice, so a caller is expected to record this rather than try again.
var ErrUncertain = errors.New("delivery outcome is uncertain")

// Sender is how a message leaves. The method transport has no Sender: it is
// dispatched by the engine into app code, because only the runtime can call
// into an app.
type Sender interface {
	Send(ctx context.Context, m Message) error
	// Delivers reports whether this transport actually sends anything. When
	// it does not, the caller hands the link back to whoever asked for it, so
	// development and a CLI invite still work.
	Delivers() bool
}

// New builds the configured transport.
func New(cfg config.Mail, log *slog.Logger) (Sender, error) {
	switch cfg.Transport {
	case config.MailSMTP:
		return &smtpSender{cfg: cfg, log: log}, nil
	default:
		// Both "log" and "method" land here. A method transport is dispatched
		// by the engine before it ever reaches a Sender, so this is only the
		// fallback if that dispatch is not wired.
		return &logSender{log: log}, nil
	}
}

type logSender struct{ log *slog.Logger }

func (l *logSender) Delivers() bool { return false }

func (l *logSender) Send(_ context.Context, m Message) error {
	// The body goes to the log in full, link included — that is the whole
	// point of this transport in development.
	l.log.Info("mail (not delivered: DDCORE_MAIL_TRANSPORT is log)",
		"to", strings.Join(m.To, ", "), "subject", m.Subject, "body", m.Text)
	return nil
}

type smtpSender struct {
	cfg config.Mail
	log *slog.Logger
}

func (s *smtpSender) Delivers() bool { return true }

func (s *smtpSender) applyDebugRedirect(m Message) Message {
	if !s.cfg.Dev || s.cfg.Debug == "" {
		return m
	}
	out := m
	out.OriginalTo = make([]string, len(m.To))
	copy(out.OriginalTo, m.To)
	out.To = make([]string, len(m.To))
	for i, to := range m.To {
		out.To[i] = config.RedirectDebugEmail(to, s.cfg.Debug)
	}
	if s.log != nil {
		s.log.Info("mail: debug redirect",
			"original_to", strings.Join(out.OriginalTo, ", "),
			"debug_to", strings.Join(out.To, ", "))
	}
	return out
}

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	if len(m.To) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	m = s.applyDebugRedirect(m)
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	body, err := s.render(m)
	if err != nil {
		return err
	}

	// Refuse to authenticate in the clear to anything but loopback. An SMTP
	// password on a plaintext connection to a remote relay is a
	// misconfiguration, not a preference, and failing loudly here is cheaper
	// than discovering it in someone's packet capture.
	if s.cfg.Username != "" && s.cfg.TLS == config.TLSNone && !isLoopback(s.cfg.Host) {
		return fmt.Errorf("mail: refusing to send SMTP credentials in the clear to %s — set DDCORE_SMTP_TLS", s.cfg.Host)
	}

	conn, err := s.dial(ctx, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	cl, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return err
	}
	defer cl.Quit()

	if s.cfg.TLS == config.TLSStartTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return fmt.Errorf("mail: %s does not offer STARTTLS — set DDCORE_SMTP_TLS=none to accept that", s.cfg.Host)
		}
		if err := cl.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
			return err
		}
	}
	if s.cfg.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return err
		}
	}
	if err := cl.Mail(addressOf(s.cfg.From)); err != nil {
		return err
	}
	for _, to := range m.To {
		if err := cl.Rcpt(addressOf(to)); err != nil {
			return err
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	// Everything above this line fails cleanly: the relay never took the
	// message. The answer to the final dot is the only one whose absence is
	// ambiguous, so that is the only error promoted to ErrUncertain.
	if err := w.Close(); err != nil {
		return fmt.Errorf("%w: %v", ErrUncertain, err)
	}
	return nil
}

func (s *smtpSender) dial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 20 * time.Second}
	if s.cfg.TLS == config.TLSImplicit {
		return tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	}
	return d.DialContext(ctx, "tcp", addr)
}

// render builds the RFC 5322 message. Written by hand because the alternative
// is a dependency for what is a handful of headers and a boundary.
func (s *smtpSender) render(m Message) ([]byte, error) {
	var b strings.Builder
	w := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }

	w("From", encodeHeader(s.cfg.From))
	w("To", strings.Join(m.To, ", "))
	if len(m.OriginalTo) > 0 {
		w("X-Original-To", strings.Join(m.OriginalTo, ", "))
	}
	w("Subject", encodeHeader(m.Subject))
	w("Date", time.Now().Format(time.RFC1123Z))
	w("Message-ID", messageID(addressOf(s.cfg.From)))
	w("MIME-Version", "1.0")

	readable, body, err := readablePart(m)
	if err != nil {
		return nil, err
	}

	if len(m.Attachments) == 0 {
		w("Content-Type", readable)
		b.WriteString("\r\n")
		b.WriteString(body)
		return []byte(b.String()), nil
	}

	// With attachments the shape gains a layer: multipart/mixed holds the
	// readable part (itself possibly a multipart/alternative) and one part per
	// file. Nesting it this way is what keeps a plain-text reader seeing the
	// message rather than a wall of base64.
	mixed, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	w("Content-Type", `multipart/mixed; boundary="`+mixed+`"`)
	b.WriteString("\r\n")
	b.WriteString("--" + mixed + "\r\n")
	b.WriteString("Content-Type: " + readable + "\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	for _, a := range m.Attachments {
		b.WriteString("--" + mixed + "\r\n")
		b.WriteString(attachmentPart(a))
	}
	b.WriteString("--" + mixed + "--\r\n")
	return []byte(b.String()), nil
}

// readablePart builds what a person reads: one text part, or both parts of a
// multipart/alternative. It returns the Content-Type to announce it with.
func readablePart(m Message) (contentType, body string, err error) {
	if m.HTML == "" {
		return `text/plain; charset="utf-8"`, dotStuff(m.Text), nil
	}
	boundary, err := randomBoundary()
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	for _, part := range []struct{ typ, body string }{
		{`text/plain; charset="utf-8"`, m.Text},
		{`text/html; charset="utf-8"`, m.HTML},
	} {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + part.typ + "\r\n\r\n")
		b.WriteString(dotStuff(part.body))
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return `multipart/alternative; boundary="` + boundary + `"`, b.String(), nil
}

// attachmentPart encodes one file. base64 rather than any attempt at 8-bit: the
// content is arbitrary bytes, and a relay that mangles them fails silently.
func attachmentPart(a Attachment) string {
	typ := a.ContentType
	if typ == "" {
		typ = "application/octet-stream"
	}
	name := encodeHeader(a.Filename)
	var b strings.Builder
	b.WriteString("Content-Type: " + typ + "; name=\"" + name + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n\r\n")

	enc := base64.StdEncoding.EncodeToString(a.Content)
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.String()
}

// dotStuff normalises line endings to CRLF and escapes a leading dot, which
// would otherwise end the DATA command early and truncate the message.
func dotStuff(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	if strings.HasPrefix(s, ".") {
		s = "." + s
	}
	return strings.ReplaceAll(s, "\r\n.", "\r\n..")
}

// encodeHeader RFC 2047-encodes a header when it is not plain ASCII, so a
// subject with an accent does not arrive as mojibake.
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return mime.QEncoding.Encode("utf-8", s)
		}
	}
	return s
}

// addressOf pulls the bare address out of `Name <a@b.c>`.
func addressOf(s string) string {
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j > 0 {
			return strings.TrimSpace(s[i+1 : i+j])
		}
	}
	return strings.TrimSpace(s)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func messageID(from string) string {
	domain := "localhost"
	if _, d, ok := strings.Cut(from, "@"); ok && d != "" {
		domain = d
	}
	b := make([]byte, 12)
	rand.Read(b)
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}

func randomBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ddcore-" + hex.EncodeToString(b), nil
}
