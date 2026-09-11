package engine

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Recovery is what a caller learns after asking for a link. Link is filled in
// only when the mail transport does not actually deliver — in development, or
// for an operator running `ddcore user invite` — so that nobody has to read a
// log file to finish the flow. When mail is really being sent, the link stays
// empty: the person who asked is not necessarily the person who owns the
// account, and handing it back would make the whole token pointless.
type Recovery struct {
	Link      string
	Expires   time.Time
	Delivered bool
}

// ResetLink builds the address a person clicks.
func (e *Engine) ResetLink(token string) string {
	return strings.TrimSuffix(e.Cfg.SiteURL, "/") + "/login/reset?token=" + url.QueryEscape(token)
}

// StartRecovery issues a token and queues the message. kind is TokenReset or
// TokenInvite.
//
// It does not check whether the user exists or is enabled — the caller does,
// because only the caller knows whether saying so is safe. A public
// forgot-password endpoint must answer identically either way; an admin
// pressing "send invitation" should be told plainly.
func (e *Engine) StartRecovery(c *Ctx, user, kind, ip string) (*Recovery, error) {
	ttl := e.Cfg.Auth.ResetTTL()
	if kind == TokenInvite {
		ttl = e.Cfg.Auth.InviteTTL()
	}
	token, expires, err := e.IssueToken(c.Ctx, user, kind, ttl, c.User, ip)
	if err != nil {
		return nil, err
	}
	link := e.ResetLink(token)

	template := MailTemplateReset
	if kind == TokenInvite {
		template = MailTemplateInvite
	}
	if err := c.SendTemplate(template, user, map[string]any{"link": link}); err != nil {
		return nil, err
	}

	r := &Recovery{Expires: expires, Delivered: e.MailDelivers()}
	if !r.Delivered {
		r.Link = link
	}
	return r, nil
}

// CompleteRecovery spends a token and sets the password.
//
// Everything here is one transaction on purpose: the token is marked used, the
// password is written, and every session of that user is ended. All sessions,
// not all-but-one — there is no caller to spare, and the reason someone is
// resetting a password may be that the account is not theirs any more.
func (e *Engine) CompleteRecovery(ctx context.Context, token, kind, password, fullName string) (string, error) {
	var user string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		at, err := e.ConsumeToken(ctx, c.Tx, token, kind)
		if err != nil {
			return err
		}
		user = at.User

		rows, err := db.Select(ctx, c.Tx, `SELECT enabled FROM tab_user WHERE name = $1`, user)
		if err != nil {
			return err
		}
		// A user disabled between asking and answering must not get back in
		// through a link that was valid when it was issued.
		if len(rows) == 0 {
			return cerr.Validation("This link is no longer valid. Ask for a new one.")
		}
		if en, ok := rows[0]["enabled"].(bool); ok && !en {
			return cerr.Auth("User is disabled")
		}

		hash, err := e.HashNewPassword(user, password)
		if err != nil {
			return err
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE tab_user SET password_hash = $2 WHERE name = $1`, user, hash); err != nil {
			return err
		}
		if fullName = strings.TrimSpace(fullName); fullName != "" && kind == TokenInvite {
			if _, err := c.Tx.Exec(ctx, `UPDATE tab_user SET full_name = $2 WHERE name = $1`, user, fullName); err != nil {
				return err
			}
		}
		if _, err := e.DropSessions(ctx, c.Tx, user, ""); err != nil {
			return err
		}
		// Any other outstanding link of this kind dies with it, including one
		// an attacker may have requested while the account was in play.
		if _, err := e.RevokeTokens(ctx, c.Tx, user, kind); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	// Outside the transaction: the lockout counter lives on the pool, and a
	// person who just proved they own the account should not still be serving
	// one out.
	e.ClearAttempts(ctx, throttleKey("login", user))
	return user, nil
}

// FindUserForRecovery resolves what someone typed into a user name, and
// reports whether a link may be sent to it. It never reports *why* not.
func (e *Engine) FindUserForRecovery(ctx context.Context, typed string) (string, bool) {
	rows, err := db.Select(ctx, e.DB.Pool,
		`SELECT name, enabled FROM tab_user
		 WHERE lower(name) = lower($1) OR lower(email) = lower($1) LIMIT 1`, strings.TrimSpace(typed))
	if err != nil || len(rows) == 0 {
		return "", false
	}
	if en, ok := rows[0]["enabled"].(bool); ok && !en {
		return "", false
	}
	name := db.Str(rows[0]["name"])
	// Guest is a real row and must never be recoverable into.
	if strings.EqualFold(name, "Guest") {
		return "", false
	}
	return name, true
}
