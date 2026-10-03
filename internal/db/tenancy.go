package db

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// TenantExpr is the space of the running transaction: the tenant it named
// with SetTenant, or the empty string — the platform space — when it named
// none. It is both the default of every `tenant` column, which is how a row
// is stamped without anybody writing the column, and the test of the policy.
const TenantExpr = `coalesce(current_setting('ddcore.tenant', true), '')`

// TenantPolicy is the name of the one policy a tenant table carries.
const TenantPolicy = "ddcore_tenant"

// DefaultTenantRole is the role document work runs as on a site with
// tenancy. It owns nothing, so row-level security applies to it — which it
// does not to the role that owns the tables, nor to a superuser.
const DefaultTenantRole = "ddcore_tenant"

var tenantIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// ValidTenantID is what a tenant's id must look like. The id is written into
// a SET statement, which takes no parameters, so the shape is not cosmetic.
func ValidTenantID(id string) bool { return tenantIDRe.MatchString(id) }

// SetTenant confines the rest of the transaction to a tenant's rows; the
// empty id is the platform space. It is a utility statement on purpose: a
// SELECT would take the transaction's snapshot, and an export sets its
// isolation level after this.
func SetTenant(ctx context.Context, q Querier, id string) error {
	if id != "" && !ValidTenantID(id) {
		return fmt.Errorf("invalid tenant id %q", id)
	}
	_, err := q.Exec(ctx, "SET LOCAL ddcore.tenant = '"+id+"'")
	return err
}

// Elevate lifts the rest of the transaction out of row-level security, back
// to the role the site connects as. It is for the framework's own work that
// has to cross spaces — a migration, finding a user by e-mail at sign-in,
// claiming a job. A statement run elevated on a tenant table names its tenant.
func Elevate(ctx context.Context, q Querier) error {
	_, err := q.Exec(ctx, "SET LOCAL ROLE NONE")
	return err
}

// Confine is the opposite of Elevate, for a transaction that crossed spaces
// to find out where it belongs and now goes on as a document context.
func Confine(ctx context.Context, q Querier, role string) error {
	_, err := q.Exec(ctx, "SET LOCAL ROLE "+Ident(role))
	return err
}

// TenancyApplied reports whether a migration has turned tenancy on in this
// database. It is what a connection checks before confining itself, and what
// refuses a binary configured without tenancy: the keys are composite by
// then, and statements that address a row by id alone no longer find one.
func TenancyApplied(ctx context.Context, q Querier) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT to_regclass('ddcore_tenancy') IS NOT NULL`).Scan(&ok)
	return ok, err
}

// tenantTables are the framework's own tables whose rows belong to a tenant,
// with the key each one is addressed by once it does.
var tenantTables = []struct{ table, key string }{
	{"ddcore_series", "tenant, prefix"},
	{"ddcore_job", ""},
	{"ddcore_notification", ""},
	{"ddcore_notification_due", "tenant, rule, reference_doctype, reference_id, due"},
	{"ddcore_vault", "tenant, name"},
}

func rowSecurity(table string) []string {
	t := Ident(table)
	return []string{
		fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", t),
		fmt.Sprintf("CREATE POLICY %s ON %s USING (tenant = %s) WITH CHECK (tenant = %s);", TenantPolicy, t, TenantExpr, TenantExpr),
	}
}

func primaryKey(table, pkName, cols string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s, ADD PRIMARY KEY (%s);", Ident(table), Ident(pkName), cols)
}

// EnsureTenantRole creates the confined role when the cluster lacks it and
// makes the session's login role a member, so it can SET ROLE to it. Running
// it again changes nothing. Migrate calls it on its own, committed, before
// anything else: a role created inside the migration's transaction is
// invisible to every other connection until the commit, and a confined one
// that a migration opens on the side would fail to take it.
func EnsureTenantRole(ctx context.Context, q Querier, role string) error {
	if !identRe.MatchString(role) {
		return fmt.Errorf("invalid tenant role name %q", role)
	}
	r := Ident(role)
	stmts := []string{
		// Roles belong to the cluster, not the database, so two databases
		// migrating at once race to create it: either error means it exists.
		`DO $$ BEGIN
		   IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '` + role + `') THEN
		     CREATE ROLE ` + r + ` NOLOGIN;
		   END IF;
		 EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL; END $$`,
		// A superuser may SET ROLE to anything; anyone else needs membership,
		// and can only grant it to itself when it created the role.
		`DO $$ BEGIN
		   IF NOT pg_has_role(session_user, '` + role + `', 'MEMBER') THEN
		     EXECUTE format('GRANT ` + r + ` TO %I', session_user);
		   END IF;
		 END $$`,
	}
	for _, s := range stmts {
		if _, err := q.Exec(ctx, s); err != nil {
			return fmt.Errorf("tenancy: preparing the role %s: %w (create it yourself and grant it to the site's login role, or name another with DDCORE_TENANT_ROLE)", role, err)
		}
	}
	return nil
}

// EnsureTenancy brings everything about tenancy that is not a DocType's table
// up to date: the marker, the confined role and its grants, and the tenant
// column, key and policy of the framework's own tables. Migrate calls it on
// every run, after the DocType tables exist. Running it again changes
// nothing, and it has to be run again: a restore drops the grants, and each
// table a later migration creates needs its own.
func EnsureTenancy(ctx context.Context, q Querier, role string) error {
	if err := EnsureTenantRole(ctx, q, role); err != nil {
		return err
	}
	r := Ident(role)
	if _, err := q.Exec(ctx, `CREATE TABLE IF NOT EXISTS ddcore_tenancy (
		   id boolean PRIMARY KEY DEFAULT true CHECK (id), role text NOT NULL, since timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `INSERT INTO ddcore_tenancy (role) VALUES ($1) ON CONFLICT (id) DO UPDATE SET role = EXCLUDED.role`, role); err != nil {
		return err
	}
	for _, t := range tenantTables {
		if err := ensureTenantTable(ctx, q, t.table, t.key); err != nil {
			return err
		}
	}
	// A queued job frees its key for the tenant that holds it, not for all.
	var def string
	if err := q.QueryRow(ctx, `SELECT coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'ddcore_job_unique'), '')`).Scan(&def); err != nil {
		return err
	}
	want := index{name: "ddcore_job_unique", table: "ddcore_job", unique: true, cols: "tenant, unique_key",
		predicate: "unique_key IS NOT NULL AND status = 'queued'"}
	if !sameIndex(def, want) {
		if _, err := q.Exec(ctx, "DROP INDEX IF EXISTS ddcore_job_unique; "+want.ddl()); err != nil {
			return err
		}
	}
	grants := []string{
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + "%s" + " TO " + r,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA " + "%s" + " TO " + r,
		"GRANT USAGE ON SCHEMA " + "%s" + " TO " + r,
	}
	var schema string
	if err := q.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return err
	}
	for _, g := range grants {
		if _, err := q.Exec(ctx, fmt.Sprintf(g, Ident(schema))); err != nil {
			return fmt.Errorf("tenancy: granting to %s: %w", role, err)
		}
	}
	return nil
}

