package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// tenantOnlyFiles adds to the tenancy suite's app a DocType that lives inside
// a tenant (#105).
var tenantOnlyFiles = map[string]string{
	"doctypes/folha/folha.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Folha", space: "tenant",
  fields: [{ fieldname: "mes", fieldtype: "Data", label: "Mes", reqd: true }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
}

// rhApp is a second app whose DocTypes are all the tenants', unless one says
// otherwise.
func rhApp(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rh", title: "RH", space: "tenant", requires: ["demo"] });`,
		"doctypes/ponto/ponto.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ponto", fields: [{ fieldname: "hora", fieldtype: "Data", label: "Hora" }] });`,
		"doctypes/aviso/aviso.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Aviso", space: "any", fields: [{ fieldname: "texto", fieldtype: "Data", label: "Texto" }] });`,
		"doctypes/cargo/cargo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cargo", shared: true, fields: [{ fieldname: "titulo", fieldtype: "Data", label: "Titulo" }] });`,
	}
	for k, v := range extra {
		files[k] = v
	}
	for rel, src := range files {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func setupTenantOnly(t *testing.T) *Engine {
	t.Helper()
	files := map[string]string{}
	for k, v := range tenancyFiles {
		files[k] = v
	}
	for k, v := range tenantOnlyFiles {
		files[k] = v
	}
	e := migratedEngine(t, Config{Apps: []js.App{
		{Name: "demo", Dir: testApp(t, files)},
		{Name: "rh", Dir: rhApp(t, nil)},
	}, Test: true, Tenancy: true})
	seedTenants(t, e)
	return e
}

func TestTenantOnlyDocTypeIsRefusedInThePlatformSpace(t *testing.T) {
	e := setupTenantOnly(t)
	ctx := context.Background()

	for name, want := range map[string]bool{"Folha": true, "Ponto": true, "Aviso": false, "Cargo": false, "Pessoa": false} {
		if d, _ := e.Meta.Get(name); d.TenantOnly != want {
			t.Errorf("%s: TenantOnly = %v, want %v", name, d.TenantOnly, want)
		}
	}

	refused := func(t *testing.T, err error) {
		t.Helper()
		wantStatus(t, err, 403)
		if !strings.Contains(err.Error(), "lives inside a tenant") {
			t.Fatalf("message = %v", err)
		}
	}

	// inside a tenant it is an ordinary DocType
	var id string
	inTenant(t, e, tenantA, func(c *Ctx) error {
		d, _ := c.NewDoc("Folha", Doc{"mes": "2026-10"})
		d, err := c.Insert(d, SaveOpts{})
		if err != nil {
			return err
		}
		id = d.ID()
		if n, err := c.Count("Folha", nil); err != nil || n != 1 {
			t.Fatalf("alfa counts %d (%v)", n, err)
		}
		return insertDoc(c, "Ponto", Doc{"hora": "08:00"})
	})

	// the platform space reads none of it, writes none of it, even with
	// permissions ignored
	t.Run("platform", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			refused(t, insertDoc(c, "Folha", Doc{"mes": "2026-11"}))
			refused(t, insertDoc(c, "Ponto", Doc{"hora": "09:00"}))
			d, _ := c.NewDoc("Folha", Doc{"mes": "x"})
			_, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
			refused(t, err)
			_, err = c.GetList("Folha", ListArgs{IgnorePermissions: true})
			refused(t, err)
			_, err = c.Count("Ponto", nil)
			refused(t, err)
			_, err = c.GetDoc("Folha", id)
			refused(t, err)
			_, err = c.DBSet("Folha", id, Doc{"mes": "y"}, true)
			refused(t, err)
			refused(t, c.Delete("Folha", id, true, false))
			for p, ok := range c.Permissions(c.St.Meta.DocTypes["Folha"]) {
				if ok {
					t.Errorf("platform has %s on Folha", p)
				}
			}
			// what is not tenant-only is untouched
			if err := insertDoc(c, "Aviso", Doc{"texto": "oi"}); err != nil {
				t.Fatalf("Aviso (space any): %v", err)
			}
			return insertDoc(c, "Cargo", Doc{"titulo": "Analista"})
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("eval without a tenant reports the refusal", func(t *testing.T) {
		_, _, err := e.Eval(ctx, `ddcore.newDoc("Folha", { mes: "2026-12" }).insert().id`, true)
		if err == nil || !strings.Contains(err.Error(), "lives inside a tenant") {
			t.Fatalf("eval: %v", err)
		}
		if got := sysScalar(t, e, `SELECT count(*) FROM tab_folha WHERE tenant = ''`); got != 0 {
			t.Fatalf("the platform space holds %d Folha", got)
		}
	})

	t.Run("tenant.run from the platform enters", func(t *testing.T) {
		out, _, err := e.Eval(ctx, `ddcore.tenant.run("beta", () => { ddcore.newDoc("Folha", { mes: "2026-10" }).insert(); return ddcore.db.count("Folha"); })`, false)
		if err != nil || string(out) != "1" {
			t.Fatalf("tenant.run: %s %v", out, err)
		}
	})

	t.Run("a patch is not refused", func(t *testing.T) {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			c.Flags["inPatch"] = true
			_, err := c.GetList("Folha", ListArgs{IgnorePermissions: true})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestTenantOnlyLoadRules(t *testing.T) {
	load := func(t *testing.T, extra map[string]string) error {
		t.Helper()
		e, err := New(context.Background(), Config{Apps: []js.App{
			{Name: "demo", Dir: testApp(t)},
			{Name: "rh", Dir: rhApp(t, extra)},
		}, Tenancy: true})
		if err == nil {
			e.Current().Pool.Close()
		}
		return err
	}
	for name, c := range map[string]struct {
		files map[string]string
		want  string
	}{
		"fixtures": {map[string]string{"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rh", space: "tenant", requires: ["demo"], fixtures: { Ponto: [{ hora: "08:00" }] } });`}, "onTenantCreate"},
		"app value": {map[string]string{"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rh", space: "any" as any, requires: ["demo"] });`}, `space "any" is not "tenant"`},
		"doctype value": {map[string]string{"doctypes/ponto/ponto.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ponto", space: "tenants" as any, fields: [] });`}, `space "tenants"`},
		"shared": {map[string]string{"doctypes/cargo/cargo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cargo", shared: true, space: "tenant", fields: [] });`}, "a shared DocType is read in every space"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := load(t, c.files); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}
