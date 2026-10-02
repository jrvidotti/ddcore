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

func mustErr(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected a refusal", what)
	}
}

// The matrix: for every way the engine reaches a row, tenant beta cannot see
// or change what tenant alfa wrote, and the two keep separate ids, unique
// values, series and Singles.
func TestTenantIsolation(t *testing.T) {
	e := setupTenancy(t)
	var pedidoA string
	runAs(t, e, userA, func(c *Ctx) error {
		if err := insertDoc(c, "Pessoa", Doc{"nome": "Alfa Um", "cpf": "111"}); err != nil {
			return err
		}
		p, err := c.NewDoc("Pedido", Doc{"cliente": "Alfa Um",
			"itens": []any{map[string]any{"descricao": "x", "qtd": 1, "valor": 10}}})
		if err != nil {
			return err
		}
		p, err = c.Insert(p, SaveOpts{})
		pedidoA = p.Str("id")
		return err
	})

	t.Run("ids, unique values and series are per tenant", func(t *testing.T) {
		runAs(t, e, userB, func(c *Ctx) error {
			// same id and same unique cpf as alfa's document
			if err := insertDoc(c, "Pessoa", Doc{"nome": "Alfa Um", "cpf": "111"}); err != nil {
				return err
			}
			p, err := c.NewDoc("Pedido", Doc{"cliente": "Alfa Um"})
			if err != nil {
				return err
			}
			p, err = c.Insert(p, SaveOpts{})
			if err != nil {
				return err
			}
			if p.Str("id") != pedidoA {
				t.Fatalf("beta's first Pedido is %s, alfa's was %s: the series is shared", p.Str("id"), pedidoA)
			}
			// and uniqueness still holds inside the tenant
			mustErr(t, "duplicate cpf in one tenant", insertDoc(c, "Pessoa", Doc{"nome": "Outro", "cpf": "111"}))
			return nil
		})
	})

	t.Run("read paths", func(t *testing.T) {
		runAs(t, e, userA, func(c *Ctx) error {
			if err := insertDoc(c, "Pessoa", Doc{"nome": "Segredo Alfa", "cpf": "222"}); err != nil {
				return err
			}
			d, err := c.GetDoc("Pessoa", "Segredo Alfa")
			if err != nil {
				return err
			}
			d["email"] = "s@alfa.test"
			if _, err := c.Save(d, SaveOpts{}); err != nil {
				return err
			}
			versions, err := c.SQL(`select id from tab_version where doc_id = $1`, []any{"Segredo Alfa"})
			if err != nil || len(versions) == 0 {
				t.Fatalf("alfa has no version of its own change: %v %v", versions, err)
			}
			return nil
		})
		runAs(t, e, userB, func(c *Ctx) error {
			if _, err := c.GetDoc("Pessoa", "Segredo Alfa"); err == nil {
				t.Fatal("getDoc")
			}
			if ok, err := c.Exists("Pessoa", "Segredo Alfa"); err != nil || ok {
				t.Fatalf("exists: %v %v", ok, err)
			}
			if v, err := c.GetValue("Pessoa", "Segredo Alfa", "cpf"); err != nil || v != nil {
				t.Fatalf("getValue: %v %v", v, err)
			}
			if n, err := c.Count("Pessoa", map[string]any{"cpf": "222"}); err != nil || n != 0 {
				t.Fatalf("count: %v %v", n, err)
			}
			// Pessoa has no title field, so LinkTitles echoes the ids it was
			// given without reading anything; it says nothing about alfa
			hits, err := c.LinkSearch("Pessoa", "Segredo", nil, 10)
			if err != nil || len(hits) != 0 {
				t.Fatalf("link search: %v %v", hits, err)
			}
			found, err := c.GlobalSearch("Segredo", 10)
			if err != nil || len(found) != 0 {
				t.Fatalf("global search: %v %v", found, err)
			}
			rows, err := c.SQL(`select id from tab_pessoa where id = $1`, []any{"Segredo Alfa"})
			if err != nil || len(rows) != 0 {
				t.Fatalf("raw sql: %v %v", rows, err)
			}
			rows, err = c.SQL(`select id from tab_item_pedido`, nil)
			if err != nil || len(rows) != 0 {
				t.Fatalf("raw sql on child rows: %v %v", rows, err)
			}
			versions, err := c.SQL(`select id from tab_version where doc_id = $1`, []any{"Segredo Alfa"})
			if err != nil || len(versions) != 0 {
				t.Fatalf("versions: %v %v", versions, err)
			}
			return nil
		})
	})

	t.Run("write paths", func(t *testing.T) {
		runAs(t, e, userA, func(c *Ctx) error {
			return insertDoc(c, "Pessoa", Doc{"nome": "Alvo", "cpf": "333", "email": "a@alfa.test"})
		})
		runAs(t, e, userB, func(c *Ctx) error {
			mustErr(t, "link to another tenant's document",
				insertDoc(c, "Pedido", Doc{"cliente": "Alvo"}))
			_, err := c.DBSet("Pessoa", "Alvo", Doc{"email": "x@beta.test"}, true)
			mustErr(t, "dbSet", err)
			mustErr(t, "setValue", c.SetValue("Pessoa", "Alvo", Doc{"email": "x@beta.test"}))
			mustErr(t, "delete", c.Delete("Pessoa", "Alvo", true, true))
			_, err = c.Rename("Pessoa", "Alvo", "Roubado")
			mustErr(t, "rename", err)
			return nil
		})
		// a statement that tries is confined by the policy, not by the engine
		err := e.Run(context.Background(), userB, func(c *Ctx) error {
			if _, err := c.Tx.Exec(c.Ctx, `UPDATE tab_pessoa SET email = 'x' WHERE id = 'Alvo'`); err != nil {
				return err
			}
			_, err := c.Tx.Exec(c.Ctx, `INSERT INTO tab_pessoa (tenant, id) VALUES ('alfa', 'Plantado')`)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "row-level security") {
			t.Fatalf("planting a row in another tenant: %v", err)
		}
		runAs(t, e, userA, func(c *Ctx) error {
			d, err := c.GetDoc("Pessoa", "Alvo")
			if err != nil {
				return err
			}
			if d.Str("email") != "a@alfa.test" {
				t.Fatalf("alfa's document was changed: %v", d.Str("email"))
			}
			return nil
		})
	})

	t.Run("a Single is one per tenant", func(t *testing.T) {
		for tenant, lema := range map[string]string{tenantA: "de alfa", tenantB: "de beta"} {
			inTenant(t, e, tenant, func(c *Ctx) error {
				d, err := c.GetDoc("Ajustes", "")
				if err != nil {
					return err
				}
				d["lema"] = lema
				_, err = c.Save(d, SaveOpts{})
				return err
			})
		}
		runAs(t, e, userA, func(c *Ctx) error {
			if v, err := c.GetSingleValue("Ajustes", "lema"); err != nil || v != "de alfa" {
				t.Fatalf("alfa reads %v %v", v, err)
			}
			return nil
		})
		runAs(t, e, "Admin", func(c *Ctx) error {
			if v, err := c.GetSingleValue("Ajustes", "lema"); err != nil || (v != nil && v != "") {
				t.Fatalf("the platform reads %v %v", v, err)
			}
			return nil
		})
	})

	t.Run("a shared DocType is read by all and written by the platform", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error { return insertDoc(c, "Pais", Doc{"sigla": "BR"}) })
		runAs(t, e, userA, func(c *Ctx) error {
			if got := listNames(t, c, "Pais", ListArgs{}); strings.Join(got, ",") != "BR" {
				t.Fatalf("alfa lists %v", got)
			}
			wantStatus(t, insertDoc(c, "Pais", Doc{"sigla": "AR"}), 403)
			_, err := c.DBSet("Pais", "BR", Doc{"sigla": "XX"}, true)
			wantStatus(t, err, 403)
			wantStatus(t, c.Delete("Pais", "BR", true, true), 403)
			return nil
		})
	})

	t.Run("tenants are the platform's", func(t *testing.T) {
		inTenant(t, e, tenantA, func(c *Ctx) error {
			_, err := c.GetList("Tenant", ListArgs{})
			wantStatus(t, err, 403)
			_, err = c.GetDoc("Tenant", tenantB)
			wantStatus(t, err, 403)
			wantStatus(t, insertDoc(c, "Tenant", Doc{"slug": "gama", "title": "Gama"}), 403)
			return nil
		})
		runAs(t, e, "Admin", func(c *Ctx) error {
			if got := listNames(t, c, "Tenant", ListArgs{}); strings.Join(got, ",") != "alfa,beta" {
				t.Fatalf("the platform lists %v", got)
			}
			return nil
		})
	})

	t.Run("acting as a user moves to that user's tenant and back", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error {
			if err := c.withUser(userA, func(u *Ctx) error {
				if u.Tenant != tenantA {
					t.Fatalf("user ctx tenant %q", u.Tenant)
				}
				if _, err := u.GetDoc("Pessoa", "Alfa Um"); err != nil {
					t.Fatalf("alfa's user cannot read alfa's document: %v", err)
				}
				return nil
			}); err != nil {
				return err
			}
			if _, err := c.GetDoc("Pessoa", "Alfa Um"); err == nil {
				t.Fatal("back in the platform space, alfa's document is still readable")
			}
			return nil
		})
		runAs(t, e, userA, func(c *Ctx) error {
			wantStatus(t, c.withUser(userB, func(u *Ctx) error { return nil }), 403)
			return nil
		})
	})

	t.Run("entering a tenant from the platform", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error {
			if err := c.InTenant(tenantB, func(b *Ctx) error {
				return insertDoc(b, "Pessoa", Doc{"nome": "Via Plataforma", "cpf": "999"})
			}); err != nil {
				return err
			}
			if ok, _ := c.Exists("Pessoa", "Via Plataforma"); ok {
				t.Fatal("the row landed in the platform space")
			}
			return nil
		})
		runAs(t, e, userB, func(c *Ctx) error {
			if ok, _ := c.Exists("Pessoa", "Via Plataforma"); !ok {
				t.Fatal("beta does not have the row written for it")
			}
			wantStatus(t, c.InTenant(tenantA, func(*Ctx) error { return nil }), 403)
			return nil
		})
	})

	t.Run("a savepoint rolled back leaves the ctx in its tenant", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error {
			if err := c.Begin(); err != nil {
				return err
			}
			_ = c.InTenant(tenantA, func(a *Ctx) error { return insertDoc(a, "Pessoa", Doc{"nome": "Temp", "cpf": "777"}) })
			if err := c.RollbackTo(); err != nil {
				return err
			}
			if got := listNames(t, c, "Pessoa", ListArgs{}); strings.Join(got, ",") != "Comum" {
				t.Fatalf("after the rollback the platform lists %v", got)
			}
			return nil
		})
	})
}

func TestTenantDisabledAdmitsNobody(t *testing.T) {
	e := setupTenancy(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.DBSet("Tenant", tenantB, Doc{"enabled": false}, true)
		return err
	})
	e.Cache.Clear()
	wantStatus(t, e.Run(context.Background(), userB, func(*Ctx) error { return nil }), 403)
	wantStatus(t, e.Run(WithTenant(context.Background(), tenantB), "Admin", func(*Ctx) error { return nil }), 403)
	runAs(t, e, userA, func(*Ctx) error { return nil })
}
