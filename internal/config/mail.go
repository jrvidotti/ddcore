package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Mail transports.
const (
	MailLog    = "log"    // write the message, link and all, to the log
	MailSMTP   = "smtp"   // talk to a relay
	MailMethod = "method" // hand the message to an app's own function
)

// Mail is where a message goes when the framework or an app has one to send.
// Delivery is not a site decision, it is a deployment one — the relay, the
// credentials and the public address differ on every machine and one of them is
// a secret — so all of it is read from the environment (.env or the real
// environment) and none of it belongs in the versioned ddcore.json.
type Mail struct {
	Transport string
	From      string
	Method    string // dotted path to an app function, for Transport == method
	Host      string
	Port      int
	Username  string
	Password  string
	TLS       string // starttls | tls | none

	// MaxAttachment caps the total bytes attached to one message. The check
	// happens when the message is queued, from File.file_size, so an oversized
	// attachment fails the app's own transaction instead of failing alone in a
	// worker half an hour later — and so that a relay's own limit is not the
	// first thing to find out about it.
	MaxAttachment int64

	// Debug redirects all outgoing mail in development mode to this address
	// with a plus tag identifying the original recipient.
	Debug string

	// Dev indicates whether the application is running in development mode.
	Dev bool
}

// DefaultMaxAttachment is deliberately well under what relays usually accept:
// the bytes are held in memory by the worker and base64 adds a third on top.
const DefaultMaxAttachment = 10 << 20 // 10 MiB

// TLS modes.
const (
	TLSStartTLS = "starttls"
	TLSImplicit = "tls"
	TLSNone     = "none"
)

func mailFromEnv() (Mail, error) {
	m := Mail{
		Transport: strings.ToLower(env("DDCORE_MAIL_TRANSPORT", MailLog)),
		From:      env("DDCORE_MAIL_FROM", ""),
		Method:    env("DDCORE_MAIL_METHOD", ""),
		Host:      env("DDCORE_SMTP_HOST", ""),
		Username:  env("DDCORE_SMTP_USERNAME", ""),
		Password:  env("DDCORE_SMTP_PASSWORD", ""),
		TLS:       strings.ToLower(env("DDCORE_SMTP_TLS", TLSStartTLS)),
	}
	size := env("DDCORE_MAIL_MAX_ATTACHMENT", "")
	m.MaxAttachment = DefaultMaxAttachment
	if size != "" {
		n, err := strconv.ParseInt(size, 10, 64)
		if err != nil || n <= 0 {
			return m, fmt.Errorf("DDCORE_MAIL_MAX_ATTACHMENT: %q is not a number of bytes", size)
		}
		m.MaxAttachment = n
	}

	port := env("DDCORE_SMTP_PORT", "587")
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return m, fmt.Errorf("DDCORE_SMTP_PORT: %q is not a port", port)
	}
	m.Port = n

	switch m.Transport {
	case MailLog:
	case MailSMTP:
		if m.Host == "" {
			return m, fmt.Errorf("DDCORE_MAIL_TRANSPORT=smtp needs DDCORE_SMTP_HOST")
		}
		if m.From == "" {
			return m, fmt.Errorf("DDCORE_MAIL_TRANSPORT=smtp needs DDCORE_MAIL_FROM")
		}
		// A username with no password is the shape of a secret that did not
		// make it into the environment — far more likely than a relay that
		// authenticates on the name alone. Refuse it here, where the message
		// can say so, rather than at the first send.
		if m.Username != "" && m.Password == "" {
			return m, fmt.Errorf("DDCORE_SMTP_USERNAME is set but DDCORE_SMTP_PASSWORD is empty")
		}
		switch m.TLS {
		case TLSStartTLS, TLSImplicit, TLSNone:
		default:
			return m, fmt.Errorf("DDCORE_SMTP_TLS: %q is not starttls, tls or none", m.TLS)
		}
	case MailMethod:
		if m.Method == "" {
			return m, fmt.Errorf("DDCORE_MAIL_TRANSPORT=method needs DDCORE_MAIL_METHOD")
		}
	default:
		return m, fmt.Errorf("DDCORE_MAIL_TRANSPORT: %q is not log, smtp or method", m.Transport)
	}

	debug := strings.Trim(strings.TrimSpace(env("DDCORE_MAIL_DEBUG", "")), "{}")
	m.Debug = debug
	if m.Debug != "" {
		user, domain, ok := strings.Cut(m.Debug, "@")
		if !ok || user == "" || domain == "" || !strings.Contains(domain, ".") {
			return m, fmt.Errorf("DDCORE_MAIL_DEBUG: %q is not a valid email address", debug)
		}
	}

	return m, nil
}

// AddressOf extracts the bare email address from a string like "Name <user@domain.com>".
func AddressOf(s string) string {
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j > 0 {
			return strings.TrimSpace(s[i+1 : i+j])
		}
	}
	return strings.TrimSpace(s)
}

// RedirectDebugEmail rewrites a recipient email address to route to a debug email
// using plus-tag subaddressing.
//
// For example:
//
//	recipient: "locatario@teste.com"
//	debugEmail: "admin@email.com" (or "{admin@email.com}")
//	output: "admin+locatario_teste_com@email.com"
func RedirectDebugEmail(recipient, debugEmail string) string {
	debugEmail = strings.Trim(strings.TrimSpace(debugEmail), "{}")
	user, domain, ok := strings.Cut(debugEmail, "@")
	if !ok || user == "" || domain == "" {
		return recipient
	}

	bare := AddressOf(recipient)
	if bare == "" {
		return debugEmail
	}

	var b strings.Builder
	for _, r := range bare {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	tag := strings.Trim(b.String(), "_")
	if tag == "" {
		return debugEmail
	}
	return fmt.Sprintf("%s+%s@%s", user, tag, domain)
}

