package engine

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Single sign-on through OpenID Connect.
//
// The flow is the authorization code flow with PKCE, and every provider —
// Google, a self-hosted PocketID — goes through the same code: they differ only
// in their issuer. The provider proves who the person is; it never decides
// that they may use this site. A User must already exist with the verified
// address, so an account at the provider is not an account here.

// OIDCStateTTL is how long a person has between leaving for the provider and
// coming back.
const OIDCStateTTL = 10 * time.Minute

// Reasons a sign-in through a provider is refused. They are short codes rather
// than messages because the browser is redirected with them in the query
// string, and the desk turns each into a translated sentence.
const (
	OIDCErrState      = "state"
	OIDCErrProvider   = "provider"
	OIDCErrUnverified = "unverified_email"
	OIDCErrNoAccount  = "no_account"
	OIDCErrDisabled   = "disabled"
	OIDCErrDomain     = "domain"
	OIDCErrThrottled  = "throttled"
)

// OIDCError is a refused sign-in. Code is safe to show; Err is for the log.
type OIDCError struct {
	Code string
	Err  error
}

func (e *OIDCError) Error() string {
	if e.Err != nil {
		return "oidc " + e.Code + ": " + e.Err.Error()
	}
	return "oidc " + e.Code
}

func (e *OIDCError) Unwrap() error { return e.Err }

func oidcErr(code string, err error) *OIDCError { return &OIDCError{Code: code, Err: err} }

type oidcClient struct {
	key      string
	cfg      config.OIDCProvider
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

// oidcHTTP bounds every call to a provider. Discovery, keys and the code
// exchange all sit on a request a person is waiting for.
var oidcHTTP = &http.Client{Timeout: 10 * time.Second}

func oidcCtx(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, oidcHTTP)
}

// OIDCProvider returns the configured provider with this id.
func (e *Engine) OIDCProvider(id string) (config.OIDCProvider, bool) {
	for _, p := range e.Cfg.OIDC {
		if p.ID == id {
			return p, true
		}
	}
	return config.OIDCProvider{}, false
}

// OIDCCallbackURL is the redirect URI to register at the provider.
func (e *Engine) OIDCCallbackURL(id string) string {
	return strings.TrimSuffix(e.Cfg.SiteURL, "/") + "/api/auth/oidc/" + url.PathEscape(id) + "/callback"
}

// oidcClientFor discovers a provider once and keeps it. A failed discovery is
// not kept, so a provider that was down at boot is tried again on the next
// sign-in rather than staying broken until a restart.
func (e *Engine) oidcClientFor(ctx context.Context, id string) (*oidcClient, error) {
	p, ok := e.OIDCProvider(id)
	if !ok {
		return nil, oidcErr(OIDCErrProvider, errors.New("unknown provider "+id))
	}
	key := p.Issuer + "\x00" + p.ClientID + "\x00" + p.ClientSecret + "\x00" + strings.Join(p.Scopes, " ") + "\x00" + e.Cfg.SiteURL
	e.oidcMu.Lock()
	cl := e.oidc[id]
	e.oidcMu.Unlock()
	if cl != nil && cl.key == key {
		return cl, nil
	}
	prov, err := oidc.NewProvider(oidcCtx(ctx), p.Issuer)
	if err != nil {
		return nil, oidcErr(OIDCErrProvider, err)
	}
	cl = &oidcClient{
		key: key, cfg: p, provider: prov,
		verifier: prov.Verifier(&oidc.Config{ClientID: p.ClientID}),
		oauth: oauth2.Config{
			ClientID: p.ClientID, ClientSecret: p.ClientSecret,
			Endpoint: prov.Endpoint(), RedirectURL: e.OIDCCallbackURL(id), Scopes: p.Scopes,
		},
	}
	e.oidcMu.Lock()
	if e.oidc == nil {
		e.oidc = map[string]*oidcClient{}
	}
	e.oidc[id] = cl
	e.oidcMu.Unlock()
	return cl, nil
}

// OIDCDiscover checks that a provider answers, for `ddcore doctor`.
func (e *Engine) OIDCDiscover(ctx context.Context, id string) error {
	_, err := e.oidcClientFor(ctx, id)
	return err
}

