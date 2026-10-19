package engine

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// refColumn is one stored column that may hold a document's id: a Link to its
// DocType, a Link to a virtual union over it (which stores "<DocType>:<id>"),
// or a Dynamic Link whose sibling column names the DocType. Rename, merge and
// the merge preview walk the same list, so what a preview counts is what a
// merge moves.
type refColumn struct {
	doctype *meta.DocType // the DocType that holds the column
	field   *meta.Field
	virtual bool   // the column stores meta.VirtualID(d, id)
	typeCol string // a Dynamic Link's sibling column, which names the DocType
}

// refColumns lists every column that may point at a document of d. A Dynamic
// Link that is one of coreRefs (a ToDo's reference) is left to the coreRefs
// loop, which knows the table's unique key.
func (c *Ctx) refColumns(d *meta.DocType) []refColumn {
	var out []refColumn
	for _, other := range c.St.Meta.DocTypes {
		if other.IsVirtual() {
			continue // no table, so nothing stored that could point here
		}
		for _, f := range other.Fields {
			if f.Computed {
				continue // no column: nothing stored to carry along
			}
			switch {
			case f.Fieldtype == "Link" && f.OptionsString() == d.Name:
				out = append(out, refColumn{doctype: other, field: f})
			case f.Fieldtype == "Link" && c.isVirtualOver(f.OptionsString(), d.Name):
				out = append(out, refColumn{doctype: other, field: f, virtual: true})
			case f.Fieldtype == "Dynamic Link" && other.Field(f.OptionsString()) != nil:
				if isCoreRef(other.TableName(), f.OptionsString(), f.Fieldname) {
					continue
				}
				out = append(out, refColumn{doctype: other, field: f, typeCol: f.OptionsString()})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].doctype.Name != out[j].doctype.Name {
			return out[i].doctype.Name < out[j].doctype.Name
		}
		return out[i].field.Fieldname < out[j].field.Fieldname
	})
	return out
}

// value is what the column stores for document id of d.
func (r refColumn) value(d *meta.DocType, id string) string {
	if r.virtual {
		return meta.VirtualID(d.Name, id)
	}
	return id
}

// where is the predicate selecting the rows that point at id, with its
// arguments numbered from n.
func (r refColumn) where(d *meta.DocType, id string, alias string, n int) (string, []any) {
	col := db.Ident(r.field.Fieldname)
	if alias != "" {
		col = alias + "." + col
	}
	s := fmt.Sprintf("%s = $%d", col, n)
	args := []any{r.value(d, id)}
	if r.typeCol != "" {
		tc := db.Ident(r.typeCol)
		if alias != "" {
			tc = alias + "." + tc
		}
		s += fmt.Sprintf(" AND %s = $%d", tc, n+1)
		args = append(args, d.Name)
	}
	return s, args
}

// repointRefs moves every reference to document oldID of d over to newID:
// the Link and Dynamic Link columns the meta knows, coreRefs and the User
// Permissions that name it. It leaves the document's own row and child rows
// alone — a rename moves those, a merge deletes them.
//
// merge is set when newID already exists. A coreRefs row that would then
// break its table's unique key (a share with the same user, say) stays with
// oldID instead, where the delete of oldID that follows a merge disposes of
// it the way a delete does.
func (c *Ctx) repointRefs(d *meta.DocType, oldID, newID string, merge bool) error {
	q := c.Q()
	for _, r := range c.refColumns(d) {
		cond, args := r.where(d, oldID, "", 2)
		sql := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s", db.Ident(r.doctype.TableName()), db.Ident(r.field.Fieldname), cond)
		if _, err := q.Exec(c.Ctx, sql, append([]any{r.value(d, newID)}, args...)...); err != nil {
			return err
		}
	}
	// The tables that name a document without linking to it. A renamed
	// document used to lose its attachments, and with them the permission
	// check on a private file, which reads File.attached_to_id to decide who
	// may download it. Every one of them is swept, keepOnDelete or not: a
	// record that survives its document must still point at the name that
	// document answers to now.
	for _, ref := range coreRefs {
		t := db.Ident(ref.table)
		sql := fmt.Sprintf("UPDATE %s t SET %s = $1 WHERE t.%s = $2 AND t.%s = $3",
			t, db.Ident(ref.idCol), db.Ident(ref.doctypeCol), db.Ident(ref.idCol))
		if merge && ref.mergeKey != nil {
			var same []string
			for _, k := range ref.mergeKey {
				same = append(same, fmt.Sprintf(" AND o.%s = t.%s", db.Ident(k), db.Ident(k)))
			}
			tenant, err := c.hasTenantColumn(ref.table)
			if err != nil {
				return err
			}
			if tenant {
				same = append(same, " AND o.tenant = t.tenant")
			}
			sql += fmt.Sprintf(" AND NOT EXISTS (SELECT 1 FROM %s o WHERE o.%s = $2 AND o.%s = $1%s)",
				t, db.Ident(ref.doctypeCol), db.Ident(ref.idCol), strings.Join(same, ""))
		}
		tag, err := q.Exec(c.Ctx, sql, newID, d.Name, oldID)
		if err != nil {
			return fmt.Errorf("coreRefs update %s: %w", ref.table, err)
		}
		if ref.table == "tab_document_share" && tag.RowsAffected() > 0 {
			if err := c.allSharesChanged(); err != nil {
				return err
			}
		}
	}
	if err := c.repointUserPermissions(d, oldID, newID, merge); err != nil {
		return err
	}
	if d.Name == "Role" {
		// The sweep rewrote Has Role rows under every user's cached roles, which
		// would otherwise go on naming a role that no longer exists.
		drop := func() { c.E.Cache.DelPrefix("roles:"); c.E.Cache.DelPrefix("evperm:") }
		drop()
		c.AfterCommit(drop)
		return c.broadcastInvalidation(nil, []string{"roles:", "evperm:"})
	}
	return nil
}

