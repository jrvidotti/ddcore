package engine

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

type ListArgs struct {
	Filters           any      `json:"filters"`
	OrFilters         any      `json:"orFilters"`
	Fields            []string `json:"fields"`
	OrderBy           string   `json:"orderBy"`
	Limit             int      `json:"limit"`
	Start             int      `json:"start"`
	GroupBy           string   `json:"groupBy"`
	IgnorePermissions bool     `json:"ignorePermissions"`
	Distinct          bool     `json:"distinct"`
	// rankText puts the rows whose rankFields equal it, then those starting
	// with it, ahead of the rest, before OrderBy. It is how LinkSearch
	// ranks by match quality; not settable from the API.
	rankText   string
	rankFields []string
	// linkOrder stands in for OrderBy (when that is empty) with text columns
	// compared as a reader would — case and accents folded — whatever the
	// database's collation. Set by LinkSearch.
	linkOrder []meta.OrderTerm
}

var aggRe = regexp.MustCompile(`(?i)^\s*(count|sum|avg|min|max)\s*\(\s*(\*|[a-z_][a-z0-9_]*)\s*\)\s*(?:as\s+([a-z_][a-z0-9_]*))?\s*$`)
var aliasRe = regexp.MustCompile(`(?i)^\s*([a-z_][a-z0-9_]*)\s+as\s+([a-z_][a-z0-9_]*)\s*$`)

// columnResolver maps a filter/field name to a SQL expression for doctype d.
// "Child DocType.field" resolves to a subquery-friendly alias via joins.
func (c *Ctx) columnResolver(d *meta.DocType, joins map[string]*meta.DocType) func(string) string {
	return func(field string) string {
		field = strings.Trim(field, "`\"")
		if i := strings.Index(field, "."); i > 0 {
			ct, cf := field[:i], field[i+1:]
			child, ok := c.St.Meta.Get(ct)
			if !ok || !child.HasColumn(cf) {
				return ""
			}
			joins[ct] = child
			return db.Ident("c_"+meta.Snake(ct)) + "." + db.Ident(cf)
		}
		if d.HasColumn(field) {
			return db.Ident("t") + "." + db.Ident(field)
		}
		return ""
	}
}

// canReadColumn reports whether a list may expose field ("field" or
// "Child DocType.field") under the given field access. A child column is
// hidden when the column itself or every Table field embedding it is.
func (c *Ctx) canReadColumn(d *meta.DocType, a FieldAccess, field string) bool {
	if a.all {
		return true
	}
	field = strings.Trim(strings.TrimSpace(field), "`\"")
	if i := strings.Index(field, "."); i > 0 {
		ct, cf := field[:i], field[i+1:]
		child, ok := c.St.Meta.Get(ct)
		if !ok {
			return true // unknown: the resolver reports it
		}
		if !a.CanRead(child.Field(cf)) {
			return false
		}
		for _, tf := range d.TableFields() {
			if strings.EqualFold(tf.OptionsString(), ct) && !a.CanRead(tf) {
				return false
			}
		}
		return true
	}
	return a.CanRead(d.Field(field))
}

// readableColumns is `"t".*` narrowed to the columns the access may read.
func (c *Ctx) readableColumns(d *meta.DocType, a FieldAccess) []string {
	var out []string
	cols := append([]string(nil), meta.StdColumns...)
	if d.IsChild {
		cols = append(cols, meta.ChildColumns...)
	}
	for _, name := range cols {
		out = append(out, `"t".`+db.Ident(name))
	}
	for _, f := range d.DataFields() {
		if a.CanRead(f) {
			out = append(out, `"t".`+db.Ident(f.Fieldname))
		}
	}
	return out
}

// hasChildTable reports whether ct is the options of a Table field of d.
func hasChildTable(d *meta.DocType, ct string) bool {
	for _, f := range d.TableFields() {
		if strings.EqualFold(f.OptionsString(), ct) {
			return true
		}
	}
	return false
}

// treeRef resolves the hierarchy a tree operator walks: the DocType's own `id`
// when it is a tree, or the target of a Link pointing at one. A Dynamic Link is
// not resolved here — its target is a value, not a declaration — so the one
// place that filters one (a User Permission scope, which knows the DocType from
// the rule) carries its own TreeRef.
//
// Returning nil leaves db.Builder.Where to refuse the filter by name.
func (c *Ctx) treeRef(d *meta.DocType, field string) *db.TreeRef {
	field = strings.Trim(field, "`\"")
	target := d
	if field != "id" {
		f := d.Field(field)
		if f == nil || f.Fieldtype != "Link" {
			return nil
		}
		t, ok := c.St.Meta.Get(f.OptionsString())
		if !ok || t == nil {
			return nil
		}
		target = t
	}
	if !target.IsTree {
		return nil
	}
	return &db.TreeRef{Table: target.TableName(), ParentCol: target.TreeParentField()}
}

