package engine

import (
	"context"
	"maps"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// sharedVaultFiles adds to the tenancy suite the two shapes a shared DocType
// keeps a secret in: a Single the whole site has one of, and a document that
// requires its secret and can be renamed.
func sharedVaultFiles() map[string]string {
	files := maps.Clone(tenancyFiles)
	files["doctypes/gateway/gateway.doctype.ts"] = `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Gateway", isSingle: true, shared: true,
  fields: [{ fieldname: "client_id", fieldtype: "Data", label: "Client ID" },
    { fieldname: "pfx", fieldtype: "Vault", label: "Certificate" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`
	files["doctypes/banco/banco.doctype.ts"] = `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Banco", shared: true, idGeneration: { field: "codigo" }, allowRename: true,
  fields: [{ fieldname: "codigo", fieldtype: "Data", label: "Codigo", reqd: true },
    { fieldname: "titulo", fieldtype: "Data", label: "Titulo" },
    { fieldname: "senha", fieldtype: "Vault", label: "Senha", reqd: true }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`
	return files
}

const (
	gatewayKey = "Gateway:pfx:singleton"
	bancoKey   = "Banco:senha:001"
)

func setupSharedVault(t *testing.T) *Engine {
	t.Helper()
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, sharedVaultFiles())}}, Test: true, Tenancy: true})
	seedTenants(t, e)
	runAs(t, e, "Admin", func(c *Ctx) error {
		d, err := c.GetDoc("Gateway", "")
		if err != nil {
			return err
		}
		d["client_id"], d["pfx"] = "abc", "the-certificate"
		if _, err := c.Save(d, SaveOpts{}); err != nil {
			return err
		}
		return insertDoc(c, "Banco", Doc{"codigo": "001", "senha": "the-password"})
	})
	return e
}

// evalIn runs server TypeScript in the ctx's space and gives back its value.
func evalIn(t *testing.T, c *Ctx, code string) (string, error) {
	t.Helper()
	rt, err := c.RT()
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Eval(code)
	return string(out), err
}

