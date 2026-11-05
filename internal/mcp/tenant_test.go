package mcp

// MCP inside a tenant (#143): every data tool takes `tenant`, the server takes
// a default one (`ddcore --tenant <slug> mcp`), and an answer from the
// platform space about a tenant-owned table says that its rows are elsewhere
// rather than letting "0 rows" read as "no data".

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/testdb"
)

// tenancyEngine: a site with tenancy, tenants alfa and beta, an ordinary
// DocType (Pedido: owned by the tenants, reachable from the platform) and a
// tenant-only one (Folha). alfa has two Pedidos, beta one, the platform none.
func tenancyEngine(t *testing.T, enter string) *engine.Engine {
	t.Helper()
	dir := t.TempDir()
	for rel, src := range map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "loja", title: "Loja" });`,
		"doctypes/pedido/pedido.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pedido", idGeneration: { field: "codigo" },
  fields: [{ fieldname: "codigo", fieldtype: "Data", label: "Codigo", reqd: true }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/folha/folha.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Folha", space: "tenant", idGeneration: { field: "mes" },
  fields: [{ fieldname: "mes", fieldtype: "Data", label: "Mes", reqd: true }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }] });`,
	} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	ctx := context.Background()
	dsn := testdb.BaseDSN()
	if err := testdb.Empty(ctx, dsn); err != nil {
		t.Skip(err)
	}
	cfg := engine.Config{DSN: dsn, Test: true, Tenancy: true, Apps: []js.App{{Name: "loja", Dir: dir}}, LogLevel: slog.LevelError}
	e, err := engine.New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	insert := func(c *engine.Ctx, doctype string, v engine.Doc) error {
		d, err := c.NewDoc(doctype, v)
		if err != nil {
			return err
		}
		_, err = c.Insert(d, engine.SaveOpts{})
		return err
	}
	if err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		for _, id := range []string{"alfa", "beta"} {
			if err := insert(c, "Site Tenant", engine.Doc{"slug": id, "title": strings.ToUpper(id)}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for tenant, codes := range map[string][]string{"alfa": {"A-1", "A-2"}, "beta": {"B-1"}} {
		if err := e.Run(engine.WithTenant(ctx, tenant), "Admin", func(c *engine.Ctx) error {
			for _, code := range codes {
				if err := insert(c, "Pedido", engine.Doc{"codigo": code}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// the server's default tenant, as `ddcore --tenant <slug> mcp` sets it
	e.Cfg.EnterTenant = enter
	return e
}

func session(t *testing.T, e *engine.Engine) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New(e).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and returns its JSON block, the note block (if any) and
// whether it failed.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (body, note string, isErr bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if i == 0 {
			body = tc.Text
		} else {
			note += tc.Text
		}
	}
	return body, note, res.IsError
}

func count(t *testing.T, body string) float64 {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("not one row: %s (%v)", body, err)
	}
	n, _ := rows[0]["n"].(float64)
	return n
}

func TestSQLQueryWorksInATenantAndSaysWhenItDidNot(t *testing.T) {
	cs := session(t, tenancyEngine(t, ""))
	q := map[string]any{"query": `SELECT count(*)::int AS n FROM "tab_pedido"`}

	// the issue's case: the platform space holds none of the rows, and says so
	body, note, isErr := call(t, cs, "sql_query", q)
	if isErr || count(t, body) != 0 {
		t.Fatalf("platform: %s", body)
	}
	if !strings.Contains(note, "platform space") || !strings.Contains(note, "Pedido") || !strings.Contains(note, "alfa, beta") {
		t.Errorf("the platform answer must say where the rows are: %q", note)
	}

	q["tenant"] = "alfa"
	body, note, isErr = call(t, cs, "sql_query", q)
	if isErr || count(t, body) != 2 || note != "" {
		t.Fatalf("alfa: %s %q", body, note)
	}

	// a query on a shared table has nothing to warn about
	if _, note, _ := call(t, cs, "sql_query", map[string]any{"query": "SELECT count(*)::int AS n FROM tab_site_tenant"}); note != "" {
		t.Errorf("a shared table needs no note: %q", note)
	}
}

func TestDocumentToolsWorkInATenant(t *testing.T) {
	cs := session(t, tenancyEngine(t, ""))

	body, note, isErr := call(t, cs, "list_docs", map[string]any{"doctype": "Pedido", "fields": []string{"codigo"}})
	if isErr || body != "[]" && body != "null" || !strings.Contains(note, "belong to tenants") {
		t.Fatalf("platform list: %s %q", body, note)
	}
	body, note, _ = call(t, cs, "list_docs", map[string]any{"doctype": "Pedido", "fields": []string{"codigo"}, "tenant": "beta"})
	if !strings.Contains(body, "B-1") || strings.Contains(body, "A-1") || note != "" {
		t.Fatalf("beta list: %s %q", body, note)
	}

	// a document of a tenant, looked for in the platform space
	body, _, isErr = call(t, cs, "get_doc", map[string]any{"doctype": "Pedido", "id": "A-1"})
	if !isErr || !strings.Contains(body, "Note:") || !strings.Contains(body, "pass `tenant`") {
		t.Fatalf("get_doc in the platform: %s", body)
	}
	if body, _, isErr = call(t, cs, "get_doc", map[string]any{"doctype": "Pedido", "id": "A-1", "tenant": "alfa"}); isErr || !strings.Contains(body, `"A-1"`) {
		t.Fatalf("get_doc in alfa: %s", body)
	}

	// writes land in the tenant named
	if body, _, isErr = call(t, cs, "insert_doc", map[string]any{"doctype": "Folha", "values": map[string]any{"mes": "2026-11"}, "tenant": "beta"}); isErr {
		t.Fatalf("insert_doc in beta: %s", body)
	}
	if body, _, _ = call(t, cs, "sql_query", map[string]any{"query": "SELECT count(*)::int AS n FROM tab_folha", "tenant": "beta"}); count(t, body) != 1 {
		t.Fatalf("the Folha is not in beta: %s", body)
	}
	if body, _, _ = call(t, cs, "sql_query", map[string]any{"query": "SELECT count(*)::int AS n FROM tab_folha", "tenant": "alfa"}); count(t, body) != 0 {
		t.Fatalf("the Folha leaked into alfa: %s", body)
	}

	// a tenant-only DocType is still refused in the platform space, with the way in
	body, _, isErr = call(t, cs, "list_docs", map[string]any{"doctype": "Folha"})
	if !isErr || !strings.Contains(body, "lives inside a tenant") || !strings.Contains(body, "pass `tenant`") {
		t.Fatalf("Folha in the platform: %s", body)
	}

	// eval enters the tenant too
	body, _, isErr = call(t, cs, "eval", map[string]any{"code": `ddcore.db.count("Pedido")`, "tenant": "alfa"})
	if isErr || !strings.Contains(body, `"result": 2`) {
		t.Fatalf("eval in alfa: %s", body)
	}

	// the job tools read the tenant's queue
	if body, _, isErr = call(t, cs, "list_jobs", map[string]any{"tenant": "alfa"}); isErr {
		t.Fatalf("list_jobs in alfa: %s", body)
	}

	// a tenant that does not exist is an error, not the platform space
	if body, _, isErr = call(t, cs, "list_docs", map[string]any{"doctype": "Pedido", "tenant": "gama"}); !isErr || !strings.Contains(body, "gama") {
		t.Fatalf("an unknown tenant: %s", body)
	}
}

func TestTheServerDefaultTenantAndTheWayBack(t *testing.T) {
	cs := session(t, tenancyEngine(t, "alfa"))
	q := map[string]any{"query": "SELECT count(*)::int AS n FROM tab_pedido"}

	body, note, _ := call(t, cs, "sql_query", q)
	if count(t, body) != 2 || note != "" {
		t.Fatalf("the server's tenant: %s %q", body, note)
	}
	q["tenant"] = "beta"
	if body, _, _ = call(t, cs, "sql_query", q); count(t, body) != 1 {
		t.Fatalf("a call's tenant overrides the server's: %s", body)
	}
	q["tenant"] = ""
	if body, note, _ = call(t, cs, "sql_query", q); count(t, body) != 0 || note == "" {
		t.Fatalf("tenant \"\" is the platform space: %s %q", body, note)
	}
}
