package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jrvidotti/ddcore/internal/testdb"
)

// The tenancy design rests on a handful of Postgres behaviours. They are
// pinned here, against the real server, so a version that changes one of
// them fails a named test rather than leaking rows between tenants.
func TestTenancySpike(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.WithDatabase(testdb.BaseDSN(), testdb.Database(testdb.BaseDSN())+"_spike")
	if err := testdb.Empty(ctx, dsn); err != nil {
		if errors.Is(err, testdb.ErrUnavailable) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.DropOwn(testdb.BaseDSN()) })

	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	const role = "ddcore_spike_confined"
	setup := []string{
		`DO $$ BEGIN CREATE ROLE ` + role + ` NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`CREATE TABLE t (
			tenant text NOT NULL DEFAULT coalesce(current_setting('ddcore.tenant', true), ''),
			id text NOT NULL, PRIMARY KEY (tenant, id))`,
		`ALTER TABLE t ENABLE ROW LEVEL SECURITY`,
		`CREATE POLICY ddcore_tenant ON t USING (tenant = coalesce(current_setting('ddcore.tenant', true), ''))
			WITH CHECK (tenant = coalesce(current_setting('ddcore.tenant', true), ''))`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON t TO ` + role,
		`INSERT INTO t (tenant, id) VALUES ('', 'p'), ('a', 'x'), ('b', 'x')`,
	}
	for _, s := range setup {
		if _, err := owner.Exec(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1 // one connection, so every statement below shares its plan cache
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ids := func(q Querier) string {
		t.Helper()
		rows, err := Select(ctx, q, `SELECT tenant || '/' || id AS k FROM t ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		out := ""
		for _, r := range rows {
			out += r["k"].(string) + " "
		}
		return out
	}

	t.Run("a pool statement sees only the platform space, even as a superuser login", func(t *testing.T) {
		if got := ids(pool); got != "/p " {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("the same prepared statement follows the tenant setting and the role", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SET LOCAL ddcore.tenant = 'a'`); err != nil {
			t.Fatal(err)
		}
		if got := ids(tx); got != "a/x " {
			t.Fatalf("tenant a: got %q", got)
		}
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE NONE`); err != nil {
			t.Fatal(err)
		}
		if got := ids(tx); got != "/p a/x b/x " {
			t.Fatalf("elevated: got %q", got)
		}
	})

	t.Run("the setting and the role end with the transaction", func(t *testing.T) {
		if got := ids(pool); got != "/p " {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("an insert is stamped by the default and checked by the policy", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SET LOCAL ddcore.tenant = 'a'`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO t (id) VALUES ('y')`); err != nil {
			t.Fatal(err)
		}
		if got := ids(tx); got != "a/x a/y " {
			t.Fatalf("got %q", got)
		}
		if _, err := tx.Exec(ctx, `SAVEPOINT s`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO t (tenant, id) VALUES ('b', 'z')`); err == nil {
			t.Fatal("a row for another tenant was accepted")
		}
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT s`); err != nil {
			t.Fatal(err)
		}
		tag, err := tx.Exec(ctx, `UPDATE t SET id = 'gone' WHERE id = 'x'`)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("update by id touched %d rows, err %v", tag.RowsAffected(), err)
		}
	})

	t.Run("rolling back to a savepoint reverts a setting made after it", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, s := range []string{`SET LOCAL ddcore.tenant = 'a'`, `SAVEPOINT s`, `SET LOCAL ddcore.tenant = 'b'`} {
			if _, err := tx.Exec(ctx, s); err != nil {
				t.Fatal(err)
			}
		}
		if got := ids(tx); got != "b/x " {
			t.Fatalf("got %q", got)
		}
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT s`); err != nil {
			t.Fatal(err)
		}
		if got := ids(tx); got != "a/x " {
			t.Fatalf("after rollback: got %q", got)
		}
	})

	t.Run("the isolation level can still be set after the tenant", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SET LOCAL ddcore.tenant = 'a'`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); err != nil {
			t.Fatal(err)
		}
	})

	// Not a guarantee but a documented limit: SQL that an app author writes
	// can name another tenant. Tenants are users, never app authors.
	t.Run("set_config inside a query changes the space", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SELECT set_config('ddcore.tenant', 'b', true)`); err != nil {
			t.Fatal(err)
		}
		if got := ids(tx); got != "b/x " {
			t.Fatalf("got %q", got)
		}
	})
}

// A database restored into a cluster that never had the role carries the
// marker but not the role. Opening the pool must not fail on it: SET ROLE to
// a missing role is 22023, not the 42704 one might expect (#66).
func TestOpenConfinedWithoutTheRole(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.WithDatabase(testdb.BaseDSN(), testdb.Database(testdb.BaseDSN())+"_norole")
	if err := testdb.Empty(ctx, dsn); err != nil {
		if errors.Is(err, testdb.ErrUnavailable) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.DropOwn(testdb.BaseDSN()) })
	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = owner.Exec(ctx, `CREATE TABLE ddcore_tenancy (id boolean PRIMARY KEY DEFAULT true CHECK (id), role text NOT NULL)`)
	owner.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := OpenConfined(ctx, dsn, Options{TenantRole: "ddcore_never_created"})
	if err != nil {
		t.Fatalf("a missing role must not stop the pool: %v", err)
	}
	defer d.Close()
	if _, err := d.Pool.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
}
