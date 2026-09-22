package api

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const (
	searchUser       = "searcher@x.com"
	searchScopedUser = "search_scoped@x.com"
	searchPlainUser  = "search_plain@x.com"
)

func searchAPIApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Search Test", roles: ["Gestor", "Searcher", "Other"] });`)
	write("doctypes/search_project/search_project.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Search Project", idGeneration: { field: "project_name" }, titleField: "project_name",
  fields: [
    { fieldname: "project_name", fieldtype: "Data", label: "Project Name", reqd: true },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Search Line" },
  ],
  permissions: [{ role: "Searcher", read: true }] });`)
	write("doctypes/search_line/search_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Search Line", isChild: true, titleField: "line",
  fields: [{ fieldname: "line", fieldtype: "Data", label: "Line" }] });`)
	write("doctypes/search_note/search_note.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Search Note", idGeneration: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Searcher", read: true }] });`)
	write("doctypes/search_hidden/search_hidden.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Search Hidden", idGeneration: { field: "title" }, titleField: "title", globalSearch: false,
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Searcher", read: true }] });`)
	write("doctypes/search_private/search_private.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Search Private", idGeneration: { field: "title" }, titleField: "title",
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Other", read: true, share: true }] });`)
	return dir
}

func setupSearchAPI(t *testing.T) *env {
	t.Helper()
	x := setupApp(t, searchAPIApp(t))
	x.asAdmin(func(c *engine.Ctx) error {
		insert := func(doctype string, doc engine.Doc) error {
			d, err := c.NewDoc(doctype, doc)
			if err != nil {
				return err
			}
			_, err = c.Insert(d, engine.SaveOpts{})
			return err
		}
		for email, roles := range map[string][]any{
			searchUser:       {map[string]any{"role": "Searcher"}},
			searchScopedUser: {map[string]any{"role": "Searcher"}},
			searchPlainUser:  {},
		} {
			if err := insert("User", engine.Doc{"email": email, "full_name": email, "new_password": "segredo123", "roles": roles}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Café Alpha", "Alpha Centauri", "Alpha"} {
			if err := insert("Search Project", engine.Doc{"project_name": name, "lines": []any{map[string]any{"line": name + " line"}}}); err != nil {
				return err
			}
		}
		if err := insert("Search Note", engine.Doc{"title": "Alpha Note"}); err != nil {
			return err
		}
		if err := insert("Search Hidden", engine.Doc{"title": "Alpha Hidden"}); err != nil {
			return err
		}
		for _, title := range []string{"Alpha Private", "Alpha Secret"} {
			if err := insert("Search Private", engine.Doc{"title": title}); err != nil {
				return err
			}
		}
		if err := insert("User Permission", engine.Doc{"user": searchScopedUser, "allow": "Search Project", "for_value": "Alpha Centauri"}); err != nil {
			return err
		}
		_, err := c.ShareDoc("Search Private", "Alpha Private", searchPlainUser, engine.ShareRights{Read: true})
		return err
	})
	return x
}

func (x *env) globalSearch(user, txt string, limit string) resp {
	x.t.Helper()
	q := url.Values{"txt": {txt}}
	if limit != "" {
		q.Set("limit", limit)
	}
	return x.call("GET", "/api/search/global?"+q.Encode(), nil, "sid:"+x.sid(user))
}

func searchHits(t *testing.T, r resp) []string {
	t.Helper()
	if r.Status != 200 {
		t.Fatalf("search answered %d: %s", r.Status, r.Raw)
	}
	var out []string
	for _, h := range r.Body["data"].([]any) {
		m := h.(map[string]any)
		out = append(out, m["doctype"].(string)+"/"+m["id"].(string))
	}
	return out
}

func sameHits(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: hits = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: hits = %v, want %v", what, got, want)
		}
	}
}

func TestGlobalSearch_RanksAndRespectsAccess(t *testing.T) {
	x := setupSearchAPI(t)

	// exact, then prefix, then substring; child tables, DocTypes without a
	// title, opted-out DocTypes and unreadable DocTypes never show up
	sameHits(t, "searcher", searchHits(t, x.globalSearch(searchUser, "alpha", "")),
		"Search Project/Alpha", "Search Project/Alpha Centauri", "Search Project/Café Alpha")

	// accent-insensitive, both ways
	sameHits(t, "accents", searchHits(t, x.globalSearch(searchUser, "cafe", "")), "Search Project/Café Alpha")
	sameHits(t, "limit", searchHits(t, x.globalSearch(searchUser, "alpha", "1")), "Search Project/Alpha")

	// a user permission scope narrows the results
	sameHits(t, "scoped", searchHits(t, x.globalSearch(searchScopedUser, "alpha", "")), "Search Project/Alpha Centauri")

	// a share stands in for the missing role grant, for the shared document only
	sameHits(t, "shared", searchHits(t, x.globalSearch(searchPlainUser, "alpha", "")), "Search Private/Alpha Private")

	r := x.globalSearch(searchUser, "a", "")
	x.expect(r, 417, "ValidationError")
	x.expect(x.call("GET", "/api/search/global?txt=alpha", nil, ""), 401, "AuthenticationError")
}

// The per-DocType cap is a cap on what comes back, not on what is ranked: an
// exact match must survive a DocType with more matches than the cap.
func TestGlobalSearch_ExactMatchSurvivesTheCap(t *testing.T) {
	x := setupSearchAPI(t)
	x.asAdmin(func(c *engine.Ctx) error {
		// "beta" first, so `modified desc` puts every other match ahead of it
		for _, name := range []string{"beta", "beta one", "beta two", "beta three", "beta four", "beta five", "beta six"} {
			d, err := c.NewDoc("Search Project", engine.Doc{"project_name": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})

	hits := searchHits(t, x.globalSearch(searchUser, "beta", ""))
	if len(hits) == 0 || hits[0] != "Search Project/beta" {
		t.Errorf("the exact match should come first, got %v", hits)
	}
}

// What the user typed is a substring to look for, not a pattern: `%` and `_`
// match themselves.
func TestGlobalSearch_WildcardsAreLiteral(t *testing.T) {
	x := setupSearchAPI(t)
	x.asAdmin(func(c *engine.Ctx) error {
		for _, name := range []string{"100% pure", "100 ok", "a_b", "axb"} {
			d, err := c.NewDoc("Search Project", engine.Doc{"project_name": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})

	sameHits(t, "percent", searchHits(t, x.globalSearch(searchUser, "100%", "")), "Search Project/100% pure")
	sameHits(t, "underscore", searchHits(t, x.globalSearch(searchUser, "a_b", "")), "Search Project/a_b")
}
