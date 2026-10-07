package engine

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

const (
	operator = "olga@plataforma.test"
	bystand  = "joao@plataforma.test"
)

// setupOperator is the tenancy suite with a DocType that records a User, an
// operator (a System Manager of the platform space) and a platform user who
// is not one.
func setupOperator(t *testing.T) *Engine {
	t.Helper()
	files := map[string]string{
		"doctypes/ocorrencia/ocorrencia.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ocorrencia",
  fields: [{ fieldname: "por", fieldtype: "Link", label: "By", options: "User" }, { fieldname: "nota", fieldtype: "Data", label: "Nota" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }, { role: "System Manager", read: true, write: true, create: true }] });`,
	}
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, tenancyFiles, files)}}, Test: true, Tenancy: true})
	seedTenants(t, e)
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "User", Doc{"email": operator, "full_name": "Olga Operadora",
			"roles": []any{map[string]any{"role": "System Manager"}}}); err != nil {
			return err
		}
		return insertDoc(c, "User", Doc{"email": bystand, "full_name": "João",
			"roles": []any{map[string]any{"role": "Gestor"}}})
	})
	return e
}

// An operator who entered a tenant is no User of it, and still is who did
// what there: a Link to User records it, and shows it by name (#110).
func TestALinkToUserRecordsTheOperatorInsideATenant(t *testing.T) {
	e := setupOperator(t)
	ctx := WithTenant(context.Background(), tenantA)
	var id string
	if err := e.Run(ctx, operator, func(c *Ctx) error {
		doc, err := c.Insert(Doc{"doctype": "Ocorrencia", "por": c.User}, SaveOpts{})
		if err != nil {
			return err
		}
		id = doc.ID()
		titles, err := c.LinkTitles("User", []string{operator, "Admin"})
		if err != nil {
			return err
		}
		if titles[operator] != "Olga Operadora" {
			t.Errorf("LinkTitles: %v", titles)
		}
		if got := c.ResolveLinkTitles("Ocorrencia", doc)["User"][operator]; got != "Olga Operadora" {
			t.Errorf("ResolveLinkTitles: %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// the tenant's own user saves it later: the operator it names still passes
	if err := e.Run(ctx, userA, func(c *Ctx) error {
		doc, err := c.GetDoc("Ocorrencia", id)
		if err != nil {
			return err
		}
		doc["nota"] = "revista"
		_, err = c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// a platform user who cannot enter tenants is no operator
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Insert(Doc{"doctype": "Ocorrencia", "por": bystand}, SaveOpts{})
		return err
	})
	var ce *cerr.Error
	if !errors.As(err, &ce) || ce.Type != "LinkExistsError" {
		t.Fatalf("a link to a platform user who is no operator: %v", err)
	}
}

var errRollback = errors.New("rollback")

// A user the transaction created is nowhere else yet: its roles are read
// from the transaction, inside the tenant and acting as it (#110).
func TestRolesOfAUserCreatedInTheTransactionInsideATenant(t *testing.T) {
	e := setupTenancy(t)
	const probe = "sonda@alfa.test"
	err := e.Run(WithTenant(context.Background(), tenantA), "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "User", Doc{"email": probe, "full_name": "Sonda",
			"roles": []any{map[string]any{"role": "Gestor"}}}); err != nil {
			return err
		}
		if roles, err := c.RolesOf(probe); err != nil || !slices.Contains(roles, "Gestor") {
			t.Errorf("RolesOf: %v %v", roles, err)
		}
		if err := c.withUser(probe, func(u *Ctx) error {
			roles, err := u.RolesOf(probe)
			if err != nil || !slices.Contains(roles, "Gestor") {
				t.Errorf("RolesOf as the user: %v %v", roles, err)
			}
			if away, err := u.awayFromHome(); err != nil || away {
				t.Errorf("the user is away from home: %v %v", away, err)
			}
			return err
		}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
	if v, ok := e.Cache.Get("roles:" + probe); ok && !slices.Contains(v.([]string), "Gestor") {
		t.Fatalf("the cache keeps %v", v)
	}
}
