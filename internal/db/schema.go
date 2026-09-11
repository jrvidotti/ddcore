package db

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// InternalSchema are the framework tables that are not DocTypes.
const InternalSchema = `
CREATE TABLE IF NOT EXISTS ddcore_session (
  sid text PRIMARY KEY, "user" text NOT NULL, created timestamptz NOT NULL DEFAULT now(),
  last_seen timestamptz NOT NULL DEFAULT now(), expires timestamptz NOT NULL, data jsonb);
ALTER TABLE ddcore_session ADD COLUMN IF NOT EXISTS ip text;
ALTER TABLE ddcore_session ADD COLUMN IF NOT EXISTS user_agent text;
CREATE INDEX IF NOT EXISTS ddcore_session_user ON ddcore_session("user");
CREATE INDEX IF NOT EXISTS ddcore_session_expires ON ddcore_session(expires);
CREATE TABLE IF NOT EXISTS ddcore_series (prefix text PRIMARY KEY, current bigint NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS ddcore_job (
  id bigserial PRIMARY KEY, method text NOT NULL, args jsonb, queue text NOT NULL DEFAULT 'default',
  status text NOT NULL DEFAULT 'queued', "user" text, enqueued timestamptz NOT NULL DEFAULT now(),
  run_after timestamptz NOT NULL DEFAULT now(), started timestamptz, finished timestamptz,
  attempts int NOT NULL DEFAULT 0, max_attempts int NOT NULL DEFAULT 3, error text, result jsonb);
ALTER TABLE ddcore_job ADD COLUMN IF NOT EXISTS lease_until timestamptz;
ALTER TABLE ddcore_job ADD COLUMN IF NOT EXISTS timeout_seconds int NOT NULL DEFAULT 300;
CREATE INDEX IF NOT EXISTS ddcore_job_status ON ddcore_job(status, run_after);
CREATE INDEX IF NOT EXISTS ddcore_job_lease ON ddcore_job(status, lease_until);
CREATE TABLE IF NOT EXISTS ddcore_patch (app text NOT NULL, name text NOT NULL, executed timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(app, name));
CREATE TABLE IF NOT EXISTS ddcore_migration (id bigserial PRIMARY KEY, executed timestamptz NOT NULL DEFAULT now(), ddl text NOT NULL);
CREATE TABLE IF NOT EXISTS ddcore_installed_app (app text PRIMARY KEY, installed timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS ddcore_rename (
  kind text NOT NULL, doctype text NOT NULL, old_name text NOT NULL, new_name text NOT NULL,
  executed timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(kind, doctype, old_name));
CREATE TABLE IF NOT EXISTS ddcore_default (
  "user" text NOT NULL, key text NOT NULL, value jsonb, PRIMARY KEY("user", key));
CREATE TABLE IF NOT EXISTS ddcore_login_attempt (
  id bigserial PRIMARY KEY, identity text NOT NULL, ip text NOT NULL DEFAULT '',
  ok boolean NOT NULL DEFAULT false, created timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS ddcore_login_attempt_identity ON ddcore_login_attempt(identity, created DESC);
CREATE INDEX IF NOT EXISTS ddcore_login_attempt_ip ON ddcore_login_attempt(ip, created DESC);
CREATE TABLE IF NOT EXISTS ddcore_auth_token (
  token_hash text PRIMARY KEY, kind text NOT NULL, "user" text NOT NULL,
  created timestamptz NOT NULL DEFAULT now(), expires timestamptz NOT NULL,
  used timestamptz, created_by text, ip text);
CREATE INDEX IF NOT EXISTS ddcore_auth_token_user ON ddcore_auth_token("user", kind);
CREATE INDEX IF NOT EXISTS ddcore_auth_token_expires ON ddcore_auth_token(expires);
`

type column struct {
	name, typ string
	notNull   bool
	// field is nil for the standard columns: they are the framework's, not an
	// app's, and no declaration governs how they change.
	field *meta.Field
}

