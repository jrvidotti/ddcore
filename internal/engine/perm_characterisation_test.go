package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// The permission characterisation tests pin how the permission code answers
// today, so that a refactor of perm.go can be checked against it. Where two
// paths disagree (the role name's case, a Dynamic Link selector's case, a
// parent listed though one of its rows is out of scope) the test pins the
// disagreement itself: a change to either side has to be a deliberate one.

// charApp is the fixture: one DocType per behaviour pinned.
func charApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	w := func(rel, src string) { writeAppFile(t, dir, rel, src) }
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "char_test", title: "Char Test",
  roles: ["Char Staff", "Char Reporter", "Char Owner", "Char Scoped", "Char Editor"] });`)
	w("doctypes/char_company/char_company.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Company", idGeneration: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Char Scoped", read: true }] });`)
	// the rights and the workflow name the role in lower case; users hold "Char Staff"
	w("doctypes/char_flow_doc/char_flow_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Flow Doc", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" },
  ],
  permissions: [{ role: "char staff", read: true, write: true, create: true }] });`)
	w("workflows/char_flow.workflow.ts", `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({
  name: "Char Flow",
  doctype: "Char Flow Doc",
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "char staff" },
    { state: "Review", docstatus: 0, allowEdit: "Char Staff" },
    { state: "Done", docstatus: 0, allowEdit: "" },
  ],
  transitions: [
    { state: "Draft", action: "Send", nextState: "Review", allowed: "Char Staff" },
    { state: "Draft", action: "Finish", nextState: "Done", allowed: "char staff" },
  ],
});`)
	// report without read; read only as owner
	w("doctypes/char_report_doc/char_report_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Report Doc", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Char Report Line" },
  ],
  permissions: [
    { role: "Char Reporter", report: true },
    { role: "Char Owner", read: true, create: true, ifOwner: true },
  ] });`)
	w("doctypes/char_report_line/char_report_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Report Line", isChild: true,
  fields: [{ fieldname: "text", fieldtype: "Data", label: "Text" }] });`)
	w("doctypes/char_owned_doc/char_owned_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Owned Doc", idGeneration: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Char Owner", read: true, write: true, create: true, ifOwner: true }] });`)
	w("doctypes/char_party_doc/char_party_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Party Doc", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "party_type", fieldtype: "Data", label: "Party Type" },
    { fieldname: "party", fieldtype: "Dynamic Link", label: "Party", options: "party_type" },
  ],
  permissions: [{ role: "Char Scoped", read: true }] });`)
	w("doctypes/char_order/char_order.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Order", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Char Company" },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Char Order Line" },
    { fieldname: "notes", fieldtype: "Table", label: "Notes", options: "Char Order Note" },
  ],
  permissions: [{ role: "Char Scoped", read: true }] });`)
	// a child table with no Link to the scoped DocType
	w("doctypes/char_order_note/char_order_note.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Order Note", isChild: true,
  fields: [{ fieldname: "text", fieldtype: "Data", label: "Text" }] });`)
	// a controller whose permissionQuery hides the "Hidden" documents from lists
	w("doctypes/char_query_doc/char_query_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Query Doc", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Char Query Line" },
  ],
  permissions: [{ role: "Char Staff", read: true }] });`)
	w("doctypes/char_query_doc/char_query_doc.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Char Query Doc", {
  permissionQuery(user) { return [["title", "!=", "Hidden"]]; },
});`)
	w("doctypes/char_query_line/char_query_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Query Line", isChild: true,
  fields: [{ fieldname: "text", fieldtype: "Data", label: "Text" }] });`)
	w("doctypes/char_order_line/char_order_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Order Line", isChild: true,
  fields: [
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Char Company" },
    { fieldname: "text", fieldtype: "Data", label: "Text" },
  ] });`)
	// a controller whose hasPermission fails on the "Boom" documents
	w("doctypes/char_hook_doc/char_hook_doc.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Char Hook Doc", idGeneration: { field: "title" }, submittable: true, allowRename: true,
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Char Editor", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true }] });`)
	w("doctypes/char_hook_doc/char_hook_doc.controller.ts", `import { defineController } from "@ddcore/sdk";
const boom: Record<string, string> = {
  "Boom Submit": "submit", "Boom Delete": "delete", "Boom Rename": "write",
  "Boom Amend": "amend", "Boom Merge": "delete",
};
export default defineController("Char Hook Doc", {
  hasPermission(doc, ptype, user) {
    if (doc && boom[doc.title] === ptype) throw new Error("permission hook exploded");
    return undefined;
  },
});`)
	return dir
}

