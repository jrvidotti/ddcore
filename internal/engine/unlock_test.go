package engine

import (
	"encoding/json"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// The operator types the address; the sign-in counted it as the person typed
// it, trimmed and lowercased. Unlocking must reach the same key, however it was
// spelled, and leave the same trace the desk's unlock does.
func TestUnlockAccountNormalisesTheTypedIdentityAndAudits(t *testing.T) {
	e := setup(t)
	ctx := t.Context()

	b, _ := json.Marshal(map[string]any{"email": "locked@example.com", "fullName": "Locked User"})
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		r, err := c.RT()
		if err != nil {
			return err
		}
		_, err = r.CallWhitelisted("core.services.users.invite", b)
		return err
	}); err != nil {
		t.Fatalf("invite failed: %v", err)
	}

	key := throttleKey("login", "locked@example.com")
	for range 3 {
		e.RecordAttempt(ctx, key, "203.0.113.9", false)
	}

	n, err := e.UnlockAccount(ctx, "  Locked@Example.com ", "cli:test")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("cleared %d attempts, want 3", n)
	}
	var left int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_login_attempt WHERE identity = $1`, key).Scan(&left); err != nil || left != 0 {
		t.Fatalf("attempts left after the unlock: %d, err=%v", left, err)
	}

	events, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.unlock", TargetID: "locked@example.com"})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 account.unlock event, got %d, err=%v", len(events), err)
	}
	if got := db.Str(events[0]["actor"]); got != "cli:test" {
		t.Errorf("actor = %q, want cli:test", got)
	}
}
