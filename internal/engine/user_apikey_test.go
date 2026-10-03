package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// auditOf returns the apikey.create events on user, read from outside every
// space.
func auditOf(t *testing.T, e *Engine, user string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := e.System(context.Background(), func(q db.Querier) (err error) {
		rows, err = db.Select(context.Background(), q, `SELECT *, detail::text AS detail FROM tab_audit_event
			WHERE action = 'apikey.create' AND target_doctype = 'User' AND target_id = $1`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return rows
}

// A System Manager issues a key for another user from server code (#72): it
// authenticates as that user, and the audit log records it without the secret.
func TestUserAPIKeyIssuedBySystemManager(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	const (
		manager = "manager@x.com"
		machine = "machine@x.com"
		plain   = "plain@x.com"
	)
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "User", Doc{"email": manager, "full_name": "Manager",
			"roles": []any{map[string]any{"role": "System Manager"}}}); err != nil {
			return err
		}
		for _, u := range []string{machine, plain} {
			if err := insertDoc(c, "User", Doc{"email": u, "full_name": u}); err != nil {
				return err
			}
		}
		return nil
	})

	var out map[string]any
	runAs(t, e, manager, func(c *Ctx) (err error) {
		out, err = e.CreateUserAPIKey(c, machine, "gateway", 30)
		return err
	})
	key, secret := db.Str(out["key"]), db.Str(out["secret"])
	if key == "" || secret == "" || out["expires"] == nil {
		t.Fatalf("createApiKey returned %v", out)
	}
	if _, ok := out["token"]; ok {
		t.Errorf("the result carries the joined token too: %v", out)
	}
	if u, err := e.UserFromAPIKey(ctx, key+":"+secret); err != nil || u != machine {
		t.Fatalf("the key authenticates as %q (%v), want %s", u, err, machine)
	}

	events := auditOf(t, e, machine)
	if len(events) != 1 {
		t.Fatalf("expected one apikey.create event, got %v", events)
	}
	detail := db.Str(events[0]["detail"])
	if db.Str(events[0]["actor"]) != manager {
		t.Errorf("actor = %v, want %s", events[0]["actor"], manager)
	}
	if strings.Contains(detail, secret) {
		t.Errorf("the audit detail holds the secret: %s", detail)
	}
	if !strings.Contains(detail, `"gateway"`) || !strings.Contains(detail, key) {
		t.Errorf("the audit detail misses the label or the key's id: %s", detail)
	}

	// refusals: a user who cannot administer users, a missing user, Guest,
	// and Admin from anyone but Admin
	if err := e.Run(ctx, plain, func(c *Ctx) error {
		_, err := e.CreateUserAPIKey(c, machine, "", 0)
		return err
	}); err == nil || cerr.From(err).Type != "PermissionError" {
		t.Errorf("a plain user issued a key: %v", err)
	}
	for _, target := range []string{"nobody@x.com", "Guest", "Admin", ""} {
		if err := e.Run(ctx, manager, func(c *Ctx) error {
			_, err := e.CreateUserAPIKey(c, target, "", 0)
			return err
		}); err == nil {
			t.Errorf("a key was issued for %q", target)
		}
	}
	if len(auditOf(t, e, machine)) != 1 {
		t.Error("a refused call recorded an apikey.create event")
	}
}

