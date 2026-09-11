package api

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// Disabled user:
// - With wrong password: does not reveal that account exists or that it is disabled (401 "Invalid username or password").
// - With correct password: 401 "User is disabled" (check runs only after Argon2).
// - Attempts count towards lockout and lock with 429 just like any other account.
func TestSEC04_DisabledUserAbuse(t *testing.T) {
	x := setup(t)

	// Disable ze@x.com
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ze@x.com", engine.Doc{"enabled": false})
	})

	// 1. Wrong password: generic response identical to nonexistent account
	bad := x.badLogin("ze@x.com")
	unknown := x.badLogin("ninguem@x.com")
	x.expect(bad, 401, "AuthenticationError")
	if bad.Status != unknown.Status || bad.errType() != unknown.errType() || msg(bad) != msg(unknown) {
		t.Errorf("wrong password on disabled user must be identical to nonexistent user: %q vs %q", msg(bad), msg(unknown))
	}

	// 2. Correct password: only now reports that user is disabled
	good := x.goodLogin("ze@x.com", "segredo123")
	x.expect(good, 401, "AuthenticationError")
	if msg(good) == msg(bad) {
		t.Errorf("correct password should indicate user is disabled, got same message: %q", msg(good))
	}
	if !strings.Contains(msg(good), "desativado") && !strings.Contains(msg(good), "disabled") {
		t.Errorf("correct password should indicate user is disabled, got %q", msg(good))
	}

	// 3. Repeated attempts lock the identity with 429
	for i := 2; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ze@x.com")
	}
	r := x.badLogin("ze@x.com")
	x.expect(r, 429, "TooManyRequestsError")
	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("lockout of disabled user must also respond with Retry-After")
	}
}

// Disabling a user revokes their sessions and keys immediately, without waiting
// for expiration or cache TTL.
func TestSEC04_DisabledUserSessionsAndKeysRevoked(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	key := x.apiKey("ana@x.com")

	// Session and key work before deactivation
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+sid), 200, "")
	x.expect(x.call("GET", "/api/boot", nil, "token:"+key), 200, "")

	// Administrator disables the user via form/controller
	x.asAdmin(func(c *engine.Ctx) error {
		u, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		u["enabled"] = false
		_, err = c.Save(u, engine.SaveOpts{})
		return err
	})

	// Session dropped immediately from database and cache
	rBoot := x.call("GET", "/api/boot", nil, "sid:"+sid)
	x.expect(rBoot, 200, "")
	if u, _ := rBoot.Body["data"].(map[string]any); u != nil && u["user"] != "Guest" {
		t.Errorf("session should have been invalidated, got user=%v", u["user"])
	}

	// API key stops authenticating immediately
	x.expect(x.call("GET", "/api/boot", nil, "token:"+key), 401, "AuthenticationError")
}

// Brute-force on API key secret:
// Since secret validation runs Argon2 on each request, 20 wrong guesses
// trigger process cache throttle (`apikeyfail:key`), preventing memory DoS.
func TestSEC04_APIKeyBruteForceBrake(t *testing.T) {
	x := setup(t)
	token := x.apiKey("ana@x.com")
	keyName := splitKey(token)

	for i := 0; i < 20; i++ {
		r := x.call("GET", "/api/boot", nil, "token:"+keyName+":segredoerrado")
		x.expect(r, 401, "AuthenticationError")
	}

	// Cache brake reached threshold
	v, ok := x.e.Cache.Get("apikeyfail:" + keyName)
	if !ok || v.(int) < 20 {
		t.Fatalf("apikeyfail brake should be active in cache: ok=%v val=%v", ok, v)
	}

	// Next request fails immediately before Argon2
	r := x.call("GET", "/api/boot", nil, "token:"+keyName+":outrosegedoerrado")
	x.expect(r, 401, "AuthenticationError")
}

// Token kind mismatch:
// A recovery token cannot be used in accept-invite, and vice-versa.
func TestSEC04_TokenKindMismatch(t *testing.T) {
	x := setup(t)

	// Reset token attempted to be used as invite
	resetTok := x.issueFor("ana@x.com", engine.TokenReset)
	r1 := x.call("POST", "/api/auth/accept-invite",
		map[string]any{"token": resetTok, "password": "senhanovaok1"}, "")
	x.expect(r1, 417, "ValidationError")

	// Invite token attempted to be used as reset
	inviteTok := x.issueFor("ana@x.com", engine.TokenInvite)
	r2 := x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": inviteTok, "password": "senhanovaok1"}, "")
	x.expect(r2, 417, "ValidationError")
}

// IP throttle on spraying logins:
// An attacker trying multiple logins across different accounts from the same IP
// is stopped by key loginip:<ip> after MaxLoginAttempts * 5 errors.
func TestSEC04_IPThrottleSpraying(t *testing.T) {
	x := setup(t)
	limit := x.e.Cfg.Auth.MaxLoginAttempts * 5

	for i := 0; i < limit; i++ {
		usr := fmt.Sprintf("spray%d@x.com", i)
		x.expect(x.badLogin(usr), 401, "AuthenticationError")
	}

	// IP is now locked, even for an account that was never attempted
	r := x.badLogin("conta_virgem@x.com")
	x.expect(r, 429, "TooManyRequestsError")
	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("IP throttle must respond with Retry-After")
	} else if n, err := strconv.Atoi(ra); err != nil || n <= 0 {
		t.Errorf("invalid Retry-After: %q", ra)
	}
}