// SafeRedirect keeps a post-sign-in destination on this site. Only a path is
// accepted: "//host" and "/\host" are addresses on another site to a browser,
// and a sign-in that ends somewhere else is a phishing page's best friend.
func SafeRedirect(to string) string {
	if to == "" || !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.HasPrefix(to, "/\\") ||
		strings.ContainsAny(to, "\r\n\t") {
		return "/app"
	}
	if u, err := url.Parse(to); err != nil || u.Scheme != "" || u.Host != "" {
		return "/app"
	}
	return to
}

// OIDCStart begins a sign-in: it records a state, a nonce and a PKCE verifier
// and returns the provider's address to send the browser to, with the state
// the caller must also bind to the browser.
func (e *Engine) OIDCStart(ctx context.Context, id, redirect, ip string) (authURL, state string, err error) {
	cl, err := e.oidcClientFor(ctx, id)
	if err != nil {
		return "", "", err
	}
	state, nonce, verifier := RandomToken(), RandomToken(), oauth2.GenerateVerifier()
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_oidc_state (state_hash, provider, nonce, verifier, redirect, ip, expires)
		VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)`,
		hashToken(state), id, nonce, verifier, SafeRedirect(redirect), ip, intervalOf(OIDCStateTTL)); err != nil {
		return "", "", err
	}
	return cl.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), state, nil
}

// OIDCCallback finishes a sign-in and returns the new session and where to
// send the browser. cookieState is the state the browser carried in its own
// cookie: the one in the query string alone proves nothing, since anyone can
// hand a victim a link carrying the attacker's state and code.
func (e *Engine) OIDCCallback(ctx context.Context, id, code, state, cookieState string, from LoginFrom) (sid, redirect string, err error) {
	redirect = "/app"
	ipKey := throttleKey("ssoip", from.IP)
	p := e.Cfg.Auth
	if from.IP != "" {
		if err := e.CheckThrottle(ctx, ipKey, p.MaxLoginAttempts*5, p.LockoutWindow()); err != nil {
			return "", redirect, oidcErr(OIDCErrThrottled, err)
		}
	}
	var user, email string
	defer func() {
		if from.IP != "" {
			e.RecordAttempt(ctx, ipKey, from.IP, err == nil)
		}
		detail := map[string]any{"provider": id}
		if email != "" {
			detail["email"] = email
		}
		outcome := "Allowed"
		var oe *OIDCError
		if errors.As(err, &oe) {
			outcome = "Denied"
			detail["reason"] = oe.Code
			if oe.Code == OIDCErrThrottled {
				return
			}
		} else if err != nil {
			outcome = "Denied"
			detail["reason"] = "error"
		}
		actor := user
		if actor == "" {
			actor = "Guest"
		}
		if aerr := e.RecordAuditOn(ctx, e.DB.Pool, actor, "account.login_sso", outcome, "User", user, from.IP, "", detail); aerr != nil {
			e.Log.Warn("could not record a sign-in", "err", aerr)
		}
		if err != nil {
			e.Log.Info("single sign-on refused", "provider", id, "err", err)
		}
	}()

	if state == "" || cookieState == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return "", redirect, oidcErr(OIDCErrState, errors.New("state does not match the browser's"))
	}
	// Spent before anything else can fail, so a state is good for one try.
	rows, err := db.Select(ctx, e.DB.Pool, `DELETE FROM ddcore_oidc_state WHERE state_hash = $1
		RETURNING provider, nonce, verifier, redirect, expires > now() AS live`, hashToken(state))
	if err != nil {
		return "", redirect, err
	}
	if len(rows) == 0 || rows[0]["live"] != true || db.Str(rows[0]["provider"]) != id {
		return "", redirect, oidcErr(OIDCErrState, errors.New("unknown, expired or foreign state"))
	}
	redirect = SafeRedirect(db.Str(rows[0]["redirect"]))
	cl, err := e.oidcClientFor(ctx, id)
	if err != nil {
		return "", redirect, err
	}
	octx := oidcCtx(ctx)
	tok, err := cl.oauth.Exchange(octx, code, oauth2.VerifierOption(db.Str(rows[0]["verifier"])))
	if err != nil {
		return "", redirect, oidcErr(OIDCErrProvider, err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return "", redirect, oidcErr(OIDCErrProvider, errors.New("no id_token in the token response"))
	}
	idt, err := cl.verifier.Verify(octx, raw)
	if err != nil {
		return "", redirect, oidcErr(OIDCErrProvider, err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(db.Str(rows[0]["nonce"]))) != 1 {
		return "", redirect, oidcErr(OIDCErrState, errors.New("nonce mismatch"))
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
	}
	if err := idt.Claims(&claims); err != nil {
		return "", redirect, oidcErr(OIDCErrProvider, err)
	}
	email = strings.ToLower(strings.TrimSpace(claims.Email))
	// Some providers send the flag as the string "true". Anything else —
	// missing included — is unverified: an address the provider has not
	// checked is one anybody could have typed.
	verified := claims.EmailVerified == true || claims.EmailVerified == "true"
	if email == "" || !verified {
		return "", redirect, oidcErr(OIDCErrUnverified, nil)
	}
	if len(cl.cfg.AllowedDomains) > 0 {
		_, domain, _ := strings.Cut(email, "@")
		if !containsString(cl.cfg.AllowedDomains, domain) {
			return "", redirect, oidcErr(OIDCErrDomain, nil)
		}
	}

	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		var linked bool
		user, linked, err = e.resolveIdentity(c, id, idt.Subject, email)
		if err != nil {
			return err
		}
		if !linked {
			if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_user_identity (provider, subject, "user", email, last_login)
				VALUES ($1, $2, $3, $4, now())`, id, idt.Subject, user, email); err != nil {
				return err
			}
			if err := e.RecordAuditOn(ctx, c.Tx, user, "account.identity_link", "Allowed", "User", user, from.IP, "",
				map[string]any{"provider": id, "email": email}); err != nil {
				return err
			}
		} else if _, err := c.Tx.Exec(ctx, `UPDATE ddcore_user_identity SET last_login = now(), email = $3
			WHERE provider = $1 AND subject = $2`, id, idt.Subject, email); err != nil {
			return err
		}
		sid, err = e.createSession(c, user, from)
		return err
	})
	if err != nil {
		return "", redirect, err
	}
	e.ClearAttempts(ctx, throttleKey("login", user))
	return sid, redirect, nil
}

