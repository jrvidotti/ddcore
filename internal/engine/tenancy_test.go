package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

const (
	tenantA, tenantB = "alfa", "beta"
	userA, userB     = "ana@alfa.test", "bia@beta.test"
)

// tenancyFiles is what the tenancy suite adds to the base test app: a shared
// DocType, a Single, and a service that reads with raw SQL.
var tenancyFiles = map[string]string{
	"doctypes/pais/pais.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pais", shared: true, idGeneration: { field: "sigla" },
  fields: [{ fieldname: "sigla", fieldtype: "Data", label: "Sigla", reqd: true }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
	"doctypes/ajustes/ajustes.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ajustes", isSingle: true,
  fields: [{ fieldname: "lema", fieldtype: "Data", label: "Lema" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`,
	"services/cru.ts": `export function nomes() { return ddcore.db.sql("select id from tab_pessoa order by id").map((r: any) => r.id); }
export function todos() { return ddcore.db.sql("select tenant, id from tab_pessoa order by 1, 2"); }`,
}

func setupTenancy(t *testing.T) *Engine {
	t.Helper()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, tenancyFiles)}}, Test: true, Tenancy: true})
	seedTenants(t, e)
	return e
}

// seedTenants creates two tenants, each with one Gestor, and a Pessoa of the
// same id in each and in the platform space.
func seedTenants(t *testing.T, e *Engine) {
	t.Helper()
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, id := range []string{tenantA, tenantB} {
			if err := insertDoc(c, "Tenant", Doc{"slug": id, "title": strings.ToUpper(id)}); err != nil {
				return err
			}
		}
		return insertDoc(c, "Pessoa", Doc{"nome": "Comum", "cpf": "000"})
	})
	for tenant, user := range map[string]string{tenantA: userA, tenantB: userB} {
		inTenant(t, e, tenant, func(c *Ctx) error {
			if err := insertDoc(c, "User", Doc{"email": user, "full_name": user,
				"roles": []any{map[string]any{"role": "Gestor"}}}); err != nil {
				return err
			}
			return insertDoc(c, "Pessoa", Doc{"nome": "Comum", "cpf": "000"})
		})
	}
}

// inTenant runs fn as Admin entered in a tenant.
func inTenant(t *testing.T, e *Engine, tenant string, fn func(c *Ctx) error) {
	t.Helper()
	if err := e.Run(WithTenant(context.Background(), tenant), "Admin", fn); err != nil {
		t.Fatalf("in %s: %v", tenant, err)
	}
}

func TestTenantCtxIsConfinedToItsSpace(t *testing.T) {
	e := setupTenancy(t)
	runAs(t, e, userA, func(c *Ctx) error {
		if c.Tenant != tenantA {
			t.Fatalf("ctx tenant = %q", c.Tenant)
		}
		return insertDoc(c, "Pessoa", Doc{"nome": "So Alfa", "cpf": "111"})
	})
	runAs(t, e, userB, func(c *Ctx) error {
		if got := listNames(t, c, "Pessoa", ListArgs{}); strings.Join(got, ",") != "Comum" {
			t.Fatalf("beta lists %v", got)
		}
		if _, err := c.GetDoc("Pessoa", "So Alfa"); err == nil {
			t.Fatal("beta read alfa's document")
		} else {
			wantStatus(t, err, 404)
		}
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		if got := listNames(t, c, "Pessoa", ListArgs{}); strings.Join(got, ",") != "Comum" {
			t.Fatalf("the platform space lists %v", got)
		}
		return nil
	})
	// the rows are where they should be, seen from outside every space
	var got []string
	if err := e.System(context.Background(), func(q db.Querier) error {
		rows, err := db.Select(context.Background(), q, `SELECT tenant || '/' || id AS k FROM tab_pessoa ORDER BY 1`)
		for _, r := range rows {
			got = append(got, r["k"].(string))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if want := "/Comum,alfa/Comum,alfa/So Alfa,beta/Comum"; strings.Join(got, ",") != want {
		t.Fatalf("rows: %v", got)
	}
}

func TestTenantUserCannotNameAnotherTenant(t *testing.T) {
	e := setupTenancy(t)
	err := e.Run(WithTenant(context.Background(), tenantB), userA, func(c *Ctx) error { return nil })
	wantStatus(t, err, 403)
	err = e.Run(WithTenant(context.Background(), "nobody"), "Admin", func(c *Ctx) error { return nil })
	wantStatus(t, err, 404)
}
