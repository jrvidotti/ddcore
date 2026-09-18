package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// Plan returns the DDL that Migrate would apply.
func (e *Engine) Plan(ctx context.Context, prune bool) ([]db.Statement, error) {
	tx, err := e.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := db.EnsureInternal(ctx, tx); err != nil {
		return nil, err
	}
	return db.Plan(ctx, tx, e.Current().Meta, prune)
}

type MigrateResult struct {
	DDL       []db.Statement
	Patches   []string
	Installed []string
	Renames   []db.Statement
	// Recorded are the patches a first install wrote down without running.
	Recorded []string
}

// Migrate brings the database to the meta, in one transaction.
//
// The order is the whole point of DAT-04. Before this, the DDL always ran first
// and the drops ran with it, so a rename was an empty new column beside a
// doomed old one and no patch could do anything about it. Now:
//
//  1. the framework's own tables, so the ledgers exist
//  2. beforeSchema patches, against the shape the database still has
//  3. the plan, computed *after* those patches — one of them may have changed
//     the schema by hand, and a plan from before would be stale
//  4. the additive DDL: renames, creates, adds, declared conversions, indexes
//  5. the reference sweep for each DocType this run renamed
//  6. installing new apps, then fixtures
//  7. afterSchema patches — where a backfill lives
//  8. the drops, last, so step 7 could still read what step 8 removes
//  9. afterMigrate
//
// It stays one transaction. Postgres has transactional DDL, so a failure
// anywhere leaves nothing behind — and that is what makes the "validate" step
// of expand → backfill → validate → contract free: a beforeSchema patch that
// throws rolls the contraction back with everything else.
func (e *Engine) Migrate(ctx context.Context, prune bool) (*MigrateResult, error) {
	res := &MigrateResult{}
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		// a migration is exactly the work a maintenance window is opened for,
		// including the one `dev --auto-migrate` runs inside a server
		c.Flags[bypassMaintenanceFlag] = true
		if err := db.EnsureInternal(ctx, c.Tx); err != nil {
			return err
		}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		fresh, err := recordFreshInstallPatches(ctx, c, res)
		if err != nil {
			return err
		}
		if err := runPatches(ctx, c, rt, res, true); err != nil {
			return err
		}
		plan, err := db.Plan(ctx, c.Tx, c.St.Meta, prune)
		if err != nil {
			return err
		}
		keep, drop := db.Destructive(plan)
		if err := db.Apply(ctx, c.Tx, keep); err != nil {
			return err
		}
		if err := absorbVaultAuditLog(ctx, c.Tx); err != nil {
			return err
		}
		if err := renameLegacyAdmin(ctx, c); err != nil {
			return err
		}
		res.DDL = plan
		for _, st := range keep {
			if st.Kind == db.KindRenameTable || st.Kind == db.KindRenameColumn {
				if err := c.recordRename(st); err != nil {
					return err
				}
				res.Renames = append(res.Renames, st)
			}
		}
		if err := c.installApps(ctx, rt, fresh, res); err != nil {
			return err
		}
		if err := c.applyFixtures(); err != nil {
			return err
		}
		if err := runPatches(ctx, c, rt, res, false); err != nil {
			return err
		}
		if err := db.Apply(ctx, c.Tx, drop); err != nil {
			return err
		}
		for _, name := range c.St.AppOrder() {
			if c.St.Snap.Apps[name].HasAfterMigrate {
				if err := rt.AppHook(name, "afterMigrate"); err != nil {
					return fmt.Errorf("%s.afterMigrate: %w", name, err)
				}
			}
		}
		return c.recordSiteVersion()
	})
	if err != nil {
		return nil, err
	}
	e.Cache.Clear()
	return res, nil
}

// recordFreshInstallPatches writes down, without running, the patches of an app
// being installed for the first time, and reports which apps those are.
//
// A patch describes a change to data that is already there. On a database that
// has none, running the whole history is at best a no-op and at worst — now
// that a patch can run before the DDL — a walk over tables that do not exist
// yet. Seeding is what afterInstall and fixtures are for.
func recordFreshInstallPatches(ctx context.Context, c *Ctx, res *MigrateResult) (map[string]bool, error) {
	rows, err := db.Select(ctx, c.Tx, `SELECT app FROM ddcore_installed_app`)
	if err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	for _, r := range rows {
		installed[db.Str(r["app"])] = true
	}
	fresh := map[string]bool{}
	for _, name := range c.St.AppOrder() {
		if !installed[name] {
			fresh[name] = true
		}
	}
	for _, p := range c.St.Snap.Patches {
		if !fresh[p.App] {
			continue
		}
		if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_patch (app, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, p.App, p.Name); err != nil {
			return nil, err
		}
		res.Recorded = append(res.Recorded, p.Path)
	}
	return fresh, nil
}