func ensureTenantTable(ctx context.Context, q Querier, table, key string) error {
	if _, err := q.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS tenant text NOT NULL DEFAULT %s",
		Ident(table), TenantExpr)); err != nil {
		return err
	}
	if key != "" {
		var name, def string
		if err := q.QueryRow(ctx, `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conrelid = to_regclass($1) AND contype = 'p'`, table).Scan(&name, &def); err != nil {
			return fmt.Errorf("tenancy: %s has no primary key: %w", table, err)
		}
		if def != "PRIMARY KEY ("+key+")" {
			if _, err := q.Exec(ctx, primaryKey(table, name, key)); err != nil {
				return err
			}
		}
	}
	var secured bool
	var policies int
	if err := q.QueryRow(ctx, `SELECT c.relrowsecurity, (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid AND p.polname = $2)
		FROM pg_class c WHERE c.oid = to_regclass($1)`, table, TenantPolicy).Scan(&secured, &policies); err != nil {
		return err
	}
	rs := rowSecurity(table)
	if !secured {
		if _, err := q.Exec(ctx, rs[0]); err != nil {
			return err
		}
	}
	if policies == 0 {
		if _, err := q.Exec(ctx, rs[1]); err != nil {
			return err
		}
	}
	return nil
}

// tenantIndex puts `tenant` in front of an index of a tenant-owned table.
// Every query a confined transaction runs carries the policy's test on the
// column, so an index that does not lead with it serves nobody; and a unique
// one is what makes a value unique within the tenant instead of the site.
//
// User is the exception for uniqueness: sign-in finds a user by e-mail
// before any tenant is known, so its id and e-mail stay unique site-wide.
func tenantIndex(d *meta.DocType, i index) index {
	if !d.TenantOwned || (i.unique && !d.TenantKeyed()) {
		return i
	}
	i.cols = meta.TenantColumn + ", " + i.cols
	return i
}

// planTenancy is what an existing or new table of a tenant-owned DocType
// still lacks: the composite key and the policy. The column itself is an
// ordinary standard column and comes from the column diff.
func planTenancy(cat *catalog, d *meta.DocType, exists bool) []Statement {
	if !d.TenantOwned {
		return nil
	}
	t := d.TableName()
	var out []Statement
	st := func(sql string, k Kind) {
		out = append(out, Statement{SQL: sql, Kind: k, Doctype: d.Name, Table: t})
	}
	if pk, ok := cat.pk[t]; exists && ok && d.TenantKeyed() && pk.def != "PRIMARY KEY (tenant, id)" {
		st(primaryKey(t, pk.name, "tenant, id"), KindPrimaryKey)
	}
	rs := rowSecurity(t)
	if !cat.secured[t] {
		st(rs[0], KindRowSecurity)
	}
	if !cat.policy[t] {
		st(rs[1], KindRowSecurity)
	}
	return out
}