func stdColumns(d *meta.DocType) []column {
	cols := []column{
		{name: "name", typ: "text", notNull: true}, {name: "owner", typ: "text"},
		{name: "creation", typ: "timestamptz"}, {name: "modified", typ: "timestamptz"},
		{name: "modified_by", typ: "text"}, {name: "docstatus", typ: "smallint", notNull: true},
	}
	if d.IsChild {
		cols = append(cols, column{name: "parent", typ: "text"}, column{name: "parenttype", typ: "text"},
			column{name: "parentfield", typ: "text"}, column{name: "idx", typ: "integer", notNull: true})
	}
	return cols
}

func wantedColumns(d *meta.DocType) []column {
	cols := stdColumns(d)
	for _, f := range d.DataFields() {
		cols = append(cols, column{name: f.Fieldname, typ: meta.ColumnType(f.Fieldtype), field: f})
	}
	return cols
}

func colDefault(c column) string {
	switch c.name {
	case "docstatus":
		return " DEFAULT 0"
	case "idx":
		return " DEFAULT 0"
	}
	return ""
}

func createTable(d *meta.DocType) string {
	var defs []string
	for _, c := range wantedColumns(d) {
		def := Ident(c.name) + " " + c.typ
		if c.notNull {
			def += " NOT NULL"
		}
		def += colDefault(c)
		if c.name == "name" {
			def += " PRIMARY KEY"
		}
		defs = append(defs, def)
	}
	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n);", Ident(d.TableName()), strings.Join(defs, ",\n  "))
}

// index is one wanted index, kept in a comparable shape so Plan can tell an
// existing index apart from the desired one (and not only by its name).
type index struct {
	name      string
	table     string
	unique    bool
	cols      string // "col, other desc" — normalised
	predicate string // "" when the index covers every row
}

func (i index) ddl() string {
	kind := "INDEX"
	if i.unique {
		kind = "UNIQUE INDEX"
	}
	s := fmt.Sprintf("CREATE %s %s ON %s (%s)", kind, Ident(i.name), Ident(i.table), i.cols)
	if i.predicate != "" {
		s += " WHERE " + i.predicate
	}
	return s + ";"
}

// uniquePredicate keeps empty strings out of a unique index — but only for
// text columns: `col <> ”` em numeric/date quebra a migração (B14).
func uniquePredicate(col, colType string) string {
	q := Ident(col)
	if colType == "text" {
		return fmt.Sprintf("%s IS NOT NULL AND %s <> ''", q, q)
	}
	return fmt.Sprintf("%s IS NOT NULL", q)
}

// fieldIndex is the index a field wants, written in terms of the column name
// given. wantedIndexes passes the fieldname; the rename path passes the *old*
// fieldname, so an index that was only renamed compares equal to the definition
// Postgres still reports and is not needlessly dropped and rebuilt.
func fieldIndex(f *meta.Field, col string) (index, bool) {
	switch {
	case f.Unique:
		return index{unique: true, cols: Ident(col), predicate: uniquePredicate(col, meta.ColumnType(f.Fieldtype))}, true
	case f.SearchIndex || f.Fieldtype == "Link" || f.Fieldtype == "Dynamic Link":
		return index{cols: Ident(col)}, true
	}
	return index{}, false
}

// uniqueKeyIndex is the partial unique index a compound business key wants.
// Every component has to hold a value for the row to be constrained, which is
// the same rule a `unique` field follows — so an incomplete key never blocks a
// draft, and the predicate stays column-type aware (B14).
//
// col maps a fieldname to the column name to write it under. wantedIndexes
// passes the fieldname; the rename path passes the *old* column, so an index
// Postgres has already carried across a RENAME COLUMN compares equal to the
// definition still on record and is not dropped and rebuilt for nothing.
func uniqueKeyIndex(d *meta.DocType, k meta.UniqueKey, col func(string) string) (index, bool) {
	cols := make([]string, 0, len(k.Fields))
	preds := make([]string, 0, len(k.Fields))
	for _, fn := range k.Fields {
		f := d.Field(fn)
		if f == nil || meta.ColumnType(f.Fieldtype) == "" {
			// Refused by Registry.Validate, so unreachable with a loaded meta.
			// Skipping beats emitting DDL for a key that names no column.
			return index{}, false
		}
		c := col(fn)
		cols = append(cols, Ident(c))
		preds = append(preds, uniquePredicate(c, meta.ColumnType(f.Fieldtype)))
	}
	return index{unique: true, cols: strings.Join(cols, ", "), predicate: strings.Join(preds, " AND ")}, true
}

