package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// treeApp is a hierarchy (Test Territory), a DocType linking into it
// (Test Client, with a child table that links too) and one role, so the same
// fixture serves the integrity, query and scope tests.
func treeApp(t *testing.T) string {
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
export default defineApp({ name: "tree_test", title: "Tree Test", roles: ["Tree User"] });`)
	write("doctypes/test_territory/test_territory.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Territory", isTree: true, allowRename: true,
  idGeneration: { field: "title" }, titleField: "title",
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Tree User", read: true, create: true, write: true, delete: true }] });`)
	write("doctypes/test_client/test_client.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Client", idGeneration: { field: "title" }, titleField: "title",
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "territory", fieldtype: "Link", label: "Territory", options: "Test Territory" },
    { fieldname: "visits", fieldtype: "Table", label: "Visits", options: "Test Visit" },
  ],
  permissions: [{ role: "Tree User", read: true, create: true, write: true, delete: true }] });`)
	write("doctypes/test_visit/test_visit.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Visit", isChild: true,
  fields: [{ fieldname: "territory", fieldtype: "Link", label: "Territory", options: "Test Territory" }] });`)
	return dir
}

func setupTree(t *testing.T) *Engine {
	t.Helper()
	return migratedEngine(t, Config{Apps: []js.App{{Name: "tree_test", Dir: treeApp(t)}}, Test: true})
}

// node inserts a territory; parent may be empty for a root.
func node(c *Ctx, title, parent string, group bool) error {
	doc, err := c.NewDoc("Test Territory", Doc{"title": title, "parent_test_territory": parent, "is_group": group})
	if err != nil {
		return err
	}
	_, err = c.Insert(doc, SaveOpts{})
	return err
}

// seedTree builds:  World > (Brazil > (South > PR), Chile), Antarctica
func seedTree(t *testing.T, e *Engine) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, n := range []struct {
			title, parent string
			group         bool
		}{
			{"World", "", true},
			{"Brazil", "World", true},
			{"South", "Brazil", true},
			{"PR", "South", false},
			{"Chile", "World", false},
			{"Antarctica", "", false},
		} {
			if err := node(c, n.title, n.parent, n.group); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func ids(rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, Doc(r).ID())
	}
	return out
}

func TestTreeMetaInjectsFields(t *testing.T) {
	e := setupTree(t)
	d, ok := e.Meta.Get("Test Territory")
	if !ok {
		t.Fatal("Test Territory did not load")
	}
	if d.TreeParentField() != "parent_test_territory" {
		t.Fatalf("parent field = %q", d.TreeParentField())
	}
	if f := d.Field("parent_test_territory"); f == nil || f.Fieldtype != "Link" {
		t.Fatalf("parent field missing from the loaded meta")
	}
	if f := d.Field("is_group"); f == nil || f.Fieldtype != "Check" {
		t.Fatalf("is_group missing from the loaded meta")
	}
}

func TestTreeIntegrityOnWrite(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()

	t.Run("parent must be a group", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error { return node(c, "Curitiba", "PR", false) })
		if err == nil || !strings.Contains(cerr.From(err).Error(), "not a group") {
			t.Fatalf("a leaf accepted children: %v", err)
		}
	})

	t.Run("parent must exist", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error { return node(c, "Nowhere", "Atlantis", false) })
		if err == nil {
			t.Fatal("a missing parent was accepted")
		}
	})

	t.Run("no self parent", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", "Brazil")
			if err != nil {
				return err
			}
			doc["parent_test_territory"] = "Brazil"
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err == nil {
			t.Fatal("a document became its own parent")
		}
	})

	t.Run("no cycle", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", "Brazil")
			if err != nil {
				return err
			}
			doc["parent_test_territory"] = "South" // South is Brazil's own descendant
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err == nil || !strings.Contains(cerr.From(err).Error(), "descendant") {
			t.Fatalf("a cycle was accepted: %v", err)
		}
	})

	t.Run("a group with children stays a group", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", "South")
			if err != nil {
				return err
			}
			doc["is_group"] = false
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err == nil || !strings.Contains(cerr.From(err).Error(), "stay a group") {
			t.Fatalf("a populated group became a leaf: %v", err)
		}
	})

	t.Run("an empty group may become a leaf", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			if err := node(c, "Empty Group", "World", true); err != nil {
				return err
			}
			doc, err := c.GetDoc("Test Territory", "Empty Group")
			if err != nil {
				return err
			}
			doc["is_group"] = false
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err != nil {
			t.Fatalf("an empty group could not become a leaf: %v", err)
		}
	})

	t.Run("a legitimate move is allowed", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", "Chile")
			if err != nil {
				return err
			}
			doc["parent_test_territory"] = "Brazil"
			if _, err := c.Save(doc, SaveOpts{}); err != nil {
				return err
			}
			doc["parent_test_territory"] = "World"
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err != nil {
			t.Fatalf("move refused: %v", err)
		}
	})
}

