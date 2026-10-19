// Package mcp exposes the development tools to agents over MCP.
package mcp

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/typegen"
)

// Docs returns an embedded reference page (index lists them all).
func Docs(name string) string {
	if name == "index" || name == "" {
		b, _ := fs.ReadFile(docs.FS, "agent/index.md")
		return string(b)
	}
	b, err := fs.ReadFile(docs.FS, "agent/"+name+".md")
	if err != nil {
		return "document not found: " + name
	}
	return string(b)
}

func docNames() []string {
	entries, _ := fs.ReadDir(docs.FS, "agent")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(out)
	return out
}

type server struct {
	e *engine.Engine
}

func text(v any) *mcp.CallToolResult {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case json.RawMessage:
		s = string(x)
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		s = string(b)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func fail(err error) (*mcp.CallToolResult, any, error) {
	e := cerr.From(err)
	msg := e.Error()
	switch e.Type {
	case "DoesNotExistError":
		msg += "\nHint: use list_doctypes / list_docs to see what exists."
	case "PermissionError":
		if strings.Contains(e.Key, "lives inside a tenant") {
			msg += "\nHint: the MCP server works in the platform space; reach a tenant's documents with eval and ddcore.tenant.run(\"<tenant>\", () => …)."
		} else {
			msg += "\nHint: the MCP server runs as Admin; check the DocType and the field."
		}
	case "ScriptError":
		msg += "\nHint: an error in the app's TypeScript; read the stack above and fix the file."
	}
	if strings.Contains(msg, "column") && strings.Contains(msg, "does not exist") {
		msg += "\nHint: a column is pending — run the migrate tool."
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil, nil
}

// New builds the MCP server with every tool registered.
func New(e *engine.Engine) *mcp.Server {
	s := &server{e: e}
	srv := mcp.NewServer(&mcp.Implementation{Name: "ddcore", Version: engine.Version}, &mcp.ServerOptions{
		Instructions: "Development server for the ddcore framework. Start by reading the resource ddcore://docs/index. " +
			"What changed recently is in ddcore://changelog (older minor series: ddcore://changelog/<minor>); whats_new reports it against the running version. " +
			"Typical flow: get_doctype / scaffold_doctype → migrate → insert_doc / list_docs → run_tests. " +
			"Moving data between sites is export then import (plan, run, reconcile). " +
			"Every label is an English key: after adding one, i18n_extract → set_translations until nothing is missing. " +
			"The app's TS files are the source of truth: edit them and the server reloads." + offlineNote(e),
	})
	srv.AddReceivingMiddleware(requireDB(e))

	s.metaTools(srv)
	s.dataTools(srv)
	s.jobTools(srv)
	s.resources(srv)
	return srv
}

// addTool registers a tool whose handler returns a value or an error: the
// value is rendered by text, an error by fail (the value is ignored then), so
// no handler builds a CallToolResult itself.
func addTool[In any](srv *mcp.Server, name, desc string, fn func(ctx context.Context, in In) (any, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc},
		func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			out, err := fn(ctx, in)
			if err != nil {
				return fail(err)
			}
			return text(out), nil, nil
		})
}

// docOp runs fn as Admin and returns the document it produced with its
// Password fields redacted, as the document tools answer.
func (s *server) docOp(ctx context.Context, doctype string, fn func(c *engine.Ctx) (engine.Doc, error)) (any, error) {
	var doc engine.Doc
	err := s.run(ctx, func(c *engine.Ctx) error {
		var err error
		doc, err = fn(c)
		if err == nil && doc != nil {
			doc, err = c.RedactDoc(doctype, doc)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *server) run(ctx context.Context, fn func(c *engine.Ctx) error) error {
	return s.e.Run(ctx, "Admin", fn)
}

// writeTypes writes .ddcore/types.d.ts into every app on disk, the core
// (embedded) excepted, and returns the paths it wrote.
func (s *server) writeTypes() ([]string, error) {
	var dirs []string
	for _, a := range s.e.Apps {
		if a.Embedded == nil {
			dirs = append(dirs, a.Dir)
		}
	}
	return typegen.WriteAll(dirs, s.e.Meta)
}

func testConfig(e *engine.Engine) engine.Config {
	cfg := e.Cfg
	cfg.Test = true
	// run_tests dereferences the test engine's DB: an unreachable test
	// database must fail New, not hand back an engine without one.
	cfg.DeferDB = false
	return cfg
}

// ServeStdio runs the MCP server over stdin/stdout.
func ServeStdio(ctx context.Context, e *engine.Engine) error {
	return New(e).Run(ctx, &mcp.StdioTransport{})
}

// HTTPHandler mounts the MCP server (streamable HTTP) for remote agents.
func HTTPHandler(e *engine.Engine) http.Handler {
	srv := New(e)
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true})
}