// sameColumn is the identity mapping uniqueKeyIndex takes when nothing was renamed.
func sameColumn(fieldname string) string { return fieldname }

func wantedIndexes(d *meta.DocType) map[string]index {
	t := d.TableName()
	idx := map[string]index{}
	add := func(suffix string, i index) {
		i.name, i.table = t+"_"+suffix, t
		idx[i.name] = i
	}
	if d.IsChild {
		add("parent", index{cols: "parent, parentfield, idx"})
	} else {
		add("modified", index{cols: "modified DESC"})
	}
	for _, f := range d.DataFields() {
		if i, ok := fieldIndex(f, f.Fieldname); ok {
			add(f.Fieldname, i)
		}
	}
	for _, k := range d.UniqueKeys {
		if i, ok := uniqueKeyIndex(d, k, sameColumn); ok {
			add(k.IndexSuffix(), i)
		}
	}
	return idx
}

var idxDefRe = regexp.MustCompile(`(?is)^CREATE\s+(UNIQUE\s+)?INDEX\s+\S+\s+ON\s+\S+\s+USING\s+\w+\s*\((.*?)\)\s*(?:WHERE\s+(.*))?$`)

// sameIndex compares an existing pg_indexes.indexdef with a wanted index,
// ignoring the formatting Postgres applies (parênteses, casts, aspas).
func sameIndex(indexdef string, want index) bool {
	m := idxDefRe.FindStringSubmatch(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(indexdef), ";")))
	if m == nil {
		return false
	}
	if (m[1] != "") != want.unique {
		return false
	}
	return normalizeSQL(m[2]) == normalizeSQL(want.cols) && normalizeSQL(m[3]) == normalizeSQL(want.predicate)
}

var spaceRe = regexp.MustCompile(`\s+`)

