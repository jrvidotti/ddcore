package db

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// The framework's own tables are not DocTypes: EnsureInternal runs InternalSchema
// as one script of idempotent statements. Without a plan of its own, what that
// script changes went unreported — a dry run said "nothing to do" while a
// column the new binary reads was missing, and the run that added it counted
// 0 DDL (#135). PlanInternal is that plan: each statement of the script,
// checked against the catalog before it runs.

var (
	internalCreateTable = regexp.MustCompile(`^CREATE TABLE IF NOT EXISTS (\w+)\b`)
	internalAddColumn   = regexp.MustCompile(`^ALTER TABLE (\w+) ADD COLUMN IF NOT EXISTS (\w+)\b`)
	internalCreateIndex = regexp.MustCompile(`^CREATE (?:UNIQUE )?INDEX IF NOT EXISTS (\w+) ON (\w+)\b`)
	internalDropIndex   = regexp.MustCompile(`^DROP INDEX IF EXISTS (\w+)$`)
)

// internalStmt is one statement of InternalSchema and what it acts on. index
// is the index a CREATE INDEX or DROP INDEX names.
type internalStmt struct {
	Statement
	index string
}

// internalStatements splits InternalSchema into its statements and classifies
// each. A form it does not know is an error rather than a statement left out
// of the plan; a test runs it over the schema, so a new form fails there.
func internalStatements() ([]internalStmt, error) {
	var lines []string
	for _, l := range strings.Split(InternalSchema, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			lines = append(lines, l)
		}
	}
	var out []internalStmt
	for _, raw := range strings.Split(strings.Join(lines, "\n"), ";") {
		sql := strings.Join(strings.Fields(raw), " ")
		if sql == "" {
			continue
		}
		st := internalStmt{Statement: Statement{SQL: sql + ";", Internal: true}}
		if m := internalCreateTable.FindStringSubmatch(sql); m != nil {
			st.Kind, st.Table = KindCreateTable, m[1]
		} else if m := internalAddColumn.FindStringSubmatch(sql); m != nil {
			st.Kind, st.Table, st.Column = KindAddColumn, m[1], m[2]
		} else if m := internalCreateIndex.FindStringSubmatch(sql); m != nil {
			st.Kind, st.index, st.Table = KindCreateIndex, m[1], m[2]
		} else if m := internalDropIndex.FindStringSubmatch(sql); m != nil {
			// the statement does not name the table: the report names the index
			st.Kind, st.index, st.Table = KindDropIndex, m[1], m[1]
		} else {
			return nil, fmt.Errorf("internal schema: statement not understood by the planner: %s", sql)
		}
		out = append(out, st)
	}
	return out, nil
}

// PlanInternal returns what EnsureInternal would change on this database, in
// the order it would change it. A table it creates stands for its columns and
// indexes: those are not listed again. Call it before EnsureInternal.
func PlanInternal(ctx context.Context, q Querier) ([]Statement, error) {
	sts, err := internalStatements()
	if err != nil {
		return nil, err
	}
	rels, err := Select(ctx, q, `SELECT rel.relname FROM pg_class rel JOIN pg_namespace ns ON ns.oid = rel.relnamespace
		WHERE ns.nspname = current_schema() AND rel.relkind IN ('r', 'i') AND rel.relname LIKE 'ddcore\_%'`)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, r := range rels {
		exists[Str(r["relname"])] = true
	}
	colRows, err := Select(ctx, q, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name LIKE 'ddcore\_%'`)
	if err != nil {
		return nil, err
	}
	cols := map[string]bool{}
	for _, r := range colRows {
		cols[Str(r["table_name"])+"."+Str(r["column_name"])] = true
	}
	created := map[string]bool{}
	var plan []Statement
	for _, st := range sts {
		switch st.Kind {
		case KindCreateTable:
			if exists[st.Table] {
				continue
			}
			exists[st.Table], created[st.Table] = true, true
		case KindAddColumn:
			if created[st.Table] || cols[st.Table+"."+st.Column] {
				continue
			}
			cols[st.Table+"."+st.Column] = true
		case KindCreateIndex:
			if created[st.Table] || exists[st.index] {
				continue
			}
			exists[st.index] = true
		case KindDropIndex:
			if !exists[st.index] {
				continue
			}
			exists[st.index] = false
		}
		plan = append(plan, st.Statement)
	}
	return plan, nil
}
