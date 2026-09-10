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
CREATE TABLE IF NOT EXISTS ddcore_default (
  "user" text NOT NULL, key text NOT NULL, value jsonb, PRIMARY KEY("user", key));
`

type column struct {
	name, typ string
	notNull   bool
}

func stdColumns(d *meta.DocType) []column {
	cols := []column{
		{"name", "text", true}, {"owner", "text", false}, {"creation", "timestamptz", false},
		{"modified", "timestamptz", false}, {"modified_by", "text", false}, {"docstatus", "smallint", true},
	}
	if d.IsChild {
		cols = append(cols, column{"parent", "text", false}, column{"parenttype", "text", false},
			column{"parentfield", "text", false}, column{"idx", "integer", true})
	}
	return cols
}

func wantedColumns(d *meta.DocType) []column {
	cols := stdColumns(d)
	for _, f := range d.DataFields() {
		cols = append(cols, column{f.Fieldname, meta.ColumnType(f.Fieldtype), false})
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
// text columns: `col <> ''` em numeric/date quebra a migração (B14).
func uniquePredicate(col, colType string) string {
	q := Ident(col)
	if colType == "text" {
		return fmt.Sprintf("%s IS NOT NULL AND %s <> ''", q, q)
	}
	return fmt.Sprintf("%s IS NOT NULL", q)
}

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
		switch {
		case f.Unique:
			add(f.Fieldname, index{unique: true, cols: Ident(f.Fieldname), predicate: uniquePredicate(f.Fieldname, meta.ColumnType(f.Fieldtype))})
		case f.SearchIndex || f.Fieldtype == "Link" || f.Fieldtype == "Dynamic Link":
			add(f.Fieldname, index{cols: Ident(f.Fieldname)})
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

// Plan computes the DDL needed to bring the database to the meta.
// It never drops tables or columns unless prune is set.
func Plan(ctx context.Context, q Querier, reg *meta.Registry, prune bool) ([]string, error) {
	existing := map[string]map[string]string{} // table -> col -> type
	rows, err := Select(ctx, q, `SELECT table_name, column_name, data_type, character_maximum_length, numeric_precision, numeric_scale
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name LIKE 'tab\_%'`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := r["table_name"].(string)
		if existing[t] == nil {
			existing[t] = map[string]string{}
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
		existing[t][r["column_name"].(string)] = typ
	}
	idxRows, err := Select(ctx, q, `SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename LIKE 'tab\_%'`)
	if err != nil {
		return nil, err
	}
	existingIdx := map[string]string{} // name -> indexdef
	for _, r := range idxRows {
		existingIdx[r["indexname"].(string)] = Str(r["indexdef"])
	}

	var ddl []string
	names := reg.Names()
	wantedTables := map[string]bool{}
	for _, n := range names {
		d := reg.DocTypes[n]
		if d.IsSingle {
			continue
		}
		t := d.TableName()
		wantedTables[t] = true
		cols, has := existing[t]
		if !has {
			ddl = append(ddl, createTable(d))
		} else {
			wanted := map[string]bool{}
			for _, c := range wantedColumns(d) {
				wanted[c.name] = true
				cur, ok := cols[c.name]
				if !ok {
					ddl = append(ddl, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s;", Ident(t), Ident(c.name), c.typ, colDefault(c)))
				} else if cur != c.typ {
					ddl = append(ddl, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;", Ident(t), Ident(c.name), c.typ, Ident(c.name), c.typ))
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
					ddl = append(ddl, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", Ident(t), Ident(c)))
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
			def, exists := existingIdx[k]
			if !exists {
				ddl = append(ddl, idx[k].ddl())
				continue
			}
			// mesmo nome não quer dizer mesma definição: searchIndex ↔ unique
			// trocam a natureza do índice sem trocar o nome (B14).
			if !sameIndex(def, idx[k]) {
				ddl = append(ddl, fmt.Sprintf("DROP INDEX %s;", Ident(k)), idx[k].ddl())
			}
		}
	}
	if prune {
		var extra []string
		for t := range existing {
			if !wantedTables[t] {
				extra = append(extra, t)
			}
		}
		sort.Strings(extra)
		for _, t := range extra {
			ddl = append(ddl, fmt.Sprintf("DROP TABLE %s;", Ident(t)))
		}
	}
	return ddl, nil
}

// Apply runs the internal schema and the given DDL statements, recording them.
func Apply(ctx context.Context, q Querier, ddl []string) error {
	if _, err := q.Exec(ctx, InternalSchema); err != nil {
		return err
	}
	for _, s := range ddl {
		if _, err := q.Exec(ctx, s); err != nil {
			return fmt.Errorf("%w\nDDL: %s", err, s)
		}
		if _, err := q.Exec(ctx, "INSERT INTO ddcore_migration (ddl) VALUES ($1)", s); err != nil {
			return err
		}
	}
	return nil
}