const (
	charStaff       = "staff@x.com"
	charSMLower     = "smlower@x.com"
	charReporter    = "reporter@x.com"
	charMixed       = "mixed@x.com"
	charOwner1      = "owner1@x.com"
	charOwner2      = "owner2@x.com"
	charParty       = "party@x.com"
	charOrderScoped = "orderscoped@x.com"
	charEditor      = "editor@x.com"
	charPlain       = "plain@x.com"
)

func setupChar(t *testing.T) *Engine {
	t.Helper()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "char_test", Dir: charApp(t)}}, Test: true})
	runAs(t, e, "Admin", func(c *Ctx) error {
		// a Role whose name differs from "System Manager" only in case
		if err := insertDoc(c, "Role", Doc{"role_name": "system manager"}); err != nil {
			return err
		}
		users := map[string][]string{
			charStaff:       {"Char Staff"},
			charSMLower:     {"system manager"},
			charReporter:    {"Char Reporter"},
			charMixed:       {"Char Reporter", "Char Owner"},
			charOwner1:      {"Char Owner"},
			charOwner2:      {"Char Owner"},
			charParty:       {"Char Scoped"},
			charOrderScoped: {"Char Scoped"},
			charEditor:      {"Char Editor"},
			charPlain:       {"Char Scoped"},
		}
		for email, roles := range users {
			var rows []any
			for _, r := range roles {
				rows = append(rows, map[string]any{"role": r})
			}
			if err := insertDoc(c, "User", Doc{"email": email, "full_name": email, "roles": rows}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			if err := insertDoc(c, "Char Company", Doc{"title": name}); err != nil {
				return err
			}
		}
		for _, up := range []Doc{
			{"user": charParty, "allow": "Char Company", "for_value": "Alfa"},
			{"user": charOrderScoped, "allow": "Char Company", "for_value": "Alfa", "applicable_for": "Char Order"},
		} {
			if err := insertDoc(c, "User Permission", up); err != nil {
				return err
			}
		}
		return nil
	})
	return e
}

// insertOwned inserts a document titled title as Admin and hands it to owner.
func insertOwned(t *testing.T, e *Engine, doctype, title, owner string) {
	t.Helper()
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, doctype, Doc{"title": title}); err != nil {
			return err
		}
		_, err := c.DBSet(doctype, title, Doc{"owner": owner}, false)
		return err
	})
}

func wantAllowed(t *testing.T, c *Ctx, doctype, ptype string, doc Doc, want bool) {
	t.Helper()
	ok, err := c.HasPermission(doctype, ptype, doc)
	if err != nil {
		t.Fatalf("HasPermission(%s, %s): %v", doctype, ptype, err)
	}
	if ok != want {
		t.Fatalf("HasPermission(%s, %s, %v) = %v, want %v", doctype, ptype, doc.ID(), ok, want)
	}
}

// KNOWN GAP (pinned, not desired): DocType rights match a role name ignoring
// case (containsFold), while the workflow (allowEdit, transitions) and every
// System Manager check go through HasRole, which matches it exactly. One rule
// for both is the desired answer; tracked separately, fix deliberately.
func TestPermChar_MixedCaseRoleNames(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		return insertDoc(c, "Char Flow Doc", Doc{"title": "F1"})
	})
	runAs(t, e, charStaff, func(c *Ctx) error {
		doc, err := c.GetDoc("Char Flow Doc", "F1")
		if err != nil {
			t.Fatalf("read through a rights row naming the role in another case: %v", err)
		}
		wantAllowed(t, c, "Char Flow Doc", "read", doc, true)
		wantAllowed(t, c, "Char Flow Doc", "write", nil, true)
		// KNOWN GAP: Draft's allowEdit is "char staff", which HasRole does not match
		wantAllowed(t, c, "Char Flow Doc", "write", doc, false)
		actions, err := c.AvailableWorkflowActions("Char Flow Doc", doc)
		if err != nil {
			return err
		}
		var names []string
		for _, a := range actions {
			names = append(names, a.Action)
		}
		if !reflect.DeepEqual(names, []string{"Send"}) {
			t.Fatalf("workflow actions = %v; pinned current behaviour: only the exact-case transition [Send]", names)
		}
		if _, err := c.ApplyWorkflowTransition("Char Flow Doc", "F1", "Finish"); err == nil {
			t.Fatalf("Finish (allowed: \"char staff\") succeeded; pinned current behaviour: refused")
		}
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Char Flow Doc", "F1", "Send")
		return err
	})
	runAs(t, e, charStaff, func(c *Ctx) error {
		doc, err := c.GetDoc("Char Flow Doc", "F1")
		if err != nil {
			return err
		}
		// Review's allowEdit is "Char Staff", exactly the role held
		wantAllowed(t, c, "Char Flow Doc", "write", doc, true)
		return nil
	})
	runAs(t, e, charSMLower, func(c *Ctx) error {
		// the Role DocType's "System Manager" row is matched ignoring case...
		wantAllowed(t, c, "Role", "write", nil, true)
		wantAllowed(t, c, "Role", "create", nil, true)
		// ...KNOWN GAP: the System Manager checks are not
		if c.HasRole("System Manager") {
			t.Fatalf("HasRole(System Manager) matched a role named %q; pinned current behaviour: no match", "system manager")
		}
		ok, err := c.CanOverrideScope()
		if err != nil {
			return err
		}
		if ok {
			t.Fatalf("CanOverrideScope for a %q role = true; pinned current behaviour: false", "system manager")
		}
		return nil
	})
}

