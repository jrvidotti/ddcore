package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// offlineTools answer from the apps' files, the embedded docs and the loaded
// meta, never from Postgres, so they keep working while the database is down.
// Every other tool needs it: a new tool is refused offline until it is listed
// here, which is the safe way round for a tool that would dereference a nil DB.
var offlineTools = map[string]bool{
	"list_doctypes":    true,
	"get_doctype":      true,
	"scaffold_doctype": true,
	"extend_doctype":   true,
	"validate_meta":    true,
	"i18n_extract":     true,
	"set_translations": true,
	"generate_types":   true,
	"reload":           true,
	"list_apps":        true,
	"whats_new":        true,
}

// requireDB guards the tools that need the database on an engine built with
// DeferDB. A call that finds no database tries to connect first, so bringing
// Postgres up is enough and the MCP client never has to reconnect; one that
// still cannot reach it gets an error saying so instead of a crash.
func requireDB(e *engine.Engine) mcp.Middleware {
	var mu sync.Mutex
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if !ok || e == nil || offlineTools[call.Params.Name] {
				return next(ctx, method, req)
			}
			mu.Lock()
			err := e.Connect(ctx)
			mu.Unlock()
			if err != nil {
				msg := fmt.Sprintf("%s needs the database, which is unreachable: %v\n"+
					"Hint: start Postgres (`make docker-up` in a ddcore checkout, or check the DSN in .env); "+
					"the next call connects on its own. Tools that work meanwhile: the meta, scaffold, i18n and docs ones.",
					call.Params.Name, db.RedactError(err))
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil
			}
			return next(ctx, method, req)
		}
	}
}

// offlineNote tells the agent up front that the database was down at boot, so
// it reaches for the tools that still work instead of learning it one refusal
// at a time.
func offlineNote(e *engine.Engine) string {
	if e == nil || e.DB != nil || e.Cfg.DSN == "" {
		return ""
	}
	return " The database was unreachable when this server started: the meta, scaffold, i18n and docs tools work; " +
		"the data tools, migrate and run_tests connect on their first call once Postgres is up."
}