// runPatches runs the pending patches of one phase, in declaration order.
func runPatches(ctx context.Context, c *Ctx, rt *js.Runtime, res *MigrateResult, before bool) error {
	done, err := db.Select(ctx, c.Tx, `SELECT app, name FROM ddcore_patch`)
	if err != nil {
		return err
	}
	ran := map[string]bool{}
	for _, r := range done {
		ran[db.Str(r["app"])+"/"+db.Str(r["name"])] = true
	}
	for _, p := range c.St.Snap.Patches {
		if p.BeforeSchema() != before || ran[p.App+"/"+p.Name] {
			continue
		}
		c.E.Log.Info("patch", "app", p.App, "name", p.Name, "phase", p.Phase)
		if err := runPatch(c, rt, p.Path); err != nil {
			return fmt.Errorf("patch %s: %w", p.Path, err)
		}
		if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_patch (app, name) VALUES ($1, $2)`, p.App, p.Name); err != nil {
			return err
		}
		res.Patches = append(res.Patches, p.Path)
	}
	return nil
}

// runPatch opens the window in which ctx.sql may write, and closes it however
// the patch ends — a panic in goja included, or the next controller in the same
// transaction would inherit the privilege.
func runPatch(c *Ctx, rt *js.Runtime, path string) error {
	c.Flags["inPatch"] = true
	defer func() { c.Flags["inPatch"] = false }()
	return rt.RunPatch(path)
}

// installApps creates the declared roles and runs afterInstall for every app
// installed for the first time.
func (c *Ctx) installApps(ctx context.Context, rt *js.Runtime, fresh map[string]bool, res *MigrateResult) error {
	for _, name := range c.St.AppOrder() {
		if !fresh[name] {
			continue
		}
		app := c.St.Snap.Apps[name]
		for _, role := range app.Roles {
			if ok, _ := c.nameExists("Role", role); !ok {
				doc, _ := c.NewDoc("Role", Doc{"role_name": role})
				if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
					return fmt.Errorf("role %s: %w", role, err)
				}
			}
		}
		if app.HasAfterInstall {
			if err := rt.AppHook(name, "afterInstall"); err != nil {
				return fmt.Errorf("%s.afterInstall: %w", name, err)
			}
		}
		if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_installed_app (app) VALUES ($1)`, name); err != nil {
			return err
		}
		res.Installed = append(res.Installed, name)
	}
	return nil
}

// applyFixtures inserts the declared fixtures that are not there yet, skipping
// by name. The DocTypes are walked in a fixed order: a Go map range is not, and
// a fixture whose Link points at another fixture's DocType would then fail on
// some runs and not others.
func (c *Ctx) applyFixtures() error {
	for _, name := range c.St.AppOrder() {
		fixtures := c.St.Snap.Apps[name].Fixtures
		doctypes := make([]string, 0, len(fixtures))
		for dt := range fixtures {
			doctypes = append(doctypes, dt)
		}
		sort.Strings(doctypes)
		for _, dt := range doctypes {
			for _, values := range fixtures[dt] {
				doc, err := c.NewDoc(dt, Doc(values))
				if err != nil {
					return err
				}
				if n := doc.Str("name"); n != "" {
					if ok, _ := c.nameExists(dt, n); ok {
						continue
					}
				} else if d, _ := c.St.DocType(dt); d != nil && d.Naming.Field != "" {
					if ok, _ := c.nameExists(dt, doc.Str(d.Naming.Field)); ok {
						continue
					}
				}
				if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
					return fmt.Errorf("fixture %s: %w", dt, err)
				}
			}
		}
	}
	return nil
}

func (r *MigrateResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d DDL, %d patches, apps installed: %v", len(r.DDL), len(r.Patches), r.Installed)
	if len(r.Renames) > 0 {
		fmt.Fprintf(&b, ", %d renames", len(r.Renames))
	}
	return b.String()
}

// absorbVaultAuditLog copies rows from legacy tab_vault_audit_log into tab_audit_event and drops tab_vault_audit_log (PRD-06).
func absorbVaultAuditLog(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'tab_vault_audit_log') THEN
				INSERT INTO tab_audit_event (name, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_name, ip, request_id, detail)
				SELECT name, owner, creation, modified, modified_by, docstatus, 'vault.' || action, 'Allowed', "user", 'Vault Secret', secret_name, ip, request_id, NULL
				FROM tab_vault_audit_log
				ON CONFLICT (name) DO NOTHING;
				DROP TABLE tab_vault_audit_log;
			END IF;
		END $$;
	`)
	return err
}