// checkFilter casts the value of a filter on a Check field to a boolean, the
// way a saved value is cast: a boolean column refuses the 1, "1" or "true" a
// workspace card, a route or app code naturally writes.
func (c *Ctx) checkFilter(fld *meta.Field, f db.Filter) db.Filter {
	if fld == nil || fld.Fieldtype != "Check" || f.Value == nil {
		return f
	}
	cast := func(v any) any { b, _ := c.castValue(fld, v); return b }
	switch strings.ToLower(strings.TrimSpace(f.Op)) {
	case "", "=", "!=":
		f.Value = cast(f.Value)
	case "in", "not in":
		vals, ok := f.Value.([]any)
		if !ok {
			if s, isStr := f.Value.(string); isStr {
				for _, p := range strings.Split(s, ",") {
					vals = append(vals, strings.TrimSpace(p))
				}
			} else {
				vals = []any{f.Value}
			}
		}
		out := make([]any, len(vals))
		for i, v := range vals {
			out[i] = cast(v)
		}
		f.Value = out
	}
	return f
}

// filterSQL renders filters into a WHERE fragment. Conditions over a child
// doctype become `EXISTS (SELECT 1 FROM tab_child ...)` instead of a JOIN: a
// parent with two matching child rows would appear twice in the list
// and be counted twice by count(*) (B19). All conditions on the same
// child go into the same EXISTS, i.e., they require the same row.
func (c *Ctx) filterSQL(d *meta.DocType, b *db.Builder, filters []db.Filter, col func(string) string) (string, error) {
	var own []db.Filter
	var order []string
	byChild := map[string][]db.Filter{}
	for _, f := range filters {
		name := strings.Trim(f.Field, "`\"")
		i := strings.Index(name, ".")
		if i <= 0 {
			own = append(own, f)
			continue
		}
		ct, cf := name[:i], name[i+1:]
		child, ok := c.St.Meta.Get(ct)
		if !ok || !child.IsChild || !child.HasColumn(cf) {
			return "", fmt.Errorf("unknown field in filter: %q", f.Field)
		}
		if !hasChildTable(d, ct) {
			return "", fmt.Errorf("%s is not a child table of %s", ct, d.Name)
		}
		if _, seen := byChild[ct]; !seen {
			order = append(order, ct)
		}
		// a copy, not a literal: a filter carries more than field/op/value
		// (Tree, IfField), and rebuilding it would drop the rest.
		sub := c.checkFilter(child.Field(cf), f)
		sub.Field = cf
		if db.TreeOps[strings.ToLower(strings.TrimSpace(sub.Op))] && sub.Tree == nil {
			sub.Tree = c.treeRef(child, cf)
		}
		byChild[ct] = append(byChild[ct], sub)
	}
	var parts []string
	for _, f := range own {
		if f.Any != nil {
			var groups []string
			for _, g := range f.Any {
				w, err := c.filterSQL(d, b, g, col)
				if err != nil {
					return "", err
				}
				if w == "" {
					w = "TRUE"
				}
				groups = append(groups, "("+w+")")
			}
			if len(groups) == 0 {
				parts = append(parts, "FALSE")
			} else {
				parts = append(parts, "("+strings.Join(groups, " OR ")+")")
			}
			continue
		}
		if f.IfField != "" {
			ifCol := col(f.IfField)
			if ifCol == "" {
				return "", fmt.Errorf("unknown field in conditional filter: %q", f.IfField)
			}
			inner := f
			inner.IfField, inner.IfValue = "", nil
			w, err := b.Where([]db.Filter{inner}, col)
			if err != nil {
				return "", err
			}
			parts = append(parts, fmt.Sprintf("(%s IS DISTINCT FROM %s OR (%s))", ifCol, b.Arg(f.IfValue), w))
			continue
		}
		op := strings.ToLower(strings.TrimSpace(f.Op))
		if db.TreeOps[op] && f.Tree == nil {
			f.Tree = c.treeRef(d, f.Field)
		}
		fld := d.Field(f.Field)
		if (op == "like" || op == "not like") && fld != nil && fld.Fieldtype == "Link" {
			targetDoc := fld.OptionsString()
			if td, ok := c.St.Meta.Get(targetDoc); ok && td != nil {
				var targetCols []string
				seenCols := map[string]bool{"id": true}
				if td.TitleField != "" && td.TitleField != "id" && td.HasColumn(td.TitleField) {
					targetCols = append(targetCols, td.TitleField)
					seenCols[td.TitleField] = true
				}
				for _, sf := range td.SearchFields {
					sf = strings.TrimSpace(sf)
					if sf != "" && !seenCols[sf] && td.HasColumn(sf) {
						targetCols = append(targetCols, sf)
						seenCols[sf] = true
					}
				}
				if len(targetCols) > 0 {
					colExpr := col(f.Field)
					if colExpr == "" {
						return "", fmt.Errorf("unknown field in filter: %q", f.Field)
					}
					arg := b.Arg(f.Value)
					alias := db.Ident("lt_" + meta.Snake(fld.Fieldname))
					var targetConds []string
					for _, tc := range targetCols {
						targetConds = append(targetConds, fmt.Sprintf("%s ILIKE %s", db.AccentInsensitive(alias+"."+db.Ident(tc)), db.AccentInsensitive(arg)))
					}
					targetWhere := strings.Join(targetConds, " OR ")
					rel, err := c.relationSQL(td, b)
					if err != nil {
						return "", err
					}
					existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM %s AS %s WHERE %s.%s = %s AND (%s))",
						rel, alias, alias, db.Ident("id"), colExpr, targetWhere)
					directCond := fmt.Sprintf("%s ILIKE %s", db.AccentInsensitive(colExpr), db.AccentInsensitive(arg))
					if op == "like" {
						parts = append(parts, fmt.Sprintf("(%s OR %s)", directCond, existsClause))
					} else {
						parts = append(parts, fmt.Sprintf("NOT (%s OR %s)", directCond, existsClause))
					}
					continue
				}
			}
		}

		w, err := b.Where([]db.Filter{c.checkFilter(fld, f)}, col)
		if err != nil {
			return "", err
		}
		if w != "" {
			parts = append(parts, w)
		}
	}
	for _, ct := range order {
		child, _ := c.St.Meta.Get(ct)
		alias := db.Ident("ec_" + meta.Snake(ct))
		w, err := b.Where(byChild[ct], func(field string) string {
			if !child.HasColumn(field) {
				return ""
			}
			return alias + "." + db.Ident(field)
		})
		if err != nil {
			return "", err
		}
		parentKey := col("id")
		if parentKey == "" {
			return "", fmt.Errorf("unknown field in filter: %q", "id")
		}
		parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM %s AS %s WHERE %s.parent = %s AND %s.parenttype = %s AND %s)",
			db.Ident(child.TableName()), alias, alias, parentKey, alias, b.Arg(d.Name), w))
	}
	switch len(parts) {
	case 0:
		return "", nil
	case 1:
		return parts[0], nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

// GetList runs a filtered query over a doctype.
func (c *Ctx) GetList(doctype string, a ListArgs) ([]map[string]any, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if err := c.spaceRefusal(d, false, a.IgnorePermissions || c.IgnorePermissions()); err != nil {
		return nil, err
	}
	if !a.IgnorePermissions && !c.IgnorePermissions() {
		if ok, err := c.HasPermission(doctype, "read", nil); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("No permission to list {0}", c.T(d.Label))
		}
	}
	joins := map[string]*meta.DocType{}
	resolve := c.columnResolver(d, joins)
	// field permissions (SEC-02): a field the user cannot read is left out of
	// the select list, and naming it anywhere it would shape the result —
	// a filter, a sort, a grouping, an aggregate — is refused, so the query
	// cannot be used as an oracle for the value.
	access := FullFieldAccess()
	if !a.IgnorePermissions && !c.IgnorePermissions() && c.hasRestrictedFields(d) {
		access = c.FieldAccess(d)
	}
	denied := ""
	col := func(field string) string {
		if !c.canReadColumn(d, access, field) {
			if denied == "" {
				denied = strings.Trim(field, "`\"")
			}
			return ""
		}
		return resolve(field)
	}
	deniedErr := func() error {
		return cerr.Permission("No permission to read field {0} of {1}", denied, c.T(d.Label))
	}
	var b db.Builder

	// select list
	var sel []string
	if len(a.Fields) == 0 {
		a.Fields = []string{"id"}
	}
	hasAgg := false
	for _, f := range a.Fields {
		f = strings.TrimSpace(f)
		if f == "*" {
			if access.all || !c.hasRestrictedFields(d) {
				sel = append(sel, `"t".*`)
			} else {
				sel = append(sel, c.readableColumns(d, access)...)
			}
			continue
		}
		if m := aggRe.FindStringSubmatch(f); m != nil {
			hasAgg = true
			arg := "*"
			if m[2] != "*" {
				arg = col(m[2])
				if denied != "" {
					return nil, deniedErr()
				}
				if arg == "" {
					return nil, cerr.Validation("Unknown field: {0}", m[2])
				}
			}
			alias := m[3]
			if alias == "" {
				alias = strings.ToLower(m[1])
			}
			sel = append(sel, fmt.Sprintf("%s(%s) AS %s", strings.ToUpper(m[1]), arg, db.Ident(alias)))
			continue
		}
		if m := aliasRe.FindStringSubmatch(f); m != nil {
			if !c.canReadColumn(d, access, m[1]) {
				continue
			}
			cexpr := col(m[1])
			if cexpr == "" {
				return nil, cerr.Validation("Unknown field: {0}", m[1])
			}
			sel = append(sel, cexpr+" AS "+db.Ident(m[2]))
			continue
		}
		if !c.canReadColumn(d, access, f) {
			continue
		}
		cexpr := col(f)
		if cexpr == "" {
			return nil, cerr.Validation("Unknown field: {0}", f)
		}
		if strings.Contains(f, ".") {
			sel = append(sel, cexpr+" AS "+db.Ident(strings.ReplaceAll(strings.ToLower(meta.Snake(f)), ".", "_")))
		} else {
			sel = append(sel, cexpr)
		}
	}

	filters, err := db.ParseFilters(a.Filters)
	if err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	orFilters, err := db.ParseFilters(a.OrFilters)
	if err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	if len(sel) == 0 {
		sel = append(sel, `"t".`+db.Ident("id"))
	}
	// a Version's diff is filtered by the reader's access to the document it
	// describes, so that DocType has to come back with the row
	versionRef := d.Name == "Version" && !a.IgnorePermissions && !c.IgnorePermissions() &&
		c.User != "Admin" && !hasAgg && a.GroupBy == ""
	if versionRef {
		sel = append(sel, `"t".`+db.Ident("ref_doctype")+" AS "+db.Ident("__version_ref"))
	}
	// checked before the permission filters join them: those are the
	// framework's own conditions, and may well sit on a restricted Link
	for _, name := range db.Fields(append(append([]db.Filter(nil), filters...), orFilters...)) {
		if !c.canReadColumn(d, access, name) {
			denied = strings.Trim(name, "`\"")
			return nil, deniedErr()
		}
	}
	if !c.IgnorePermissions() && !d.IsVirtual() {
		var pf []db.Filter
		if a.IgnorePermissions {
			pf, err = c.scopeFilters(d)
		} else {
			pf, err = c.permissionFilters(d)
		}
		if err != nil {
			return nil, err
		}
		filters = append(filters, pf...)
	}
	// filters resolve without the field check: the caller's were vetted above,
	// and the permission filters are the framework's own
	where, err := c.filterSQL(d, &b, filters, resolve)
	if err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	// A delete keeps a document's Versions (#28), so a document later created
	// under the same id would list the deleted one's history as its own. Only a
	// System Manager, who reads every Version anyway, sees past the deletion.
	if d.Name == "Version" && !a.IgnorePermissions && !c.IgnorePermissions() && c.User != "Admin" && !c.HasRole("System Manager") {
		w := `NOT EXISTS (SELECT 1 FROM tab_version dv WHERE dv.ref_doctype = "t".ref_doctype AND dv.doc_id = "t".doc_id AND dv.deleted AND dv.creation >= "t".creation)`
		if where == "" {
			where = w
		} else {
			where += " AND " + w
		}
	}
	if len(orFilters) > 0 {
		var ors []string
		for _, f := range orFilters {
			w, err := c.filterSQL(d, &b, []db.Filter{f}, resolve)
			if err != nil {
				return nil, cerr.Validation("Invalid filters: {0}", err)
			}
			ors = append(ors, w)
		}
		orw := "(" + strings.Join(ors, " OR ") + ")"
		if where == "" {
			where = orw
		} else {
			where += " AND " + orw
		}
	}
	orderBy, err := db.ParseOrderBy(a.OrderBy, col)
	if denied != "" {
		return nil, deniedErr()
	}
	if err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	if orderBy == "" && len(a.linkOrder) > 0 && !hasAgg && a.GroupBy == "" {
		var parts []string
		for _, t := range a.linkOrder {
			e := col(t.Field)
			if e == "" {
				continue
			}
			dir := " ASC"
			if t.Desc {
				dir = " DESC"
			}
			if f := d.Field(t.Field); t.Field == "id" || (f != nil && meta.ColumnType(f.Fieldtype) == "text") {
				parts = append(parts, db.AccentInsensitive(e)+dir)
			}
			parts = append(parts, e+dir)
		}
		orderBy = strings.Join(parts, ", ")
	}
	if orderBy == "" && !hasAgg && a.GroupBy == "" {
		sf, so := d.SortField, strings.ToUpper(d.SortOrder)
		if sf == "" || !d.HasColumn(sf) {
			sf, so = "modified", "DESC"
		}
		if so == "" {
			so = "DESC"
		}
		orderBy = `"t".` + db.Ident(sf) + " " + so
	}
	if a.rankText != "" && !hasAgg && a.GroupBy == "" {
		exact := db.AccentInsensitive(b.Arg(a.rankText))
		prefix := db.AccentInsensitive(b.Arg(escapeLike(a.rankText) + "%"))
		var eqs, starts []string
		for _, f := range a.rankFields {
			if e := col(f); e != "" {
				eqs = append(eqs, db.AccentInsensitive(e)+" = "+exact)
				starts = append(starts, db.AccentInsensitive(e)+" LIKE "+prefix)
			}
		}
		if len(eqs) > 0 {
			rank := "CASE WHEN " + strings.Join(eqs, " OR ") + " THEN 0 WHEN " + strings.Join(starts, " OR ") + " THEN 1 ELSE 2 END"
			orderBy = rank + ", " + orderBy
		}
	}

	sql := "SELECT "
	if a.Distinct {
		sql += "DISTINCT "
	}
	from := db.Ident(d.TableName())
	if d.IsVirtual() {
		// each source is authorised inside its own branch of the union; the
		// virtual DocType's own rules only decided, above, who may list it
		perms := "list"
		if a.IgnorePermissions {
			perms = "scope"
		}
		if from, err = c.virtualRelation(d, &b, perms); err != nil {
			return nil, err
		}
		if len(joins) > 0 {
			return nil, cerr.Validation("{0} is a virtual DocType and has no child tables", c.T(d.Label))
		}
	}
	sql += strings.Join(sel, ", ") + " FROM " + from + ` AS "t"`
	for ct, child := range joins {
		alias := db.Ident("c_" + meta.Snake(ct))
		sql += fmt.Sprintf(" JOIN %s AS %s ON %s.parent = \"t\".id AND %s.parenttype = %s", db.Ident(child.TableName()), alias, alias, alias, b.Arg(d.Name))
	}
	if where != "" {
		sql += " WHERE " + where
	}
	if a.GroupBy != "" {
		g := col(a.GroupBy)
		if denied != "" {
			return nil, deniedErr()
		}
		if g == "" {
			return nil, cerr.Validation("Unknown groupBy: {0}", a.GroupBy)
		}
		sql += " GROUP BY " + g
	}
	if orderBy != "" {
		sql += " ORDER BY " + orderBy
	}
	if a.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", a.Limit)
	}
	if a.Start > 0 {
		sql += fmt.Sprintf(" OFFSET %d", a.Start)
	}
	rows, err := db.Select(c.Ctx, c.Q(), sql, b.Args...)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	if versionRef {
		for _, r := range rows {
			if r["data"] != nil {
				r["data"] = c.RedactVersionData(db.Str(r["__version_ref"]), r["data"])
			}
			delete(r, "__version_ref")
		}
	}
	for _, r := range rows {
		c.clampRatings(d, Doc(r))
	}
	return rows, nil
}