// repointUserPermissions carries the User Permissions that name the document
// along with it. for_value is a Data field — the DocType it names is in allow
// — so the meta cannot see it as a link, and a rename used to leave a user's
// scope pointing at a name nothing answers to. In a merge, a user already
// allowed the surviving document keeps that one row.
func (c *Ctx) repointUserPermissions(d *meta.DocType, oldID, newID string, merge bool) error {
	var users []Doc
	collect := func(sql string, args ...any) error {
		rows, err := db.Select(c.Ctx, c.Q(), sql, args...)
		if err != nil {
			return err
		}
		for _, r := range rows {
			users = append(users, Doc{"user": db.Str(r["user"])})
		}
		return nil
	}
	if merge {
		if err := collect(`DELETE FROM tab_user_permission a WHERE a.allow = $1 AND a.for_value = $2
			AND EXISTS (SELECT 1 FROM tab_user_permission b WHERE b.allow = $1 AND b.for_value = $3
				AND b."user" = a."user" AND b.applicable_for IS NOT DISTINCT FROM a.applicable_for)
			RETURNING a."user"`, d.Name, oldID, newID); err != nil {
			return err
		}
	}
	if err := collect(`UPDATE tab_user_permission SET for_value = $3 WHERE allow = $1 AND for_value = $2 RETURNING "user"`,
		d.Name, oldID, newID); err != nil {
		return err
	}
	return c.invalidateUserPermissionCache(users...)
}

