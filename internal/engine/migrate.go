package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/db"
)

// Plan returns the DDL that Migrate would apply.
func (e *Engine) Plan(ctx context.Context, prune bool) ([]string, error) {
	tx, err := e.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, db.InternalSchema); err != nil {
		return nil, err
	}
	return db.Plan(ctx, tx, e.Current().Meta, prune)
}

type MigrateResult struct {
	DDL       []string
	Patches   []string
	Installed []string
}

// Migrate applies DDL, installs new apps (afterInstall), runs pending
// patches and afterMigrate hooks — all in one transaction.
func (e *Engine) Migrate(ctx context.Context, prune bool) (*MigrateResult, error) {
	res := &MigrateResult{}
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		ddl, err := db.Plan(ctx, c.Tx, c.St.Meta, prune)
		if err != nil {
			return err
		}
		if err := db.Apply(ctx, c.Tx, ddl); err != nil {
			return err
		}
		res.DDL = ddl
		rows, err := db.Select(ctx, c.Tx, `SELECT app FROM ddcore_installed_app`)
		if err != nil {
			return err
		}
		installed := map[string]bool{}
		for _, r := range rows {
			installed[db.Str(r["app"])] = true
		}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		for _, name := range c.St.AppOrder() {
			app := c.St.Snap.Apps[name]
			if !installed[name] {
				for _, role := range app.Roles {
					if ok, _ := c.Exists("Role", role); !ok {
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
		}
		// fixtures: insert-or-skip by name
		for _, name := range c.St.AppOrder() {
			for dt, docs := range c.St.Snap.Apps[name].Fixtures {
				for _, values := range docs {
					doc, err := c.NewDoc(dt, Doc(values))
					if err != nil {
						return err
					}
					if n := doc.Str("name"); n != "" {
						if ok, _ := c.Exists(dt, n); ok {
							continue
						}
					} else if d, _ := c.St.DocType(dt); d != nil && d.Naming.Field != "" {
						if ok, _ := c.Exists(dt, doc.Str(d.Naming.Field)); ok {
							continue
						}
					}
					if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
						return fmt.Errorf("fixture %s: %w", dt, err)
					}
				}
			}
		}
		done, err := db.Select(ctx, c.Tx, `SELECT app, name FROM ddcore_patch`)
		if err != nil {
			return err
		}
		ran := map[string]bool{}
		for _, r := range done {
			ran[db.Str(r["app"])+"/"+db.Str(r["name"])] = true
		}
		for _, p := range c.St.Snap.Patches {
			if ran[p.App+"/"+p.Name] {
				continue
			}
			e.Log.Info("patch", "app", p.App, "name", p.Name)
			if err := rt.RunPatch(p.Path); err != nil {
				return fmt.Errorf("patch %s: %w", p.Path, err)
			}
			if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_patch (app, name) VALUES ($1, $2)`, p.App, p.Name); err != nil {
				return err
			}
			res.Patches = append(res.Patches, p.Path)
		}
		for _, name := range c.St.AppOrder() {
			if c.St.Snap.Apps[name].HasAfterMigrate {
				if err := rt.AppHook(name, "afterMigrate"); err != nil {
					return fmt.Errorf("%s.afterMigrate: %w", name, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	e.Cache.Clear()
	return res, nil
}

func (r *MigrateResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d DDL, %d patches, apps installed: %v", len(r.DDL), len(r.Patches), r.Installed)
	return b.String()
}
