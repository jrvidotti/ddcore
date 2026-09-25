package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Virtual DocTypes (DAT-07): a union of local DocTypes with no table. The
// list reads `FROM (<one SELECT per source> UNION ALL …) AS "t"`, so the
// caller's filters, order, grouping and paging run over the union exactly as
// they run over a table, and Postgres pushes them down into each branch.

// virtualStdTypes are the standard columns' types, for a branch that has no
// source to read them from.
var virtualStdTypes = map[string]string{
	"id": "text", "owner": "text", "creation": "timestamptz", "modified": "timestamptz",
	"modified_by": "text", "docstatus": "smallint",
}

// virtualRelation renders the union a virtual DocType reads from, as a
// parenthesised subquery ready for ` AS "t"`.
//
// perms decides how each branch is authorised, mirroring GetList:
//   - "list": the source must be readable, its permission filters (roles,
//     ifOwner, permissionQuery, scopes, shares, portal) narrow its rows and a
//     mapped field above the user's level on that source reads as NULL;
//   - "scope": every source, narrowed only by the user's access scopes — what
//     GetList does under IgnorePermissions;
//   - "none": every row, for the framework's own lookups.
func (c *Ctx) virtualRelation(d *meta.DocType, b *db.Builder, perms string) (string, error) {
	if c.IgnorePermissions() {
		perms = "none"
	}
	var branches []string
	for _, s := range d.Virtual.Sources {
		src, ok := c.St.Meta.Get(s.DocType)
		if !ok || src == nil {
			return "", fmt.Errorf("virtual source %q of %s does not exist", s.DocType, d.Name)
		}
		access := FullFieldAccess()
		var pf []db.Filter
		switch perms {
		case "list":
			ok, err := c.HasPermission(src.Name, "read", nil)
			if err != nil {
				return "", err
			}
			if !ok {
				continue // a source the user cannot read contributes no rows
			}
			if c.hasRestrictedFields(src) {
				access = c.FieldAccess(src)
			}
			if pf, err = c.permissionFilters(src); err != nil {
				return "", err
			}
		case "scope":
			var err error
			if pf, err = c.scopeFilters(src); err != nil {
				return "", err
			}
		}
		// The branch aliases its table "t" too: inside the subquery that name
		// is the source's, which is what lets the permission filters resolve
		// through the ordinary column resolver.
		cols := []string{
			b.Arg(s.DocType+meta.VirtualSep) + `::text || "t"."id" AS "id"`,
		}
		for _, sc := range meta.StdColumns[1:] {
			cols = append(cols, `"t".`+db.Ident(sc))
		}
		for _, f := range d.DataFields() {
			typ := meta.ColumnType(f.Fieldtype)
			if f.Fieldname == meta.SourceDocTypeField {
				cols = append(cols, b.Arg(s.DocType)+"::text AS "+db.Ident(f.Fieldname))
				continue
			}
			expr := "NULL"
			if sf, mapped := s.Fields[f.Fieldname]; mapped && src.HasColumn(sf) && access.CanRead(src.Field(sf)) {
				expr = `"t".` + db.Ident(sf)
			}
			cols = append(cols, fmt.Sprintf("CAST(%s AS %s) AS %s", expr, typ, db.Ident(f.Fieldname)))
		}
		branch := "SELECT " + strings.Join(cols, ", ") + " FROM " + db.Ident(src.TableName()) + ` AS "t"`
		joins := map[string]*meta.DocType{}
		where, err := c.filterSQL(src, b, pf, c.columnResolver(src, joins))
		if err != nil {
			return "", err
		}
		if where != "" {
			branch += " WHERE " + where
		}
		branches = append(branches, branch)
	}
	if len(branches) == 0 {
		// nothing readable: the same columns, no rows
		var cols []string
		for _, sc := range meta.StdColumns {
			cols = append(cols, "CAST(NULL AS "+virtualStdTypes[sc]+") AS "+db.Ident(sc))
		}
		for _, f := range d.DataFields() {
			cols = append(cols, "CAST(NULL AS "+meta.ColumnType(f.Fieldtype)+") AS "+db.Ident(f.Fieldname))
		}
		branches = append(branches, "SELECT "+strings.Join(cols, ", ")+" WHERE FALSE")
	}
	return "(" + strings.Join(branches, " UNION ALL ") + ")", nil
}

// relationSQL is what a query reads d from: its table, or, for a virtual
// DocType, the union of its sources with no permission applied — for the
// framework's own lookups (a Link's title in a `like` filter), which never
// check permissions on a table either.
func (c *Ctx) relationSQL(d *meta.DocType, b *db.Builder) (string, error) {
	if !d.IsVirtual() {
		return db.Ident(d.TableName()), nil
	}
	return c.virtualRelation(d, b, "none")
}