func normalizeSQL(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("::text", "", "(", "", ")", "", `"`, "").Replace(s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// Kind is what a planned statement does. Migrate needs it because the order in
// which the statements run is not the order in which they were discovered: a
// drop waits for the patches that may still read the column.
type Kind int

const (
	KindCreateTable Kind = iota
	KindRenameTable
	KindRenameIndex
	KindRenameColumn
	KindAddColumn
	KindAlterType
	KindCreateIndex
	KindDropIndex
	KindDropColumn
	KindDropTable
)

// Statement is one planned DDL statement and enough about it to report it, to
// order it and to know whether it throws data away.
type Statement struct {
	SQL     string
	Kind    Kind
	Doctype string
	Table   string
	Column  string
	// OldName is the previous table or column name on a rename.
	OldName string
	// Destructive marks the statements that lose data: they only ever appear
	// under prune, and Migrate runs them after the patches.
	Destructive bool
}

// SQL is the statements' text, for a caller that only wants to print or run them.
func SQL(sts []Statement) []string {
	out := make([]string, len(sts))
	for i, st := range sts {
		out[i] = st.SQL
	}
	return out
}

// Label is how a statement reads in a report: the verb, not the SQL.
func (k Kind) Label() string {
	switch k {
	case KindCreateTable:
		return "create"
	case KindRenameTable, KindRenameColumn:
		return "rename"
	case KindRenameIndex:
		return "rename index"
	case KindAddColumn:
		return "add"
	case KindAlterType:
		return "convert"
	case KindCreateIndex:
		return "index"
	case KindDropIndex:
		return "drop index"
	case KindDropColumn, KindDropTable:
		return "drop"
	}
	return "?"
}

// Report prints the plan in the order Migrate applies it, with the destructive
// statements under their own heading — the ones that need --prune and that run
// after the patches.
func Report(sts []Statement) string {
	if len(sts) == 0 {
		return "-- nothing to do\n"
	}
	keep, drop := Destructive(sts)
	var b strings.Builder
	line := func(st Statement) {
		where := st.Table
		if st.Column != "" {
			where += "." + st.Column
		}
		if st.OldName != "" {
			where = st.OldName + " → " + where
		}
		fmt.Fprintf(&b, "  %-12s %s\n", st.Kind.Label(), where)
		fmt.Fprintf(&b, "               %s\n", st.SQL)
	}
	if len(keep) > 0 {
		b.WriteString("expand\n")
		for _, st := range keep {
			line(st)
		}
	}
	if len(drop) > 0 {
		b.WriteString("contract (after the patches)\n")
		for _, st := range drop {
			line(st)
		}
	}
	return b.String()
}

// Destructive splits the plan into the statements that keep the data and the
// statements that throw it away.
func Destructive(sts []Statement) (keep, drop []Statement) {
	for _, st := range sts {
		if st.Destructive {
			drop = append(drop, st)
		} else {
			keep = append(keep, st)
		}
	}
	return keep, drop
}

// catalog is the database as Plan found it, mutated as renames are planned so
// that everything downstream — the column diff, the index diff, prune — sees
// the post-rename shape and stays idempotent on the next run.
type catalog struct {
	cols map[string]map[string]string // table -> column -> type
	idx  map[string]idxRow            // index name -> the table it is on and its definition
}

// idxRow keeps the owning table beside the definition. The table matters:
// index names are "<table>_<suffix>", so "tab_pedido_" is a prefix of
// "tab_pedido_item_modified" — an index belonging to a different DocType
// entirely. Renaming by name alone would take it along.
type idxRow struct{ table, def string }

func loadCatalog(ctx context.Context, q Querier) (*catalog, error) {
	c := &catalog{cols: map[string]map[string]string{}, idx: map[string]idxRow{}}
	rows, err := Select(ctx, q, `SELECT table_name, column_name, data_type, character_maximum_length, numeric_precision, numeric_scale
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name LIKE 'tab\_%'`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := r["table_name"].(string)
		if c.cols[t] == nil {
			c.cols[t] = map[string]string{}
		}
		typ := r["data_type"].(string)
		switch typ {
		case "timestamp with time zone":
			typ = "timestamptz"
		case "time without time zone":
			typ = "time"
		case "numeric":
			typ = fmt.Sprintf("numeric(%v,%v)", r["numeric_precision"], r["numeric_scale"])
		}
		c.cols[t][r["column_name"].(string)] = typ
	}
	idxRows, err := Select(ctx, q, `SELECT indexname, indexdef, tablename FROM pg_indexes WHERE schemaname = current_schema() AND tablename LIKE 'tab\_%'`)
	if err != nil {
		return nil, err
	}
	for _, r := range idxRows {
		c.idx[r["indexname"].(string)] = idxRow{table: Str(r["tablename"]), def: Str(r["indexdef"])}
	}
	return c, nil
}

// renameIndex moves one index, if it is there and the new name is free.
func (c *catalog) renameIndex(old, name string) []Statement {
	row, ok := c.idx[old]
	if !ok {
		return nil
	}
	if _, taken := c.idx[name]; taken {
		return nil
	}
	c.idx[name] = row
	delete(c.idx, old)
	return []Statement{{
		SQL:  fmt.Sprintf("ALTER INDEX %s RENAME TO %s;", Ident(old), Ident(name)),
		Kind: KindRenameIndex, OldName: old,
	}}
}

// renameTableIndexes moves every index that is actually on oldTable, including
// the primary key — an index no wanted-index pass ever names, and the one that
// would collide on the next rename if it were left behind.
func (c *catalog) renameTableIndexes(oldTable, newTable string) []Statement {
	var names []string
	for name, row := range c.idx {
		if row.table == oldTable {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out []Statement
	for _, name := range names {
		row := c.idx[name]
		row.table = newTable
		c.idx[name] = row
		if !strings.HasPrefix(name, oldTable+"_") {
			continue // not derived from the table name: nothing to rename
		}
		out = append(out, c.renameIndex(name, newTable+"_"+strings.TrimPrefix(name, oldTable+"_"))...)
	}
	return out
}

// planRenames turns the declared renamedFrom into RENAME statements, and is why
// a rename keeps its data: without it the new name is an empty ADD COLUMN and
// the old one is either orphaned or, under prune, dropped in the same run.
//
// It runs before the diff and mutates the catalog, so tables come before
// columns — a DocType that was renamed *and* renamed a field needs the column
// rename to address the table by its new name. renamedIdx reports, per index
// name, the column it was built on, so the index diff can tell a rename apart
// from a change of definition; renamedCols reports the same fact per table and
// fieldname, which is what a compound key — an index no single field owns —
// needs in order to be compared against the definition the catalog still holds.
func planRenames(c *catalog, reg *meta.Registry, names []string) (tables, columns []Statement, renamedIdx map[string]string, renamedCols map[string]map[string]string, refusals []Refusal) {
	renamedIdx = map[string]string{}
	renamedCols = map[string]map[string]string{}
	for _, n := range names {
		d := reg.DocTypes[n]
		if d.IsSingle {
			continue
		}
		t := d.TableName()
		if _, newExists := c.cols[t]; newExists {
			// The new table is already there: the rename has run. That is
			// indistinguishable from someone having created it by hand, so
			// this is a skip and not a refusal — the leftover old table is
			// reported as an orphan instead.
			continue
		}
		for _, prev := range d.RenamedFrom {
			ot := "tab_" + meta.Snake(prev)
			cols, oldExists := c.cols[ot]
			if !oldExists || ot == t {
				continue
			}
			tables = append(tables, Statement{
				SQL:  fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", Ident(ot), Ident(t)),
				Kind: KindRenameTable, Doctype: d.Name, Table: t, OldName: prev,
			})
			c.cols[t] = cols
			delete(c.cols, ot)
			// Postgres does not rename a table's indexes with it, and the
			// primary key is one of them — an index wantedIndexes never names.
			tables = append(tables, c.renameTableIndexes(ot, t)...)
			break
		}
	}
	for _, n := range names {
		d := reg.DocTypes[n]
		if d.IsSingle {
			continue
		}
		t := d.TableName()
		cols := c.cols[t]
		if cols == nil {
			continue // a table being created has nothing to rename
		}
		for _, f := range d.DataFields() {
			if _, newOK := cols[f.Fieldname]; newOK {
				// Already renamed. And necessarily a skip rather than a
				// refusal: a release that renames a → b and gives the freed
				// name to a new field leaves exactly this shape behind, so
				// there is nothing here to tell the two apart.
				continue
			}
			for _, prev := range f.RenamedFrom {
				typ, oldOK := cols[prev]
				if !oldOK || prev == f.Fieldname {
					continue
				}
				columns = append(columns, Statement{
					SQL:  fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", Ident(t), Ident(prev), Ident(f.Fieldname)),
					Kind: KindRenameColumn, Doctype: d.Name, Table: t, Column: f.Fieldname, OldName: prev,
				})
				cols[f.Fieldname] = typ
				delete(cols, prev)
				if renamedCols[t] == nil {
					renamedCols[t] = map[string]string{}
				}
				renamedCols[t][f.Fieldname] = prev
				if st := c.renameIndex(t+"_"+prev, t+"_"+f.Fieldname); st != nil {
					columns = append(columns, st...)
					renamedIdx[t+"_"+f.Fieldname] = prev
				}
				break
			}
		}
	}
	return tables, columns, renamedIdx, renamedCols, refusals
}

// safeWidening is the set of column-type changes migrate applies on its own,
// because the target holds every value of the source for every row without
// consulting anything outside the column.
//
// Everything absent is destructive, and that includes the pairs Postgres cannot
// cast at all (numeric to boolean, date to time, text to jsonb): today those
// produce a raw "cannot cast type" with no route forward. Refusing them is how
// an unactionable error becomes an actionable one.
var safeWidening = map[[2]string]bool{
	// every type this framework uses renders into text
	{"bigint", "text"}: true, {"double precision", "text"}: true, {"numeric(21,9)", "text"}: true,
	{"boolean", "text"}: true, {"date", "text"}: true, {"timestamptz", "text"}: true,
	{"time", "text"}: true, {"jsonb", "text"}: true,
	// Int → Float. Formally lossy above 2^53, but an app's numbers are goja
	// float64: an Int that ever passed through the app is already within range.
	{"bigint", "double precision"}: true,
	// Int → Currency. Exact; overflows above 10^12, and Postgres raises on
	// overflow rather than truncating. A loud abort is not data loss.
	{"bigint", "numeric(21,9)"}: true,
	// Float → Currency. Rounds at nine decimals, past float64's meaningful
	// precision, and Currency is the precision-correct target.
	{"double precision", "numeric(21,9)"}: true,
}

// Refusal is a change Plan will not make on its own, with the reason and what
// the author can do instead.
type Refusal struct {
	Table, Column, Reason, Remedy string
}

// RefusedError carries every refusal in one plan. Migrate reports them together
// and applies nothing: a half-applied schema is what this is here to prevent.
type RefusedError struct{ Refusals []Refusal }

func (e *RefusedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "migrate refused %d change(s) — nothing was applied.\n", len(e.Refusals))
	for _, r := range e.Refusals {
		where := r.Table
		if r.Column != "" {
			where += "." + r.Column
		}
		fmt.Fprintf(&b, "\n  %s: %s\n    → %s\n", where, r.Reason, r.Remedy)
	}
	return b.String()
}

// alterType decides whether a column-type change may happen at all: safe on its
// own per safeWidening, allowed when the author declared convert, refused
// otherwise.
func alterType(d *meta.DocType, t string, c column, cur string) (Statement, *Refusal) {
	st := Statement{
		Kind: KindAlterType, Doctype: d.Name, Table: t, Column: c.name,
		SQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
			Ident(t), Ident(c.name), c.typ, Ident(c.name), c.typ),
	}
	// c.field == nil is a standard column: the framework owns its type and no
	// declaration governs it.
	if c.field == nil || safeWidening[[2]string{cur, c.typ}] {
		return st, nil
	}
	cv := c.field.Convert
	if cv == nil {
		return st, &Refusal{Table: t, Column: c.name,
			Reason: fmt.Sprintf("the column is %s and %s wants %s, which cannot be done without knowing the data", cur, c.field.Fieldtype, c.typ),
			Remedy: "declare convert: { from: \"<the fieldtype the column still holds>\" } to accept Postgres's own cast, or route it through expand → backfill → validate → contract (`ddcore docs migrations`)"}
	}
	if from := meta.ColumnType(cv.From); from != cur {
		return st, &Refusal{Table: t, Column: c.name,
			Reason: fmt.Sprintf("convert.from is %q, whose column is %s, but the column is %s", cv.From, from, cur),
			Remedy: "point convert.from at the fieldtype the database still holds, or delete the declaration if the conversion already happened"}
	}
	return st, nil
}

// hasData reports whether dropping this would actually lose something. It is
// the only query Plan makes beyond the catalog, and only under prune, where the
// candidates are few.
func hasData(ctx context.Context, q Querier, table, col string) (bool, error) {
	sql := fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", Ident(table))
	if col != "" {
		sql = fmt.Sprintf("SELECT 1 FROM %s WHERE %s IS NOT NULL LIMIT 1", Ident(table), Ident(col))
	}
	rows, err := Select(ctx, q, sql)
	return len(rows) > 0, err
}

// Plan computes the DDL needed to bring the database to the meta.
//
// It never drops tables or columns unless prune is set, it refuses a conversion
// the author has not declared, and it refuses to drop something that still
// holds data — which is what makes a rename declaration safe to delete: get it
// wrong and the migration stops and says so, instead of emptying a column.
//
// The order of the result is the order Migrate applies it: renames, the
// additive DDL, the indexes, and last the drops, which run after the patches so
// a backfill can still read the column the same migration is about to remove.
func Plan(ctx context.Context, q Querier, reg *meta.Registry, prune bool) ([]Statement, error) {
	cat, err := loadCatalog(ctx, q)
	if err != nil {
		return nil, err
	}
	names := reg.Names()
	renameTables, renameCols, renamedIdx, renamedCols, refusals := planRenames(cat, reg, names)

	var add, alter, indexes, drops []Statement
	wantedTables := map[string]bool{}
	for _, n := range names {
		d := reg.DocTypes[n]
		if d.IsSingle {
			continue
		}
		t := d.TableName()
		wantedTables[t] = true
		cols, has := cat.cols[t]
		if !has {
			add = append(add, Statement{SQL: createTable(d), Kind: KindCreateTable, Doctype: d.Name, Table: t})
		} else {
			wanted := map[string]bool{}
			for _, c := range wantedColumns(d) {
				wanted[c.name] = true
				cur, ok := cols[c.name]
				switch {
				case !ok:
					add = append(add, Statement{
						SQL:  fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s;", Ident(t), Ident(c.name), c.typ, colDefault(c)),
						Kind: KindAddColumn, Doctype: d.Name, Table: t, Column: c.name,
					})
				case cur != c.typ:
					st, refusal := alterType(d, t, c, cur)
					if refusal != nil {
						refusals = append(refusals, *refusal)
						continue
					}
					alter = append(alter, st)
				}
			}
			if prune {
				var extra []string
				for c := range cols {
					if !wanted[c] {
						extra = append(extra, c)
					}
				}
				sort.Strings(extra)
				for _, c := range extra {
					full, err := hasData(ctx, q, t, c)
					if err != nil {
						return nil, err
					}
					if full {
						refusals = append(refusals, Refusal{Table: t, Column: c,
							Reason: "dropping a column that still holds data",
							Remedy: "if the field was renamed, declare renamedFrom on the new field; if the data really is to be discarded, drop the column yourself in a beforeSchema patch"})
						continue
					}
					drops = append(drops, Statement{
						SQL:  fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", Ident(t), Ident(c)),
						Kind: KindDropColumn, Doctype: d.Name, Table: t, Column: c, Destructive: true,
					})
				}
			}
		}
		idx := wantedIndexes(d)
		keys := make([]string, 0, len(idx))
		for k := range idx {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			row, exists := cat.idx[k]
			if !exists {
				indexes = append(indexes, Statement{SQL: idx[k].ddl(), Kind: KindCreateIndex, Doctype: d.Name, Table: t})
				continue
			}
			want := idx[k]
			// An index that was only renamed still reports its old column, so
			// compare it against its own old definition rather than rebuilding
			// it — which on a large table is the expensive way to change nothing.
			if old, renamed := renamedIdx[k]; renamed {
				if f := d.Field(strings.TrimPrefix(k, t+"_")); f != nil {
					if oldIdx, ok := fieldIndex(f, old); ok {
						want = oldIdx
					}
				}
			}
			// A compound key's index belongs to no single field, so renamedIdx
			// cannot speak for it. Render it against the column names the
			// catalog still holds — the ones Postgres carried the index across
			// the RENAME COLUMN from — so a renamed component does not cost a
			// rebuild, and with nothing renamed this is the identity.
			if suffix := strings.TrimPrefix(k, t+"_"); strings.HasPrefix(suffix, "uk_") {
				if uk := d.UniqueKey(strings.TrimPrefix(suffix, "uk_")); uk != nil {
					if old, ok := uniqueKeyIndex(d, *uk, func(fn string) string {
						if was, renamed := renamedCols[t][fn]; renamed {
							return was
						}
						return fn
					}); ok {
						want = old
					}
				}
			}
			// Same name does not imply same definition: searchIndex ↔ unique
			// changes the nature of the index without changing its name (B14).
			if !sameIndex(row.def, want) {
				indexes = append(indexes,
					Statement{SQL: fmt.Sprintf("DROP INDEX %s;", Ident(k)), Kind: KindDropIndex, Doctype: d.Name, Table: t},
					Statement{SQL: idx[k].ddl(), Kind: KindCreateIndex, Doctype: d.Name, Table: t})
			}
		}
		// A key the meta no longer declares leaves an index the loop above
		// never names — it walks the wanted names only — and prune covers
		// columns and tables, not indexes. Left alone it would go on enforcing
		// a constraint nothing declares. The `uk_` infix is this feature's own
		// namespace and idxRow carries the owning table, so "tab_pedido_" can
		// never reach into "tab_pedido_item_". Dropping an index loses no row,
		// so this needs neither prune nor a destructive flag.
		var stale []string
		for name, row := range cat.idx {
			if _, wanted := idx[name]; wanted || row.table != t {
				continue
			}
			// identRe as well as the prefix: Ident panics on anything else,
			// and migrate refusing to run is a worse answer than leaving a
			// hand-made index alone.
			if strings.HasPrefix(name, t+"_uk_") && identRe.MatchString(name) {
				stale = append(stale, name)
			}
		}
		sort.Strings(stale)
		for _, name := range stale {
			indexes = append(indexes, Statement{
				SQL:  fmt.Sprintf("DROP INDEX %s;", Ident(name)),
				Kind: KindDropIndex, Doctype: d.Name, Table: t,
			})
		}
	}
	if prune {
		var extra []string
		for t := range cat.cols {
			if !wantedTables[t] {
				extra = append(extra, t)
			}
		}
		sort.Strings(extra)
		for _, t := range extra {
			full, err := hasData(ctx, q, t, "")
			if err != nil {
				return nil, err
			}
			if full {
				refusals = append(refusals, Refusal{Table: t,
					Reason: "dropping a table that still holds rows",
					Remedy: "if the DocType was renamed, declare renamedFrom on it; if the rows really are to be discarded, drop the table yourself in a beforeSchema patch"})
				continue
			}
			drops = append(drops, Statement{
				SQL:  fmt.Sprintf("DROP TABLE %s;", Ident(t)),
				Kind: KindDropTable, Table: t, Destructive: true,
			})
		}
	}
	if len(refusals) > 0 {
		return nil, &RefusedError{Refusals: refusals}
	}
	plan := make([]Statement, 0, len(renameTables)+len(renameCols)+len(add)+len(alter)+len(indexes)+len(drops))
	for _, group := range [][]Statement{renameTables, renameCols, add, alter, indexes, drops} {
		plan = append(plan, group...)
	}
	return plan, nil
}

// Apply runs the given statements, recording each one in ddcore_migration.
func Apply(ctx context.Context, q Querier, sts []Statement) error {
	for _, st := range sts {
		if _, err := q.Exec(ctx, st.SQL); err != nil {
			return fmt.Errorf("%w\nDDL: %s", err, st.SQL)
		}
		if _, err := q.Exec(ctx, "INSERT INTO ddcore_migration (ddl) VALUES ($1)", st.SQL); err != nil {
			return err
		}
	}
	return nil
}

// EnsureInternal creates the framework's own tables, the ones that are not
// DocTypes. It is separate from Apply because it has to be in place before
// anything else in a migration runs, patches included.
func EnsureInternal(ctx context.Context, q Querier) error {
	_, err := q.Exec(ctx, InternalSchema)
	return err
}
