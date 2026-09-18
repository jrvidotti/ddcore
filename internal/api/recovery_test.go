package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// callNoCSRF simulates what an anonymous form does: no CSRF header, because
// there is no session behind it.
func (x *env) callNoCSRF(method, path string, body any) resp {
	x.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, x.ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Header: res.Header}
	json.Unmarshal(raw, &out.Body)
	return out
}

// issueFor gets a recovery token directly from the engine, as email would.
func (x *env) issueFor(user, kind string) string {
	x.t.Helper()
	var token string
	x.asAdmin(func(c *engine.Ctx) error {
		t, _, err := x.e.IssueToken(x.ctx, user, kind, x.e.Cfg.Auth.ResetTTL(), "Admin", "127.0.0.1")
		token = t
		return err
	})
	return token
}

func (x *env) countTokens(user string) int {
	x.t.Helper()
	var n int
	x.asAdmin(func(c *engine.Ctx) error {
		rows, err := c.SQL(`SELECT count(*) AS n FROM ddcore_auth_token WHERE "user" = $1 AND used IS NULL`, []any{user})
		if err != nil {
			return err
		}
		if v, ok := rows[0]["n"].(int64); ok {
			n = int(v)
		}
		return nil
	})
	return n
}

// Unknown, known, and disabled users respond identically — status and body. If
// they differed, the endpoint would become an email address verifier.
func TestSEC04_ForgotPasswordIsGeneric(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ze@x.com", engine.Doc{"enabled": false})
	})

	var bodies []string
	for _, usr := range []string{"ana@x.com", "ninguem@x.com", "ze@x.com"} {
		r := x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": usr}, "")
		x.expect(r, 200, "")
		bodies = append(bodies, r.Raw)
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("responses must be identical:\n%s\nvs\n%s", bodies[0], bodies[i])
		}
	}

	// only the active and existing user received a token
	if n := x.countTokens("ana@x.com"); n != 1 {
		t.Errorf("ana should have 1 token, got %d", n)
	}
	if n := x.countTokens("ze@x.com"); n != 0 {
		t.Errorf("a disabled user cannot get a token, got %d", n)
	}
}

func TestSEC04_ResetTokenIsSingleUse(t *testing.T) {
	x := setup(t)
	token := x.issueFor("ana@x.com", engine.TokenReset)

	// peeking reveals who it belongs to, without consuming it
	r := x.call("POST", "/api/auth/token", map[string]any{"token": token}, "")
	x.expect(r, 200, "")
	if d, _ := r.Body["data"].(map[string]any); d == nil || d["user"] != "ana@x.com" || d["kind"] != "reset" {
		t.Fatalf("peek should return user and kind: %s", r.Raw)
	}

	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "senhanovaok1"}, ""), 200, "")

	// reuse is refused
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "outrasenha12"}, ""), 417, "ValidationError")

	// and the new password works
	x.expect(x.goodLogin("ana@x.com", "senhanovaok1"), 200, "")
}

// Password reset drops all sessions: the reason for resetting may be that the
// account was compromised.
func TestSEC04_ResetDropsEverySession(t *testing.T) {
	x := setup(t)
	old := x.sid("ana@x.com")
	token := x.issueFor("ana@x.com", engine.TokenReset)

	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "senhanovaok1"}, ""), 200, "")

	r := x.call("GET", "/api/boot", nil, "sid:"+old)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("the old session should have been dropped")
	}
}

// The password policy also applies at the end of password reset.
func TestSEC04_ResetAppliesThePasswordPolicy(t *testing.T) {
	x := setup(t)
	token := x.issueFor("ana@x.com", engine.TokenReset)
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "curta"}, ""), 417, "ValidationError")
	// and the token was not consumed by a rejected password
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "agoravalida1"}, ""), 200, "")
}

// An invite token sets the initial password — before that, login is not possible.
func TestSEC04_InviteAcceptSetsPassword(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "novo@x.com", "full_name": "Novo"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})

	// without a password, login is denied
	x.expect(x.goodLogin("novo@x.com", "qualquercoisa"), 401, "AuthenticationError")

	token := x.issueFor("novo@x.com", engine.TokenInvite)

	// an invite cannot be used as a password reset
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "primeirasenha1"}, ""), 417, "ValidationError")

	x.expect(x.call("POST", "/api/auth/accept-invite",
		map[string]any{"token": token, "password": "primeirasenha1", "fullName": "Novo Nome"}, ""), 200, "")
	x.expect(x.goodLogin("novo@x.com", "primeirasenha1"), 200, "")
}

func TestSEC04_AuthEndpointsAreThrottled(t *testing.T) {
	x := setup(t)
	var last resp
	for i := 0; i < 6; i++ {
		last = x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": "ana@x.com"}, "")
	}
	if last.Status != 429 {
		t.Fatalf("expected 429 after repeated requests, got %d: %s", last.Status, last.Raw)
	}
	if last.Header.Get("Retry-After") == "" {
		t.Error("429 response must include Retry-After")
	}
}

// An invalid link responds with 404, and never reveals if the token existed but expired.
func TestSEC04_UnknownTokenIsRefused(t *testing.T) {
	x := setup(t)
	x.expect(x.call("POST", "/api/auth/token",
		map[string]any{"token": "0123456789abcdef0123456789abcdef0123456789abcdef"}, ""), 404, "DoesNotExistError")
}

// Recovery routes are exempt from CSRF because there is no session to
// forge: without this, a "forgot password" form would never work.
func TestSEC04_AuthRoutesDoNotNeedTheCSRFHeader(t *testing.T) {
	x := setup(t)
	r := x.callNoCSRF("POST", "/api/auth/forgot-password", map[string]any{"usr": "ana@x.com"})
	x.expect(r, 200, "")
}
