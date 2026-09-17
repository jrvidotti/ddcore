package api

import (
	"errors"
	"net/http"
	"net/url"

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
	to, state, err := s.E.OIDCStart(r.Context(), id, r.URL.Query().Get("redirect"), clientIP(r))
	if err != nil {
		s.E.Log.Warn("single sign-on could not start", "provider", id, "err", err)
		s.oidcFail(w, r, err)
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
		s.oidcFail(w, r, &engine.OIDCError{Code: engine.OIDCErrProvider})
		return
	}
	sid, redirect, err := s.E.OIDCCallback(r.Context(), id, q.Get("code"), q.Get("state"), cookieState, engine.LoginFrom{
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		s.oidcFail(w, r, err)
		return
	}
	if user, _ := s.E.UserFromSession(r.Context(), sid); user != "" {
		s.E.Cache.Del("lang:" + user)
	}
	http.SetCookie(w, s.sessionCookie(r, sid))
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s *Server) oidcFail(w http.ResponseWriter, r *http.Request, err error) {
	code := engine.OIDCErrServer
	var oe *engine.OIDCError
	if errors.As(err, &oe) {
		code = oe.Code
	} else {
		s.E.Log.Error("single sign-on failed", "err", err)
	}
	http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(code), http.StatusFound)
}
