package engine

import (
	"github.com/jrvidotti/ddcore/internal/config"
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