// The secret of a shared DocType's Vault field lives in the platform space,
// with its document, and every space finds it there.
func TestTenantSharedVaultFieldIsReadEverywhere(t *testing.T) {
	e := setupSharedVault(t)

	t.Run("the field reads as configured in a tenant", func(t *testing.T) {
		for _, user := range []string{"Admin", userA, userB} {
			runAs(t, e, user, func(c *Ctx) error {
				d, err := c.GetDoc("Gateway", "")
				if err != nil {
					return err
				}
				if d.Str("client_id") != "abc" {
					t.Fatalf("%s reads client_id %q: not the one document of the site", user, d.Str("client_id"))
				}
				got, _ := c.RedactDoc("Gateway", d)["pfx"].(map[string]any)
				if got["configured"] != true {
					t.Fatalf("%s sees pfx as %v", user, c.RedactDoc("Gateway", d)["pfx"])
				}
				return nil
			})
		}
	})

	t.Run("a tenant reads the secret with shared, and only with it", func(t *testing.T) {
		inTenant(t, e, tenantA, func(c *Ctx) error {
			if out, err := evalIn(t, c, `ddcore.vault.get("`+gatewayKey+`")`); err != nil || out != "null" {
				t.Fatalf("the tenant's own vault answers %s %v", out, err)
			}
			if out, err := evalIn(t, c, `ddcore.vault.get("`+gatewayKey+`", { shared: true })`); err != nil || out != `"the-certificate"` {
				t.Fatalf("shared read gives %s %v", out, err)
			}
			if out, err := evalIn(t, c, `ddcore.vault.get("nothing:here", { shared: true })`); err != nil || out != "null" {
				t.Fatalf("a secret that does not exist reads %s %v", out, err)
			}
			return nil
		})
		// the platform space reads its own secret either way
		runAs(t, e, "Admin", func(c *Ctx) error {
			for _, code := range []string{`ddcore.vault.get("` + gatewayKey + `")`, `ddcore.vault.get("` + gatewayKey + `", { shared: true })`} {
				if out, err := evalIn(t, c, code); err != nil || out != `"the-certificate"` {
					t.Fatalf("%s gives %s %v", code, out, err)
				}
			}
			return nil
		})
	})

	t.Run("a tenant's secret of the same name is another secret", func(t *testing.T) {
		inTenant(t, e, tenantB, func(c *Ctx) error {
			if _, err := evalIn(t, c, `ddcore.vault.set("`+gatewayKey+`", "of beta")`); err != nil {
				return err
			}
			if out, err := evalIn(t, c, `ddcore.vault.get("`+gatewayKey+`")`); err != nil || out != `"of beta"` {
				t.Fatalf("beta's own reads %s %v", out, err)
			}
			if out, err := evalIn(t, c, `ddcore.vault.get("`+gatewayKey+`", { shared: true })`); err != nil || out != `"the-certificate"` {
				t.Fatalf("shared read in beta gives %s %v", out, err)
			}
			return nil
		})
		inTenant(t, e, tenantA, func(c *Ctx) error {
			if out, err := evalIn(t, c, `ddcore.vault.get("`+gatewayKey+`")`); err != nil || out != "null" {
				t.Fatalf("alfa reads beta's secret: %s %v", out, err)
			}
			return nil
		})
	})

	t.Run("only the platform writes a shared secret", func(t *testing.T) {
		inTenant(t, e, tenantA, func(c *Ctx) error {
			_, err := evalIn(t, c, `ddcore.vault.set("`+gatewayKey+`", "stolen", { shared: true })`)
			wantStatus(t, err, 403)
			_, err = evalIn(t, c, `ddcore.vault.del("`+gatewayKey+`", { shared: true })`)
			wantStatus(t, err, 403)
			d, err := c.GetDoc("Gateway", "")
			if err != nil {
				return err
			}
			d["pfx"] = "stolen"
			_, err = c.Save(d, SaveOpts{})
			wantStatus(t, err, 403)
			return nil
		})
		runAs(t, e, "Admin", func(c *Ctx) error {
			if _, err := evalIn(t, c, `ddcore.vault.set("plataforma:token", "v1", { shared: true })`); err != nil {
				return err
			}
			if v, ok, err := e.VaultGet(c, gatewayKey); err != nil || !ok || v != "the-certificate" {
				t.Fatalf("the secret after the refused writes: %q %v %v", v, ok, err)
			}
			return nil
		})
		inTenant(t, e, tenantA, func(c *Ctx) error {
			if out, err := evalIn(t, c, `ddcore.vault.get("plataforma:token", { shared: true })`); err != nil || out != `"v1"` {
				t.Fatalf("a secret the platform set as shared reads %s %v", out, err)
			}
			return nil
		})
	})

	t.Run("the read is audited in the space it came from", func(t *testing.T) {
		const q = `select detail::jsonb->>'shared' as shared from tab_audit_event
			where action = 'vault.read' and target_id = $1 and detail::jsonb->>'shared' = 'true'`
		inTenant(t, e, tenantA, func(c *Ctx) error {
			rows, err := c.SQL(q, []any{gatewayKey})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				t.Fatal("alfa's audit log has no shared read")
			}
			return nil
		})
		// the platform's two reads above said shared once; a tenant's did not land here
		runAs(t, e, "Admin", func(c *Ctx) error {
			rows, err := c.SQL(q, []any{gatewayKey})
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("the platform's audit log has %d shared reads, want its own one", len(rows))
			}
			return nil
		})
	})

	t.Run("a required secret counts as present wherever it is checked", func(t *testing.T) {
		runAs(t, e, "Admin", func(c *Ctx) error {
			d, err := c.GetDoc("Banco", "001")
			if err != nil {
				return err
			}
			d["titulo"] = "Banco Um"
			_, err = c.Save(d, SaveOpts{})
			return err
		})
		runAs(t, e, userA, func(c *Ctx) error {
			d, err := c.GetDoc("Banco", "001")
			if err != nil {
				return err
			}
			got, _ := c.RedactDoc("Banco", d)["senha"].(map[string]any)
			if got["configured"] != true {
				t.Fatalf("alfa sees senha as %v", c.RedactDoc("Banco", d)["senha"])
			}
			return nil
		})
	})

	t.Run("renaming the document leaves a tenant's secret of that name alone", func(t *testing.T) {
		inTenant(t, e, tenantA, func(c *Ctx) error { return e.VaultSet(c, bancoKey, "of alfa") })
		runAs(t, e, "Admin", func(c *Ctx) error {
			_, err := c.Rename("Banco", "001", "002")
			return err
		})
		inTenant(t, e, tenantA, func(c *Ctx) error {
			if v, ok, err := e.VaultGet(c, bancoKey); err != nil || !ok || v != "of alfa" {
				t.Fatalf("alfa's own secret after the rename: %q %v %v", v, ok, err)
			}
			if v, ok, err := e.VaultGetShared(c, "Banco:senha:002"); err != nil || !ok || v != "the-password" {
				t.Fatalf("the renamed document's secret: %q %v %v", v, ok, err)
			}
			if _, ok, _ := e.VaultGetShared(c, bancoKey); ok {
				t.Fatal("the secret is still under the old name")
			}
			return nil
		})
	})
}

// Adopting the platform's rows into a first tenant leaves the shared
// documents where they are, and their secrets with them.
func TestTenantAdoptKeepsSharedVaultSecrets(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, sharedVaultFiles())}}, Test: true, Tenancy: true})
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "Banco", Doc{"codigo": "001", "senha": "the-password"}); err != nil {
			return err
		}
		if err := e.VaultSet(c, "cliente:token:1", "of the customer"); err != nil {
			return err
		}
		return insertDoc(c, "Site Tenant", Doc{"slug": tenantA, "title": "Alfa"})
	})
	moved, err := e.AdoptPlatformRows(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if moved["ddcore_vault"] != 1 {
		t.Fatalf("moved %d vault secrets, want the customer's one (%v)", moved["ddcore_vault"], moved)
	}
	var tenant string
	err = e.System(context.Background(), func(q db.Querier) error {
		return q.QueryRow(context.Background(), `SELECT tenant FROM ddcore_vault WHERE name = $1`, bancoKey).Scan(&tenant)
	})
	if err != nil || tenant != "" {
		t.Fatalf("the shared document's secret is in %q (%v)", tenant, err)
	}
	inTenant(t, e, tenantA, func(c *Ctx) error {
		if v, ok, err := e.VaultGet(c, "cliente:token:1"); err != nil || !ok || v != "of the customer" {
			t.Fatalf("the adopted secret: %q %v %v", v, ok, err)
		}
		if v, ok, err := e.VaultGetShared(c, bancoKey); err != nil || !ok || v != "the-password" {
			t.Fatalf("the shared secret from the tenant: %q %v %v", v, ok, err)
		}
		return nil
	})
}
