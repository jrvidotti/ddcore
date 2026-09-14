package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

func TestAuditSanitization(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	c := e.NewCtx(ctx, "admin@example.com")

	longStr := strings.Repeat("A", 600)
	rawDetail := map[string]any{
		"user":           "alice@example.com",
		"password":       "super-secret-pw",
		"api_key":        "key-12345",
		"secret_token":   "tok-xyz",
		"auth_hash":      "hash-987",
		"ciphertext_val": "aes-encrypted-data",
		"long_desc":      longStr,
		"nested": map[string]any{
			"db_password": "nested-pw",
			"status":      "active",
		},
	}

	if err := c.Audit("test.sanitize", "TestDoc", "doc-1", rawDetail); err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	rows, err := db.Select(ctx, e.DB.Pool, `SELECT detail FROM tab_audit_event WHERE action = 'test.sanitize'`)
	if err != nil || len(rows) == 0 {
		t.Fatalf("failed to query audit row: %v", err)
	}

	detailStr := db.Str(rows[0]["detail"])
	if strings.Contains(detailStr, "super-secret-pw") {
		t.Errorf("password was not redacted: %s", detailStr)
	}
	if strings.Contains(detailStr, "key-12345") {
		t.Errorf("api_key was not redacted: %s", detailStr)
	}
	if strings.Contains(detailStr, "tok-xyz") {
		t.Errorf("secret_token was not redacted: %s", detailStr)
	}
	if strings.Contains(detailStr, "hash-987") {
		t.Errorf("auth_hash was not redacted: %s", detailStr)
	}
	if strings.Contains(detailStr, "nested-pw") {
		t.Errorf("nested password was not redacted: %s", detailStr)
	}
	if strings.Contains(detailStr, longStr) {
		t.Errorf("oversized string was not truncated: %s", detailStr)
	}
	if !strings.Contains(detailStr, "alice@example.com") {
		t.Errorf("safe field was incorrectly removed: %s", detailStr)
	}
}

func TestAuditImmutability(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	c := e.NewCtx(ctx, "Administrator")
	c.Flags["ignorePermissions"] = false

	// Attempt direct Insert
	doc, _ := c.NewDoc("Audit Event", Doc{
		"action":         "tamper.insert",
		"outcome":        "Allowed",
		"target_doctype": "User",
		"target_name":    "admin",
	})
	if _, err := c.Insert(doc, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("expected PermissionError on direct Insert of Audit Event, got: %v", err)
	}

	// Write a legitimate audit entry first via c.Audit
	if err := c.Audit("legit.action", "User", "user-1", nil); err != nil {
		t.Fatalf("legit audit failed: %v", err)
	}

	rows, err := db.Select(ctx, e.DB.Pool, `SELECT name FROM tab_audit_event WHERE action = 'legit.action'`)
	if err != nil || len(rows) == 0 {
		t.Fatalf("failed to find audit event: %v", err)
	}
	eventName := db.Str(rows[0]["name"])

	// Attempt direct Save / update
	eventDoc, err := c.GetDocIgnoringPerms("Audit Event", eventName)
	if err != nil {
		t.Fatalf("GetDocIgnoringPerms failed: %v", err)
	}
	eventDoc["action"] = "tampered.action"
	if _, err := c.Save(eventDoc, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("expected PermissionError on direct Save of Audit Event, got: %v", err)
	}

	// Attempt direct Delete
	if err := c.Delete("Audit Event", eventName, false, false); err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("expected PermissionError on direct Delete of Audit Event, got: %v", err)
	}
}

