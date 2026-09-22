package db

import "context"

// legacyPKColumns are the framework's own tables that carried a document key
// under a pre-0.17 name. DocType tables are found by looking, not listed; these
// three cannot be, because `ddcore_*` holds genuine names too — a secret's name
// in ddcore_vault, a patch file's name in ddcore_patch — and those stay.
var legacyPKColumns = [][3]string{
	{"ddcore_notification", "name", "id"},
	{"ddcore_notification", "reference_name", "reference_id"},
	{"ddcore_notification_due", "reference_name", "reference_id"},
}

// RenameLegacyPK moves the document key from `name` to `id` (0.17.0).
//
// It runs before anything else reads the database — before EnsureInternal,
// the app patches and Plan — because from 0.17 on every generated statement says `id`, and a
// beforeSchema patch calling ctx.getDoc would hit a column that is not there
// yet. Plan calls it too, on its throwaway transaction: without that, a dry run
// against a 0.16 database would report an ADD COLUMN for every table (and, with
// prune, refuse on the `name` column that still holds data).
//
// Postgres carries the rest: ALTER TABLE ... RENAME COLUMN rewrites the primary
// key, the ddcore_single_identity CHECK and every index definition in place,
// because all of them are stored against the column's attnum and not its name.
// So there is no constraint to drop, no index to rename and nothing to reindex.
//
// The statements are executed directly rather than returned as a plan. Migrate
// records every KindRenameColumn it applies in ddcore_rename, and one phantom
// entry per table would sit in `ddcore doctor`'s "retirable" report forever.
//
// It reports how many DocType tables it moved, so the caller can say so; on a
// database that already speaks `id` that is zero and nothing ran.
func RenameLegacyPK(ctx context.Context, q Querier) (int, error) {
	tables, err := legacyPKTables(ctx, q)
	if err != nil {
		return 0, err
	}
	for _, t := range tables {
		if _, err := q.Exec(ctx, `ALTER TABLE `+Ident(t)+` RENAME COLUMN "name" TO "id"`); err != nil {
			return 0, err
		}
	}
	for _, c := range legacyPKColumns {
		// A table that does not exist yet is simply not found: EnsureInternal,
		// which runs next, creates it with the new names.
		ok, err := hasLegacyColumn(ctx, q, c[0], c[1], c[2])
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		if _, err := q.Exec(ctx, `ALTER TABLE `+Ident(c[0])+` RENAME COLUMN `+Ident(c[1])+` TO `+Ident(c[2])); err != nil {
			return 0, err
		}
	}
	return len(tables), nil
}

// legacyPKTables lists the DocType tables still keyed by `name`.
//
// The predicate is a conjunction on purpose: a table qualifies only when it has
// `name` *and* has no `id`. `name` is an ordinary fieldname from 0.17 on, so a
// DocType may declare one — and its table also has `id`, which is what keeps
// this from mistaking a data column for the old key. The same conjunction is
// what makes the sweep idempotent and a no-op on a new database.
func legacyPKTables(ctx context.Context, q Querier) ([]string, error) {
	// The identifier regexp guards Ident, which panics on anything else:
	// refusing to migrate because someone left a hand-made "tab_Foo" behind
	// would be a worse answer than passing it by. Plan's stale-index sweep
	// takes the same precaution.
	rows, err := Select(ctx, q, `
		SELECT t.table_name FROM information_schema.tables t
		WHERE t.table_schema = current_schema() AND t.table_type = 'BASE TABLE'
		  AND t.table_name LIKE 'tab\_%' AND t.table_name ~ '^tab_[a-z0-9_]*$'
		  AND EXISTS (SELECT 1 FROM information_schema.columns c
		    WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'name')
		  AND NOT EXISTS (SELECT 1 FROM information_schema.columns c
		    WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'id')
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, Str(r["table_name"]))
	}
	return out, nil
}

// hasLegacyColumn reports whether table still carries `old` and not yet `new`.
func hasLegacyColumn(ctx context.Context, q Querier, table, old, name string) (bool, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1
		  AND column_name = $2
		  AND NOT EXISTS (SELECT 1 FROM information_schema.columns c2
		    WHERE c2.table_schema = current_schema() AND c2.table_name = $1 AND c2.column_name = $3)`,
		table, old, name).Scan(&n)
	return n > 0, err
}