// Count returns the number of documents matching filters. orFilters is
// optional and shares the semantics of ListArgs.OrFilters, so a listagem e a
// contagem podem usar exatamente as mesmas condições (B19).
func (c *Ctx) Count(doctype string, filters any, orFilters ...any) (int64, error) {
	var or any
	if len(orFilters) > 0 {
		or = orFilters[0]
	}
	rows, err := c.GetList(doctype, ListArgs{Filters: filters, OrFilters: or, Fields: []string{"count(*) as n"}})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int64(toFloat(rows[0]["n"])), nil
}

// Exists is db.exists by name: no role permission check, but the user's access
// scope applies, as for ExistsWhere. A DocType closed to users with access
// scopes never exists for such a user.
//
// Only server code calls it (ddcore.db.exists), so a DocType tenants reach
// through server code alone answers it there (#108).
func (c *Ctx) Exists(doctype, name string) (bool, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, err
	}
	if err := c.spaceRefusal(d, false, true); err != nil {
		return false, err
	}
	if refused, err := c.refusedToScopedUser(d.Name); err != nil || refused {
		return false, err
	}
	if perms, err := c.UserPermissions(); err != nil {
		return false, err
	} else if len(perms) > 0 {
		found, err := c.ExistsWhere(d.Name, map[string]any{"id": name})
		return found != "", err
	}
	return c.idExists(d.Name, name)
}

