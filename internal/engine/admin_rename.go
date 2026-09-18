package engine

import (
	"context"
	"fmt"

	"github.com/jrvidotti/ddcore/internal/db"
)

// legacyAdmin is the name the superuser had before 0.16.0.
const legacyAdmin = "Administrator"

// userColumns are the framework tables that hold a user name outside any
// DocType, so neither the meta nor coreRefs can find them.
var userColumns = [][2]string{
	{"ddcore_session", "user"},
	{"ddcore_auth_token", "user"},
	{"ddcore_auth_token", "created_by"},
	{"ddcore_default", "user"},
	{"ddcore_job", "user"},
	{"ddcore_job", "cancelled_by"},
	{"ddcore_maintenance", "actor"},
	{"ddcore_notification", "recipient"},
}

// renameLegacyAdmin turns the superuser of a database installed before 0.16.0
// from Administrator into Admin. The code only knows Admin now, so leaving the
// old row in place would leave a site whose superuser is an ordinary user and
// whose migrations write as somebody who does not exist.
//
// It does nothing on a new database, on one already renamed, and on one where
// both names exist — that last case is somebody's deliberate data, not ours to
// merge.
func renameLegacyAdmin(ctx context.Context, c *Ctx) error {
	d, err := c.St.DocType("User")
	if err != nil {
		return nil // no core app loaded: nothing to rename
	}
	rows, err := db.Select(ctx, c.Tx, `SELECT name FROM tab_user WHERE name IN ($1, $2)`, legacyAdmin, "Admin")
	if err != nil {
		return err
	}
	if len(rows) != 1 || db.Str(rows[0]["name"]) != legacyAdmin {
		return nil
	}
	if err := c.moveName(d, legacyAdmin, "Admin"); err != nil {
		return fmt.Errorf("rename %s: %w", legacyAdmin, err)
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE tab_user SET full_name = 'Admin' WHERE name = 'Admin' AND full_name = $1`, legacyAdmin); err != nil {
		return err
	}
	// owner and modified_by are standard columns, not fields, so the Link
	// sweep in moveName never sees them.
	for _, dt := range c.St.Meta.DocTypes {
		t := db.Ident(dt.TableName())
		for _, col := range []string{"owner", "modified_by"} {
			if _, err := c.Tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = 'Admin' WHERE %s = $1", t, col, col), legacyAdmin); err != nil {
				return fmt.Errorf("%s.%s: %w", dt.TableName(), col, err)
			}
		}
	}
	// The audit actor is a Data field, written by the engine rather than
	// typed by a person.
	if _, err := c.Tx.Exec(ctx, `UPDATE tab_audit_event SET actor = 'Admin' WHERE actor = $1`, legacyAdmin); err != nil {
		return err
	}
	for _, tc := range userColumns {
		q := fmt.Sprintf("UPDATE %s SET %s = 'Admin' WHERE %s = $1", tc[0], db.Ident(tc[1]), db.Ident(tc[1]))
		if _, err := c.Tx.Exec(ctx, q, legacyAdmin); err != nil {
			return fmt.Errorf("%s.%s: %w", tc[0], tc[1], err)
		}
	}
	c.E.Log.Info("renamed the superuser", "from", legacyAdmin, "to", "Admin")
	return nil
}
