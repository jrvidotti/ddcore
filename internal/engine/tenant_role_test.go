package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/testdb"
)

// A fresh cluster has never seen the tenant role. The first migrate creates
// it, and the roles it then inserts queue webhooks, whose lookup runs on a
// confined connection of its own: that connection cannot SET ROLE to a role
// only the migration's transaction can see yet (#66). A random role name is
// what makes the dev cluster, which has had ddcore_tenant for ages, fresh.
func TestMigrateOnAClusterWithoutTheTenantRole(t *testing.T) {
	ctx := context.Background()
	b := make([]byte, 6)
	rand.Read(b)
	role := "ddcore_t_" + hex.EncodeToString(b)
	dsn := testdb.WithDatabase(testDSN, testdb.Database(testDSN)+"_role")
	err := testdb.Empty(ctx, dsn)
	if errors.Is(err, testdb.ErrUnavailable) && os.Getenv("DDCORE_TEST_DSN") == "" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// the database holds grants to the role, so it goes first
		admin, err := pgx.Connect(ctx, testdb.AdminDSN(dsn))
		if err != nil {
			t.Log(err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+db.Ident(testdb.Database(dsn))+" WITH (FORCE)"); err != nil {
			t.Log(err)
		}
		if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+db.Ident(role)); err != nil {
			t.Log(err)
		}
	})

	e, err := New(ctx, Config{DSN: dsn, Test: true, Tenancy: true, TenantRole: role,
		Apps: []js.App{{Name: "demo", Dir: testApp(t, tenancyFiles)}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.DB.Close()
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatalf("migrate on a cluster without the role: %v", err)
	}
	var exists bool
	if err := e.DB.Sys.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("role %s was not created", role)
	}
}