// Inside a tenant only that tenant's users get a key; the platform issues one
// in the user's own space, as the CLI does.
func TestUserAPIKeyTenancy(t *testing.T) {
	e := setupTenancy(t)
	ctx := context.Background()
	const manager = "sm@alfa.test"
	inTenant(t, e, tenantA, func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": manager, "full_name": "SM",
			"roles": []any{map[string]any{"role": "System Manager"}}})
	})

	runAs(t, e, manager, func(c *Ctx) error {
		if _, err := e.CreateUserAPIKey(c, userA, "", 0); err != nil {
			t.Errorf("own tenant: %v", err)
		}
		return nil
	})
	err := e.Run(ctx, manager, func(c *Ctx) error {
		_, err := e.CreateUserAPIKey(c, userB, "", 0)
		return err
	})
	if err == nil {
		t.Fatal("a tenant's System Manager issued a key for another tenant's user")
	}
	// the same answer as for nobody, so the call does not reveal the account
	if want := "User " + userB + " does not exist"; err.Error() != want {
		t.Errorf("refusal = %q, want %q", err, want)
	}

	var out map[string]any
	runAs(t, e, "Admin", func(c *Ctx) (err error) {
		out, err = e.CreateUserAPIKey(c, userB, "provision", 0)
		return err
	})
	key := db.Str(out["key"])
	if u, err := e.UserFromAPIKey(ctx, key+":"+db.Str(out["secret"])); err != nil || u != userB {
		t.Fatalf("the platform's key authenticates as %q (%v), want %s", u, err, userB)
	}
	var keyTenant string
	if err := e.System(ctx, func(q db.Querier) error {
		return q.QueryRow(ctx, `SELECT tenant FROM tab_api_key WHERE id = $1`, key).Scan(&keyTenant)
	}); err != nil {
		t.Fatal(err)
	}
	if keyTenant != tenantB {
		t.Errorf("the key was stored in space %q, want %s", keyTenant, tenantB)
	}
	if ev := auditOf(t, e, userB); len(ev) != 1 || db.Str(ev[0]["tenant"]) != tenantB {
		t.Errorf("the audit event is not in %s: %v", tenantB, ev)
	}
}

const createAPIKeySuite = `import "@ddcore/sdk/test";
const machine = "machine@x.com";
const plain = "plain@x.com";

describe("ddcore.users.createApiKey", () => {
  it("issues a key for another user, to System Managers only", () => {
    ddcore.newDoc("User", { email: machine, full_name: "Machine" }).insert();
    ddcore.newDoc("User", { email: plain, full_name: "Plain" }).insert();
    const out = ddcore.users.createApiKey(machine, { label: "gateway", days: 5 });
    expect(typeof out.key).toBe("string");
    expect(typeof out.secret).toBe("string");
    expect(out.expires).toBeTruthy();
    expect(ddcore.db.getValue("API Key", out.key, "user")).toBe(machine);
    expect(ddcore.db.getValue("API Key", out.key, "label")).toBe("gateway");
    const bare = ddcore.users.createApiKey(machine);
    expect(bare.expires).toBeFalsy();
    ddcore.test.asUser(plain, () => {
      expect(() => ddcore.users.createApiKey(machine)).toThrow();
    });
  });
});`

func TestUserAPIKeyFromJS(t *testing.T) {
	dir := sec01App(t)
	if err := os.WriteFile(filepath.Join(dir, "doctypes/test_record/test_record.test.ts"), []byte(createAPIKeySuite), 0o644); err != nil {
		t.Fatal(err)
	}
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "scope_test", Dir: dir}}, Test: true})
	res, err := e.RunTests(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 test, got %d", len(res))
	}
	for _, r := range res {
		if !r.OK {
			t.Errorf("%s: %s", r.Name, r.Error)
		}
	}
}

// `ddcore apikey` goes through IssueAPIKey: the key lands in the user's own
// space and is audited the same way, with Admin as the actor.
func TestUserAPIKeyIssuedOutsideARequest(t *testing.T) {
	e := setupTenancy(t)
	ctx := context.Background()
	out, err := e.IssueAPIKey(ctx, userA, "cli", 7)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := e.UserFromAPIKey(ctx, db.Str(out["key"])+":"+db.Str(out["secret"])); err != nil || u != userA {
		t.Fatalf("the key authenticates as %q (%v), want %s", u, err, userA)
	}
	ev := auditOf(t, e, userA)
	if len(ev) != 1 || db.Str(ev[0]["actor"]) != "Admin" || db.Str(ev[0]["tenant"]) != tenantA {
		t.Fatalf("audit = %v", ev)
	}
	if _, err := e.IssueAPIKey(ctx, "nobody@alfa.test", "cli", 0); err == nil {
		t.Error("a key was issued for a missing user")
	}
}
