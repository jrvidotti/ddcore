package api

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// badLogin attempts to log in with the wrong password via the real route.
func (x *env) badLogin(usr string) resp {
	x.t.Helper()
	return x.call("POST", "/api/login", map[string]any{"usr": usr, "pwd": "not-the-password"}, "")
}

func (x *env) goodLogin(usr, pwd string) resp {
	x.t.Helper()
	return x.call("POST", "/api/login", map[string]any{"usr": usr, "pwd": pwd}, "")
}

// countAttempts counts registered attempts for an identity.
func (x *env) countAttempts(identity string) int {
	x.t.Helper()
	rows, err := db.Select(x.ctx, x.e.DB.Pool,
		`SELECT count(*) AS n FROM ddcore_login_attempt WHERE identity = $1`, identity)
	if err != nil {
		x.t.Fatal(err)
	}
	n, _ := rows[0]["n"].(int64)
	return int(n)
}

// After maxLoginAttempts errors the account locks, and the response indicates for how
// long — including in the header, which is where an HTTP client looks.
func TestSEC04_LoginLockout(t *testing.T) {
	x := setup(t)
	limit := x.e.Cfg.Auth.MaxLoginAttempts

	for i := 0; i < limit; i++ {
		x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	}
	r := x.badLogin("ana@x.com")
	x.expect(r, 429, "TooManyRequestsError")

	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("a 429 response must include Retry-After")
	} else if n, err := strconv.Atoi(ra); err != nil || n <= 0 {
		t.Errorf("Retry-After should be a number of seconds, got %q", ra)
	}
	if e, ok := r.Body["error"].(map[string]any); ok {
		if extra, ok := e["extra"].(map[string]any); !ok || extra["retryAfter"] == nil {
			t.Errorf("body must also carry retryAfter, got %s", r.Raw)
		}
	}

	// The right password does not bypass the lockout: if it did, one could simply guess
	// after exhausting attempts for the lockout to have never existed.
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 429, "TooManyRequestsError")

	// Another account can still log in: the lockout belongs to the identity, not the server.
	x.expect(x.goodLogin("bia@x.com", "segredo123"), 200, "")
}

// Architectural regression check: Engine.Login runs inside Ctx.Run, which
// rolls back on error. If the attempt were recorded within the transaction, the very
// error it counts would erase it — and the lockout would never count anything.
func TestSEC04_FailedLoginSurvivesRollback(t *testing.T) {
	x := setup(t)
	before := x.countAttempts("login:ana@x.com")
	x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	if after := x.countAttempts("login:ana@x.com"); after != before+1 {
		t.Fatalf("failed attempt must survive rollback: %d → %d", before, after)
	}
}

// A nonexistent address must respond identically to an existing one, and lock out
// identically: otherwise the lockout itself becomes an oracle it is meant to prevent.
func TestSEC04_LockoutDoesNotEnumerate(t *testing.T) {
	x := setup(t)
	known := x.badLogin("ana@x.com")
	unknown := x.badLogin("ninguem@x.com")

	if known.Status != unknown.Status || known.errType() != unknown.errType() {
		t.Errorf("different status/kind: known %d %s, unknown %d %s",
			known.Status, known.errType(), unknown.Status, unknown.errType())
	}
	if msg(known) != msg(unknown) {
		t.Errorf("different messages: %q vs %q", msg(known), msg(unknown))
	}

	// and the unknown user also locks out
	for i := 1; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ninguem@x.com")
	}
	x.expect(x.badLogin("ninguem@x.com"), 429, "TooManyRequestsError")
}

// Getting the password right resets the counter: someone who finally remembered does not
// remain locked out.
func TestSEC04_SuccessClearsTheCounter(t *testing.T) {
	x := setup(t)
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts-1; i++ {
		x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	}
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	// the counter reset: a new error cannot jump directly to 429
	x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
}

// The session cookie follows the policy, rather than a hardcoded number in the handler.
func TestSEC04_SessionCookieFollowsPolicy(t *testing.T) {
	x := setup(t)
	r := x.goodLogin("ana@x.com", "segredo123")
	x.expect(r, 200, "")

	want := int(x.e.Cfg.Auth.SessionTTL().Seconds())
	for _, c := range r.Header.Values("Set-Cookie") {
		if !strings.Contains(c, "sid=") {
			continue
		}
		if !strings.Contains(c, "Max-Age="+strconv.Itoa(want)) {
			t.Errorf("Max-Age should be %d (the policy), got %q", want, c)
		}
		if !strings.Contains(c, "HttpOnly") {
			t.Errorf("session cookie must be HttpOnly: %q", c)
		}
		// Secure is omitted: httptest serves http and the site has not declared https.
		if strings.Contains(c, "Secure") {
			t.Errorf("without TLS and without https url, Secure would make the cookie useless: %q", c)
		}
	}
}

