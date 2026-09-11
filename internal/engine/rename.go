package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// File, Comment, Version and Email Delivery each hold a DocType name and a
// document name in plain Data columns, so the meta cannot derive them the way
// it derives a Link. One list, used by every operation that moves or removes a
// document, because keeping three copies is how tab_file came to be forgotten
// in two of them.
var coreRefs = []struct {
	table, doctypeCol, nameCol string
	// keepOnDelete leaves the row where it is when the document it names is
	// deleted. An attachment, a comment and a version belong to their document
	// and go with it. A delivery record does not: it says that a message left
	// this site for somebody's inbox, and deleting the order cannot un-send the
	// invoice. The reference is left dangling on purpose — it is a record of
	// what the message was about at the time, not a live link.
	keepOnDelete bool
}{
	{table: "tab_file", doctypeCol: "attached_to_doctype", nameCol: "attached_to_name"},
	{table: "tab_comment", doctypeCol: "reference_doctype", nameCol: "reference_name"},
	{table: "tab_version", doctypeCol: "ref_doctype", nameCol: "docname"},
	{table: "tab_email_delivery", doctypeCol: "reference_doctype", nameCol: "reference_name", keepOnDelete: true},
}

// docTypeRefColumns is every (table, column) that stores a DocType *name*,
// derived from the meta rather than listed: child tables keep it in parenttype,
// and a Dynamic Link keeps it in the sibling column its options name.
func (c *Ctx) docTypeRefColumns() [][2]string {
	seen := map[[2]string]bool{}
	var out [][2]string
	add := func(table, col string) {
		k := [2]string{table, col}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, d := range c.St.Meta.DocTypes {
		t := d.TableName()
		if d.IsChild {
			add(t, "parenttype")
		}
		for _, f := range d.Fields {
			// A Dynamic Link's options names the sibling column holding the
			// DocType; two of them may share one, hence the dedupe. The
			// sibling has to be a field with a column of its own — options
			// naming a layout field would produce SQL against nothing.
			if f.Fieldtype == "Dynamic Link" {
				opt := f.OptionsString()
				if opt == "" {
					continue
				}
				if sib := d.Field(opt); sib != nil && meta.ColumnType(sib.Fieldtype) != "" {
					add(t, opt)
				}
			}
		}
	}
	for _, r := range coreRefs {
		add(r.table, r.doctypeCol)
	}
	// Meta.DocTypes is a map, so the walk above is unordered. Sorting keeps the
	// statements in a fixed order, which is what makes a migration reproducible
	// and keeps two of them from taking the same locks in opposite orders.
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// sweepDocTypeRename repoints every stored reference to the old DocType name.
//
// The DDL renamed tab_<old> to tab_<new>, which moves the rows. This moves the
// rows that *name* the DocType — and they are not optional: a File whose
// attached_to_doctype still says the old name fails its permission check, and a
// child whose parenttype does is invisible to its parent.
//
// Deliberately not swept, and said out loud in docs/agent/migrations.md: a
// queued job's args, which may embed the old name (drain the queue first), and
// tab_version.data, whose historical diffs embed it — rewriting history would
// be worse than leaving it. A Link's target comes from the meta, so it follows
// on its own, and Registry.Validate refuses to load a Link still pointing at a
// DocType that no longer exists.
func (c *Ctx) sweepDocTypeRename(old, name string) error {
	for _, ref := range c.docTypeRefColumns() {
		sql := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2", db.Ident(ref[0]), db.Ident(ref[1]), db.Ident(ref[1]))
		if _, err := c.Tx.Exec(c.Ctx, sql, name, old); err != nil {
			return fmt.Errorf("rename %s to %s: %s.%s: %w", old, name, ref[0], ref[1], err)
		}
	}
	return nil
}

// recordRename sweeps the references a DocType rename invalidates and writes the
// rename into ddcore_rename.
//
// It is driven by the plan, not by the declaration: a RENAME statement only
// exists while the old table or column is still there, so this runs exactly
// once however many times migrate is run. The ledger is the belt to that
// braces, and it is what lets `ddcore doctor` answer the only question a
// renamedFrom declaration raises — whether it can be deleted yet.
func (c *Ctx) recordRename(st db.Statement) error {
	kind, doctype, name := "field", st.Doctype, st.Column
	if st.Kind == db.KindRenameTable {
		kind, name = "doctype", st.Doctype
		if err := c.sweepDocTypeRename(st.OldName, st.Doctype); err != nil {
			return err
		}
	}
	_, err := c.Tx.Exec(c.Ctx,
		`INSERT INTO ddcore_rename (kind, doctype, old_name, new_name) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		kind, doctype, st.OldName, name)
	return err
}

// Rename is one applied rename, as ddcore doctor reports it.
type Rename struct {
	Kind, Doctype, OldName, NewName, Executed string
	// Retirable is true when nothing in this database still needs the
	// declaration: the rename is recorded and the old name is gone from the
	// catalog. It says nothing about other sites, which is why doctor prints
	// "in this database".
	Retirable bool
}

// AppliedRenames lists what this database has already renamed, and which of
// those declarations it no longer needs.
func (c *Ctx) AppliedRenames() ([]Rename, error) {
	rows, err := db.Select(c.Ctx, c.Q(),
		`SELECT kind, doctype, old_name, new_name, executed FROM ddcore_rename ORDER BY executed, doctype, old_name`)
	if err != nil {
		return nil, err
	}
	out := make([]Rename, 0, len(rows))
	for _, r := range rows {
		rn := Rename{
			Kind: db.Str(r["kind"]), Doctype: db.Str(r["doctype"]),
			OldName: db.Str(r["old_name"]), NewName: db.Str(r["new_name"]),
			Executed: db.Str(r["executed"]),
		}
		gone, err := c.oldNameGone(rn)
		if err != nil {
			return nil, err
		}
		rn.Retirable = gone
		out = append(out, rn)
	}
	return out, nil
}

func (c *Ctx) oldNameGone(rn Rename) (bool, error) {
	var sql string
	var args []any
	if rn.Kind == "doctype" {
		sql = `SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`
		args = []any{"tab_" + meta.Snake(rn.OldName)}
	} else {
		d, err := c.St.DocType(rn.Doctype)
		if err != nil {
			return false, nil // the DocType is gone; the declaration is too
		}
		sql = `SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`
		args = []any{d.TableName(), rn.OldName}
	}
	rows, err := db.Select(c.Ctx, c.Q(), sql, args...)
	return len(rows) == 0, err
}

// AppliedRenames is AppliedRenames for a caller outside a transaction.
func (e *Engine) AppliedRenames(ctx context.Context) ([]Rename, error) {
	var out []Rename
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		var err error
		out, err = c.AppliedRenames()
		return err
	})
	return out, err
}

// PendingPatches is the patches this database has not run, in the order Migrate
// would run them: every beforeSchema patch, then every afterSchema one.
func (e *Engine) PendingPatches() []Patch {
	st := e.Current()
	ran := map[string]bool{}
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT app, name FROM ddcore_patch`)
	if err != nil {
		return nil // the ledger is not there yet: nothing has run
	}
	for _, r := range rows {
		ran[db.Str(r["app"])+"/"+db.Str(r["name"])] = true
	}
	var out []Patch
	for _, before := range []bool{true, false} {
		for _, p := range st.Snap.Patches {
			if p.BeforeSchema() == before && !ran[p.App+"/"+p.Name] {
				if p.Phase == "" {
					p.Phase = "afterSchema"
				}
				out = append(out, p)
			}
		}
	}
	return out
}