// KNOWN GAP (pinned, not desired): a Dynamic Link is restricted when its
// selector names the allowed DocType, but the memory check compares the
// selector ignoring case and the SQL filter exactly. A selector in another case
// is therefore listed but not readable. The desired answer is one comparison
// for both, so the list holds [P1] alone; tracked separately, fix deliberately.
func TestPermChar_DynamicLinkSelectorCase(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, d := range []Doc{
			{"title": "P1", "party_type": "Char Company", "party": "Alfa"},
			{"title": "P2", "party_type": "Char Company", "party": "Beta"},
		} {
			if err := insertDoc(c, "Char Party Doc", d); err != nil {
				return err
			}
		}
		// written past validation: a selector naming the DocType in another case
		if err := insertDoc(c, "Char Party Doc", Doc{"title": "P3", "party_type": "Char Company", "party": "Beta"}); err != nil {
			return err
		}
		_, err := c.Q().Exec(c.Ctx, `UPDATE tab_char_party_doc SET party_type = 'char company' WHERE id = 'P3'`)
		return err
	})
	runAs(t, e, charParty, func(c *Ctx) error {
		// KNOWN GAP: P3 is listed though GetDoc refuses it below
		if got := listNames(t, c, "Char Party Doc", ListArgs{}); !reflect.DeepEqual(got, []string{"P1", "P3"}) {
			t.Fatalf("list = %v; pinned current behaviour: [P1 P3]", got)
		}
		if _, err := c.GetDoc("Char Party Doc", "P1"); err != nil {
			t.Fatalf("GetDoc P1: %v", err)
		}
		_, err := c.GetDoc("Char Party Doc", "P2")
		wantPermissionError(t, "GetDoc P2", err)
		_, err = c.GetDoc("Char Party Doc", "P3")
		wantPermissionError(t, "GetDoc P3 (selector in another case)", err)
		return nil
	})
}

