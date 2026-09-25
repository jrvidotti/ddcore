package engine

import (
	"encoding/json"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Field permissions (SEC-02).
//
// A field declares `permlevel: N`; a permission row with `permlevel: N` grants
// read and/or write on that level's fields to its role. Level 0 is the
// document itself, so it is governed by the ordinary document checks: whoever
// may read the document reads its level-0 fields, whoever may write it writes
// them. Everything here is about the levels above 0.
//
// A level is never a way into a document: a level-N grant only widens what a
// user who already reaches the document sees and changes in it. Child rows
// carry no permissions of their own, so their fields are judged by the rows of
// the parent that embeds them.

// FieldAccess is the set of field levels one user may read and write on a
// DocType. The zero value grants level 0 only.
type FieldAccess struct {
	all         bool
	read, write uint32
}

// FullFieldAccess sees and writes every level (Admin, privileged work).
func FullFieldAccess() FieldAccess { return FieldAccess{all: true} }

// Level0FieldAccess sees only unrestricted fields — the rule for payloads that
// leave the site with no reading user, such as a webhook.
func Level0FieldAccess() FieldAccess { return FieldAccess{} }

func levelBit(level int) uint32 {
	if level <= 0 {
		return 0
	}
	return 1 << uint(level)
}

// CanReadLevel reports whether fields at level may be read.
func (a FieldAccess) CanReadLevel(level int) bool {
	return a.all || level <= 0 || a.read&levelBit(level) != 0
}

// CanWriteLevel reports whether fields at level may be written.
func (a FieldAccess) CanWriteLevel(level int) bool {
	return a.all || level <= 0 || a.write&levelBit(level) != 0
}

// CanRead reports whether the field may be read. A nil field is a standard
// column, which is always level 0.
func (a FieldAccess) CanRead(f *meta.Field) bool { return f == nil || a.CanReadLevel(f.Permlevel) }

// CanWrite reports whether the field may be written.
func (a FieldAccess) CanWrite(f *meta.Field) bool { return f == nil || a.CanWriteLevel(f.Permlevel) }

// Levels lists the readable and writable levels, 0 included, for the desk.
func (a FieldAccess) Levels() (read, write []int) {
	read, write = []int{0}, []int{0}
	for l := 1; l <= meta.MaxPermlevel; l++ {
		if a.CanReadLevel(l) {
			read = append(read, l)
		}
		if a.CanWriteLevel(l) {
			write = append(write, l)
		}
	}
	return read, write
}

func (a FieldAccess) intersect(b FieldAccess) FieldAccess {
	switch {
	case a.all:
		return b
	case b.all:
		return a
	}
	return FieldAccess{read: a.read & b.read, write: a.write & b.write}
}

// FieldAccess resolves the current user's field levels on d. For a child
// DocType it is what every embedding parent grants — a row's column is only
// as visible as the strictest parent it can be listed under.
func (c *Ctx) FieldAccess(d *meta.DocType) FieldAccess {
	if c.User == "Admin" || c.IgnorePermissions() {
		return FullFieldAccess()
	}
	if d == nil {
		return Level0FieldAccess()
	}
	if d.IsChild {
		parents := c.ParentsOf(d.Name)
		if len(parents) == 0 {
			return Level0FieldAccess()
		}
		out := FullFieldAccess()
		for _, cp := range parents {
			out = out.intersect(c.FieldAccess(cp.Parent))
		}
		return out
	}
	roles, err := c.Roles()
	if err != nil {
		return Level0FieldAccess()
	}
	var a FieldAccess
	for _, p := range d.Permissions {
		if p.Permlevel <= 0 || !contains(roles, p.Role) {
			continue
		}
		if p.Read || p.Write {
			a.read |= levelBit(p.Permlevel)
		}
		if p.Write {
			a.write |= levelBit(p.Permlevel)
		}
	}
	return a
}

// hasRestrictedFields reports whether d or any child table it embeds declares
// a field above level 0 — when neither does, field permissions cost nothing.
func (c *Ctx) hasRestrictedFields(d *meta.DocType) bool {
	if d == nil {
		return false
	}
	if d.HasRestrictedFields() {
		return true
	}
	for _, tf := range d.TableFields() {
		if cd, err := c.St.DocType(tf.OptionsString()); err == nil && cd.HasRestrictedFields() {
			return true
		}
	}
	return false
}

// redactFields removes every field the access cannot read, child rows included.
func (c *Ctx) redactFields(d *meta.DocType, a FieldAccess, doc Doc) {
	if d == nil || doc == nil || a.all || !c.hasRestrictedFields(d) {
		return
	}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		if !a.CanRead(f) {
			delete(doc, f.Fieldname)
			continue
		}
		if !meta.IsTableType(f.Fieldtype) {
			continue
		}
		cd, err := c.St.DocType(f.OptionsString())
		if err != nil || !cd.HasRestrictedFields() {
			continue
		}
		// rows are maps shared with the document, so deleting in place sticks
		for _, row := range doc.Children(f.Fieldname) {
			for _, cf := range cd.Fields {
				if cf.Fieldname != "" && !a.CanRead(cf) {
					delete(row, cf.Fieldname)
				}
			}
		}
	}
}

