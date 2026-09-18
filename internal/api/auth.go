package api

import (
	"net/http"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// The recovery endpoints are dedicated Go routes rather than whitelisted TS
// methods, for three reasons that each decide it on their own:
//
//   - they answer for Guest, so they need a throttle of their own;
//   - that throttle writes outside the request transaction, and a whitelisted
//     method runs inside s.run's;
//   - they need the raw request address and control over the response shape.
//
// They are exempt from the CSRF header check for the same reason /api/login
// is: there is no session yet to forge a request from.

// authThrottle is the pair of limits on an unauthenticated recovery endpoint:
// what someone typed, and where it came from.
//
// With 192 bits of token entropy, grinding one is not the threat. These bound
// cost — mail sent, Argon2 run, rows written — and stop one address from being
// mailed a link every second by someone who merely knows it exists.
func (s *Server) authThrottle(r *http.Request, key, identity string) error {
	p := s.E.Cfg.Auth
	if err := s.E.CheckThrottle(r.Context(), key+":"+strings.ToLower(strings.TrimSpace(identity)),
		3, p.LockoutWindow()); err != nil {
		return err
	}
	if ip := clientIP(r); ip != "" {
		return s.E.CheckThrottle(r.Context(), key+"ip:"+ip, 10, p.LockoutWindow())
	}
	return nil
}

func (s *Server) authRecord(r *http.Request, key, identity string, ok bool) {
	ip := clientIP(r)
	s.E.RecordAttempt(r.Context(), key+":"+strings.ToLower(strings.TrimSpace(identity)), ip, ok)
	if ip != "" {
		s.E.RecordAttempt(r.Context(), key+"ip:"+ip, ip, ok)
	}
}

// forgotPassword answers 200 for every input it is given.
//
// An unknown address, a known one and a disabled account are indistinguishable
// from outside — status, body and timing alike. Timing falls out for free:
// there is no Argon2 on this path, and the message goes through the job queue
// rather than out a socket, so a real send does not take measurably longer
// than no send at all.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Usr string `json:"usr"`
	}
	if err := readJSON(r, &body); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.authThrottle(r, "forgot", body.Usr); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.authRecord(r, "forgot", body.Usr, false)

	// With password sign-in off a reset link would set a password nobody can
	// use — except Admin's, which is the way back in when the
	// identity provider is down.
	if user, ok := s.E.FindUserForRecovery(r.Context(), body.Usr); ok &&
		(s.E.Cfg.Auth.AllowPasswordLogin() || user == "Admin") {
		var rec *engine.Recovery
		err := s.E.Run(r.Context(), "Admin", func(c *engine.Ctx) error {
			var e error
			rec, e = s.E.StartRecovery(c, user, engine.TokenReset, clientIP(r))
			return e
		})
		if err != nil {
			// Still answered as success: a failure here would otherwise be one
			// more way to tell a real address from an imaginary one.
			s.E.Log.Error("could not start a password recovery", "err", err)
		} else if rec.Link != "" {
			s.E.Log.Info("password recovery link (mail is not being delivered)", "user", user, "link", rec.Link)
		}
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"ok": true}})
}

// authToken reports what a link is for without spending it, so the desk can
// say whose account it is and tell an invitation from a recovery. It reveals
// nothing the holder of the link does not already have.
func (s *Server) authToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &body); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.authThrottle(r, "authtoken", engine.TokenHandle(body.Token)); err != nil {
		s.writeErr(w, r, err)
		return
	}
	at, err := s.E.PeekToken(r.Context(), body.Token)
	if err != nil {
		s.authRecord(r, "authtoken", engine.TokenHandle(body.Token), false)
		s.writeErr(w, r, err)
		return
	}
	name := ""
	s.E.Run(r.Context(), "Admin", func(c *engine.Ctx) error {
		if m, err := c.GetValues("User", at.User, []string{"full_name"}); err == nil && m != nil {
			name, _ = m["full_name"].(string)
		}
		return nil
	})
	writeJSON(w, 200, map[string]any{"data": map[string]any{
		"kind": at.Kind, "user": at.User, "fullName": name, "expires": at.Expires,
	}})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	s.completeRecovery(w, r, engine.TokenReset)
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	s.completeRecovery(w, r, engine.TokenInvite)
}

// completeRecovery spends the link and sets the password. It deliberately does
// not sign the person in: a session minted from a token-bearing request is a
// second way to get one, and one way is enough to reason about. The desk sends
// them to the sign-in page with the password they just chose.
func (s *Server) completeRecovery(w http.ResponseWriter, r *http.Request, kind string) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		FullName string `json:"fullName"`
	}
	if err := readJSON(r, &body); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if body.Token == "" {
		s.writeErr(w, r, cerr.Validation("This link is no longer valid. Ask for a new one."))
		return
	}
	handle := engine.TokenHandle(body.Token)
	if err := s.authThrottle(r, "resettoken", handle); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if _, err := s.E.CompleteRecovery(r.Context(), body.Token, kind, body.Password, body.FullName); err != nil {
		s.authRecord(r, "resettoken", handle, false)
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"ok": true}})
}

// csrfExempt names the paths that cannot carry a CSRF header because there is
// no session behind them yet.
func csrfExempt(path string) bool {
	return strings.HasPrefix(path, "/api/login") || strings.HasPrefix(path, "/api/auth/")
}
