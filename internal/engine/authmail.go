package engine

import (
	"context"
	"strings"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/mail"
)

// Mailer is the configured transport, built once.
func (e *Engine) Mailer() mail.Sender {
	e.mailOnce.Do(func() {
		s, err := mail.New(e.Cfg.Mail, e.Log)
		if err != nil {
			e.Log.Error("mail transport is unusable; falling back to the log", "err", err)
			s, _ = mail.New(config.Mail{Transport: config.MailLog}, e.Log)
		}
		e.mailer = s
	})
	return e.mailer
}

// MailDelivers reports whether a message actually goes anywhere. When it does
// not — the log transport, which is the default — the caller hands the link
// back to whoever asked, so development and `ddcore user invite` still work
// without anyone reading a log file.
func (e *Engine) MailDelivers() bool {
	if e.Cfg.Mail.Transport == config.MailMethod {
		return true
	}
	return e.Mailer().Delivers()
}

// SendMail queues a message. It goes through ddcore_job rather than out the
// socket here, which buys durable retries for free and, because Enqueue writes
// on the request's transaction, means the mail is only queued if the request
// commits. A recovery e-mail for a password change that rolled back would be
// worse than none.
func (e *Engine) SendMail(c *Ctx, m mail.Message) error {
	_, err := c.Enqueue("core.services.mail.send", map[string]any{
		"to": m.To, "subject": m.Subject, "text": m.Text, "html": m.HTML,
	}, nil)
	return err
}

// deliver is what the job ends up calling, through the sendMail host op.
//
// The "method" transport never reaches here: core/services/mail.ts checks for
// a configured dotted path first and calls the app's own function through
// ddcore.callMethod. Routing it in TS rather than Go means the pluggable
// transport needs no new mechanism at all — a dotted path is exactly how a
// scheduler entry and ddcore.enqueue already name app code.
func (e *Engine) deliver(ctx context.Context, m mail.Message) error {
	return e.Mailer().Send(ctx, m)
}

// authMail builds the two messages the framework sends. The body is the
// reader's, so it is translated into their language and not the language of
// whoever triggered it: an admin inviting someone should not decide which
// language that person reads.
// Every key below is written out at a literal `.T(…)` call rather than through
// a local helper. The extractor collects `.T(…)` by syntax, so a wrapper would
// hide these keys from the catalogue — and `make check` would report nothing
// missing while the e-mail went out in English.
func (e *Engine) authMail(kind, user, link string, lang string) mail.Message {
	i18n := e.Current().I18n
	site := e.Cfg.SiteName
	if site == "" {
		site = "ddcore"
	}

	var subject, intro, action string
	if kind == TokenInvite {
		subject = i18n.T(lang, "You have been invited to {0}", site)
		intro = i18n.T(lang, "An account was created for you on {0}. Choose a password to get in.", site)
		action = i18n.T(lang, "Set my password")
	} else {
		subject = i18n.T(lang, "Reset your password on {0}", site)
		intro = i18n.T(lang, "Someone asked to reset the password for this account on {0}. If it was not you, ignore this message: nothing has changed.", site)
		action = i18n.T(lang, "Choose a new password")
	}
	ignore := i18n.T(lang, "This link can only be used once.")

	text := strings.Join([]string{intro, "", link, "", ignore}, "\n")
	html := "<p>" + htmlEscape(intro) + "</p>" +
		`<p><a href="` + htmlEscape(link) + `">` + htmlEscape(action) + "</a></p>" +
		"<p>" + htmlEscape(ignore) + "</p>"

	return mail.Message{To: []string{user}, Subject: subject, Text: text, HTML: html}
}

// langOf is the language a given user reads, for a message nobody is currently
// requesting.
func (e *Engine) langOf(ctx context.Context, user string) string {
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT language FROM tab_user WHERE name = $1`, user)
	if err == nil && len(rows) > 0 {
		if l := db.Str(rows[0]["language"]); l != "" {
			return l
		}
	}
	return e.Cfg.Lang
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
