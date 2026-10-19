package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// childTexts lists a child DocType's rows by their text, in order.
func childTexts(t *testing.T, c *Ctx, doctype string, args ListArgs) []string {
	t.Helper()
	args.Fields, args.OrderBy = []string{"text"}, "text asc"
	rows, err := c.GetList(doctype, args)
	if err != nil {
		t.Fatalf("GetList %s: %v", doctype, err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, db.Str(r["text"]))
	}
	return out
}

func wantTexts(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// setupChildList inserts the orders and the query documents of the char
// fixture, and returns the ids of the child rows by their text.
func setupChildList(t *testing.T) (*Engine, map[string]string) {
	t.Helper()
	e := setupChar(t)
	ids := map[string]string{}
	runAs(t, e, "Admin", func(c *Ctx) error {
		line := func(company, text string) map[string]any { return map[string]any{"company": company, "text": text} }
		note := func(text string) map[string]any { return map[string]any{"text": text} }
		for _, d := range []Doc{
			{"title": "OA", "company": "Alfa", "lines": []any{line("Alfa", "a1"), line("Beta", "a2")}, "notes": []any{note("na")}},
			{"title": "OB", "company": "Beta", "lines": []any{line("Beta", "b1")}, "notes": []any{note("nb")}},
			{"title": "OC", "company": "Alfa", "lines": []any{line("Alfa", "c1")}, "notes": []any{note("nc")}},
		} {
			if err := insertDoc(c, "Char Order", d); err != nil {
				return err
			}
		}
		for _, d := range []Doc{
			{"title": "Q1", "lines": []any{note("q1")}},
			{"title": "Hidden", "lines": []any{note("qh")}},
		} {
			if err := insertDoc(c, "Char Query Doc", d); err != nil {
				return err
			}
		}
		for _, dt := range []string{"Char Order Line", "Char Order Note", "Char Query Line"} {
			rows, err := c.GetList(dt, ListArgs{Fields: []string{"id", "text"}})
			if err != nil {
				return err
			}
			for _, r := range rows {
				ids[db.Str(r["text"])] = db.Str(r["id"])
			}
		}
		return nil
	})
	return e, ids
}

// A child listing holds the rows whose parent the user may list, by the
// parent's whole list filter, and nothing else (B02): a scope over a DocType
// the child has no Link to still hides the rows of parents out of scope.
func TestB02_ChildListingFollowsParentScopes(t *testing.T) {
	e, ids := setupChildList(t)
	runAs(t, e, charParty, func(c *Ctx) error {
		wantTexts(t, "notes", childTexts(t, c, "Char Order Note", ListArgs{}), "na", "nc")
		n, err := c.Count("Char Order Note", nil)
		if err != nil {
			return err
		}
		if n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
		// naming the parent does not reach past it
		wantTexts(t, "notes of OB", childTexts(t, c, "Char Order Note", ListArgs{Filters: map[string]any{"parent": "OB"}}))
		rows, err := c.GetList("Char Order Note", ListArgs{Fields: []string{"parent", "count(*) as n"}, GroupBy: "parent", OrderBy: "parent asc"})
		if err != nil {
			return err
		}
		if len(rows) != 2 || db.Str(rows[0]["parent"]) != "OA" || db.Str(rows[1]["parent"]) != "OC" {
			t.Fatalf("grouped by parent = %v, want OA and OC", rows)
		}
		_, err = c.GetDoc("Char Order Note", ids["nb"])
		wantPermissionError(t, "GetDoc nb (its parent is out of scope)", err)
		if _, err := c.GetDoc("Char Order Note", ids["na"]); err != nil {
			t.Fatalf("GetDoc na: %v", err)
		}
		return nil
	})
	// a rule applicable for the parent: rows of out-of-scope parents and rows
	// out of scope themselves are left out, listed and read alike
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		wantTexts(t, "notes", childTexts(t, c, "Char Order Note", ListArgs{}), "na", "nc")
		wantTexts(t, "lines", childTexts(t, c, "Char Order Line", ListArgs{}), "a1", "c1")
		if _, err := c.GetDoc("Char Order Line", ids["a1"]); err != nil {
			t.Fatalf("GetDoc a1: %v", err)
		}
		_, err := c.GetDoc("Char Order Line", ids["a2"])
		wantPermissionError(t, "GetDoc a2 (the row names Beta)", err)
		_, err = c.GetDoc("Char Order Line", ids["b1"])
		wantPermissionError(t, "GetDoc b1 (its parent is out of scope)", err)
		return nil
	})
	// a share bound by the scopes opens nothing out of scope
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Char Order", "OB", charOrderScoped, ShareRights{Read: true})
		return err
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		wantTexts(t, "lines (OB shared within the scopes)", childTexts(t, c, "Char Order Line", ListArgs{}), "a1", "c1")
		_, err := c.GetDoc("Char Order Line", ids["b1"])
		wantPermissionError(t, "GetDoc b1 (OB shared within the scopes)", err)
		return nil
	})
	// a share that overrides the scope opens the parent, and its rows with it
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Char Order", "OB", charOrderScoped, ShareRights{Read: true, OverrideScope: true})
		return err
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		wantTexts(t, "lines", childTexts(t, c, "Char Order Line", ListArgs{}), "a1", "b1", "c1")
		if _, err := c.GetDoc("Char Order Line", ids["b1"]); err != nil {
			t.Fatalf("GetDoc b1 (shared overriding the scope): %v", err)
		}
		return nil
	})
}