// RedactFieldsFor is RedactDoc's field step with an explicit access, for a
// payload whose reader is not the current user.
func (c *Ctx) RedactFieldsFor(doctype string, doc Doc, a FieldAccess) Doc {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return doc
	}
	c.redactFields(d, a, doc)
	return doc
}

// fieldPermissionsApply reports whether a write by the current user is subject
// to field permissions at all.
func (c *Ctx) fieldPermissionsApply(d *meta.DocType, opts SaveOpts) bool {
	return !opts.IgnorePermissions && !c.IgnorePermissions() && !c.inWorkflowTransition &&
		c.User != "Admin" && c.hasRestrictedFields(d)
}

// applyFieldWrites enforces field permissions on an incoming document before
// any hook runs, so a controller may still set a restricted field itself.
//
// It first restores what the writer could not see: a field it cannot read that
// arrives absent or null keeps the stored value (or, on insert, the default).
// The desk saves the whole document it was given, and that document never had
// the field — without this, every save by such a user would wipe it. Then any
// field the writer cannot write must be unchanged from base.
func (c *Ctx) applyFieldWrites(d *meta.DocType, base, doc Doc) error {
	a := c.FieldAccess(d)
	if a.all {
		return nil
	}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		if meta.IsTableType(f.Fieldtype) {
			if err := c.applyTableWrites(f, a, base, doc); err != nil {
				return err
			}
			continue
		}
		if !a.CanRead(f) && doc[f.Fieldname] == nil {
			doc[f.Fieldname] = base[f.Fieldname]
		}
		if !a.CanWrite(f) && !c.sameFieldValue(f, doc[f.Fieldname], base[f.Fieldname]) {
			return c.fieldWriteDenied(f)
		}
	}
	return nil
}

func (c *Ctx) applyTableWrites(tf *meta.Field, a FieldAccess, base, doc Doc) error {
	cd, err := c.St.DocType(tf.OptionsString())
	if err != nil {
		return err
	}
	if !a.CanRead(tf) && doc[tf.Fieldname] == nil {
		rows := base.Children(tf.Fieldname)
		list := make([]any, len(rows))
		for i, r := range rows {
			list[i] = map[string]any(r.Clone())
		}
		doc[tf.Fieldname] = list
	}
	baseRows := base.Children(tf.Fieldname)
	byName := make(map[string]Doc, len(baseRows))
	for _, r := range baseRows {
		if n := r.ID(); n != "" {
			byName[n] = r
		}
	}
	var defaults Doc
	rows := doc.Children(tf.Fieldname)
	for i, row := range rows {
		prev := byName[row.ID()]
		if prev == nil && row.ID() == "" && i < len(baseRows) && baseRows[i].ID() == "" {
			// an amendment's rows lost their names; they line up by position
			prev = baseRows[i]
		}
		if prev == nil {
			if defaults == nil {
				defaults = c.fieldDefaults(cd)
			}
			prev = defaults
		}
		for _, cf := range cd.Fields {
			if cf.Fieldname == "" || meta.LayoutTypes[cf.Fieldtype] || meta.IsTableType(cf.Fieldtype) {
				continue
			}
			if !a.CanRead(cf) && row[cf.Fieldname] == nil {
				row[cf.Fieldname] = prev[cf.Fieldname]
			}
			if !a.CanWrite(cf) && !c.sameFieldValue(cf, row[cf.Fieldname], prev[cf.Fieldname]) {
				return c.fieldWriteDenied(cf)
			}
		}
	}
	if a.CanWrite(tf) {
		return nil
	}
	// the table itself is restricted: no row may be added, removed or edited
	if len(rows) != len(baseRows) {
		return c.fieldWriteDenied(tf)
	}
	for i, row := range rows {
		prev := baseRows[i]
		if row.ID() != "" && prev.ID() != "" && row.ID() != prev.ID() {
			return c.fieldWriteDenied(tf)
		}
		for _, cf := range cd.Fields {
			if cf.Fieldname == "" || !meta.HasColumnField(cf) {
				continue
			}
			if !c.sameFieldValue(cf, row[cf.Fieldname], prev[cf.Fieldname]) {
				return c.fieldWriteDenied(tf)
			}
		}
	}
	return nil
}

