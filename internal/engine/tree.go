package engine

import (
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// truthy reads a Check the way the cast does, whatever shape the value arrived
// in: a boolean column, a JSON number, or the string a form sends.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	}
	return toFloat(v) != 0
}

// lockTree serialises the structural writes of one tree DocType.
//
// A cycle needs two writers: A moved under B while B is moved under A, each
// checking against a hierarchy the other is about to change. A row lock cannot
// see that — the two documents are different rows — so the whole hierarchy is
// the unit of exclusion. It is an advisory lock on the DocType name, held to
// the end of the transaction.
//
// Every tree write takes it *before* any row lock (Save's FOR UPDATE, and the
// DELETE in Delete), so two transactions always queue in the same order and
// cannot deadlock against one another. Taking it twice in one transaction is
// free: the same session re-enters its own advisory lock.
func (c *Ctx) lockTree(d *meta.DocType) error {
	if d == nil || !d.IsTree || c.Tx == nil {
		return nil
	}
	_, err := c.Tx.Exec(c.Ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "ddcore.tree:"+d.Name)
	return err
}

// treeAncestorsInclusive lists id and every ancestor above it, the document
// itself first. UNION rather than UNION ALL, so rows that are already cyclic
// (written by raw SQL, or built before the DocType declared isTree) end the
// walk instead of spinning.
func (c *Ctx) treeAncestorsInclusive(d *meta.DocType, id string) ([]string, error) {
	pf := d.TreeParentField()
	if pf == "" || id == "" {
		return nil, nil
	}
	q := fmt.Sprintf(
		`WITH RECURSIVE a(id) AS (`+
			`SELECT $1::text `+
			`UNION SELECT n.%[2]s FROM %[1]s n JOIN a ON n.id = a.id WHERE COALESCE(n.%[2]s::text, '') <> ''`+
			`) SELECT a.id FROM a`,
		db.Ident(d.TableName()), db.Ident(pf))
	rows, err := db.Select(c.Ctx, c.Q(), q, id)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if s := db.Str(r["id"]); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// treeHasChildren reports whether any document names id as its parent.
func (c *Ctx) treeHasChildren(d *meta.DocType, id string) (bool, error) {
	pf := d.TreeParentField()
	if pf == "" || id == "" {
		return false, nil
	}
	q := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1 LIMIT 1", db.Ident(d.TableName()), db.Ident(pf))
	rows, err := db.Select(c.Ctx, c.Q(), q, id)
	return len(rows) > 0, err
}

// checkTree holds the hierarchy together on every write: the parent exists and
// is a group, the document is not moved under itself or under one of its own
// descendants, and a group with children stays a group.
//
// It runs after beforeSave, so a hook that sets the parent is checked like a
// value the caller sent. `before` is nil on an insert.
func (c *Ctx) checkTree(d *meta.DocType, doc, before Doc, opts SaveOpts) error {
	if !d.IsTree {
		return nil
	}
	pf := d.TreeParentField()
	parent, id := doc.Str(pf), doc.ID()
	label := c.T(d.Label)
	if parent != "" && (before == nil || before.Str(pf) != parent) {
		if parent == id {
			return cerr.Validation("{0} {1} cannot be its own parent", label, id)
		}
		q := fmt.Sprintf("SELECT COALESCE(%s, false) AS is_group FROM %s WHERE id = $1",
			db.Ident(meta.IsGroupField), db.Ident(d.TableName()))
		rows, err := db.Select(c.Ctx, c.Q(), q, parent)
		if err != nil {
			return err
		}
		switch {
		case len(rows) == 0:
			// checkLinks already reports a parent that does not exist, unless
			// the caller asked for links to be ignored.
			if !opts.IgnoreLinks {
				return cerr.Validation("{0} {1} does not exist", label, parent)
			}
		case !truthy(rows[0]["is_group"]):
			return cerr.Validation("{0} {1} is not a group and cannot have children", label, parent)
		}
		if before != nil {
			// Walking up from the new parent is what catches a cycle: reaching
			// this document means the parent sits somewhere below it.
			above, err := c.treeAncestorsInclusive(d, parent)
			if err != nil {
				return err
			}
			for _, a := range above {
				if a == id {
					return cerr.Validation("{0} {1} cannot be moved under its own descendant {2}", label, id, parent)
				}
			}
		}
	}
	if before != nil && truthy(before[meta.IsGroupField]) && !truthy(doc[meta.IsGroupField]) {
		has, err := c.treeHasChildren(d, id)
		if err != nil {
			return err
		}
		if has {
			return cerr.Validation("{0} {1} has child nodes and must stay a group", label, id)
		}
	}
	if before != nil && before.Str(pf) != parent {
		// Moving a node changes who may read its whole subtree, and only this
		// document's own event fires. The per-document read cache would go on
		// answering for the rest of the subtree until it expired.
		c.AfterCommit(func() { c.E.Cache.DelPrefix("evperm:") })
	}
	return nil
}

// checkTreeDelete refuses to delete a document that still has children.
//
// The self-referencing Link would already stop it, but with the generic "is
// linked from" message, which reads as if some other record were in the way.
// It does not yield to `force` either: forcing it would leave the children
// pointing at nothing, and their next save would fail on the missing link.
func (c *Ctx) checkTreeDelete(d *meta.DocType, id string) error {
	if !d.IsTree {
		return nil
	}
	has, err := c.treeHasChildren(d, id)
	if err != nil || !has {
		return err
	}
	return cerr.LinkExists("{0} {1} has child nodes; delete or move them first", c.T(d.Label), id).
		WithTitleKey("Cannot delete")
}

// treeRootCandidates is how many readable nodes TreeChildren inspects when it
// looks for roots to promote. It only matters to a user whose access is
// filtered; an unscoped one never gets here.
const treeRootCandidates = 2000

// TreeChildren lists one level of a hierarchy for the Desk's tree view: the
// children of `parent`, or the roots when it is empty.
//
// Every read goes through GetList, so roles, User Permission scopes, shares and
// field permissions apply exactly as they do to a list — including the child
// counts, which are counted over what the user may read, never over what is
// there. A user who can read a node but not its parent would otherwise see
// nothing at all, so such a node is promoted to a root.
func (c *Ctx) TreeChildren(doctype, parent string, limit int) (map[string]any, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if !d.IsTree {
		return nil, cerr.Validation("{0} is not a tree", c.T(d.Label))
	}
	pf := d.TreeParentField()
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	fields := []string{"id", pf, meta.IsGroupField}
	title := d.TitleField
	if title != "" && title != "id" {
		fields = append(fields, title)
	}
	order := meta.IsGroupField + " desc, id asc"
	if title != "" && title != "id" {
		order = meta.IsGroupField + " desc, " + title + " asc"
	}

	var rows []map[string]any
	if parent != "" {
		rows, err = c.GetList(doctype, ListArgs{
			Filters: []any{[]any{pf, "=", parent}}, Fields: fields, OrderBy: order, Limit: limit + 1})
		if err != nil {
			return nil, err
		}
	} else {
		if rows, err = c.GetList(doctype, ListArgs{
			Filters: []any{[]any{pf, "not set", ""}}, Fields: fields, OrderBy: order, Limit: limit + 1}); err != nil {
			return nil, err
		}
		promoted, err := c.treePromotedRoots(d, pf, fields, order, limit+1-len(rows))
		if err != nil {
			return nil, err
		}
		rows = append(rows, promoted...)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	counts, err := c.treeChildCounts(d, pf, rows)
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		id := db.Str(r["id"])
		label := id
		if title != "" && title != "id" {
			if s := db.Str(r[title]); s != "" {
				label = s
			}
		}
		nodes = append(nodes, map[string]any{
			"id": id, "title": label, "parent": db.Str(r[pf]),
			meta.IsGroupField: truthy(r[meta.IsGroupField]), "children": counts[id],
		})
	}
	return map[string]any{"nodes": nodes, "hasMore": hasMore}, nil
}

// treePromotedRoots finds readable nodes whose parent the user cannot read, so
// a branch granted in the middle of a hierarchy still has a top.
//
// A user whose access is not filtered at all reads every parent, so there is
// nothing to promote and nothing to scan.
func (c *Ctx) treePromotedRoots(d *meta.DocType, pf string, fields []string, order string, room int) ([]map[string]any, error) {
	if room <= 0 {
		return nil, nil
	}
	filters, err := c.permissionFilters(d)
	if err != nil || len(filters) == 0 {
		return nil, err
	}
	candidates, err := c.GetList(d.Name, ListArgs{
		Filters: []any{[]any{pf, "set", ""}}, Fields: fields, OrderBy: order, Limit: treeRootCandidates})
	if err != nil {
		return nil, err
	}
	parents := map[string]bool{}
	for _, r := range candidates {
		if p := db.Str(r[pf]); p != "" {
			parents[p] = true
		}
	}
	if len(parents) == 0 {
		return nil, nil
	}
	list := make([]any, 0, len(parents))
	for p := range parents {
		list = append(list, p)
	}
	readable, err := c.GetList(d.Name, ListArgs{
		Filters: []any{[]any{"id", "in", list}}, Fields: []string{"id"}, Limit: len(list)})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(readable))
	for _, r := range readable {
		seen[db.Str(r["id"])] = true
	}
	var out []map[string]any
	for _, r := range candidates {
		if !seen[db.Str(r[pf])] {
			out = append(out, r)
			if len(out) == room {
				break
			}
		}
	}
	return out, nil
}

// treeChildCounts counts each node's readable children in one grouped query, so
// the view knows which rows expand without asking per row — and a user never
// learns how many children they are not allowed to see.
func (c *Ctx) treeChildCounts(d *meta.DocType, pf string, rows []map[string]any) (map[string]int, error) {
	out := map[string]int{}
	if len(rows) == 0 {
		return out, nil
	}
	list := make([]any, 0, len(rows))
	for _, r := range rows {
		if truthy(r[meta.IsGroupField]) {
			list = append(list, db.Str(r["id"]))
		}
	}
	if len(list) == 0 {
		return out, nil
	}
	grouped, err := c.GetList(d.Name, ListArgs{
		Filters: []any{[]any{pf, "in", list}},
		Fields:  []string{pf, "count(id) as n"}, GroupBy: pf, Limit: len(list)})
	if err != nil {
		return nil, err
	}
	for _, r := range grouped {
		out[db.Str(r[pf])] = int(toFloat(r["n"]))
	}
	return out, nil
}
