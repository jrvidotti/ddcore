package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// The JS suite runs through Engine.RunTests exactly as ddcore test does: as
// Admin, one savepoint per test, the whole run rolled back.
const asUserSuite = `import "@ddcore/sdk/test";
const scoped = "scoped@x.com";

function fixture() {
  for (const title of ["Alfa", "Beta"]) ddcore.newDoc("Test Company", { title }).insert();
  for (const title of ["Alfa Record", "Beta Record"]) {
    ddcore.newDoc("Test Record", { title, company: title.split(" ")[0] }).insert();
  }
  ddcore.newDoc("User", { email: scoped, full_name: "Scoped", roles: [{ role: "Scope User" }] }).insert();
}

describe("asUser", () => {
  it("scopes reads to the user's User Permissions", () => {
    fixture();
    ddcore.newDoc("User Permission", { user: scoped, allow: "Test Company", for_value: "Alfa" }).insert();
    ddcore.test.asUser(scoped, () => {
      expect(ddcore.session.user).toBe(scoped);
      expect(ddcore.getRoles()).toContain("Scope User");
      const rows = ddcore.db.getList("Test Record", { fields: ["company"] });
      expect(rows.length).toBe(1);
      expect(rows[0].company).toBe("Alfa");
      expect(ddcore.getDoc("Test Record", "Alfa Record").company).toBe("Alfa");
      expect(() => ddcore.getDoc("Test Record", "Beta Record")).toThrow();
      expect(ddcore.hasPermission("Test Record", "read", "Beta Record")).toBe(false);
    });
    expect(ddcore.session.user).toBe("Admin");
    expect(ddcore.db.getList("Test Record", {}).length).toBe(2);
  });

  it("checks hasPermission for the user it is given", () => {
    fixture();
    ddcore.newDoc("User Permission", { user: scoped, allow: "Test Company", for_value: "Alfa" }).insert();
    expect(ddcore.hasPermission("Test Record", "read", "Beta Record", scoped)).toBe(false);
    expect(ddcore.hasPermission("Test Record", "read", "Alfa Record", scoped)).toBe(true);
    expect(ddcore.hasPermission("Test Record", "read", { company: "Beta" }, scoped)).toBe(false);
    expect(ddcore.hasPermission("Test Record", "read", "Beta Record")).toBe(true);
    expect(ddcore.hasPermission("Test Record", "read", "Beta Record", "Admin")).toBe(true);
  });

  it("sees the previous test's scopes rolled back, not cached", () => {
    fixture();
    ddcore.test.asUser(scoped, () => {
      expect(ddcore.db.getList("Test Record", {}).length).toBe(2);
    });
  });

  it("restores the previous user when the callback throws", () => {
    fixture();
    expect(() => ddcore.test.asUser(scoped, () => { throw new Error("boom"); })).toThrow("boom");
    expect(ddcore.session.user).toBe("Admin");
  });

  it("nests", () => {
    fixture();
    ddcore.test.asUser(scoped, () => {
      ddcore.test.asUser("Admin", () => expect(ddcore.session.user).toBe("Admin"));
      expect(ddcore.session.user).toBe(scoped);
    });
  });

  it("runs the next test as Admin again", () => {
    expect(ddcore.session.user).toBe("Admin");
  });
});`

func asUserApp(t *testing.T) string {
	t.Helper()
	dir := sec01App(t)
	path := filepath.Join(dir, "doctypes/test_record/test_record.test.ts")
	if err := os.WriteFile(path, []byte(asUserSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAsUserInTests(t *testing.T) {
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "scope_test", Dir: asUserApp(t)}}, Test: true})
	res, err := e.RunTests(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 6 {
		t.Fatalf("expected 6 tests, got %d", len(res))
	}
	for _, r := range res {
		if !r.OK {
			t.Errorf("%s: %s", r.Name, r.Error)
		}
	}
	// the rolled-back scopes never reached the process cache
	if _, ok := e.Cache.Get("user_perms:scoped@x.com"); ok {
		t.Error("asUser cached a User Permission the test rolled back")
	}
}

func TestAsUserOnlyInTestMode(t *testing.T) {
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "scope_test", Dir: sec01App(t)}}, Test: true})
	e.Cfg.Test = false
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		if err := c.testAsUser("someone@x.com"); err == nil {
			t.Error("testAsUser outside test mode should fail")
		}
		if c.User != "Admin" {
			t.Errorf("user changed to %q", c.User)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A switch left open — a test that ended inside asUser — is unwound before the
// test's savepoint is rolled back, so the next test starts as Admin.
func TestAsUserUnwoundAtRollback(t *testing.T) {
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "scope_test", Dir: sec01App(t)}}, Test: true})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		c.Flags["rollback"] = true
		rt, err := c.RT()
		if err != nil {
			return err
		}
		if err := c.Begin(); err != nil {
			return err
		}
		if err := c.testAsUser("a@x.com"); err != nil {
			return err
		}
		if err := rt.Ctx.(*Ctx).testAsUser("b@x.com"); err != nil {
			return err
		}
		inner := rt.Ctx.(*Ctx)
		if inner.User != "b@x.com" {
			t.Fatalf("switched to %q", inner.User)
		}
		root := inner.testRootCtx()
		if root != c || rt.Ctx != any(c) {
			t.Fatalf("not unwound to the test's ctx: %v", rt.Ctx.(*Ctx).User)
		}
		return root.RollbackTo()
	})
	if err != nil {
		t.Fatal(err)
	}
}
