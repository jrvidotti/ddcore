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

// TestCallMethodRefusesNotWhitelisted: call_method keeps /api/method's
// contract (#141) — a function exported without whitelisted() is refused,
// where it used to run.
func TestCallMethodRefusesNotWhitelisted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "shop", title: "Shop" });`), 0o644)
	os.MkdirAll(filepath.Join(dir, "services"), 0o755)
	os.WriteFile(filepath.Join(dir, "services/x.ts"), []byte(`import { whitelisted } from "@ddcore/sdk";
export function internal() { return "internal-ran"; }
export const open = whitelisted(() => "open-ran");`), 0o644)
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
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New(e).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(method string) (bool, string) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_method", Arguments: map[string]any{"method": method}})
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return res.IsError, b.String()
	}

	if isErr, out := call("shop.services.x.open"); isErr || !strings.Contains(out, "open-ran") {
		t.Fatalf("whitelisted function: isError=%v %s", isErr, out)
	}
	isErr, out := call("shop.services.x.internal")
	if !isErr || strings.Contains(out, "internal-ran") || !strings.Contains(out, "not whitelisted") {
		t.Fatalf("a function not whitelisted must be refused: isError=%v %s", isErr, out)
	}
}