// A rule applicable for a parent DocType is checked on the parent's child rows
// when the parent is read, and a listing of the child DocType follows it: a row
// is listed only when its parent passes the parent's list filter (b1, of OB,
// does not) and the row itself passes the rules for the parent (a2 names
// Beta). KNOWN GAP (pinned, not desired): a parent whose own rows break the
// rule is listed but not readable (OA). Tracked separately; fix deliberately.
func TestPermChar_ParentScopedRuleOnChildListing(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, d := range []Doc{
			{"title": "OA", "company": "Alfa", "lines": []any{
				map[string]any{"company": "Alfa", "text": "a1"}, map[string]any{"company": "Beta", "text": "a2"}}},
			{"title": "OB", "company": "Beta", "lines": []any{map[string]any{"company": "Beta", "text": "b1"}}},
			{"title": "OC", "company": "Alfa", "lines": []any{map[string]any{"company": "Alfa", "text": "c1"}}},
		} {
			if err := insertDoc(c, "Char Order", d); err != nil {
				return err
			}
		}
		return nil
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		if got := listNames(t, c, "Char Order", ListArgs{}); !reflect.DeepEqual(got, []string{"OA", "OC"}) {
			t.Fatalf("list = %v; pinned current behaviour: [OA OC]", got)
		}
		if _, err := c.GetDoc("Char Order", "OC"); err != nil {
			t.Fatalf("GetDoc OC: %v", err)
		}
		// KNOWN GAP: listed, yet refused: its row a2 names Beta, and the rule applies to it
		_, err := c.GetDoc("Char Order", "OA")
		wantPermissionError(t, "GetDoc OA (a child row out of scope)", err)
		_, err = c.GetDoc("Char Order", "OB")
		wantPermissionError(t, "GetDoc OB", err)

		rows, err := c.GetList("Char Order Line", ListArgs{Fields: []string{"text"}, OrderBy: "text asc"})
		if err != nil {
			return err
		}
		var texts []string
		for _, r := range rows {
			texts = append(texts, db.Str(r["text"]))
		}
		if !reflect.DeepEqual(texts, []string{"a1", "c1"}) {
			t.Fatalf("child listing = %v, want [a1 c1]", texts)
		}
		return nil
	})
	// a rule with no applicable_for filters the child's own Link instead
	runAs(t, e, charParty, func(c *Ctx) error {
		rows, err := c.GetList("Char Order Line", ListArgs{Fields: []string{"text"}, OrderBy: "text asc"})
		if err != nil {
			return err
		}
		var texts []string
		for _, r := range rows {
			texts = append(texts, db.Str(r["text"]))
		}
		if !reflect.DeepEqual(texts, []string{"a1", "c1"}) {
			t.Fatalf("child listing = %v, want [a1 c1]", texts)
		}
		return nil
	})
}

// ifOwner rows: the doctype-level answer is yes, the document-level answer is
// ownership, and the listing filters on the owner.
func TestPermChar_IfOwnerRows(t *testing.T) {
	e := setupChar(t)
	// KNOWN GAP (pinned, not desired): Insert checks create (doc.go, Insert's
	// HasPermission(create)) before it stamps the new document's owner, so an
	// ifOwner create row never lets its holder create; Insert's submit check on
	// a document inserted submitted runs before the stamp too, the same gap for
	// an ifOwner submit row. The desired answer is that the creator counts as
	// the owner. Tracked separately; fix deliberately.
	runAs(t, e, charOwner1, func(c *Ctx) error {
		wantPermissionError(t, "create through an ifOwner row (pinned current behaviour: refused)", insertDoc(c, "Char Owned Doc", Doc{"title": "O0"}))
		return nil
	})
	insertOwned(t, e, "Char Owned Doc", "O1", charOwner1)
	insertOwned(t, e, "Char Owned Doc", "O2", charOwner2)
	runAs(t, e, charOwner1, func(c *Ctx) error {
		d, err := c.St.DocType("Char Owned Doc")
		if err != nil {
			return err
		}
		if !c.readIsOwnerOnly(d) {
			t.Fatalf("readIsOwnerOnly = false, want true")
		}
		pf, err := c.permissionFilters(d)
		if err != nil {
			return err
		}
		if want := []db.Filter{{Field: "owner", Op: "=", Value: charOwner1}}; !reflect.DeepEqual(pf, want) {
			t.Fatalf("permissionFilters = %#v, want %#v", pf, want)
		}
		for _, tc := range []struct {
			doc  Doc
			want bool
		}{{nil, true}, {Doc{"id": "O1", "owner": charOwner1}, true}, {Doc{"id": "O2", "owner": charOwner2}, false}} {
			got, err := c.grantedBy(d, "read", tc.doc)
			if err != nil {
				return err
			}
			if got != tc.want {
				t.Fatalf("grantedBy(read, %v) = %v, want %v", tc.doc.ID(), got, tc.want)
			}
		}
		sameNames(t, listNames(t, c, "Char Owned Doc", ListArgs{}), "O1")
		_, err = c.GetDoc("Char Owned Doc", "O2")
		wantPermissionError(t, "GetDoc O2", err)
		return nil
	})
}