// hasTenantColumn reports whether table keeps its rows per tenant, which a
// unique key then includes.
func (c *Ctx) hasTenantColumn(table string) (bool, error) {
	if !c.Tenancy() {
		return false, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'tenant'`, table)
	return len(rows) > 0, err
}

// MergeCount is one line of a merge preview: how many rows of a DocType point
// at the document that goes away.
type MergeCount struct {
	Doctype    string `json:"doctype"`
	Label      string `json:"label"`
	Fieldname  string `json:"fieldname,omitempty"`
	FieldLabel string `json:"fieldLabel,omitempty"`
	Count      int64  `json:"count"`
}

// MergePreview is what a merge of From into Into would do, without doing it.
type MergePreview struct {
	From string `json:"from"`
	Into string `json:"into"`
	// Links are the documents (and child rows) whose Link or Dynamic Link
	// fields would be repointed.
	Links []MergeCount `json:"links"`
	// Records are the comments, attachments, shares, Versions and the other
	// records that name the document, and the User Permissions that allow it.
	Records []MergeCount `json:"records"`
	// Blockers are the reasons the merge would be refused; empty when it can run.
	Blockers []string `json:"blockers"`
}

// Merge folds document fromID of doctype into intoID, which already exists:
// every reference to fromID is repointed to intoID, and fromID is deleted.
// It is Frappe's "merge with existing", for duplicates that turned up in the
// data. beforeMerge and afterMerge run on fromID's document with
// doc.flags.mergeInto naming intoID, and so do the onTrash and afterDelete of
// its delete.
func (c *Ctx) Merge(doctype, fromID, intoID string) error {
	d, from, into, err := c.mergeDocs(doctype, fromID, intoID)
	if err != nil {
		return err
	}
	intoID = into.ID()
	blockers, err := c.mergeChecks(d, from, into)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return blockers[0]
	}
	if err := c.lockTree(d); err != nil {
		return err
	}
	flags := map[string]any{"mergeInto": intoID}
	if err := c.runHook(d, "beforeMerge", from, nil, flags); err != nil {
		return err
	}
	if err := c.acrossSpaces(d, func() error { return c.repointRefs(d, fromID, intoID, true) }); err != nil {
		return err
	}
	// the cached copies may hold a self-reference the sweep just rewrote
	delete(c.docCache, c.docKey(doctype, fromID))
	delete(c.docCache, c.docKey(doctype, intoID))
	// The link check stays on: after the sweep nothing should point at fromID,
	// and a column it missed must stop the merge rather than dangle.
	if err := c.deleteWithFlags(doctype, fromID, false, false, flags); err != nil {
		return err
	}
	if err := c.Audit("doc.merge", d.Name, intoID, map[string]any{"from": fromID}); err != nil {
		return err
	}
	if err := c.runHook(d, "afterMerge", from, nil, flags); err != nil {
		return err
	}
	c.AfterCommit(func() {
		c.publish(Event{Name: "doc_update", Doctype: doctype, DocID: intoID, Payload: map[string]any{"doctype": doctype, "id": intoID}})
	})
	return nil
}

// PreviewMerge reports what Merge would move and why it would refuse,
// writing nothing. It checks the same permissions as Merge.
func (c *Ctx) PreviewMerge(doctype, fromID, intoID string) (*MergePreview, error) {
	d, from, into, err := c.mergeDocs(doctype, fromID, intoID)
	if err != nil {
		return nil, err
	}
	p := &MergePreview{From: from.ID(), Into: into.ID(), Links: []MergeCount{}, Records: []MergeCount{}, Blockers: []string{}}
	blockers, err := c.mergeChecks(d, from, into)
	if err != nil {
		return nil, err
	}
	for _, b := range blockers {
		p.Blockers = append(p.Blockers, c.T(b.Key, b.Args...))
	}
	err = c.acrossSpaces(d, func() error {
		count := func(sql string, args ...any) (int64, error) {
			var n int64
			err := c.Q().QueryRow(c.Ctx, sql, args...).Scan(&n)
			return n, err
		}
		for _, r := range c.refColumns(d) {
			cond, args := r.where(d, fromID, "", 1)
			n, err := count(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", db.Ident(r.doctype.TableName()), cond), args...)
			if err != nil {
				return err
			}
			if n > 0 {
				p.Links = append(p.Links, MergeCount{Doctype: r.doctype.Name, Label: c.T(r.doctype.Label),
					Fieldname: r.field.Fieldname, FieldLabel: c.T(r.field.Label), Count: n})
			}
		}
		byLabel := map[string]int{}
		add := func(label string, n int64) {
			if n == 0 {
				return
			}
			if i, ok := byLabel[label]; ok {
				p.Records[i].Count += n
				return
			}
			byLabel[label] = len(p.Records)
			p.Records = append(p.Records, MergeCount{Doctype: label, Label: c.T(label), Count: n})
		}
		for _, ref := range coreRefs {
			if ref.label == "" {
				continue
			}
			n, err := count(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1 AND %s = $2",
				db.Ident(ref.table), db.Ident(ref.doctypeCol), db.Ident(ref.idCol)), d.Name, fromID)
			if err != nil {
				return err
			}
			add(ref.label, n)
		}
		n, err := count(`SELECT count(*) FROM tab_user_permission WHERE allow = $1 AND for_value = $2`, d.Name, fromID)
		if err != nil {
			return err
		}
		add("User Permission", n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// mergeDocs runs the refusals a merge shares with a rename, and loads both
// documents with the permissions a merge needs: write on the two, and delete
// on the one that goes away.
func (c *Ctx) mergeDocs(doctype, fromID, intoID string) (*meta.DocType, Doc, Doc, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, nil, nil, err
	}
	// a shared document's merge rewrites every tenant's references to it,
	// and is the platform's alone
	if err := c.spaceRefusal(d, true, c.IgnorePermissions()); err != nil {
		return nil, nil, nil, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return nil, nil, nil, err
	}
	if err := refuseVirtual(d); err != nil {
		return nil, nil, nil, err
	}
	if d.IsSingle {
		return nil, nil, nil, cerr.Validation("Single DocTypes cannot be deleted or renamed")
	}
	if !d.AllowRename {
		return nil, nil, nil, cerr.Validation("{0} cannot be renamed", c.T(d.Label))
	}
	if d.Name == "User" {
		// a user is also a sign-in, roles, sessions and API keys, which no
		// reference sweep can fold together
		return nil, nil, nil, cerr.Validation("Users cannot be merged")
	}
	intoID = strings.TrimSpace(intoID)
	if intoID == "" {
		return nil, nil, nil, cerr.Mandatory("Choose the {0} to merge into", c.T(d.Label))
	}
	if intoID == fromID {
		return nil, nil, nil, cerr.Validation("{0} {1} cannot be merged into itself", c.T(d.Label), fromID)
	}
	from, err := c.GetDoc(doctype, fromID)
	if err != nil {
		return nil, nil, nil, err
	}
	into, err := c.GetDoc(doctype, intoID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !c.IgnorePermissions() {
		for _, need := range []struct {
			doc   Doc
			ptype string
		}{{from, "write"}, {from, "delete"}, {into, "write"}} {
			if ok, _ := c.HasPermission(doctype, need.ptype, need.doc); !ok {
				return nil, nil, nil, cerr.Permission("No permission to merge {0} {1} into {2}", c.T(d.Label), fromID, intoID)
			}
		}
	}
	return d, from, into, nil
}

// mergeChecks lists the reasons from cannot be merged into into, the first of
// which Merge refuses with and all of which a preview shows.
func (c *Ctx) mergeChecks(d *meta.DocType, from, into Doc) ([]*cerr.Error, error) {
	var out []*cerr.Error
	label, fromID, intoID := c.T(d.Label), from.ID(), into.ID()
	if from.Docstatus() == 1 {
		out = append(out, cerr.Validation("Cancel {0} {1} before merging it", label, fromID))
	}
	if d.IsTree {
		anc, err := c.treeAncestorsInclusive(d, intoID)
		if err != nil {
			return nil, err
		}
		if slices.Contains(anc, fromID) {
			out = append(out, cerr.Validation("{0} {1} cannot be merged into {2}, which is below it in the tree", label, fromID, intoID))
		} else if has, err := c.treeHasChildren(d, fromID); err != nil {
			return nil, err
		} else if has && !truthy(into[meta.IsGroupField]) {
			out = append(out, cerr.Validation("{0} {1} has children, and {2} is not a group", label, fromID, intoID))
		}
	}
	err := c.acrossSpaces(d, func() error {
		for _, r := range c.refColumns(d) {
			for _, key := range uniqueKeysOf(r) {
				clash, err := c.uniqueClash(d, r, key, fromID, intoID)
				if err != nil {
					return err
				}
				if clash {
					out = append(out, cerr.Validation("{0} {1} cannot be merged into {2}: {3} {4} must be unique, and both have one",
						label, fromID, intoID, c.T(r.doctype.Label), c.T(r.field.Label)))
					break
				}
			}
		}
		return nil
	})
	return out, err
}

// uniqueKeysOf lists, for each unique constraint the column is part of, the
// other fields of that constraint: none for a `unique` field, the rest of the
// key for a uniqueKeys entry.
func uniqueKeysOf(r refColumn) [][]*meta.Field {
	var out [][]*meta.Field
	if r.field.Unique {
		out = append(out, nil)
	}
	for _, k := range r.doctype.UniqueKeys {
		if !slices.Contains(k.Fields, r.field.Fieldname) {
			continue
		}
		var rest []*meta.Field
		for _, fn := range k.Fields {
			if fn != r.field.Fieldname {
				if f := r.doctype.Field(fn); f != nil {
					rest = append(rest, f)
				}
			}
		}
		out = append(out, rest)
	}
	return out
}

// uniqueClash reports whether repointing the column from fromID to intoID
// would break a unique constraint: a row already points at each, with the
// same values in the rest of the key (and in the same tenant).
func (c *Ctx) uniqueClash(d *meta.DocType, r refColumn, rest []*meta.Field, fromID, intoID string) (bool, error) {
	ca, args := r.where(d, fromID, "a", 1)
	cb, argsB := r.where(d, intoID, "b", len(args)+1)
	args = append(args, argsB...)
	var same []string
	for _, f := range rest {
		col := db.Ident(f.Fieldname)
		same = append(same, fmt.Sprintf(" AND a.%s = b.%s", col, col))
		if meta.ColumnType(f.Fieldtype) == "text" {
			same = append(same, fmt.Sprintf(" AND a.%s <> ''", col))
		}
	}
	tenant, err := c.hasTenantColumn(r.doctype.TableName())
	if err != nil {
		return false, err
	}
	if tenant {
		same = append(same, " AND a.tenant = b.tenant")
	}
	t := db.Ident(r.doctype.TableName())
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT 1 FROM %s a JOIN %s b ON %s%s WHERE %s LIMIT 1",
		t, t, cb, strings.Join(same, ""), ca), args...)
	return len(rows) > 0, err
}