// The parent controller's permissionQuery narrows the child listing as it
// narrows the parent's.
func TestB02_ChildListingFollowsParentPermissionQuery(t *testing.T) {
	e, _ := setupChildList(t)
	runAs(t, e, charStaff, func(c *Ctx) error {
		sameNames(t, listNames(t, c, "Char Query Doc", ListArgs{}), "Q1")
		wantTexts(t, "lines", childTexts(t, c, "Char Query Line", ListArgs{}), "q1")
		return nil
	})
}

// A user without scopes lists every row of the parents the role reads, and
// Admin and an ignorePermissions read every row, as before.
func TestB02_ChildListingUnscopedUnchanged(t *testing.T) {
	e, _ := setupChildList(t)
	runAs(t, e, charPlain, func(c *Ctx) error {
		wantTexts(t, "lines", childTexts(t, c, "Char Order Line", ListArgs{}), "a1", "a2", "b1", "c1")
		wantTexts(t, "notes", childTexts(t, c, "Char Order Note", ListArgs{}), "na", "nb", "nc")
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		wantTexts(t, "lines", childTexts(t, c, "Char Order Line", ListArgs{}), "a1", "a2", "b1", "c1")
		wantTexts(t, "query lines", childTexts(t, c, "Char Query Line", ListArgs{}), "q1", "qh")
		return nil
	})
	runAs(t, e, charStaff, func(c *Ctx) error {
		wantTexts(t, "query lines (ignorePermissions)", childTexts(t, c, "Char Query Line", ListArgs{IgnorePermissions: true}), "q1", "qh")
		return nil
	})
}

// A scope over a tree, applicable for the parent, narrows the child listing to
// the rows of parents within the branch.
func TestTreeScopeChildListingFollowsParent(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	const user = "scoped@x.com"
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "User", Doc{"email": user, "full_name": "Scoped",
			"roles": []any{map[string]any{"role": "Tree User"}}}); err != nil {
			return err
		}
		for _, cl := range []struct{ title, territory string }{{"Inside", "PR"}, {"Outside", "Antarctica"}} {
			if err := insertDoc(c, "Test Client", Doc{"title": cl.title, "territory": cl.territory,
				"visits": []any{map[string]any{"territory": "PR"}}}); err != nil {
				return err
			}
		}
		return insertDoc(c, "User Permission", Doc{"user": user, "allow": "Test Territory", "for_value": "Brazil", "applicable_for": "Test Client"})
	})
	if err := e.Run(context.Background(), user, func(c *Ctx) error {
		rows, err := c.GetList("Test Visit", ListArgs{Fields: []string{"parent"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["parent"] != "Inside" {
			t.Fatalf("visits = %v, want Inside's only", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A Table field above level 0 hides its rows from a child listing and from a
// direct read, as it hides the field from the parent document (SEC-02); a user
// who reads that level sees them.
func TestSEC02_ChildRowsFollowTableFieldLevel(t *testing.T) {
	e := setupSEC02(t)
	var bonus string
	runAs(t, e, "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Employee Bonus", ListArgs{})
		if err != nil || len(rows) != 1 {
			t.Fatalf("Admin bonuses = %v, %v", rows, err)
		}
		bonus = Doc(rows[0]).ID()
		return nil
	})
	runAs(t, e, sec02Staff, func(c *Ctx) error {
		if names := listNames(t, c, "Employee Bonus", ListArgs{}); len(names) != 0 {
			t.Fatalf("level-0 user lists bonuses %v", names)
		}
		_, err := c.GetDoc("Employee Bonus", bonus)
		wantPermissionError(t, "GetDoc bonus at level 0", err)
		if names := listNames(t, c, "Employee Line", ListArgs{}); len(names) != 1 {
			t.Fatalf("level-0 user lists lines %v, want the one", names)
		}
		return nil
	})
	runAs(t, e, sec02HR, func(c *Ctx) error {
		sameNames(t, listNames(t, c, "Employee Bonus", ListArgs{}), bonus)
		if _, err := c.GetDoc("Employee Bonus", bonus); err != nil {
			t.Fatalf("GetDoc bonus at level 1: %v", err)
		}
		return nil
	})
}