// resolveIdentity finds the User a provider's subject signs in as: the one it
// was linked to before, or else the enabled User with the verified address.
func (e *Engine) resolveIdentity(c *Ctx, provider, subject, email string) (user string, linked bool, err error) {
	rows, err := db.Select(c.Ctx, c.Tx, `SELECT u.name, u.enabled FROM ddcore_user_identity i
		JOIN tab_user u ON u.name = i."user" WHERE i.provider = $1 AND i.subject = $2`, provider, subject)
	if err != nil {
		return "", false, err
	}
	linked = len(rows) > 0
	if !linked {
		rows, err = db.Select(c.Ctx, c.Tx, `SELECT name, enabled FROM tab_user
			WHERE lower(email) = $1 OR lower(name) = $1 ORDER BY (lower(email) = $1) DESC LIMIT 1`, email)
		if err != nil {
			return "", false, err
		}
		// The identity row may point at a User that was deleted; it is
		// replaced below rather than trusted.
		if _, err := c.Tx.Exec(c.Ctx, `DELETE FROM ddcore_user_identity WHERE provider = $1 AND subject = $2`, provider, subject); err != nil {
			return "", false, err
		}
	}
	if len(rows) == 0 {
		return "", false, oidcErr(OIDCErrNoAccount, nil)
	}
	name := db.Str(rows[0]["name"])
	if name == "Guest" {
		return "", false, oidcErr(OIDCErrNoAccount, nil)
	}
	if en, ok := rows[0]["enabled"].(bool); ok && !en {
		return name, linked, oidcErr(OIDCErrDisabled, nil)
	}
	return name, linked, nil
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