func TestTreeDeleteWithChildren(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	for _, force := range []bool{false, true} {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			return c.Delete("Test Territory", "South", true, force)
		})
		if err == nil {
			t.Fatalf("deleted a node with children (force=%v)", force)
		}
		if !strings.Contains(cerr.From(err).Error(), "child nodes") {
			t.Fatalf("force=%v: generic error instead of the tree one: %v", force, err)
		}
	}
	// the leaf first, then its parent
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		if err := c.Delete("Test Territory", "PR", true, false); err != nil {
			return err
		}
		return c.Delete("Test Territory", "South", true, false)
	}); err != nil {
		t.Fatalf("deleting bottom-up: %v", err)
	}
}

func TestTreeDBSetRefusesCycle(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		_, err := c.DBSet("Test Territory", "Brazil", Doc{"parent_test_territory": "PR"}, false)
		return err
	})
	if err == nil {
		t.Fatal("dbSet wrote a cycle")
	}
}

func TestTreeRenameMovesChildren(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Rename("Test Territory", "Brazil", "Brasil")
		return err
	}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Test Territory", "South")
		if err != nil {
			return err
		}
		if got := doc.Str("parent_test_territory"); got != "Brasil" {
			t.Fatalf("child still points at %q", got)
		}
		rows, err := c.GetList("Test Territory", ListArgs{
			Filters: []any{[]any{"id", "descendants of", "Brasil"}}, OrderBy: "id asc"})
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("descendants after rename = %v", ids(rows))
		}
		return nil
	}); err != nil {
		t.Fatalf("after rename: %v", err)
	}
}

func TestTreeQueryOperators(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		cl, err := c.NewDoc("Test Client", Doc{"title": "Acme", "territory": "PR",
			"visits": []any{map[string]any{"territory": "Chile"}}})
		if err != nil {
			return err
		}
		_, err = c.Insert(cl, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}

	cases := []struct {
		name    string
		doctype string
		filter  []any
		want    []string
	}{
		{"descendants", "Test Territory", []any{"id", "descendants of", "Brazil"}, []string{"PR", "South"}},
		{"descendants inclusive", "Test Territory", []any{"id", "descendants of (inclusive)", "Brazil"}, []string{"Brazil", "PR", "South"}},
		{"ancestors", "Test Territory", []any{"id", "ancestors of", "PR"}, []string{"Brazil", "South", "World"}},
		{"not descendants keeps roots", "Test Territory", []any{"id", "not descendants of", "World"}, []string{"Antarctica", "World"}},
		{"link column", "Test Client", []any{"territory", "descendants of (inclusive)", "Brazil"}, []string{"Acme"}},
		{"link column outside", "Test Client", []any{"territory", "descendants of", "Antarctica"}, nil},
		{"child table", "Test Client", []any{"Test Visit.territory", "descendants of", "World"}, []string{"Acme"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.Run(ctx, "Admin", func(c *Ctx) error {
				rows, err := c.GetList(tc.doctype, ListArgs{Filters: []any{tc.filter}, OrderBy: "id asc"})
				if err != nil {
					return err
				}
				got := strings.Join(ids(rows), ",")
				if want := strings.Join(tc.want, ","); got != want {
					t.Fatalf("got [%s], want [%s]", got, want)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		})
	}
}

// Rows made cyclic behind the engine's back (raw SQL, or a hierarchy that
// predates isTree) must not hang a query.
func TestTreeQueryTerminatesOnCyclicData(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx,
		`UPDATE tab_test_territory SET parent_test_territory = 'PR' WHERE id = 'Brazil'`); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Test Territory", ListArgs{
			Filters: []any{[]any{"id", "descendants of", "Brazil"}}, OrderBy: "id asc"})
		if err != nil {
			return err
		}
		// the walk closes the loop and stops: Brazil is reachable from itself
		if got := strings.Join(ids(rows), ","); got != "Brazil,PR,South" {
			t.Fatalf("descendants of a cyclic branch = %s", got)
		}
		rows, err = c.GetList("Test Territory", ListArgs{
			Filters: []any{[]any{"id", "ancestors of", "PR"}}, OrderBy: "id asc"})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			t.Fatal("ancestors of a cyclic branch returned nothing")
		}
		return nil
	}); err != nil {
		t.Fatalf("query over cyclic data: %v", err)
	}
}

