package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// oidcStateCookie binds a sign-in in progress to the browser that started it.
// Its path is the callback's own, so it never travels with anything else.
const oidcStateCookie = "ddcore_oidc_state"

func (s *Server) oidcCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: oidcStateCookie, Value: value, Path: "/api/auth/oidc/", HttpOnly: true,
		// Lax, not Strict: the provider sends the browser back with a
		// top-level GET from its own site, which Strict would strip the
		// cookie from.
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(r),
		MaxAge:   maxAge,
	}
}

// oidcStart sends the browser to the provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "provider")
	if _, ok := s.E.OIDCProvider(id); !ok {
		s.writeErr(w, r, cerr.NotFound("Unknown sign-in provider"))
		return
	}
	redirect := r.URL.Query().Get("redirect")
	to, state, err := s.E.OIDCStart(r.Context(), id, redirect, clientIP(r))
	if err != nil {
		s.E.Log.Warn("single sign-on could not start", "provider", id, "err", err)
		s.oidcFail(w, r, id, engine.SafeRedirect(redirect), err)
		return
	}
	http.SetCookie(w, s.oidcCookie(r, state, int(engine.OIDCStateTTL.Seconds())))
	http.Redirect(w, r, to, http.StatusFound)
}

// oidcCallback is where the provider sends the browser back. Every outcome is
// a redirect: the person is looking at a browser tab, not reading JSON.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "provider")
	q := r.URL.Query()
	cookieState := ""
	if ck, err := r.Cookie(oidcStateCookie); err == nil {
		cookieState = ck.Value
	}
	http.SetCookie(w, s.oidcCookie(r, "", -1))
	if q.Get("error") != "" {
		// The person cancelled, or the provider refused the client. The state
		// is left to expire: spending it here would let anyone who can make
		// the browser load this URL cancel someone else's sign-in.
		s.E.Log.Info("single sign-on returned an error", "provider", id, "error", q.Get("error"))
		s.oidcFail(w, r, id, s.E.OIDCPendingRedirect(r.Context(), id, q.Get("state")), &engine.OIDCError{Code: engine.OIDCErrProvider})
		return
	}
	sid, redirect, err := s.E.OIDCCallback(r.Context(), id, q.Get("code"), q.Get("state"), cookieState, engine.LoginFrom{
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		if redirect == "" {
			// refused before the state was spent (no cookie, a foreign one,
			// the throttle): the sign-in may still be on record
			redirect = s.E.OIDCPendingRedirect(r.Context(), id, q.Get("state"))
		}
		s.oidcFail(w, r, id, redirect, err)
		return
	}
	if user, _ := s.E.UserFromSession(r.Context(), sid); user != "" {
		r = r.WithContext(s.E.LoginSpace(r.Context(), sid))
		s.dropLang(r, user)
		redirect = s.homeFor(r, user, redirect)
	}
	http.SetCookie(w, s.sessionCookie(r, sid))
	http.Redirect(w, r, redirect, http.StatusFound)
}

// oidcFail sends a failed sign-in back with ?sso_error=<code>. redirect is
// where the sign-in started, "" when that is not known.
func (s *Server) oidcFail(w http.ResponseWriter, r *http.Request, provider, redirect string, err error) {
	code := engine.OIDCErrServer
	var oe *engine.OIDCError
	if errors.As(err, &oe) {
		code = oe.Code
	} else {
		s.E.Log.Error("single sign-on failed", "err", err)
	}
	http.Redirect(w, r, s.ssoFailTarget(provider, redirect, code), http.StatusFound)
}

// ssoFailTarget is where a failed sign-in lands (#171). An app's static site
// shows the failure itself, so a sign-in it started goes back to it. The desk
// and the portal show it on /login, which keeps the destination so that
// trying again goes where the first attempt was going: /app and /portal
// would only bounce a signed-out visitor there and lose the message. When
// where it started is not known, auth.sso.<id>.errorRedirect decides.
func (s *Server) ssoFailTarget(provider, redirect, code string) string {
	login := "/login?sso_error=" + url.QueryEscape(code)
	switch {
	case redirect == "":
		// a page of the site's choosing, meant to read the code
		if to := s.E.Cfg.Auth.SSO[provider].ErrorRedirect; to != "" && to != "/login" {
			return withQuery(to, "sso_error", code)
		}
		return login
	case s.isAppSite(redirect):
		return withQuery(redirect, "sso_error", code)
	case redirect == "/app":
		return login
	default:
		return login + "&redirect=" + url.QueryEscape(redirect)
	}
}

// withQuery adds key=value to a same-site path, after its own query and
// before its fragment.
func withQuery(p, key, value string) string {
	p, frag, hasFrag := strings.Cut(p, "#")
	sep := "?"
	if strings.Contains(p, "?") {
		sep = "&"
		if strings.HasSuffix(p, "?") || strings.HasSuffix(p, "&") {
			sep = ""
		}
	}
	p += sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	if hasFrag {
		p += "#" + frag
	}
	return p
}
