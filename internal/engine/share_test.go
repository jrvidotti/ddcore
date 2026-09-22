package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// shareApp is the SEC-03 fixture: a note three roles reach differently, and a
// company the scoped user is limited to.
func writeAppFile(t *testing.T, dir, rel, src string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func shareApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeAppFile(t, dir, "ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "share_test", title: "Share Test", roles: ["Note Editor", "Note Reader", "Scoped Staff"] });`)
	writeAppFile(t, dir, "doctypes/share_company/share_company.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Share Company", idGeneration: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scoped Staff", read: true }, { role: "Note Editor", read: true }] });`)
	writeAppFile(t, dir, "doctypes/shared_note/shared_note.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Shared Note", idGeneration: { field: "title" }, allowRename: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Share Company" },
    { fieldname: "body", fieldtype: "Data", label: "Body" },
    { fieldname: "secret", fieldtype: "Data", label: "Secret", permlevel: 1 },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Shared Note Line" },
  ],
  permissions: [
    { role: "Note Editor", read: true, write: true, create: true, delete: true, share: true, report: true, export: true },
    { role: "Note Editor", permlevel: 1, read: true, write: true },
    { role: "Note Reader", read: true, share: true },
    { role: "Scoped Staff", read: true, write: true, delete: true, report: true, export: true },
  ] });`)
	writeAppFile(t, dir, "doctypes/shared_note/shared_note.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Shared Note", {
  hasPermission(doc, ptype, user) {
    if (doc && doc.title === "Vetoed" && ptype === "write") return false;
    return undefined;
  },
});`)
	writeAppFile(t, dir, "doctypes/shared_note_line/shared_note_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Shared Note Line", isChild: true,
  fields: [{ fieldname: "text", fieldtype: "Data", label: "Text" }] });`)
	return dir
}

const (
	shareSM      = "sm@x.com"
	shareEditor  = "editor@x.com"
	shareReader  = "reader@x.com"
	sharePlain   = "plain@x.com"
	shareScoped  = "scoped@x.com"
	shareOutside = "outside@x.com"
)

func setupShare(t *testing.T) *Engine {
	t.Helper()
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	for _, q := range []string{"DROP DATABASE IF EXISTS " + dbName, "CREATE DATABASE " + dbName} {
		if _, err := e0.DB.Pool.Exec(ctx, q); err != nil {
			e0.DB.Close()
			t.Fatal(err)
		}
	}
	e0.DB.Close()
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "share_test", Dir: shareApp(t)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		e.DB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		users := map[string][]string{
			shareSM:      {"System Manager", "Note Editor"},
			shareEditor:  {"Note Editor"},
			shareReader:  {"Note Reader"},
			sharePlain:   nil,
			shareScoped:  {"Scoped Staff"},
			shareOutside: {"Scoped Staff"},
		}
		for email, roles := range users {
			var rows []any
			for _, r := range roles {
				rows = append(rows, map[string]any{"role": r})
			}
			u, err := c.NewDoc("User", Doc{"email": email, "full_name": email, "roles": rows})
			if err != nil {
				return err
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			if err := insertDoc(c, "Share Company", Doc{"title": name}); err != nil {
				return err
			}
		}
		for _, n := range []Doc{
			{"title": "N1", "company": "Alfa", "secret": "s1", "lines": []any{map[string]any{"text": "l1"}}},
			{"title": "N2", "company": "Beta", "secret": "s2"},
			{"title": "N3", "company": "Beta"},
			{"title": "Vetoed", "company": "Alfa"},
		} {
			if err := insertDoc(c, "Shared Note", n); err != nil {
				return err
			}
		}
		for _, user := range []string{shareScoped, shareOutside} {
			if err := insertDoc(c, "User Permission", Doc{"user": user, "allow": "Share Company", "for_value": "Alfa"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func insertDoc(c *Ctx, doctype string, values Doc) error {
	doc, err := c.NewDoc(doctype, values)
	if err != nil {
		return err
	}
	_, err = c.Insert(doc, SaveOpts{})
	return err
}

func runAs(t *testing.T, e *Engine, user string, fn func(c *Ctx) error) {
	t.Helper()
	if err := e.Run(context.Background(), user, fn); err != nil {
		t.Fatalf("as %s: %v", user, err)
	}
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected status %d, got success", status)
	}
	if got := cerr.From(err).Status; got != status {
		t.Fatalf("expected status %d, got %d: %v", status, got, err)
	}
}

func listNames(t *testing.T, c *Ctx, doctype string, args ListArgs) []string {
	t.Helper()
	if args.Fields == nil {
		args.Fields = []string{"id"}
	}
	args.OrderBy = "id asc"
	rows, err := c.GetList(doctype, args)
	if err != nil {
		t.Fatalf("GetList %s: %v", doctype, err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r["id"].(string))
	}
	return out
}

func sameNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("names = %v, want %v", got, want)
		}
	}
}

func auditCount(t *testing.T, e *Engine, action, outcome string) int64 {
	t.Helper()
	n, err := e.CountAuditEvents(context.Background(), AuditFilter{Action: action, Outcome: outcome})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestShare_GrantsReadWriteAndRevokes(t *testing.T) {
	e := setupShare(t)

	// without a role or a share, nothing
	runAs(t, e, sharePlain, func(c *Ctx) error {
		_, err := c.GetDoc("Shared Note", "N1")
		wantStatus(t, err, 403)
		_, err = c.GetList("Shared Note", ListArgs{})
		wantStatus(t, err, 403)
		return nil
	})

	runAs(t, e, shareEditor, func(c *Ctx) error {
		s, err := c.ShareDoc("Shared Note", "N1", sharePlain, ShareRights{})
		if err != nil {
			return err
		}
		if !s.Read || s.Write || s.Share || s.OverrideScope {
			t.Fatalf("share = %+v, want read only", s)
		}
		return nil
	})

	runAs(t, e, sharePlain, func(c *Ctx) error {
		if _, err := c.GetDoc("Shared Note", "N1"); err != nil {
			t.Fatalf("shared read: %v", err)
		}
		_, err := c.GetDoc("Shared Note", "N2")
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1")
		if n, err := c.Count("Shared Note", nil); err != nil || n != 1 {
			t.Fatalf("count = %d, %v", n, err)
		}
		links, err := c.LinkSearch("Shared Note", "", nil, 10)
		if err != nil || len(links) != 1 {
			t.Fatalf("link search = %v, %v", links, err)
		}
		// the share is level 0: a permlevel 1 column is not readable
		if acc := c.FieldAccess(c.St.Meta.DocTypes["Shared Note"]); acc.CanReadLevel(1) {
			t.Fatal("a share must not grant permlevel 1")
		}
		// child rows are reached through the parent, not listed on their own
		doc, _ := c.GetDoc("Shared Note", "N1")
		if len(doc.Children("lines")) != 1 {
			t.Fatalf("lines = %v", doc["lines"])
		}
		// read only: no write, no re-share, no delete
		doc["body"] = "changed"
		_, err = c.Save(doc, SaveOpts{})
		wantStatus(t, err, 403)
		_, err = c.ShareDoc("Shared Note", "N1", shareReader, ShareRights{})
		wantStatus(t, err, 403)
		wantStatus(t, c.Delete("Shared Note", "N1", false, false), 403)
		// a notification says so
		page, err := c.ListNotifications(10, 0, nil)
		if err != nil || page.Total != 1 {
			t.Fatalf("notifications = %+v, %v", page, err)
		}
		return nil
	})

	// upgrade to write and share
	runAs(t, e, shareEditor, func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "N1", sharePlain, ShareRights{Write: true, Share: true})
		return err
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		doc, err := c.GetDoc("Shared Note", "N1")
		if err != nil {
			return err
		}
		doc["body"] = "changed"
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			t.Fatalf("shared write: %v", err)
		}
		// may re-share, but never override a scope
		if _, err := c.ShareDoc("Shared Note", "N1", shareReader, ShareRights{Write: true}); err != nil {
			t.Fatalf("re-share: %v", err)
		}
		_, err = c.ShareDoc("Shared Note", "N1", shareScoped, ShareRights{OverrideScope: true})
		wantStatus(t, err, 403)
		return nil
	})

	// a read-only sharer cannot hand out write
	runAs(t, e, shareReader, func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "N2", sharePlain, ShareRights{Write: true})
		wantStatus(t, err, 403)
		return nil
	})

	// the controller still vetoes a shared write (it vetoes the editor too, so
	// only Admin can hand that write out)
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "Vetoed", sharePlain, ShareRights{Write: true})
		return err
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		doc, err := c.GetDoc("Shared Note", "Vetoed")
		if err != nil {
			return err
		}
		doc["body"] = "x"
		_, err = c.Save(doc, SaveOpts{})
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "Vetoed")
		return nil
	})

	// only a sharer or the recipient sees and removes the rows
	runAs(t, e, shareReader, func(c *Ctx) error {
		list, err := c.ListDocShares("Shared Note", "N1")
		if err != nil {
			return err
		}
		if !list.CanShare || list.CanOverrideScope || len(list.Shares) != 2 {
			t.Fatalf("list = %+v", list)
		}
		return nil
	})

	runAs(t, e, shareEditor, func(c *Ctx) error {
		return c.UnshareDoc("Shared Note", "N1", sharePlain)
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		_, err := c.GetDoc("Shared Note", "N1")
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "Vetoed")
		page, err := c.ListNotifications(10, 0, nil)
		if err != nil {
			return err
		}
		for _, n := range page.Data {
			if n.ReferenceID == "N1" {
				t.Fatal("a revoked share's notification is still listed")
			}
		}
		// the recipient may drop their own share
		return c.UnshareDoc("Shared Note", "Vetoed", sharePlain)
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		_, err := c.GetList("Shared Note", ListArgs{})
		wantStatus(t, err, 403)
		return nil
	})

	if n := auditCount(t, e, "permission.share_grant", "Allowed"); n != 3 {
		t.Fatalf("share_grant allowed = %d", n)
	}
	if n := auditCount(t, e, "permission.share_update", "Allowed"); n != 1 {
		t.Fatalf("share_update = %d", n)
	}
	if n := auditCount(t, e, "permission.share_revoke", "Allowed"); n != 2 {
		t.Fatalf("share_revoke = %d", n)
	}
	if n := auditCount(t, e, "permission.share_grant", "Denied"); n != 3 {
		t.Fatalf("share_grant denied = %d", n)
	}
}

func TestShare_ScopesAndOverride(t *testing.T) {
	e := setupShare(t)

	runAs(t, e, shareScoped, func(c *Ctx) error {
		_, err := c.GetDoc("Shared Note", "N2")
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "Vetoed")
		return nil
	})

	// a plain share does not lift the scope
	runAs(t, e, shareEditor, func(c *Ctx) error {
		if _, err := c.ShareDoc("Shared Note", "N2", shareScoped, ShareRights{Write: true}); err != nil {
			return err
		}
		// and an editor who is not a System Manager cannot override it
		_, err := c.ShareDoc("Shared Note", "N2", shareScoped, ShareRights{OverrideScope: true})
		wantStatus(t, err, 403)
		return nil
	})
	runAs(t, e, shareScoped, func(c *Ctx) error {
		_, err := c.GetDoc("Shared Note", "N2")
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "Vetoed")
		return nil
	})

	// an unscoped System Manager can
	runAs(t, e, shareSM, func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "N2", shareScoped, ShareRights{Write: true, OverrideScope: true})
		return err
	})
	runAs(t, e, shareScoped, func(c *Ctx) error {
		doc, err := c.GetDoc("Shared Note", "N2")
		if err != nil {
			t.Fatalf("override read: %v", err)
		}
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "N2", "Vetoed")
		if ok, err := c.Exists("Shared Note", "N2"); err != nil || !ok {
			t.Fatalf("exists = %v, %v", ok, err)
		}
		if ok, err := c.Exists("Shared Note", "N3"); err != nil || ok {
			t.Fatalf("exists N3 = %v, %v", ok, err)
		}
		if v, err := c.GetValues("Shared Note", "N2", []string{"title"}); err != nil || v == nil {
			t.Fatalf("getValue = %v, %v", v, err)
		}
		doc["body"] = "override write"
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			t.Fatalf("override write: %v", err)
		}
		if _, err := c.DBSet("Shared Note", "N2", Doc{"body": "dbset"}, false); err != nil {
			t.Fatalf("override dbSet: %v", err)
		}
		// delete is never shared, and the scope still refuses it
		wantStatus(t, c.Delete("Shared Note", "N2", false, false), 403)
		// a scoped user cannot administer shares directly
		_, err = c.GetList(shareDoctype, ListArgs{})
		wantStatus(t, err, 403)
		return nil
	})
	// another scoped user is unaffected
	runAs(t, e, shareOutside, func(c *Ctx) error {
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "Vetoed")
		return nil
	})

	runAs(t, e, shareSM, func(c *Ctx) error {
		return c.UnshareDoc("Shared Note", "N2", shareScoped)
	})
	runAs(t, e, shareScoped, func(c *Ctx) error {
		_, err := c.GetDoc("Shared Note", "N2")
		wantStatus(t, err, 403)
		sameNames(t, listNames(t, c, "Shared Note", ListArgs{}), "N1", "Vetoed")
		if ok, err := c.Exists("Shared Note", "N2"); err != nil || ok {
			t.Fatalf("exists after revoke = %v, %v", ok, err)
		}
		return nil
	})
}

func TestShare_RefusalsRenameAndDelete(t *testing.T) {
	e := setupShare(t)
	runAs(t, e, shareEditor, func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "N1", shareEditor, ShareRights{})
		wantStatus(t, err, 417)
		_, err = c.ShareDoc("Shared Note", "N1", "nobody@x.com", ShareRights{})
		wantStatus(t, err, 417)
		_, err = c.ShareDoc("Shared Note", "missing", sharePlain, ShareRights{})
		wantStatus(t, err, 404)
		_, err = c.ShareDoc("Shared Note", "N1", sharePlain, ShareRights{})
		return err
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("User Permission", "x", sharePlain, ShareRights{})
		wantStatus(t, err, 417)
		if _, err := c.Rename("Shared Note", "N1", "N1-renamed"); err != nil {
			return err
		}
		return nil
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		if _, err := c.GetDoc("Shared Note", "N1-renamed"); err != nil {
			t.Fatalf("share did not follow the rename: %v", err)
		}
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		return c.Delete("Shared Note", "N1-renamed", false, false)
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		n, err := c.Count(shareDoctype, nil)
		if err != nil || n != 0 {
			t.Fatalf("shares after delete = %d, %v", n, err)
		}
		return nil
	})
	runAs(t, e, sharePlain, func(c *Ctx) error {
		_, err := c.GetList("Shared Note", ListArgs{})
		wantStatus(t, err, 403)
		return nil
	})
}

// A right the caller misspelled is a mistake, not a silent no: the server SDK
// refuses an unknown key the way the endpoint does, or an app would think it
// granted an override it never granted.
func TestShare_ServerSDKRefusesUnknownRight(t *testing.T) {
	e := setupShare(t)
	runAs(t, e, shareEditor, func(c *Ctx) error {
		rt := &js.Runtime{Ctx: c}
		raw := `{"doctype":"Shared Note","name":"N1","user":"` + sharePlain + `","rights":{"override_scope":true}}`
		_, err := e.HostCall(rt, "share.add", json.RawMessage(raw))
		wantStatus(t, err, 417)
		return nil
	})
}
