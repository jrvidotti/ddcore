package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// tenantAccessFiles adds to the tenancy suite two shared DocTypes the
// platform keeps (#108): a lookup cache keyed by document, and one on a
// naming series. Gestor — the tenants' users' role — may read and write both,
// which is exactly what must not matter inside a tenant.
var tenantAccessFiles = map[string]string{
	"doctypes/consulta/consulta.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Consulta", shared: true, tenantAccess: "server", trackChanges: true,
  idGeneration: { field: "documento" },
  fields: [{ fieldname: "documento", fieldtype: "Data", label: "Documento", reqd: true },
           { fieldname: "nome", fieldtype: "Data", label: "Nome" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
	"doctypes/protocolo/protocolo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Protocolo", shared: true, tenantAccess: "server", idGeneration: { series: "PRT-.####" },
  fields: [{ fieldname: "assunto", fieldtype: "Data", label: "Assunto" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`,
}

func setupTenantAccess(t *testing.T) *Engine {
	t.Helper()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, tenancyFiles, tenantAccessFiles)}}, Test: true, Tenancy: true})
	seedTenants(t, e)
	return e
}

func TestTenantAccessServer(t *testing.T) {
	e := setupTenantAccess(t)
	ctx := context.Background()
	kept := func(t *testing.T, err error) {
		t.Helper()
		wantStatus(t, err, 403)
		if !strings.Contains(err.Error(), "kept by the platform") {
			t.Fatalf("message = %v", err)
		}
	}

	t.Run("a tenant's user is refused, whatever the role", func(t *testing.T) {
		runAs(t, e, userA, func(c *Ctx) error {
			kept(t, insertDoc(c, "Consulta", Doc{"documento": "111"}))
			_, err := c.GetList("Consulta", ListArgs{})
			kept(t, err)
			_, err = c.Count("Consulta", nil)
			kept(t, err)
			_, err = c.GetDoc("Consulta", "111")
			kept(t, err)
			for p, ok := range c.Permissions(c.St.Meta.DocTypes["Consulta"]) {
				if ok {
					t.Errorf("alfa's user has %s on Consulta", p)
				}
			}
			if !c.SpaceRefusesName("Consulta") {
				t.Error("boot would list Consulta to a tenant")
			}
			return nil
		})
	})

	t.Run("a tenant's server code writes in the platform space", func(t *testing.T) {
		inTenant(t, e, tenantA, func(c *Ctx) error {
			d, _ := c.NewDoc("Consulta", Doc{"documento": "222", "nome": "Ana"})
			d, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
			if err != nil {
				return err
			}
			d["nome"] = "Ana Maria"
			if _, err := c.Save(d, SaveOpts{IgnorePermissions: true}); err != nil {
				return err
			}
			if c.Tenant != tenantA {
				t.Fatalf("the write left the ctx in %q", c.Tenant)
			}
			got, err := c.GetDocOpts("Consulta", "222", GetOpts{IgnorePermissions: true})
			if err != nil || got.Str("nome") != "Ana Maria" {
				t.Fatalf("server read: %v %v", got, err)
			}
			// what is not insert or save stays refused, server code or not
			_, err = c.DBSet("Consulta", "222", Doc{"nome": "x"}, true)
			wantStatus(t, err, 403)
			wantStatus(t, c.Delete("Consulta", "222", true, false), 403)
			return nil
		})
		// one cache: beta's server code reads alfa's lookup
		inTenant(t, e, tenantB, func(c *Ctx) error {
			v, err := c.GetValue("Consulta", "222", "nome")
			if err != nil || v != "Ana Maria" {
				t.Fatalf("beta reads %v (%v)", v, err)
			}
			return nil
		})
		if got := sysScalar(t, e, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Consulta' AND tenant = '' AND data::jsonb->>'source_tenant' = 'alfa'`); got != 1 {
			t.Fatalf("the platform has %d Versions from alfa", got)
		}
		if got := sysScalar(t, e, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Consulta' AND tenant <> ''`); got != 0 {
			t.Fatalf("a tenant holds %d Versions of Consulta", got)
		}
	})

	t.Run("a series counts in the platform space", func(t *testing.T) {
		var ids []string
		for _, tenant := range []string{tenantA, tenantB, ""} {
			inTenant(t, e, tenant, func(c *Ctx) error {
				d, _ := c.NewDoc("Protocolo", Doc{"assunto": tenant})
				d, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
				if err != nil {
					return err
				}
				ids = append(ids, d.ID())
				return nil
			})
		}
		if got := strings.Join(ids, ","); got != "PRT-0001,PRT-0002,PRT-0003" {
			t.Fatalf("ids = %s", got)
		}
		if got := sysScalar(t, e, `SELECT count(*) FROM ddcore_series WHERE prefix LIKE 'PRT-%' AND tenant <> ''`); got != 0 {
			t.Fatalf("a tenant holds %d PRT counters", got)
		}
	})

	t.Run("server code in TS", func(t *testing.T) {
		out, _, err := e.Eval(WithTenant(ctx, tenantA), `
ddcore.newDoc("Consulta", { documento: "333", nome: "Bia" }).insert({ ignorePermissions: true });
[ddcore.db.getAll("Consulta", { filters: { documento: "333" }, fields: ["nome"] })[0].nome,
 ddcore.db.getValue("Consulta", "333", "nome"),
 ddcore.db.exists("Consulta", "333"),
 ddcore.getDoc("Consulta", "333", { ignorePermissions: true }).nome].join("|")`, false)
		if err != nil || string(out) != `"Bia|Bia|333|Bia"` {
			t.Fatalf("eval: %s %v", out, err)
		}
		_, _, err = e.Eval(WithTenant(ctx, tenantA), `ddcore.getDoc("Consulta", "222")`, false)
		if err == nil || !strings.Contains(err.Error(), "kept by the platform") {
			t.Fatalf("a read that keeps permissions: %v", err)
		}
	})

	t.Run("the platform space is unchanged", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error {
			if err := insertDoc(c, "Consulta", Doc{"documento": "444"}); err != nil {
				return err
			}
			n, err := c.Count("Consulta", nil)
			if err != nil || n < 2 {
				t.Fatalf("platform counts %d (%v)", n, err)
			}
			return c.ValidateWebhook(Doc{"event_type": "Document", "webhook_doctype": "Consulta", "url": "https://x.test/h", "on_insert": true, "timeout": 10, "max_attempts": 3})
		})
		inTenant(t, e, tenantA, func(c *Ctx) error {
			err := c.ValidateWebhook(Doc{"event_type": "Document", "webhook_doctype": "Consulta", "url": "https://x.test/h", "on_insert": true, "timeout": 10, "max_attempts": 3})
			if err == nil || !strings.Contains(err.Error(), "cannot be watched") {
				t.Fatalf("a tenant may watch Consulta with a webhook: %v", err)
			}
			return nil
		})
	})

	t.Run("its events stay with the platform", func(t *testing.T) {
		platform := e.Events.SubscribeIn("Admin", "", func(string, string) bool { return true })
		beta := e.Events.SubscribeIn(userB, tenantB, func(string, string) bool { return true })
		inTenant(t, e, tenantA, func(c *Ctx) error {
			d, _ := c.NewDoc("Consulta", Doc{"documento": "555"})
			_, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
			return err
		})
		if len(platform) == 0 {
			t.Fatal("the platform heard nothing of a tenant's write")
		}
		if len(beta) != 0 {
			t.Fatalf("beta heard %d events", len(beta))
		}
	})
}
