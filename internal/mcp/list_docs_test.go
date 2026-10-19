package mcp

import (
	"context"
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

func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

func TestListDocsRedactsPasswordFields(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "shop", title: "Shop" });`), 0o644)
	os.MkdirAll(filepath.Join(dir, "doctypes/credential"), 0o755)
	os.WriteFile(filepath.Join(dir, "doctypes/credential/credential.doctype.ts"), []byte(`import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Credential", idGeneration: { field: "label" },
  fields: [
    { fieldname: "label", fieldtype: "Data", label: "Label", reqd: true },
    { fieldname: "secret", fieldtype: "Password", label: "Secret" },
    { fieldname: "token", fieldtype: "Vault", label: "Token" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }] });`), 0o644)
	ctx := context.Background()
	dsn := testdb.BaseDSN()
	if err := testdb.Empty(ctx, dsn); err != nil {
		t.Skip(err)
	}
	e, err := engine.New(ctx, engine.Config{DSN: dsn, Test: true, Apps: []js.App{{Name: "shop", Dir: dir}}, LogLevel: slog.LevelError})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		d, err := c.NewDoc("Credential", engine.Doc{"label": "k1", "secret": "hunter2-plain", "token": "tok-plain"})
		if err != nil {
			return err
		}
		_, err = c.Insert(d, engine.SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New(e).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_docs", Arguments: map[string]any{"doctype": "Credential", "fields": []string{"label", "secret"}}})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	out := b.String()
	if res.IsError || !strings.Contains(out, "k1") {
		t.Fatalf("list_docs: isError=%v %s", res.IsError, out)
	}
	if strings.Contains(out, "hunter2-plain") || strings.Contains(out, "tok-plain") {
		t.Fatalf("list_docs leaked a secret: %s", out)
	}
}
