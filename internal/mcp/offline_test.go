package mcp

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
)

// Port 1 refuses at once, so the boot and every retry fail fast.
const unreachableDSN = "postgres://ddcore:ddcore@127.0.0.1:1/ddcore?connect_timeout=2"

func offlineSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	app := t.TempDir()
	os.WriteFile(filepath.Join(app, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "shop", title: "Shop" });`), 0o644)
	e, err := engine.New(context.Background(), engine.Config{
		DSN: unreachableDSN, DeferDB: true, Apps: []js.App{{Name: "shop", Dir: app}}, LogOut: io.Discard, LogLevel: slog.LevelError,
	})
	if err != nil {
		t.Fatalf("an unreachable database must not stop a DeferDB engine: %v", err)
	}
	if e.DB != nil {
		t.Fatal("the engine claims a database it never reached")
	}
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

func callText(t *testing.T, cs *mcp.ClientSession, name string) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestMCPServesWithoutTheDatabase(t *testing.T) {
	cs := offlineSession(t)
	if !strings.Contains(cs.InitializeResult().Instructions, "database was unreachable") {
		t.Errorf("the instructions do not warn the agent: %q", cs.InitializeResult().Instructions)
	}
	if out, isErr := callText(t, cs, "list_apps"); isErr || !strings.Contains(out, "shop") {
		t.Errorf("list_apps offline: isError=%v %s", isErr, out)
	}
	out, isErr := callText(t, cs, "list_docs")
	if !isErr || !strings.Contains(out, "list_docs needs the database") || !strings.Contains(out, "make docker-up") {
		t.Errorf("list_docs offline should refuse with a hint: isError=%v %s", isErr, out)
	}
	// The refusal leaves the server up for the next call.
	if _, isErr := callText(t, cs, "validate_meta"); isErr {
		t.Error("validate_meta failed after a refused call")
	}
}

// Every tool is either known to work offline or guarded: a new tool that
// forgets to choose is guarded, and one listed here that no longer exists is
// a stale entry.
func TestOfflineToolsExist(t *testing.T) {
	cs := offlineSession(t)
	have := map[string]bool{}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		have[tool.Name] = true
	}
	for name := range offlineTools {
		if !have[name] {
			t.Errorf("offlineTools lists %q, which is not a tool", name)
		}
	}
}

// run_tests builds a second engine from the first one's Config and
// dereferences its DB, so the deferral must not be inherited.
func TestTestConfigDoesNotDeferTheDatabase(t *testing.T) {
	e := &engine.Engine{Cfg: engine.Config{DSN: unreachableDSN, DeferDB: true}}
	cfg := testConfig(e)
	if cfg.DeferDB || !cfg.Test {
		t.Fatalf("testConfig: DeferDB=%v Test=%v", cfg.DeferDB, cfg.Test)
	}
}
