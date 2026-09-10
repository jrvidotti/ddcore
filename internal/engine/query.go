package engine

import (
	"fmt"
	"regexp"
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

// hasChildTable reports whether ct is the options of a Table field of d.
func hasChildTable(d *meta.DocType, ct string) bool {
	for _, f := range d.TableFields() {
		if strings.EqualFold(f.OptionsString(), ct) {
			return true
		}
	}
	return false
}

// filterSQL renders filters into a WHERE fragment. Conditions over a child
// doctype become `EXISTS (SELECT 1 FROM tab_child ...)` instead of a JOIN: um
// pai com duas linhas filhas correspondentes apareceria duas vezes na
// listagem e seria contado duas vezes por count(*) (B19). Todas as condições
// do mesmo filho vão para o mesmo EXISTS, isto é, exigem a mesma linha.
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
			return "", fmt.Errorf("campo desconhecido no filtro: %q", f.Field)
		}
		if !hasChildTable(d, ct) {
			return "", fmt.Errorf("%s não é uma tabela filha de %s", ct, d.Name)
		}
		if _, seen := byChild[ct]; !seen {
			order = append(order, ct)
		}
		byChild[ct] = append(byChild[ct], db.Filter{Field: cf, Op: f.Op, Value: f.Value})
	}
	var parts []string
	for _, f := range own {
		op := strings.ToLower(strings.TrimSpace(f.Op))
		fld := d.Field(f.Field)
		if (op == "like" || op == "not like") && fld != nil && fld.Fieldtype == "Link" {
			targetDoc := fld.OptionsString()
			if td, ok := c.St.Meta.Get(targetDoc); ok && td != nil {
				var targetCols []string
				seenCols := map[string]bool{"name": true}
				if td.TitleField != "" && td.TitleField != "name" && td.HasColumn(td.TitleField) {
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
						return "", fmt.Errorf("campo desconhecido no filtro: %q", f.Field)
					}
					arg := b.Arg(f.Value)
					alias := db.Ident("lt_" + meta.Snake(fld.Fieldname))
					var targetConds []string
					for _, tc := range targetCols {
						targetConds = append(targetConds, fmt.Sprintf("%s ILIKE %s", db.AccentInsensitive(alias+"."+db.Ident(tc)), db.AccentInsensitive(arg)))
					}
					targetWhere := strings.Join(targetConds, " OR ")
					existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM %s AS %s WHERE %s.%s = %s AND (%s))",
						db.Ident(td.TableName()), alias, alias, db.Ident("name"), colExpr, targetWhere)
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

		w, err := b.Where([]db.Filter{f}, col)
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
		parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM %s AS %s WHERE %s.parent = \"t\".name AND %s.parenttype = %s AND %s)",
			db.Ident(child.TableName()), alias, alias, alias, b.Arg(d.Name), w))
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
	if !a.IgnorePermissions && !c.IgnorePermissions() {
		if ok, err := c.HasPermission(doctype, "read", nil); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("Sem permissão para listar {0}", c.T(d.Label))
		}
	}
	joins := map[string]*meta.DocType{}
	col := c.columnResolver(d, joins)
	var b db.Builder

	// select list
	var sel []string
	if len(a.Fields) == 0 {
		a.Fields = []string{"name"}
	}
	hasAgg := false
	for _, f := range a.Fields {
		f = strings.TrimSpace(f)
		if f == "*" {
			sel = append(sel, `"t".*`)
			continue
		}
		if m := aggRe.FindStringSubmatch(f); m != nil {
			hasAgg = true
			arg := "*"
			if m[2] != "*" {
				arg = col(m[2])
				if arg == "" {
					return nil, cerr.Validation("Campo desconhecido: {0}", m[2])
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
			cexpr := col(m[1])
			if cexpr == "" {
				return nil, cerr.Validation("Campo desconhecido: {0}", m[1])
			}
			sel = append(sel, cexpr+" AS "+db.Ident(m[2]))
			continue
		}
		cexpr := col(f)
		if cexpr == "" {
			return nil, cerr.Validation("Campo desconhecido: {0}", f)
		}
		if strings.Contains(f, ".") {
			sel = append(sel, cexpr+" AS "+db.Ident(strings.ReplaceAll(strings.ToLower(meta.Snake(f)), ".", "_")))
		} else {
			sel = append(sel, cexpr)
		}
	}

	filters, err := db.ParseFilters(a.Filters)
	if err != nil {
		return nil, cerr.Validation("{0}", err)
	}
	orFilters, err := db.ParseFilters(a.OrFilters)
	if err != nil {
		return nil, cerr.Validation("{0}", err)
	}
	if !a.IgnorePermissions && !c.IgnorePermissions() {
		pf, err := c.permissionFilters(d)
		if err != nil {
			return nil, err
		}
		filters = append(filters, pf...)
	}
	where, err := c.filterSQL(d, &b, filters, col)
	if err != nil {
		return nil, cerr.Validation("{0}", err)
	}
	if len(orFilters) > 0 {
		var ors []string
		for _, f := range orFilters {
			w, err := c.filterSQL(d, &b, []db.Filter{f}, col)
			if err != nil {
				return nil, cerr.Validation("{0}", err)
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
	if err != nil {
		return nil, cerr.Validation("{0}", err)
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

	sql := "SELECT "
	if a.Distinct {
		sql += "DISTINCT "
	}
	sql += strings.Join(sel, ", ") + " FROM " + db.Ident(d.TableName()) + ` AS "t"`
	for ct, child := range joins {
		alias := db.Ident("c_" + meta.Snake(ct))
		sql += fmt.Sprintf(" JOIN %s AS %s ON %s.parent = \"t\".name AND %s.parenttype = %s", db.Ident(child.TableName()), alias, alias, alias, b.Arg(d.Name))
	}
	if where != "" {
		sql += " WHERE " + where
	}
	if a.GroupBy != "" {
		g := col(a.GroupBy)
		if g == "" {
			return nil, cerr.Validation("groupBy desconhecido: {0}", a.GroupBy)
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

// Exists reports whether a document exists (no permission check).
func (c *Ctx) Exists(doctype, name string) (bool, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, err
	}
	var one int
	err = c.Q().QueryRow(c.Ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE name = $1", db.Ident(d.TableName())), name).Scan(&one)
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
	rows, err := c.GetList(doctype, ListArgs{Filters: filters, Fields: []string{"name"}, Limit: 1, IgnorePermissions: true})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return db.Str(rows[0]["name"]), nil
}

// GetValue reads one field of a document (no permission check, like frappe.db).
func (c *Ctx) GetValue(doctype, name, field string) (any, error) {
	m, err := c.GetValues(doctype, name, []string{field})
	if err != nil || m == nil {
		return nil, err
	}
	return m[field], nil
}

// GetValues reads several fields; name may be a name or filters.
func (c *Ctx) GetValues(doctype string, name any, fields []string) (map[string]any, error) {
	var filters any
	if s, ok := name.(string); ok {
		filters = map[string]any{"name": s}
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

// SQL runs a read-only query (used by reports and ddcore.db.sql).
//
// The prefix check is only a first filter: a CTE can hide an UPDATE behind a
// SELECT. The query really runs inside a savepoint with
// `SET LOCAL transaction_read_only = on`, so any write is refused by Postgres
// and the savepoint is rolled back afterwards, leaving the caller's
// transaction usable (and its `SET LOCAL` undone).
func (c *Ctx) SQL(query string, params []any) ([]map[string]any, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	if !strings.HasPrefix(q, "select") && !strings.HasPrefix(q, "with") {
		return nil, cerr.Permission("ddcore.db.sql aceita apenas SELECT")
	}
	if c.Tx == nil {
		rows, err := db.Select(c.Ctx, c.Q(), query, params...)
		if err != nil {
			return nil, cerr.Validation("{0}", err)
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
		return nil, cerr.Validation("{0}", err)
	}
	rollback := func() {
		c.Tx.Exec(c.Ctx, "ROLLBACK TO SAVEPOINT "+sp)
		c.Tx.Exec(c.Ctx, "RELEASE SAVEPOINT "+sp)
	}
	if _, err := c.Tx.Exec(c.Ctx, "SET LOCAL transaction_read_only = on"); err != nil {
		rollback()
		return nil, cerr.Validation("{0}", err)
	}
	rows, err := db.Select(c.Ctx, c.Tx, query, params...)
	rollback()
	if err != nil {
		return nil, cerr.Validation("{0}", err)
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

// Lock takes a transaction-scoped advisory lock, serialising every ctx that
// asks for the same key until the transaction ends.
func (c *Ctx) Lock(key string) error {
	if strings.TrimSpace(key) == "" {
		return cerr.Validation("ddcore.db.lock: informe uma chave")
	}
	if c.Tx == nil {
		return cerr.Validation("ddcore.db.lock exige uma transação")
	}
	_, err := c.Tx.Exec(c.Ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", key)
	return err
}

// LinkSearch backs the Link control: searches name + searchFields.
func (c *Ctx) LinkSearch(doctype, txt string, filters any, limit int) ([]map[string]any, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	fields := []string{"name"}
	seen := map[string]bool{"name": true}
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
	args := ListArgs{Fields: fields, Filters: filters, Limit: limit}
	if txt != "" {
		var ors []any
		for _, f := range fields {
			ors = append(ors, []any{f, "like", "%" + txt + "%"})
		}
		args.OrFilters = ors
	}
	if limit <= 0 {
		args.Limit = 20
	}
	return c.GetList(doctype, args)
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
	collect := func(target string, val any) {
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
			case "Table":
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
		if err != nil || td.TitleField == "" || td.TitleField == "name" {
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
		q := fmt.Sprintf("SELECT name, %s FROM %s WHERE name = ANY($1)", db.Ident(td.TitleField), db.Ident(td.TableName()))
		rows, err := db.Select(c.Ctx, c.Q(), q, names)
		if err != nil {
			continue
		}
		targetMap := map[string]string{}
		for _, row := range rows {
			name := fmt.Sprint(row["name"])
			title := fmt.Sprint(row[td.TitleField])
			if title != "" && title != "<nil>" {
				targetMap[name] = title
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
	if len(names) == 0 || d.TitleField == "" || d.TitleField == "name" || !d.HasColumn(d.TitleField) {
		out := map[string]string{}
		for _, n := range names {
			out[n] = n
		}
		return out, nil
	}
	q := fmt.Sprintf("SELECT name, %s FROM %s WHERE name = ANY($1)", db.Ident(d.TitleField), db.Ident(d.TableName()))
	rows, err := db.Select(c.Ctx, c.Q(), q, names)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, row := range rows {
		name := fmt.Sprint(row["name"])
		title := fmt.Sprint(row[d.TitleField])
		if title != "" && title != "<nil>" {
			out[name] = title
		} else {
			out[name] = name
		}
	}
	for _, n := range names {
		if _, ok := out[n]; !ok {
			out[n] = n
		}
	}
	return out, nil
}