// virtualSourceOf splits a virtual id and returns the source DocType it names,
// or nil when the id names no source of d.
func (c *Ctx) virtualSourceOf(d *meta.DocType, id string) (*meta.DocType, string) {
	source, sid, ok := meta.SplitVirtualID(id)
	if !ok || d.VirtualSource(source) == nil {
		return nil, ""
	}
	src, ok := c.St.Meta.Get(source)
	if !ok {
		return nil, ""
	}
	return src, sid
}

// getVirtualDoc reads one row of a virtual DocType through the list path, so
// it is authorised exactly as the list is.
func (c *Ctx) getVirtualDoc(d *meta.DocType, id string, checkPerm bool) (Doc, error) {
	if src, _ := c.virtualSourceOf(d, id); src == nil {
		return nil, cerr.NotFound("{0} {1} not found", c.T(d.Label), id)
	}
	rows, err := c.GetList(d.Name, ListArgs{Filters: map[string]any{"id": id}, Fields: []string{"*"}, Limit: 1, IgnorePermissions: !checkPerm})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if checkPerm {
			// the row exists but the source's rules hide it: the same answer a
			// table gives for a document the user may not read
			if ok, _ := c.virtualIDExists(d, id); ok {
				return nil, cerr.Permission("No permission to read {0} {1}", c.T(d.Label), id)
			}
		}
		return nil, cerr.NotFound("{0} {1} not found", c.T(d.Label), id)
	}
	doc := Doc(rows[0])
	doc["doctype"] = d.Name
	return doc, nil
}

// virtualIDExists is idExists for a virtual DocType: the id names one of its
// sources and that source has the row.
func (c *Ctx) virtualIDExists(d *meta.DocType, id string) (bool, error) {
	src, sid := c.virtualSourceOf(d, id)
	if src == nil {
		return false, nil
	}
	return c.idExists(src.Name, sid)
}

// refuseVirtual is the write guard: a virtual DocType has no table to write.
func refuseVirtual(d *meta.DocType) error {
	if d != nil && d.IsVirtual() {
		return cerr.Validation("{0} is a virtual DocType and cannot be written", d.Label)
	}
	return nil
}

// virtualTitles resolves the titles of virtual ids, reading each source's
// column mapped from the virtual titleField. No permission check, like
// LinkTitles on a table.
func (c *Ctx) virtualTitles(d *meta.DocType, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if d.TitleField == "" || d.TitleField == "id" {
		return out, nil
	}
	bySource := map[string][]string{}
	for _, id := range ids {
		if src, sid := c.virtualSourceOf(d, id); src != nil {
			bySource[src.Name] = append(bySource[src.Name], sid)
		}
	}
	for name, sids := range bySource {
		src, _ := c.St.Meta.Get(name)
		col := d.VirtualSource(name).Fields[d.TitleField]
		if col == "" || !src.HasColumn(col) {
			continue
		}
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id, %s::text AS title FROM %s WHERE id = ANY($1)",
			db.Ident(col), db.Ident(src.TableName())), sids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if t := db.Str(r["title"]); t != "" {
				out[meta.VirtualID(name, db.Str(r["id"]))] = t
			}
		}
	}
	return out, nil
}

// virtualIDsSQL is the ids of a virtual DocType as a relation with one `id`
// column, for the framework's own set-based checks (dangling links) that join
// against a target's ids with no Ctx at hand. Source names are declarations,
// not input, but they are quoted as literals all the same.
func virtualIDsSQL(d *meta.DocType) string {
	var parts []string
	for _, s := range d.Virtual.Sources {
		lit := "'" + strings.ReplaceAll(s.DocType+meta.VirtualSep, "'", "''") + "'"
		parts = append(parts, fmt.Sprintf("SELECT %s || id AS id FROM %s", lit, db.Ident("tab_"+meta.Snake(s.DocType))))
	}
	return "(" + strings.Join(parts, " UNION ALL ") + ")"
}

// tableOrVirtualIDs is what a set-based check joins a Link target on.
func tableOrVirtualIDs(d *meta.DocType) string {
	if d.IsVirtual() {
		return virtualIDsSQL(d)
	}
	return db.Ident(d.TableName())
}

// isVirtualOver reports whether target is a virtual DocType taking rows from
// source — whether a Link to target may hold "<source>:<id>".
func (c *Ctx) isVirtualOver(target, source string) bool {
	td, ok := c.St.Meta.Get(target)
	return ok && td != nil && td.VirtualSource(source) != nil
}

// virtualLinkColumns is every (table, column) of a Link to a virtual DocType
// that takes rows from source, sorted so the statements run in a fixed order.
func (c *Ctx) virtualLinkColumns(source string) [][2]string {
	var out [][2]string
	for _, d := range c.St.Meta.DocTypes {
		if d.IsVirtual() {
			continue
		}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" && c.isVirtualOver(f.OptionsString(), source) {
				out = append(out, [2]string{d.TableName(), f.Fieldname})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