// A role with report but not read never lists the DocType on its own; with one
// document shared, the listing must hold that document and nothing else, as
// HasPermission(read) answers — and the same for the DocType's child rows.
func TestPermChar_ReportOnlyRoleWithShareListsOnlyShared(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, n := range []string{"R1", "R2", "R3"} {
			if err := insertDoc(c, "Char Report Doc", Doc{"title": n, "lines": []any{map[string]any{"text": n + "-line"}}}); err != nil {
				return err
			}
		}
		return nil
	})
	runAs(t, e, charReporter, func(c *Ctx) error {
		_, err := c.GetList("Char Report Doc", ListArgs{})
		wantPermissionError(t, "GetList without a share", err)
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Char Report Doc", "R2", charReporter, ShareRights{Read: true})
		return err
	})
	runAs(t, e, charReporter, func(c *Ctx) error {
		wantAllowed(t, c, "Char Report Doc", "read", Doc{"id": "R1", "owner": "Admin"}, false)
		wantAllowed(t, c, "Char Report Doc", "read", Doc{"id": "R2", "owner": "Admin"}, true)
		sameNames(t, listNames(t, c, "Char Report Doc", ListArgs{}), "R2")
		if _, err := c.GetDoc("Char Report Doc", "R2"); err != nil {
			t.Fatalf("GetDoc R2: %v", err)
		}
		_, err := c.GetDoc("Char Report Doc", "R1")
		wantPermissionError(t, "GetDoc R1", err)
		// a share does not open a listing of the child rows, here as for a
		// user holding no role on the parent at all
		_, err = c.GetList("Char Report Line", ListArgs{Fields: []string{"text"}})
		wantPermissionError(t, "GetList Char Report Line", err)
		return nil
	})
	// report beside an ifOwner read: the listing is the owner's, as is reading
	insertOwned(t, e, "Char Report Doc", "RM", charMixed)
	runAs(t, e, charMixed, func(c *Ctx) error {
		wantAllowed(t, c, "Char Report Doc", "read", Doc{"id": "R1", "owner": "Admin"}, false)
		sameNames(t, listNames(t, c, "Char Report Doc", ListArgs{}), "RM")
		// a parent read only as owner lists no child rows: a policy choice (a
		// ParentFilter could express it), so their listing is refused
		_, err := c.GetList("Char Report Line", ListArgs{Fields: []string{"text"}})
		wantPermissionError(t, "GetList Char Report Line (ifOwner read + report)", err)
		return nil
	})
}

// A user with no read row and no share gets a list filter matching nothing:
// permissionFilters fails closed whatever its caller checked before it.
func TestPermChar_NoReadNoShareFiltersMatchNothing(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, charReporter, func(c *Ctx) error {
		d, err := c.St.DocType("Char Report Doc")
		if err != nil {
			return err
		}
		pf, err := c.permissionFilters(d)
		if err != nil {
			return err
		}
		if want := []db.Filter{{Any: [][]db.Filter{}}}; !reflect.DeepEqual(pf, want) {
			t.Fatalf("permissionFilters = %#v, want %#v", pf, want)
		}
		return nil
	})
}

// An error from HasPermission — here a controller's hasPermission throwing —
// surfaces as that error, never as a refusal.
func TestPermError_HasPermissionErrorPropagates(t *testing.T) {
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, n := range []string{"Boom Delete", "Boom Rename", "Boom Merge", "Merge Target"} {
			if err := insertDoc(c, "Char Hook Doc", Doc{"title": n}); err != nil {
				return err
			}
		}
		doc, err := c.NewDoc("Char Hook Doc", Doc{"title": "Boom Amend"})
		if err != nil {
			return err
		}
		if doc, err = c.Insert(doc, SaveOpts{}); err != nil {
			return err
		}
		if doc, err = c.Submit(doc); err != nil {
			return err
		}
		_, err = c.Cancel(doc)
		return err
	})
	wantHookError := func(what string, err error) {
		t.Helper()
		if err == nil || cerr.From(err).Type == "PermissionError" || !strings.Contains(err.Error(), "permission hook exploded") {
			t.Fatalf("%s: expected the hook's error, got %v", what, err)
		}
	}
	ctx := context.Background()
	for _, tc := range []struct {
		what string
		run  func(c *Ctx) error
	}{
		{"insert submitted", func(c *Ctx) error {
			_, err := c.Insert(Doc{"doctype": "Char Hook Doc", "title": "Boom Submit", "docstatus": 1}, SaveOpts{})
			return err
		}},
		{"delete", func(c *Ctx) error { return c.Delete("Char Hook Doc", "Boom Delete", false, false) }},
		{"rename", func(c *Ctx) error {
			_, err := c.Rename("Char Hook Doc", "Boom Rename", "Renamed")
			return err
		}},
		{"amend", func(c *Ctx) error {
			_, err := c.Amend("Char Hook Doc", "Boom Amend")
			return err
		}},
		{"merge", func(c *Ctx) error { return c.Merge("Char Hook Doc", "Boom Merge", "Merge Target") }},
	} {
		wantHookError(tc.what, e.Run(ctx, charEditor, tc.run))
	}
}
