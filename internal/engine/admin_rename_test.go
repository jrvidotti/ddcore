package engine

import (
	"context"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// A database installed before 0.16.0 has its superuser as Administrator, with
// that name in every owner column, in its sessions and in its role rows. The
// next migrate must hand all of it to Admin.
func TestMigrateRenamesLegacyAdministrator(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	pessoa := insertPessoa(t, e, Doc{"nome": "Ana", "tipo": "PF"})

	// Turn this fresh database into one a 0.15 binary would have left.
	for _, q := range []string{
		`UPDATE tab_user SET id = 'Administrator', email = 'Administrator', full_name = 'Administrator' WHERE id = 'Admin'`,
		`UPDATE tab_has_role SET parent = 'Administrator' WHERE parent = 'Admin' AND parenttype = 'User'`,
		`UPDATE tab_pessoa SET owner = 'Administrator', modified_by = 'Administrator'`,
		`INSERT INTO ddcore_session (sid, "user", expires) VALUES ('legacy', 'Administrator', now() + interval '1 day')`,
	} {
		if _, err := e.DB.Pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}

	if rows := sqlRows(t, e, `SELECT id FROM tab_user WHERE id = 'Administrator'`); len(rows) != 0 {
		t.Fatal("Administrator is still a user")
	}
	u := sqlRows(t, e, `SELECT email, full_name FROM tab_user WHERE id = 'Admin'`)
	if len(u) != 1 || db.Str(u[0]["email"]) != "Admin" || db.Str(u[0]["full_name"]) != "Admin" {
		t.Fatalf("Admin not renamed in place: %v", u)
	}
	if rows := sqlRows(t, e, `SELECT role FROM tab_has_role WHERE parent = 'Admin' AND role = 'System Manager'`); len(rows) != 1 {
		t.Fatal("Admin lost System Manager")
	}
	p := sqlRows(t, e, `SELECT owner, modified_by FROM tab_pessoa WHERE id = $1`, pessoa)
	if db.Str(p[0]["owner"]) != "Admin" || db.Str(p[0]["modified_by"]) != "Admin" {
		t.Fatalf("owner columns not moved: %v", p)
	}
	if rows := sqlRows(t, e, `SELECT 1 FROM ddcore_session WHERE sid = 'legacy' AND "user" = 'Admin'`); len(rows) != 1 {
		t.Fatal("session not moved")
	}

	// Once done, it is done: a second migrate finds nothing to rename.
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
}
