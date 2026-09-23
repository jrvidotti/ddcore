package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// portalApp is the OPS-10 fixture: members linked to users, the person each
// member points at, and documents members file about themselves.
func portalApp(t *testing.T, portalSrc string) string {
	t.Helper()
	dir := t.TempDir()
	writeAppFile(t, dir, "ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "portal_test", title: "Portal Test", roles: ["Member", "Clerk"] });`)
	writeAppFile(t, dir, "doctypes/portal_person/portal_person.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Portal Person", titleField: "person_name",
  fields: [
    { fieldname: "person_name", fieldtype: "Data", label: "Name", reqd: true },
    { fieldname: "phone", fieldtype: "Data", label: "Phone" },
    { fieldname: "tax_id", fieldtype: "Data", label: "Tax ID" },
  ],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	writeAppFile(t, dir, "doctypes/portal_member/portal_member.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Portal Member",
  fields: [
    { fieldname: "person", fieldtype: "Link", label: "Person", options: "Portal Person", reqd: true },
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User" },
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Active", "Inactive"], default: "Active" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary" },
  ],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	writeAppFile(t, dir, "doctypes/portal_kind/portal_kind.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Portal Kind", idGeneration: { field: "kind_name" },
  fields: [{ fieldname: "kind_name", fieldtype: "Data", label: "Kind", reqd: true }],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	writeAppFile(t, dir, "doctypes/portal_doc/portal_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Portal Doc", titleField: "title",
  fields: [
    { fieldname: "member", fieldtype: "Link", label: "Member", options: "Portal Member", reqd: true },
    { fieldname: "person_name", fieldtype: "Data", label: "Person", fetchFrom: "member.person", readOnly: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "kind", fieldtype: "Link", label: "Kind", options: "Portal Kind" },
    { fieldname: "file", fieldtype: "Attach", label: "File" },
    { fieldname: "review", fieldtype: "Data", label: "Review" },
  ],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	writeAppFile(t, dir, "services/portal.ts", `import { whitelisted } from "@ddcore/sdk";
export const defaults = whitelisted(() => ({ title: "Default title", review: "not editable" }), { portal: true });
export const deskOnly = whitelisted(() => ({ ok: true }));
export const invite = whitelisted((args: any) => ddcore.users.invite(args), { roles: ["Clerk"] });
export const myDocs = whitelisted(() => ddcore.db.getList("Portal Doc", { fields: ["id", "title"] }), { portal: true });`)
	if portalSrc == "" {
		portalSrc = portalFixture
	}
	writeAppFile(t, dir, "portal/members.portal.ts", portalSrc)
	return dir
}

const portalFixture = `import { definePortal } from "@ddcore/sdk";
export default definePortal({
  name: "Members", title: "Member Portal", roles: ["Member"],
  identity: { doctype: "Portal Member", userField: "user", filters: { status: "Active" } },
  pages: [
    { name: "me", label: "My data", kind: "record", doctype: "Portal Person", match: { id: "person" },
      fields: ["person_name", "phone"], actions: [{ label: "File a document", page: "docs", new: true }] },
    { name: "docs", label: "My documents", doctype: "Portal Doc", match: { member: "id" },
      fields: ["title", "kind", "file", "person_name", "review"], editable: ["title", "kind", "file"], create: true,
      defaultsMethod: "portal_test.services.portal.defaults" },
  ],
});`

const (
	portalAlice = "alice@x.com"
	portalBob   = "bob@x.com"
	portalCarol = "carol@x.com" // a Member whose member record is inactive
	portalClerk = "clerk@x.com" // a desk user
)

type portalFixtureIDs struct {
	alicePerson, bobPerson, aliceMember, bobMember, aliceDoc, bobDoc string
}

func setupPortal(t *testing.T) (*Engine, portalFixtureIDs) {
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
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "portal_test", Dir: portalApp(t, "")}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		e.DB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	var ids portalFixtureIDs
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		users := []struct {
			email, typ string
			roles      []string
		}{
			{portalAlice, "Website User", []string{"Member"}},
			{portalBob, "Website User", []string{"Member"}},
			{portalCarol, "Website User", []string{"Member"}},
			{portalClerk, "System User", []string{"Clerk", "Member"}},
		}
		for _, u := range users {
			var rows []any
			for _, r := range u.roles {
				rows = append(rows, map[string]any{"role": r})
			}
			d, err := c.NewDoc("User", Doc{"email": u.email, "full_name": u.email, "user_type": u.typ, "roles": rows})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		ins := func(doctype string, v Doc) (string, error) {
			d, err := c.NewDoc(doctype, v)
			if err != nil {
				return "", err
			}
			d, err = c.Insert(d, SaveOpts{})
			if err != nil {
				return "", err
			}
			return d.ID(), nil
		}
		if _, err := ins("Portal Kind", Doc{"kind_name": "ID card"}); err != nil {
			return err
		}
		if ids.alicePerson, err = ins("Portal Person", Doc{"person_name": "Alice", "tax_id": "111"}); err != nil {
			return err
		}
		if ids.bobPerson, err = ins("Portal Person", Doc{"person_name": "Bob", "tax_id": "222"}); err != nil {
			return err
		}
		carolPerson, err := ins("Portal Person", Doc{"person_name": "Carol"})
		if err != nil {
			return err
		}
		if ids.aliceMember, err = ins("Portal Member", Doc{"person": ids.alicePerson, "user": portalAlice, "salary": 10}); err != nil {
			return err
		}
		if ids.bobMember, err = ins("Portal Member", Doc{"person": ids.bobPerson, "user": portalBob}); err != nil {
			return err
		}
		if _, err = ins("Portal Member", Doc{"person": carolPerson, "user": portalCarol, "status": "Inactive"}); err != nil {
			return err
		}
		if ids.aliceDoc, err = ins("Portal Doc", Doc{"member": ids.aliceMember, "title": "Alice's"}); err != nil {
			return err
		}
		ids.bobDoc, err = ins("Portal Doc", Doc{"member": ids.bobMember, "title": "Bob's"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e, ids
}

func portalAs(t *testing.T, e *Engine, user string, fn func(c *Ctx) error) error {
	t.Helper()
	return e.Run(context.Background(), user, fn)
}

func isPermissionErr(err error) bool {
	ce := cerr.From(err)
	return ce != nil && ce.Status == 403
}

func TestPortal_WebsiteUserReachesOnlyOwnRecords(t *testing.T) {
	e, ids := setupPortal(t)
	err := portalAs(t, e, portalAlice, func(c *Ctx) error {
		if !c.IsWebsiteUser() || !c.PortalMode() {
			t.Fatalf("alice should be a Website User in portal mode")
		}
		rows, err := c.GetList("Portal Doc", ListArgs{Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || db.Str(rows[0]["id"]) != ids.aliceDoc {
			t.Fatalf("alice lists %v, want only her document", rows)
		}
		if _, err := c.GetDoc("Portal Doc", ids.bobDoc); !isPermissionErr(err) {
			t.Fatalf("alice read Bob's document: %v", err)
		}
		if _, err := c.GetDoc("Portal Person", ids.alicePerson); err != nil {
			t.Fatalf("alice cannot read her own person: %v", err)
		}
		if _, err := c.GetDoc("Portal Person", ids.bobPerson); !isPermissionErr(err) {
			t.Fatalf("alice read Bob's person: %v", err)
		}
		// no page shows Portal Member itself, so the identity is not readable
		if _, err := c.GetDoc("Portal Member", ids.aliceMember); !isPermissionErr(err) {
			t.Fatalf("alice read her member record, which no page grants: %v", err)
		}
		if ok, _ := c.HasPermission("Portal Kind", "read", nil); ok {
			t.Fatalf("a DocType no page names must be closed")
		}
		// a page grants create but not write, delete or submit
		doc, _ := c.GetDoc("Portal Doc", ids.aliceDoc)
		for _, p := range []string{"write", "delete", "submit", "share", "export"} {
			if ok, _ := c.HasPermission("Portal Doc", p, doc); ok {
				t.Fatalf("the page grants %s it does not declare", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_CreateIsBoundToIdentity(t *testing.T) {
	e, ids := setupPortal(t)
	err := portalAs(t, e, portalAlice, func(c *Ctx) error {
		d, _ := c.NewDoc("Portal Doc", Doc{"member": ids.bobMember, "title": "forged"})
		if _, err := c.Insert(d, SaveOpts{}); !isPermissionErr(err) {
			t.Fatalf("alice filed a document under Bob: %v", err)
		}
		d, _ = c.NewDoc("Portal Doc", Doc{"member": ids.aliceMember, "title": "mine", "kind": "ID card"})
		d, err := c.Insert(d, SaveOpts{})
		if err != nil {
			// fetchFrom and link validation read Portal Member and Portal Kind,
			// which no page grants: they must stay elevated
			t.Fatalf("alice could not file her own document: %v", err)
		}
		if d.Str("person_name") != ids.alicePerson {
			t.Fatalf("fetchFrom did not run: %v", d["person_name"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_NoIdentityNoAccess(t *testing.T) {
	e, _ := setupPortal(t)
	err := portalAs(t, e, portalCarol, func(c *Ctx) error {
		rows, err := c.GetList("Portal Doc", ListArgs{Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("an inactive member sees %v", rows)
		}
		n, err := c.Count("Portal Person", nil)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("an inactive member counts %d people", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_DeskUserInPortalModeSeesOwnOnly(t *testing.T) {
	e, _ := setupPortal(t)
	err := portalAs(t, e, portalClerk, func(c *Ctx) error {
		if c.IsWebsiteUser() {
			t.Fatalf("the clerk is a System User")
		}
		rows, err := c.GetList("Portal Doc", ListArgs{Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("on the desk the clerk sees every document, got %d", len(rows))
		}
		c.SetPortalMode(true)
		rows, err = c.GetList("Portal Doc", ListArgs{Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("in the portal the clerk has no member record and sees %d", len(rows))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_AttachmentsClaimedOnSave(t *testing.T) {
	e, ids := setupPortal(t)
	newFile := func(c *Ctx, url string) {
		t.Helper()
		f, _ := c.NewDoc("File", Doc{"file_name": "x.pdf", "file_url": url, "is_private": true})
		if _, err := c.Insert(f, SaveOpts{IgnorePermissions: true}); err != nil {
			t.Fatal(err)
		}
	}
	var docID string
	err := portalAs(t, e, portalAlice, func(c *Ctx) error {
		newFile(c, "/private/files/alice.pdf")
		d, _ := c.NewDoc("Portal Doc", Doc{"member": ids.aliceMember, "title": "with file", "file": "/private/files/alice.pdf"})
		d, err := c.Insert(d, SaveOpts{})
		docID = d.ID()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = portalAs(t, e, portalAlice, func(c *Ctx) error {
		newFile(c, "/private/files/alice-draft.pdf")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = portalAs(t, e, portalBob, func(c *Ctx) error {
		// Bob names Alice's detached file: it is not his to claim
		d, _ := c.NewDoc("Portal Doc", Doc{"member": ids.bobMember, "title": "steal", "file": "/private/files/alice-draft.pdf"})
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(context.Background(), "Admin", func(c *Ctx) error {
		f, err := c.GetValues("File", map[string]any{"file_url": "/private/files/alice.pdf"}, FilePermFields)
		if err != nil {
			return err
		}
		if db.Str(f["attached_to_id"]) != docID || db.Str(f["attached_to_field"]) != "file" {
			t.Fatalf("alice's file was not attached on save: %v", f)
		}
		f, _ = c.GetValues("File", map[string]any{"file_url": "/private/files/alice-draft.pdf"}, FilePermFields)
		if db.Str(f["attached_to_id"]) != "" {
			t.Fatalf("bob claimed alice's detached file: %v", f)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// the clerk reviews: the file follows the document now
	err = portalAs(t, e, portalClerk, func(c *Ctx) error {
		f, _ := c.GetValues("File", map[string]any{"file_url": "/private/files/alice.pdf"}, FilePermFields)
		if !c.CanReadFile(f) {
			t.Fatalf("a reviewer who reads the document cannot read its file")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = portalAs(t, e, portalBob, func(c *Ctx) error {
		f, _ := c.GetValues("File", map[string]any{"file_url": "/private/files/alice.pdf"}, FilePermFields)
		if c.CanReadFile(f) {
			t.Fatalf("bob can read alice's file")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_WhitelistedMethodRunsInPortalMode(t *testing.T) {
	e, ids := setupPortal(t)
	err := portalAs(t, e, portalAlice, func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		raw, err := rt.CallWhitelisted("portal_test.services.portal.myDocs", []byte(`{}`))
		if err != nil {
			return err
		}
		if !strings.Contains(string(raw), ids.aliceDoc) || strings.Contains(string(raw), ids.bobDoc) {
			t.Fatalf("a portal method's getList saw %s", raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_InviteRules(t *testing.T) {
	e, _ := setupPortal(t)
	err := portalAs(t, e, portalClerk, func(c *Ctx) error {
		if _, err := e.InviteUser(c, Invitation{Email: "sys@x.com", FullName: "Sys", UserType: "System User"}); !isPermissionErr(err) {
			t.Fatalf("a clerk invited a System User: %v", err)
		}
		if _, err := e.InviteUser(c, Invitation{Email: "sm@x.com", FullName: "SM", Roles: []string{"System Manager"}}); !isPermissionErr(err) {
			t.Fatalf("a clerk handed out System Manager: %v", err)
		}
		out, err := e.InviteUser(c, Invitation{Email: "new@x.com", FullName: "New", Roles: []string{"Member"}})
		if err != nil {
			return err
		}
		if out["user"] != "new@x.com" {
			t.Fatalf("unexpected result %v", out)
		}
		if typ := e.UserType(c, "new@x.com"); typ != "Website User" {
			t.Fatalf("a clerk's invitation defaults to a Website User, got %q", typ)
		}
		if _, err := e.InviteUser(c, Invitation{Email: "new@x.com", FullName: "New"}); err == nil {
			t.Fatalf("invited the same address twice")
		}
		if _, err := e.ResendInvite(c, "new@x.com"); err != nil {
			t.Fatalf("resend to a Website User: %v", err)
		}
		if _, err := e.ResendInvite(c, portalClerk); !isPermissionErr(err) {
			t.Fatalf("a clerk resent a System User's invitation: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = portalAs(t, e, "Admin", func(c *Ctx) error {
		_, err := e.InviteUser(c, Invitation{Email: "desk@x.com", FullName: "Desk", Roles: []string{"Clerk"}})
		if err != nil {
			return err
		}
		if typ := e.UserType(c, "desk@x.com"); typ != "System User" {
			t.Fatalf("an administrator's invitation defaults to a System User, got %q", typ)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortal_LoadValidation(t *testing.T) {
	cases := []struct {
		name, portal, want string
	}{
		{"unknown field", `fields: ["nope"]`, "is not a field"},
		{"editable not shown", `fields: ["title"], editable: ["kind"], create: true`, "not in fields"},
		{"editable match", `fields: ["title", "member"], editable: ["member"], create: true`, "cannot be editable"},
		{"read-only editable", `fields: ["title", "person_name"], editable: ["person_name"], create: true`, "read-only"},
		{"create without editable", `fields: ["title"], create: true`, "needs editable fields"},
		{"desk-only defaults", `fields: ["title"], editable: ["title"], create: true, defaultsMethod: "portal_test.services.portal.deskOnly"`, "portal: true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `import { definePortal } from "@ddcore/sdk";
export default definePortal({ name: "Bad", roles: ["Member"], identity: { doctype: "Portal Member", userField: "user" },
  pages: [{ name: "docs", label: "Docs", doctype: "Portal Doc", match: { member: "id" }, ` + tc.portal + ` }] });`
			e, err := New(context.Background(), Config{DSN: testDSN, Apps: []js.App{{Name: "portal_test", Dir: portalApp(t, src)}}, Test: true})
			if err == nil {
				e.DB.Close()
				t.Fatalf("loaded a portal with %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q, want it to mention %q", err, tc.want)
			}
		})
	}
}
