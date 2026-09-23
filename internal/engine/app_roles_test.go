package engine

import (
	"context"
	"os"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// A role added to an app that is already installed must exist after the next
// migrate: installing happens once, and the role would otherwise never be
// created, so granting it fails with "Role does not exist".
func TestMigrateCreatesRolesAddedToAnInstalledApp(t *testing.T) {
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	for _, q := range []string{"DROP DATABASE IF EXISTS " + dbName, "CREATE DATABASE " + dbName} {
		if _, err := e0.DB.Pool.Exec(ctx, q); err != nil {
			e0.DB.Close()
			t.Fatal(err)
		}
	}
	e0.DB.Close()

	dir := t.TempDir()
	writeAppFile(t, dir, "ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "roles_test", roles: ["First Role"] });`)
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "roles_test", Dir: dir}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}

	writeAppFile(t, dir, "ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "roles_test", roles: ["First Role", "Second Role"] });`)
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	res, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Installed) != 0 {
		t.Fatalf("the app was installed again: %v", res.Installed)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, role := range []string{"First Role", "Second Role"} {
			if ok, err := c.Exists("Role", role); err != nil || !ok {
				t.Errorf("role %q missing after migrate (err %v)", role, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