// idExists reports whether a name is taken, whoever can see it: the
// framework's own duplicate-name, rename and link checks use it.
func (c *Ctx) idExists(doctype, name string) (bool, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, err
	}
	if d.IsVirtual() {
		return c.virtualIDExists(d, name)
	}
	var one int
	err = c.Q().QueryRow(c.Ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE id = $1", db.Ident(d.TableName())), name).Scan(&one)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ExistsWhere returns the name of the first document matching filters.
func (c *Ctx) ExistsWhere(doctype string, filters any) (string, error) {
	rows, err := c.GetList(doctype, ListArgs{Filters: filters, Fields: []string{"id"}, Limit: 1, IgnorePermissions: true})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return db.Str(rows[0]["id"]), nil
}

// GetValue reads one field of a document (no permission check, like frappe.db).
func (c *Ctx) GetValue(doctype, name, field string) (any, error) {
	m, err := c.GetValues(doctype, name, []string{field})
	if err != nil || m == nil {
		return nil, err
	}
	return m[field], nil
}

// GetSingleValue is db.getSingleValue: one field of a Single, read like
// GetValue from its "singleton" row. Before the first save there is no row,
// so it answers the field's default, as getDoc would.
func (c *Ctx) GetSingleValue(doctype, field string) (any, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if !d.IsSingle {
		return nil, cerr.Validation("{0} is not a Single DocType", d.Name)
	}
	m, err := c.GetValues(doctype, "singleton", []string{field})
	if err != nil || m != nil {
		return m[field], err
	}
	doc, err := c.NewDoc(doctype, nil)
	if err != nil {
		return nil, err
	}
	return doc[field], nil
}

// GetValues reads several fields; name may be a name or filters.
func (c *Ctx) GetValues(doctype string, name any, fields []string) (map[string]any, error) {
	var filters any
	if s, ok := name.(string); ok {
		filters = map[string]any{"id": s}
	} else {
		filters = name
	}
	rows, err := c.GetList(doctype, ListArgs{Filters: filters, Fields: fields, Limit: 1, IgnorePermissions: true})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// SetValue is db.setValue: direct column update with modified refresh.
func (c *Ctx) SetValue(doctype, name string, values Doc) error {
	_, err := c.DBSet(doctype, name, values, true)
	return err
}

// PatchSQL runs any statement — read or write — inside the migration's
// transaction. It is the backfill primitive, and the only write-SQL in the
// framework.
//
// ddcore.db.sql is read-only by design, and going through setValue row by row
// rewrites modified/modified_by on every row and writes a Version per row for a
// trackChanges DocType: an audit trail that says a person edited the data when
// a migration moved it. A patch is already the most privileged thing here — it
// runs as Admin inside the migration — so DDL is allowed too. That is
// deliberate: it keeps the planner from ever being a dead end, because anything
// it refuses the author can still do by hand in a beforeSchema patch.
func (c *Ctx) PatchSQL(query string, params []any) ([]map[string]any, error) {
	if c.Flags["inPatch"] != true {
		return nil, cerr.Permission("ctx.sql is only available inside a patch")
	}
	rows, err := db.Select(c.Ctx, c.Q(), query, params...)
	if err != nil {
		return nil, cerr.Validation("patch sql: {0}", err.Error())
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

// SQL runs a read-only query (used by reports and ddcore.db.sql).
//
// The prefix check is only a first filter: a CTE can hide an UPDATE behind a
// SELECT. The query really runs inside a savepoint with
// `SET LOCAL transaction_read_only = on`, so any write is refused by Postgres
// and the savepoint is rolled back afterwards, leaving the caller's
// transaction usable (and its `SET LOCAL` undone).
func (c *Ctx) SQL(query string, params []any) ([]map[string]any, error) {
	if !isReadQuery(query) {
		return nil, cerr.Permission("ddcore.db.sql only accepts SELECT")
	}
	if c.Tx == nil {
		rows, err := db.Select(c.Ctx, c.Q(), query, params...)
		if err != nil {
			return nil, cerr.Validation("Invalid filters: {0}", err)
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		return rows, nil
	}
	c.roSavepoint++
	sp := fmt.Sprintf("ddcore_ro%d", c.roSavepoint)
	defer func() { c.roSavepoint-- }()
	if _, err := c.Tx.Exec(c.Ctx, "SAVEPOINT "+sp); err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	rollback := func() {
		c.Tx.Exec(c.Ctx, "ROLLBACK TO SAVEPOINT "+sp)
		c.Tx.Exec(c.Ctx, "RELEASE SAVEPOINT "+sp)
	}
	if _, err := c.Tx.Exec(c.Ctx, "SET LOCAL transaction_read_only = on"); err != nil {
		rollback()
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	rows, err := db.Select(c.Ctx, c.Tx, query, params...)
	rollback()
	if err != nil {
		return nil, cerr.Validation("Invalid filters: {0}", err)
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

// Lock takes a transaction-scoped advisory lock, serialising every ctx that
// asks for the same key until the transaction ends. Inside a tenant the key
// is scoped to it, as cache keys are: two tenants locking the same key never
// wait for each other, and a site-wide lock is taken from the platform space.
func (c *Ctx) Lock(key string) error {
	if strings.TrimSpace(key) == "" {
		return cerr.Validation("ddcore.db.lock: provide a key")
	}
	if c.Tx == nil {
		return cerr.Validation("ddcore.db.lock requires a transaction")
	}
	_, err := c.Tx.Exec(c.Ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", c.lockKey(key))
	return err
}

// TryLock is Lock without the wait: it takes the key and returns true, or
// returns false at once when another transaction holds it. A job that cannot
// have the key can then queue itself again and free its worker, instead of
// parking it for as long as the holder takes.
func (c *Ctx) TryLock(key string) (bool, error) {
	if strings.TrimSpace(key) == "" {
		return false, cerr.Validation("ddcore.db.tryLock: provide a key")
	}
	if c.Tx == nil {
		return false, cerr.Validation("ddcore.db.tryLock requires a transaction")
	}
	var got bool
	err := c.Tx.QueryRow(c.Ctx, "SELECT pg_try_advisory_xact_lock(hashtext($1))", c.lockKey(key)).Scan(&got)
	return got, err
}

// lockKey scopes a lock's key to the tenant, so Lock and TryLock on the same
// key meet. The separator is text, not appCacheKey's NUL, which Postgres text
// refuses.
func (c *Ctx) lockKey(key string) string {
	if c.Tenant != "" {
		return "tenant:" + c.Tenant + ":" + key
	}
	return key
}

// linkSearchScan caps how many documents a TranslateID search reads to match
// the typed text against translated ids; such DocTypes hold a few dozen keys.
const linkSearchScan = 1000

// LinkSearch backs the Link control: searches name + searchFields. The
// options come best match first — the id or title equal to txt, then starting
// with it — and otherwise in linkOrder, not in the list's sortField.
func (c *Ctx) LinkSearch(doctype, txt string, filters any, limit int) ([]map[string]any, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if d.TitleIsTranslatedID() {
		return c.linkSearchTranslated(d, txt, filters, limit)
	}
	args := searchArgs(d, txt)
	// The subtitle columns are read for display only; the match stays on
	// searchFieldsOf.
	for _, f := range d.LinkSubtitle {
		if d.HasColumn(f) && !slices.Contains(args.Fields, f) {
			args.Fields = append(args.Fields, f)
		}
	}
	args.Filters = filters
	args.Limit = limit
	args.linkOrder = linkOrder(d)
	if txt = strings.TrimSpace(txt); txt != "" {
		args.rankText = txt
		args.rankFields = []string{"id"}
		if d.TitleField != "" && d.TitleField != "id" && d.HasColumn(d.TitleField) {
			args.rankFields = append(args.rankFields, d.TitleField)
		}
	}
	return c.GetList(doctype, args)
}

// linkOrder is the order a Link dropdown lists its options in: the DocType's
// linkOrderBy, or else its title A to Z. Empty — no title field — leaves it
// to sortField. The id comes last so equal titles keep a stable order.
func linkOrder(d *meta.DocType) []meta.OrderTerm {
	terms, _ := d.LinkOrder()
	if len(terms) == 0 && d.TitleField != "" && d.HasColumn(d.TitleField) {
		terms = []meta.OrderTerm{{Field: d.TitleField}}
	}
	if len(terms) == 0 {
		return nil
	}
	if !slices.ContainsFunc(terms, func(t meta.OrderTerm) bool { return t.Field == "id" }) {
		terms = append(terms, meta.OrderTerm{Field: "id"})
	}
	return terms
}

// linkSearchTranslated searches a TranslateID DocType: the typed text matches
// the id or its translation, so "Gerente" finds "HR Manager" in pt-BR, and
// each row carries the translation as `_title` for the Link control to show.
// The rows rank as LinkSearch's do, and are otherwise in the order of their
// translation — unless linkOrderBy says otherwise.
func (c *Ctx) linkSearchTranslated(d *meta.DocType, txt string, filters any, limit int) ([]map[string]any, error) {
	args := ListArgs{Fields: []string{"id"}, Filters: filters, Limit: linkSearchScan}
	args.linkOrder, _ = d.LinkOrder()
	for _, f := range d.LinkSubtitle {
		if d.HasColumn(f) && !slices.Contains(args.Fields, f) {
			args.Fields = append(args.Fields, f)
		}
	}
	rows, err := c.GetList(d.Name, args)
	if err != nil {
		return nil, err
	}
	needle := db.FoldAccents(strings.TrimSpace(txt))
	type match struct {
		row   map[string]any
		rank  int
		title string
	}
	var found []match
	for _, r := range rows {
		id := fmt.Sprint(r["id"])
		title := c.T(id)
		fid, ftitle := db.FoldAccents(id), db.FoldAccents(title)
		rank := 2
		switch {
		case needle == "":
		case fid == needle || ftitle == needle:
			rank = 0
		case strings.HasPrefix(fid, needle) || strings.HasPrefix(ftitle, needle):
			rank = 1
		case !strings.Contains(fid, needle) && !strings.Contains(ftitle, needle):
			continue
		}
		r["_title"] = title
		found = append(found, match{r, rank, ftitle})
	}
	slices.SortStableFunc(found, func(a, b match) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if d.LinkOrderBy != "" {
			return 0
		}
		return strings.Compare(a.title, b.title)
	})
	out := []map[string]any{}
	for _, m := range found {
		if len(out) == limit {
			break
		}
		out = append(out, m.row)
	}
	return out, nil
}

// searchFieldsOf lists the columns a text search matches: name, the title
// field, then each search field, without duplicates.
func searchFieldsOf(d *meta.DocType) []string {
	fields := []string{"id"}
	seen := map[string]bool{"id": true}
	if d.TitleField != "" && d.HasColumn(d.TitleField) && !seen[d.TitleField] {
		fields = append(fields, d.TitleField)
		seen[d.TitleField] = true
	}
	for _, sf := range d.SearchFields {
		if d.HasColumn(sf) && !seen[sf] {
			fields = append(fields, sf)
			seen[sf] = true
		}
	}
	return fields
}

// searchArgs selects the search fields and, for a non-empty txt, ORs a
// substring match over each of them.
func searchArgs(d *meta.DocType, txt string) ListArgs {
	fields := searchFieldsOf(d)
	args := ListArgs{Fields: fields}
	if txt != "" {
		needle := escapeLike(txt)
		var ors []any
		for _, f := range fields {
			ors = append(ors, []any{f, "like", "%" + needle + "%"})
		}
		args.OrFilters = ors
	}
	return args
}

// escapeLike neuters LIKE's wildcards in text a person typed: searching for
// `100%` or `a_b` looks for those characters, not for anything at all. A
// `like` filter an app writes itself is left alone — there the wildcards are
// the point.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return r.Replace(s)
}

// ResolveLinkTitles loads the titles for all Link and Dynamic Link values in docs
// according to the target doctype's TitleField.
// Returns map[targetDoctype]map[name]title and, if len(docs) == 1, attaches "_linkTitles" to docs[0].
func (c *Ctx) ResolveLinkTitles(doctype string, docs ...Doc) map[string]map[string]string {
	if len(docs) == 0 {
		return map[string]map[string]string{}
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return map[string]map[string]string{}
	}
	byTarget := map[string]map[string]bool{}
	refused := map[string]bool{}
	collect := func(target string, val any) {
		// no title from a DocType this space may not read: a shared one the
		// platform keeps, inside a tenant (#108)
		if r, seen := refused[target]; !seen {
			r = c.SpaceRefusesName(target)
			refused[target] = r
			if r {
				return
			}
		} else if r {
			return
		}
		if s, ok := val.(string); ok && s != "" {
			if byTarget[target] == nil {
				byTarget[target] = map[string]bool{}
			}
			byTarget[target][s] = true
		}
	}

	for _, doc := range docs {
		for _, f := range d.Fields {
			switch f.Fieldtype {
			case "Link":
				if opt := f.OptionsString(); opt != "" {
					collect(opt, doc[f.Fieldname])
				}
			case "Dynamic Link":
				if opt := f.OptionsString(); opt != "" {
					if target, ok := doc[opt].(string); ok && target != "" {
						collect(target, doc[f.Fieldname])
					}
				}
			case "Table", "Table MultiSelect":
				if opt := f.OptionsString(); opt != "" {
					if rows, ok := doc[f.Fieldname].([]any); ok {
						childDt, _ := c.St.DocType(opt)
						if childDt != nil {
							for _, r := range rows {
								if rMap, ok := r.(map[string]any); ok {
									for _, cf := range childDt.Fields {
										if cf.Fieldtype == "Link" && cf.OptionsString() != "" {
											collect(cf.OptionsString(), rMap[cf.Fieldname])
										} else if cf.Fieldtype == "Dynamic Link" && cf.OptionsString() != "" {
											if target, ok := rMap[cf.OptionsString()].(string); ok && target != "" {
												collect(target, rMap[cf.Fieldname])
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	out := map[string]map[string]string{}
	for target, nameSet := range byTarget {
		td, err := c.St.DocType(target)
		if err != nil {
			continue
		}
		if td.TitleIsTranslatedID() {
			targetMap := map[string]string{}
			for n := range nameSet {
				targetMap[n] = c.T(n)
			}
			out[target] = targetMap
			continue
		}
		if td.TitleField == "" || td.TitleField == "id" {
			continue
		}
		if !td.HasColumn(td.TitleField) {
			continue
		}
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		if len(names) == 0 {
			continue
		}
		if td.IsVirtual() {
			if m, err := c.virtualTitles(td, names); err == nil && len(m) > 0 {
				out[target] = m
			}
			continue
		}
		q := fmt.Sprintf("SELECT id, %s FROM %s WHERE id = ANY($1)", db.Ident(td.TitleField), db.Ident(td.TableName()))
		rows, err := db.Select(c.Ctx, c.Q(), q, names)
		if err != nil {
			continue
		}
		targetMap := map[string]string{}
		found := map[string]bool{}
		for _, row := range rows {
			name := fmt.Sprint(row["id"])
			found[name] = true
			title := fmt.Sprint(row[td.TitleField])
			if title != "" && title != "<nil>" {
				targetMap[name] = title
			}
		}
		if target == "User" && c.Tenant != "" {
			var missing []string
			for _, n := range names {
				if !found[n] {
					missing = append(missing, n)
				}
			}
			for n, title := range c.operatorTitles(missing) {
				targetMap[n] = title
			}
		}
		if len(targetMap) > 0 {
			out[target] = targetMap
		}
	}

	if len(docs) == 1 && len(out) > 0 {
		docs[0]["_linkTitles"] = out
	}
	return out
}

// LinkTitles returns map[name]title for the given names of a doctype.
func (c *Ctx) LinkTitles(doctype string, names []string) (map[string]string, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if err := c.spaceRefusal(d, false, false); err != nil {
		return nil, err
	}
	if d.TitleIsTranslatedID() {
		out := map[string]string{}
		for _, n := range names {
			out[n] = c.T(n)
		}
		return out, nil
	}
	if len(names) == 0 || d.TitleField == "" || d.TitleField == "id" || !d.HasColumn(d.TitleField) {
		out := map[string]string{}
		for _, n := range names {
			out[n] = n
		}
		return out, nil
	}
	if d.IsVirtual() {
		out, err := c.virtualTitles(d, names)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if _, ok := out[n]; !ok {
				out[n] = n
			}
		}
		return out, nil
	}
	q := fmt.Sprintf("SELECT id, %s FROM %s WHERE id = ANY($1)", db.Ident(d.TitleField), db.Ident(d.TableName()))
	rows, err := db.Select(c.Ctx, c.Q(), q, names)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, row := range rows {
		name := fmt.Sprint(row["id"])
		title := fmt.Sprint(row[d.TitleField])
		if title != "" && title != "<nil>" {
			out[name] = title
		} else {
			out[name] = name
		}
	}
	if d.Name == "User" && c.Tenant != "" {
		var missing []string
		for _, n := range names {
			if _, ok := out[n]; !ok {
				missing = append(missing, n)
			}
		}
		for n, title := range c.operatorTitles(missing) {
			out[n] = title
		}
	}
	for _, n := range names {
		if _, ok := out[n]; !ok {
			out[n] = n
		}
	}
	return out, nil
}