func TestAuditListCountAndPurge(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	_, _ = e.DB.Pool.Exec(ctx, "TRUNCATE tab_audit_event")
	c := e.NewCtx(ctx, "admin@example.com")

	// Insert events with varying actions and outcomes
	_ = c.Audit("role.assign", "User", "user1@example.com", map[string]any{"role": "Accounts User"})
	_ = c.Audit("account.invite", "User", "user2@example.com", nil)
	c.AuditDenied("job.cancel", "Job", "101", nil)

	// List without filters
	events, err := e.ListAuditEvents(ctx, AuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents failed: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	count, err := e.CountAuditEvents(ctx, AuditFilter{})
	if err != nil || count < 3 {
		t.Fatalf("CountAuditEvents failed: count=%d, err=%v", count, err)
	}

	// Filter by action
	roleEvents, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.assign"})
	if err != nil || len(roleEvents) != 1 {
		t.Fatalf("expected 1 role.assign event, got %d, err=%v", len(roleEvents), err)
	}

	// Filter by outcome
	deniedEvents, err := e.ListAuditEvents(ctx, AuditFilter{Outcome: "Denied"})
	if err != nil || len(deniedEvents) != 1 {
		t.Fatalf("expected 1 Denied event, got %d, err=%v", len(deniedEvents), err)
	}

	// Backdate one event to 40 days ago
	_, err = e.DB.Pool.Exec(ctx, `UPDATE tab_audit_event SET creation = now() - interval '40 days'
		WHERE action = 'role.assign'`)
	if err != nil {
		t.Fatal(err)
	}

	// Purge dry-run: 30 days
	dryCount, err := e.PurgeAuditEvents(ctx, 30, true)
	if err != nil || dryCount != 1 {
		t.Fatalf("dry run purge expected 1, got %d, err=%v", dryCount, err)
	}

	// Verify it was NOT deleted
	roleEventsAfterDry, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.assign"})
	if err != nil || len(roleEventsAfterDry) != 1 {
		t.Fatalf("dry run should not delete row: %v", roleEventsAfterDry)
	}

	// Real purge: 30 days
	purgedCount, err := e.PurgeAuditEvents(ctx, 30, false)
	if err != nil || purgedCount != 1 {
		t.Fatalf("real purge expected 1, got %d, err=%v", purgedCount, err)
	}

	// Verify row is now deleted
	roleEventsAfterPurge, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.assign"})
	if err != nil || len(roleEventsAfterPurge) != 0 {
		t.Fatalf("purged row still present: %v", roleEventsAfterPurge)
	}
}

func TestAudit_RoleChanges(t *testing.T) {
	e := setup(t)
	ctx := t.Context()

	// 1. Create a user as Administrator with an initial role
	var userName string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{
			"email":     "testuser@example.com",
			"full_name": "Test User",
			"enabled":   true,
			"roles": []any{
				map[string]any{"role": "Gestor"},
			},
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(u, SaveOpts{})
		if err != nil {
			return err
		}
		userName = saved.Str("name")
		return nil
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	// Verify initial role.assign
	events, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.assign", TargetName: userName})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 role.assign on insert, got %d, err=%v", len(events), err)
	}
	if !strings.Contains(db.Str(events[0]["detail"]), "Gestor") {
		t.Fatalf("expected Gestor role in detail: %v", events[0])
	}

	// 2. Modify user roles: remove Gestor, add All
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, err := c.GetDoc("User", userName)
		if err != nil {
			return err
		}
		u["roles"] = []any{
			map[string]any{"role": "All"},
		}
		_, err = c.Save(u, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("update roles failed: %v", err)
	}

	// Verify role.revoke and new role.assign
	revokes, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.revoke", TargetName: userName})
	if err != nil || len(revokes) != 1 {
		t.Fatalf("expected 1 role.revoke, got %d, err=%v", len(revokes), err)
	}
	if !strings.Contains(db.Str(revokes[0]["detail"]), "Gestor") {
		t.Fatalf("expected Gestor in revoke detail: %v", revokes[0])
	}

	assigns, err := e.ListAuditEvents(ctx, AuditFilter{Action: "role.assign", TargetName: userName})
	if err != nil || len(assigns) != 2 {
		t.Fatalf("expected 2 total role.assigns, got %d, err=%v", len(assigns), err)
	}

	// 3. Disable user
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, err := c.GetDoc("User", userName)
		if err != nil {
			return err
		}
		u["enabled"] = false
		_, err = c.Save(u, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("disable user failed: %v", err)
	}

	disables, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.disable", TargetName: userName})
	if err != nil || len(disables) != 1 {
		t.Fatalf("expected 1 account.disable, got %d, err=%v", len(disables), err)
	}
}

func TestAudit_AccountAdmin(t *testing.T) {
	e := setup(t)
	ctx := t.Context()

	// 1. Call core.services.users.invite as Administrator
	invitePayload := map[string]any{
		"email":    "invited@example.com",
		"fullName": "Invited User",
	}
	b, _ := json.Marshal(invitePayload)
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		r, err := c.RT()
		if err != nil {
			return err
		}
		_, err = r.CallWhitelisted("core.services.users.invite", b)
		return err
	})
	if err != nil {
		t.Fatalf("invite failed: %v", err)
	}

	// Verify account.invite audit event
	invites, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.invite", TargetName: "invited@example.com"})
	if err != nil || len(invites) != 1 {
		t.Fatalf("expected 1 account.invite event, got %d, err=%v", len(invites), err)
	}

	// 2. Call core.services.users.sendPasswordReset
	resetPayload, _ := json.Marshal(map[string]any{"user": "invited@example.com"})
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		r, err := c.RT()
		if err != nil {
			return err
		}
		_, err = r.CallWhitelisted("core.services.users.sendPasswordReset", resetPayload)
		return err
	})
	if err != nil {
		t.Fatalf("sendPasswordReset failed: %v", err)
	}

	resets, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.reset_password", TargetName: "invited@example.com"})
	if err != nil || len(resets) != 1 {
		t.Fatalf("expected 1 account.reset_password event, got %d, err=%v", len(resets), err)
	}

	// 3. Call core.services.users.unlockUser
	unlockPayload, _ := json.Marshal(map[string]any{"user": "invited@example.com"})
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		r, err := c.RT()
		if err != nil {
			return err
		}
		_, err = r.CallWhitelisted("core.services.users.unlockUser", unlockPayload)
		return err
	})
	if err != nil {
		t.Fatalf("unlockUser failed: %v", err)
	}

	unlocks, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.unlock", TargetName: "invited@example.com"})
	if err != nil || len(unlocks) != 1 {
		t.Fatalf("expected 1 account.unlock event, got %d, err=%v", len(unlocks), err)
	}

	// 4. Call core.services.users.revokeUserSessions
	revokePayload, _ := json.Marshal(map[string]any{"user": "invited@example.com"})
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		r, err := c.RT()
		if err != nil {
			return err
		}
		_, err = r.CallWhitelisted("core.services.users.revokeUserSessions", revokePayload)
		return err
	})
	if err != nil {
		t.Fatalf("revokeUserSessions failed: %v", err)
	}

	sessionRevokes, err := e.ListAuditEvents(ctx, AuditFilter{Action: "account.revoke_sessions", TargetName: "invited@example.com"})
	if err != nil || len(sessionRevokes) != 1 {
		t.Fatalf("expected 1 account.revoke_sessions event, got %d, err=%v", len(sessionRevokes), err)
	}
}