func (c *Ctx) fieldWriteDenied(f *meta.Field) error {
	return cerr.Permission("Not permitted to change {0}", c.T(f.Label)).WithTitleKey("Restricted field")
}

// fieldDefaults is a new document's value for every field of d.
func (c *Ctx) fieldDefaults(d *meta.DocType) Doc {
	doc := Doc{}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		switch {
		case meta.IsTableType(f.Fieldtype):
			doc[f.Fieldname] = []any{}
		case f.Default != nil:
			doc[f.Fieldname] = c.defaultValue(f)
		case f.Fieldtype == "Check":
			doc[f.Fieldname] = false
		default:
			doc[f.Fieldname] = nil
		}
	}
	return doc
}

func (c *Ctx) sameFieldValue(f *meta.Field, a, b any) bool {
	nv, _ := c.castValue(f, a)
	ov, _ := c.castValue(f, b)
	if f.Fieldtype == "Datetime" && nv != nil && ov != nil {
		return sameTime(nv, ov, c.E.Location())
	}
	if f.Fieldtype == "JSON" {
		return string(mustJSON(nv)) == string(mustJSON(ov))
	}
	return db.Str(nv) == db.Str(ov)
}

// insertFieldBase is what an insert is compared against: the defaults, or —
// for an amendment — the cancelled document it copies, whose restricted values
// the amending user carries over without having to see them.
func (c *Ctx) insertFieldBase(d *meta.DocType, doc Doc) (Doc, error) {
	if from := doc.Str("amended_from"); from != "" && d.Field("amended_from") != nil && d.Submittable {
		src, err := c.GetDocIgnoringPerms(d.Name, from)
		if err == nil && src.Docstatus() == 2 {
			if ok, err := c.HasPermission(d.Name, "read", src); err != nil {
				return nil, err
			} else if ok {
				for _, tf := range d.TableFields() {
					for _, row := range src.Children(tf.Fieldname) {
						delete(row, "id")
					}
				}
				return src, nil
			}
		}
	}
	return c.fieldDefaults(d), nil
}

// RedactVersionData filters a Version's `data` diff for the current user: a
// change to a field they cannot read is a copy of its value, so it goes too.
// The diff is stored once for every reader, which is why this happens on the
// way out rather than when the Version is written.
func (c *Ctx) RedactVersionData(refDoctype string, data any) any {
	d, err := c.St.DocType(refDoctype)
	if err != nil || !c.hasRestrictedFields(d) {
		return data
	}
	a := c.FieldAccess(d)
	if a.all {
		return data
	}
	var m map[string]any
	switch v := data.(type) {
	case map[string]any:
		m = v
	case string:
		if json.Unmarshal([]byte(v), &m) != nil {
			return nil
		}
	case []byte:
		if json.Unmarshal(v, &m) != nil {
			return nil
		}
	default:
		return data
	}
	changed, _ := m["changed"].(map[string]any)
	for key, pair := range changed {
		f := d.Field(key)
		if !a.CanRead(f) {
			delete(changed, key)
			continue
		}
		if f == nil || !meta.IsTableType(f.Fieldtype) {
			continue
		}
		cd, err := c.St.DocType(f.OptionsString())
		if err != nil || !cd.HasRestrictedFields() {
			continue
		}
		sides, _ := pair.([]any)
		for _, side := range sides {
			rows, _ := side.([]any)
			for _, r := range rows {
				row, _ := r.(map[string]any)
				for _, cf := range cd.Fields {
					if cf.Fieldname != "" && !a.CanRead(cf) {
						delete(row, cf.Fieldname)
					}
				}
			}
		}
	}
	if _, ok := data.(map[string]any); ok {
		return m
	}
	return string(mustJSON(m))
}