// A revoked session dies immediately, not a minute later: DropSessions must
// invalidate the cache along with the row.
func TestSEC04_DropSessionsInvalidatesTheCache(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+sid), 200, "")

	x.asAdmin(func(c *engine.Ctx) error {
		n, err := x.e.DropSessions(context.Background(), c.Tx, "ana@x.com", "")
		if err != nil {
			return err
		}
		if n == 0 {
			t.Error("expected at least one dropped session")
		}
		return nil
	})

	r := x.call("GET", "/api/boot", nil, "sid:"+sid)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] != "Guest" {
		t.Errorf("dropped session remained valid: %v", u["user"])
	}
}

// exceptSid allows changing one's own password without logging out of the tab where
// it was entered.
func TestSEC04_DropSessionsSparesTheCaller(t *testing.T) {
	x := setup(t)
	keep := x.sid("ana@x.com")
	other := x.sid("ana@x.com")

	x.asAdmin(func(c *engine.Ctx) error {
		_, err := x.e.DropSessions(context.Background(), c.Tx, "ana@x.com", keep)
		return err
	})

	x.expect(x.call("GET", "/api/boot", nil, "sid:"+keep), 200, "")
	r := x.call("GET", "/api/boot", nil, "sid:"+other)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("the other session should have been dropped")
	}
}

// The policy applies on all paths that set passwords, not just in the form: that
// is why it lives in hashing, not in each caller.
func TestSEC04_PasswordPolicyOnEveryPath(t *testing.T) {
	x := setup(t)
	shortPwd := "abc"

	// 1. User form and `ddcore user add`, via new_password -> __hashPassword
	err := x.e.Run(x.ctx, "Admin", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "nova@x.com", "full_name": "Nova", "new_password": shortPwd})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err == nil {
		t.Error("User form should reject a password below the minimum")
	}

	// 2. `ddcore user passwd`, recovery and invite, via SetPassword
	if err := x.e.SetPassword(x.ctx, "ana@x.com", shortPwd); err == nil {
		t.Error("SetPassword should reject a password below the minimum")
	}
	if err := x.e.SetPassword(x.ctx, "ana@x.com", "outrasenha1"); err != nil {
		t.Errorf("a valid password should succeed: %v", err)
	}
}

// Changing password drops old sessions: if the change was because the password
// leaked, leaving sessions active would defeat the purpose.
func TestSEC04_PasswordChangeRevokesSessions(t *testing.T) {
	x := setup(t)
	old := x.sid("ana@x.com")
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+old), 200, "")

	if err := x.e.SetPassword(x.ctx, "ana@x.com", "senhanova123"); err != nil {
		t.Fatal(err)
	}

	r := x.call("GET", "/api/boot", nil, "sid:"+old)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("the old session should have been dropped upon password change")
	}
	x.expect(x.goodLogin("ana@x.com", "senhanova123"), 200, "")
}

// A changed password also clears lockout: whoever just proved they can
// reset it should not remain locked out.
func TestSEC04_PasswordChangeClearsTheLockout(t *testing.T) {
	x := setup(t)
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ana@x.com")
	}
	x.expect(x.badLogin("ana@x.com"), 429, "TooManyRequestsError")

	if err := x.e.SetPassword(x.ctx, "ana@x.com", "destravada123"); err != nil {
		t.Fatal(err)
	}
	x.expect(x.goodLogin("ana@x.com", "destravada123"), 200, "")
}

func msg(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		if m, ok := e["message"].(string); ok {
			return m
		}
	}
	return ""
}

// The desk saves the whole document it read, and a read blanks
// password_hash: that null used to be written back, so saving a User form
// without touching the password erased it and signed the user out.
func TestSEC04_SavingAUserKeepsThePassword(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	ana := "sid:" + x.sid("ana@x.com")
	r := x.call("GET", "/api/resource/User/ana@x.com", nil, admin)
	x.expect(r, 200, "")
	doc := r.Body["data"].(map[string]any)
	if doc["password_hash"] != nil {
		t.Fatalf("password_hash left the server: %v", doc["password_hash"])
	}

	x.expect(x.call("PUT", "/api/resource/User/ana@x.com", doc, admin), 200, "")
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	doc = x.call("GET", "/api/resource/User/ana@x.com", nil, admin).Body["data"].(map[string]any)
	x.expect(x.call("POST", "/api/resource/User/ana@x.com/save", map[string]any{"doc": doc}, admin), 200, "")
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	if u, _ := x.call("GET", "/api/boot", nil, ana).Body["data"].(map[string]any); u == nil || u["user"] != "ana@x.com" {
		t.Error("saving the User dropped her session")
	}
}
