package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// getDoc with IgnorePermissions skips the role grant, as getList and Save do,
// and keeps the user's scopes and the DocTypes closed to scoped users (#69).
func TestGetDocIgnorePermissionsKeepsScopes(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const (
		roleless = "roleless@x.com"
		scoped   = "scoped_roleless@x.com"
	)
	var scopeRow string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, user := range []string{roleless, scoped} {
			u, err := c.NewDoc("User", Doc{"email": user, "full_name": user})
			if err != nil {
				return err
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, _ := c.NewDoc("Test Company", Doc{"title": name})
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
			rec, _ := c.NewDoc("Test Record", Doc{"title": name + " Record", "company": name})
			if _, err := c.Insert(rec, SaveOpts{}); err != nil {
				return err
			}
		}
		perm, _ := c.NewDoc("User Permission", Doc{"user": scoped, "allow": "Test Company", "for_value": "Alfa"})
		saved, err := c.Insert(perm, SaveOpts{})
		scopeRow = saved.ID()
		return err
	}); err != nil {
		t.Fatal(err)
	}

	isPerm := func(err error) bool { return err != nil && cerr.From(err).Type == "PermissionError" }

	if err := e.Run(ctx, roleless, func(c *Ctx) error {
		if _, err := c.GetDoc("Test Record", "Beta Record"); !isPerm(err) {
			t.Errorf("GetDoc without a role: expected PermissionError, got %v", err)
		}
		doc, err := c.GetDocOpts("Test Record", "Beta Record", GetOpts{IgnorePermissions: true})
		if err != nil {
			t.Errorf("GetDocOpts IgnorePermissions without a role: %v", err)
		} else if doc.Str("company") != "Beta" {
			t.Errorf("GetDocOpts returned %v", doc)
		}
		if _, err := c.GetDocOpts("Test Record", "Nope", GetOpts{IgnorePermissions: true}); err == nil || cerr.From(err).Type != "DoesNotExistError" {
			t.Errorf("missing document: expected DoesNotExistError, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, scoped, func(c *Ctx) error {
		if _, err := c.GetDocOpts("Test Record", "Alfa Record", GetOpts{IgnorePermissions: true}); err != nil {
			t.Errorf("in-scope read with IgnorePermissions: %v", err)
		}
		if _, err := c.GetDocOpts("Test Record", "Beta Record", GetOpts{IgnorePermissions: true}); !isPerm(err) {
			t.Errorf("out-of-scope read with IgnorePermissions: expected PermissionError, got %v", err)
		}
		if _, err := c.GetDocOpts("User Permission", scopeRow, GetOpts{IgnorePermissions: true}); !isPerm(err) {
			t.Errorf("User Permission read by a scoped user with IgnorePermissions: expected PermissionError, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

const getDocOptsSuite = `import "@ddcore/sdk/test";
const roleless = "roleless@x.com";
const scoped = "scoped_roleless@x.com";

describe("getDoc ignorePermissions", () => {
  it("skips the role grant and keeps the scope", () => {
    for (const title of ["Alfa", "Beta"]) {
      ddcore.newDoc("Test Company", { title }).insert();
      ddcore.newDoc("Test Record", { title: title + " Record", company: title }).insert();
    }
    ddcore.newDoc("User", { email: roleless, full_name: "Roleless" }).insert();
    ddcore.newDoc("User", { email: scoped, full_name: "Scoped" }).insert();
    ddcore.newDoc("User Permission", { user: scoped, allow: "Test Company", for_value: "Alfa" }).insert();
    ddcore.test.asUser(roleless, () => {
      expect(() => ddcore.getDoc("Test Record", "Beta Record")).toThrow();
      const doc = ddcore.getDoc("Test Record", "Beta Record", { ignorePermissions: true });
      expect(doc.company).toBe("Beta");
      doc.title = "Beta Record";
      doc.save({ ignorePermissions: true });
      expect(doc.reload({ ignorePermissions: true }).company).toBe("Beta");
      expect(() => doc.reload()).toThrow();
    });
    ddcore.test.asUser(scoped, () => {
      expect(ddcore.getDoc("Test Record", "Alfa Record", { ignorePermissions: true }).company).toBe("Alfa");
      expect(() => ddcore.getDoc("Test Record", "Beta Record", { ignorePermissions: true })).toThrow();
    });
  });
});`

func TestGetDocIgnorePermissionsFromJS(t *testing.T) {
	dir := sec01App(t)
	if err := os.WriteFile(filepath.Join(dir, "doctypes/test_record/test_record.test.ts"), []byte(getDocOptsSuite), 0o644); err != nil {
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