// Two transactions moving A under B and B under A at the same time: the
// advisory lock makes them queue, so at most one cycle-free move survives and
// neither deadlocks.
func TestTreeConcurrentOppositeMoves(t *testing.T) {
	e := setupTree(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		if err := node(c, "A", "", true); err != nil {
			return err
		}
		return node(c, "B", "", true)
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	move := func(child, parent string) error {
		return e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", child)
			if err != nil {
				return err
			}
			doc["parent_test_territory"] = parent
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = move("A", "B") }()
	go func() { defer wg.Done(); errs[1] = move("B", "A") }()
	wg.Wait()
	if errs[0] == nil && errs[1] == nil {
		t.Fatalf("both moves succeeded: the hierarchy is now a cycle")
	}
	if errs[0] != nil && errs[1] != nil {
		t.Fatalf("both moves failed: %v / %v", errs[0], errs[1])
	}
}

// A User Permission over a hierarchy covers the branch below the value it
// names: the scoped user sees Brazil and everything in it, and nothing else.
func TestTreeScopeCoversDescendants(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	const user = "scoped@x.com"
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": user, "full_name": "Scoped",
			"roles": []any{map[string]any{"role": "Tree User"}}})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, cl := range []struct{ title, territory string }{
			{"Inside", "PR"}, {"Outside", "Antarctica"},
		} {
			doc, err := c.NewDoc("Test Client", Doc{"title": cl.title, "territory": cl.territory})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		p, err := c.NewDoc("User Permission", Doc{"user": user, "allow": "Test Territory", "for_value": "Brazil"})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("list of the tree itself", func(t *testing.T) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			rows, err := c.GetList("Test Territory", ListArgs{OrderBy: "id asc"})
			if err != nil {
				return err
			}
			if got := strings.Join(ids(rows), ","); got != "Brazil,PR,South" {
				t.Fatalf("visible territories = %s", got)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("direct read", func(t *testing.T) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			if _, err := c.GetDoc("Test Territory", "PR"); err != nil {
				t.Fatalf("a descendant of the allowed node was refused: %v", err)
			}
			if _, err := c.GetDoc("Test Territory", "Antarctica"); err == nil {
				t.Fatal("a node outside the branch was readable")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("link search", func(t *testing.T) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			rows, err := c.LinkSearch("Test Territory", "", nil, 20)
			if err != nil {
				return err
			}
			for _, r := range rows {
				if id := Doc(r).ID(); id == "Antarctica" || id == "World" {
					t.Fatalf("link search offered %s", id)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a Link into the tree is filtered", func(t *testing.T) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			rows, err := c.GetList("Test Client", ListArgs{OrderBy: "id asc"})
			if err != nil {
				return err
			}
			if got := strings.Join(ids(rows), ","); got != "Inside" {
				t.Fatalf("visible clients = %s", got)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("insert under an allowed node", func(t *testing.T) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			return node(c, "Parana Norte", "South", false)
		}); err != nil {
			t.Fatalf("inserting inside the branch was refused: %v", err)
		}
		if err := e.Run(ctx, user, func(c *Ctx) error {
			return node(c, "Rogue", "", false)
		}); err == nil {
			t.Fatal("a root was created outside the scope")
		}
	})

	t.Run("a move out of scope is refused", func(t *testing.T) {
		err := e.Run(ctx, user, func(c *Ctx) error {
			doc, err := c.GetDoc("Test Territory", "PR")
			if err != nil {
				return err
			}
			doc["parent_test_territory"] = "Antarctica"
			_, err = c.Save(doc, SaveOpts{})
			return err
		})
		if err == nil {
			t.Fatal("a node was moved out of the user's branch")
		}
	})

	t.Run("a client outside the branch is refused on write", func(t *testing.T) {
		err := e.Run(ctx, user, func(c *Ctx) error {
			doc, err := c.NewDoc("Test Client", Doc{"title": "Rogue Client", "territory": "Antarctica"})
			if err != nil {
				return err
			}
			_, err = c.Insert(doc, SaveOpts{})
			return err
		})
		if err == nil {
			t.Fatal("a document was written into a territory outside the scope")
		}
	})
}

func TestTreeChildrenEndpointData(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		res, err := c.TreeChildren("Test Territory", "", TreeArgs{Limit: 50})
		if err != nil {
			return err
		}
		nodes := res["nodes"].([]map[string]any)
		var got []string
		for _, n := range nodes {
			got = append(got, db.Str(n["id"]))
		}
		if strings.Join(got, ",") != "World,Antarctica" {
			t.Fatalf("roots = %v (groups first)", got)
		}
		if nodes[0]["children"] != 2 {
			t.Fatalf("World has %v children, want 2", nodes[0]["children"])
		}
		if res["hasMore"] != false {
			t.Fatalf("hasMore = %v", res["hasMore"])
		}
		res, err = c.TreeChildren("Test Territory", "World", TreeArgs{Limit: 50})
		if err != nil {
			return err
		}
		got = nil
		for _, n := range res["nodes"].([]map[string]any) {
			got = append(got, db.Str(n["id"]))
		}
		if strings.Join(got, ",") != "Brazil,Chile" {
			t.Fatalf("children of World = %v", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("tree children: %v", err)
	}

	// fields come back under `values`; an order replaces groups-first outright
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		res, err := c.TreeChildren("Test Territory", "", TreeArgs{Limit: 50, Fields: []string{"title"}, OrderBy: "title asc"})
		if err != nil {
			return err
		}
		nodes := res["nodes"].([]map[string]any)
		var got []string
		for _, n := range nodes {
			got = append(got, db.Str(n["id"]))
		}
		if strings.Join(got, ",") != "Antarctica,World" {
			t.Fatalf("roots ordered by title = %v", got)
		}
		values, _ := nodes[1]["values"].(map[string]any)
		if db.Str(values["title"]) != "World" {
			t.Fatalf("values = %v", nodes[1]["values"])
		}
		if _, err := c.TreeChildren("Test Territory", "", TreeArgs{Fields: []string{"nope"}}); err == nil {
			t.Fatal("an unknown field was accepted")
		}
		if _, err := c.TreeChildren("Test Territory", "", TreeArgs{OrderBy: "nope asc"}); err == nil {
			t.Fatal("an unknown order field was accepted")
		}
		return nil
	}); err != nil {
		t.Fatalf("tree fields and order: %v", err)
	}

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.TreeChildren("Test Client", "", TreeArgs{Limit: 50})
		if err == nil {
			t.Fatal("a DocType that is not a tree was accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A user who may read only a branch sees its top as a root, even though that
// node has a parent they cannot read.
func TestTreeChildrenPromotesScopedRoot(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	ctx := context.Background()
	const user = "branch@x.com"
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": user, "full_name": "Branch",
			"roles": []any{map[string]any{"role": "Tree User"}}})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		p, err := c.NewDoc("User Permission", Doc{"user": user, "allow": "Test Territory", "for_value": "Brazil"})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := e.Run(ctx, user, func(c *Ctx) error {
		res, err := c.TreeChildren("Test Territory", "", TreeArgs{Limit: 50})
		if err != nil {
			return err
		}
		nodes := res["nodes"].([]map[string]any)
		if len(nodes) != 1 || db.Str(nodes[0]["id"]) != "Brazil" {
			t.Fatalf("roots for a scoped user = %v", nodes)
		}
		if nodes[0]["children"] != 1 {
			t.Fatalf("Brazil counts %v readable children, want 1", nodes[0]["children"])
		}
		return nil
	}); err != nil {
		t.Fatalf("scoped roots: %v", err)
	}
}
