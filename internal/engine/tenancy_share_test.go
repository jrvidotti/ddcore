package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

func cidadeDoctype(shared bool) string {
	flag := ""
	if shared {
		flag = "shared: true, "
	}
	return `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cidade", ` + flag + `idGeneration: { field: "codigo" },
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Código", reqd: true, unique: true },
    { fieldname: "bairros", fieldtype: "Table", label: "Bairros", options: "Bairro" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`
}

var cidadeFiles = map[string]string{
	"doctypes/cidade/cidade.doctype.ts": cidadeDoctype(false),
	"doctypes/bairro/bairro.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Bairro", isChild: true, fields: [
  { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true } ] });`,
}

// tenantShape is what tenancy gave a table: row-level security, the policy,
// the column, and the key.
type tenantShape struct {
	secured, policy, column bool
	key                     string
}

func shapeOf(t *testing.T, e *Engine, table string) tenantShape {
	t.Helper()
	var s tenantShape
	if err := e.DB.Sys.QueryRow(context.Background(), `SELECT c.relrowsecurity,
		EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid = c.oid AND p.polname = $2),
		EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'tenant'),
		(SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = c.oid AND contype = 'p')
		FROM pg_class c WHERE c.oid = to_regclass($1)`, table, db.TenantPolicy).Scan(&s.secured, &s.policy, &s.column, &s.key); err != nil {
		t.Fatal(err)
	}
	return s
}

func reloadCidade(t *testing.T, e *Engine, shared bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.App("demo").Dir, "doctypes/cidade/cidade.doctype.ts"), []byte(cidadeDoctype(shared)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
}

// A DocType made shared after tenancy was applied loses what tenancy gave its
// table, so a tenant reads its documents; made tenant-owned again, it gets it
// all back (#109).
func TestMakingADocTypeSharedUndoesItsTenancy(t *testing.T) {
	ctx := context.Background()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, cidadeFiles)}}, Test: true, Tenancy: true})
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "Site Tenant", Doc{"slug": tenantA, "title": "Alfa"}); err != nil {
			return err
		}
		return insertDoc(c, "Cidade", Doc{"codigo": "5103403", "bairros": []any{map[string]any{"nome": "Centro"}}})
	})
	owned := tenantShape{secured: true, policy: true, column: true, key: "PRIMARY KEY (tenant, id)"}
	for _, table := range []string{"tab_cidade", "tab_bairro"} {
		if got := shapeOf(t, e, table); got != owned {
			t.Fatalf("%s before: %+v", table, got)
		}
	}

	reloadCidade(t, e, true)
	if _, err := e.Plan(ctx, true); err != nil {
		t.Fatalf("the plan under prune: %v", err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	shared := tenantShape{key: "PRIMARY KEY (id)"}
	for _, table := range []string{"tab_cidade", "tab_bairro"} {
		if got := shapeOf(t, e, table); got != shared {
			t.Fatalf("%s after: %+v", table, got)
		}
	}
	if plan, err := e.Plan(ctx, true); err != nil || len(plan) != 0 {
		t.Fatalf("not idempotent: %v %v", db.SQL(plan), err)
	}
	inTenant(t, e, tenantA, func(c *Ctx) error {
		doc, err := c.GetDoc("Cidade", "5103403")
		if err != nil {
			return err
		}
		if rows, _ := doc["bairros"].([]any); len(rows) != 1 {
			t.Fatalf("the tenant reads %v", doc)
		}
		return nil
	})

	reloadCidade(t, e, false)
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"tab_cidade", "tab_bairro"} {
		if got := shapeOf(t, e, table); got != owned {
			t.Fatalf("%s owned again: %+v", table, got)
		}
	}
	if plan, err := e.Plan(ctx, true); err != nil || len(plan) != 0 {
		t.Fatalf("not idempotent: %v %v", db.SQL(plan), err)
	}
}

// Rows of a tenant would collide with the platform's on the key, or lose
// their tenant with the column: the migration names them and does nothing.
func TestMakingADocTypeSharedIsRefusedWhileATenantHoldsRows(t *testing.T) {
	ctx := context.Background()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, cidadeFiles)}}, Test: true, Tenancy: true})
	runAs(t, e, "Admin", func(c *Ctx) error {
		return insertDoc(c, "Site Tenant", Doc{"slug": tenantA, "title": "Alfa"})
	})
	inTenant(t, e, tenantA, func(c *Ctx) error {
		return insertDoc(c, "Cidade", Doc{"codigo": "5103403"})
	})

	reloadCidade(t, e, true)
	var refused *db.RefusedError
	if _, err := e.Plan(ctx, false); !errors.As(err, &refused) || !strings.Contains(err.Error(), "alfa: 1") {
		t.Fatalf("the plan: %v", err)
	}
	if _, err := e.Migrate(ctx, false); !errors.As(err, &refused) {
		t.Fatalf("the migration: %v", err)
	}
	if got := shapeOf(t, e, "tab_cidade"); !got.secured || !got.policy || !got.column {
		t.Fatalf("a refused migration changed the table: %+v", got)
	}
}
