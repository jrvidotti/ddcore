package engine

import (
	"testing"
)

// setupScopeChildRows inserts orders whose child rows sit in and out of the
// Char Company scope, and returns their titles.
func setupScopeChildRows(t *testing.T) (*Engine, []string) {
	t.Helper()
	e := setupChar(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		line := func(company, text string) map[string]any {
			r := map[string]any{"text": text}
			if company != "" {
				r["company"] = company
			}
			return r
		}
		for _, d := range []Doc{
			// a row naming Beta under a parent naming Alfa
			{"title": "OA", "company": "Alfa", "lines": []any{line("Alfa", "a1"), line("Beta", "a2")}},
			{"title": "OB", "company": "Beta", "lines": []any{line("Beta", "b1")}},
			{"title": "OC", "company": "Alfa", "lines": []any{line("Alfa", "c1")}},
			// a row with no company: out of scope, as on a direct read
			{"title": "OD", "company": "Alfa", "lines": []any{line("", "d1")}},
			// no rows at all, only a table without a Link to the scope
			{"title": "OE", "company": "Alfa", "notes": []any{map[string]any{"text": "ne"}}},
		} {
			if err := insertDoc(c, "Char Order", d); err != nil {
				return err
			}
		}
		return nil
	})
	return e, []string{"OA", "OB", "OC", "OD", "OE"}
}

// listedIsReadable fails unless the parent list holds exactly the documents a
// direct read opens (#150).
func listedIsReadable(t *testing.T, c *Ctx, who string, titles []string, args ListArgs) {
	t.Helper()
	listed := map[string]bool{}
	for _, n := range listNames(t, c, "Char Order", args) {
		listed[n] = true
	}
	for _, n := range titles {
		_, err := c.GetDoc("Char Order", n)
		if readable := err == nil; readable != listed[n] {
			t.Fatalf("%s: %s listed = %v, readable = %v (%v)", who, n, listed[n], readable, err)
		}
	}
}

// A parent list checks the Link fields of every child row against the user's
// scopes, as a direct read does: a document holding a row out of scope is
// neither read nor listed (#150).
func TestScopeParentListChecksChildRows(t *testing.T) {
	e, titles := setupScopeChildRows(t)
	for _, user := range []string{charOrderScoped, charParty} {
		runAs(t, e, user, func(c *Ctx) error {
			sameNames(t, listNames(t, c, "Char Order", ListArgs{}), "OC", "OE")
			listedIsReadable(t, c, user, titles, ListArgs{})
			n, err := c.Count("Char Order", nil)
			if err != nil {
				return err
			}
			if n != 2 {
				t.Fatalf("%s: count = %d, want 2", user, n)
			}
			// ignorePermissions skips the roles, not the scopes
			sameNames(t, listNames(t, c, "Char Order", ListArgs{IgnorePermissions: true}), "OC", "OE")
			// a child listing holds only the rows of the parents listed, and a
			// row of a parent out of reach is not read on its own either
			wantTexts(t, user+" lines", childTexts(t, c, "Char Order Line", ListArgs{}), "c1")
			wantTexts(t, user+" notes", childTexts(t, c, "Char Order Note", ListArgs{}), "ne")
			return nil
		})
	}
	// users without scopes, and Admin, list every order as before
	for _, user := range []string{charPlain, "Admin"} {
		runAs(t, e, user, func(c *Ctx) error {
			sameNames(t, listNames(t, c, "Char Order", ListArgs{}), titles...)
			return nil
		})
	}
}

// A row of a parent the user cannot read is refused when read on its own, as
// the child listing leaves it out (#150).
func TestScopeChildRowOfUnreadableParentRefused(t *testing.T) {
	e, _ := setupScopeChildRows(t)
	ids := map[string]string{}
	runAs(t, e, "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Char Order Line", ListArgs{Fields: []string{"id", "text"}})
		for _, r := range rows {
			ids[r["text"].(string)] = r["id"].(string)
		}
		return err
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		// a1 names Alfa, but its sibling a2 puts OA out of scope
		_, err := c.GetDoc("Char Order Line", ids["a1"])
		wantPermissionError(t, "GetDoc a1 (OA holds a row out of scope)", err)
		if _, err := c.GetDoc("Char Order Line", ids["c1"]); err != nil {
			t.Fatalf("GetDoc c1: %v", err)
		}
		return nil
	})
}

// A share bound by the scopes opens no parent whose rows break them; a share
// overriding the scopes lists and opens it (#150).
func TestScopeParentListChildRowsAndShares(t *testing.T) {
	e, titles := setupScopeChildRows(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Char Order", "OA", charOrderScoped, ShareRights{Read: true})
		return err
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		sameNames(t, listNames(t, c, "Char Order", ListArgs{}), "OC", "OE")
		listedIsReadable(t, c, "shared within the scopes", titles, ListArgs{})
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.ShareDoc("Char Order", "OA", charOrderScoped, ShareRights{Read: true, OverrideScope: true})
		return err
	})
	runAs(t, e, charOrderScoped, func(c *Ctx) error {
		sameNames(t, listNames(t, c, "Char Order", ListArgs{}), "OA", "OC", "OE")
		listedIsReadable(t, c, "shared overriding the scopes", titles, ListArgs{})
		return nil
	})
}
