// Package mcp exposes the development tools to agents over MCP.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/meta"
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

// noted is a tool's answer with a note after it: the value stays the JSON
// the tool always returns, and the note is a second block an agent reads.
type noted struct {
	Value any
	Note  string
}

func text(v any) *mcp.CallToolResult {
	if n, ok := v.(noted); ok {
		r := text(n.Value)
		if n.Note != "" {
			r.Content = append(r.Content, &mcp.TextContent{Text: "Note: " + n.Note})
		}
		return r
	}
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
	note := ""
	var ne noteErr
	if errors.As(err, &ne) {
		err, note = ne.error, "\nNote: "+ne.note
	}
	e := cerr.From(err)
	msg := e.Error() + note
	switch e.Type {
	case "DoesNotExistError":
		msg += "\nHint: use list_doctypes / list_docs to see what exists."
	case "PermissionError":
		if strings.Contains(e.Key, "lives inside a tenant") {
			msg += "\nHint: this call ran in the platform space; pass `tenant` to the tool, or start the server inside one with `ddcore --tenant <slug> mcp`."
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
			"On a site with tenancy, the data tools take `tenant` (the tenant's slug; \"\" is the platform space) and default to the one the server was started in (`ddcore --tenant <slug> mcp`). " +
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

// docOp runs fn as Admin in the call's space and returns the document it
// produced with its Password fields redacted, as the document tools answer.
func (s *server) docOp(ctx context.Context, sp Space, doctype string, fn func(c *engine.Ctx) (engine.Doc, error)) (any, error) {
	var doc engine.Doc
	err := s.runIn(ctx, sp, func(c *engine.Ctx) error {
		var err error
		doc, err = fn(c)
		if err == nil && doc != nil {
			doc, err = c.RedactDoc(doctype, doc)
		}
		return notFoundNote(c, doctype, err)
	})
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *server) run(ctx context.Context, fn func(c *engine.Ctx) error) error {
	return s.e.Run(ctx, "Admin", fn)
}

// runIn is run in the space the call names.
func (s *server) runIn(ctx context.Context, sp Space, fn func(c *engine.Ctx) error) error {
	ctx, err := s.space(ctx, sp)
	if err != nil {
		return err
	}
	return s.run(ctx, fn)
}

// notFoundNote adds the platform note to a document that was not found: in
// the platform space, the document may well exist in a tenant.
func notFoundNote(c *engine.Ctx, doctype string, err error) error {
	if err == nil || cerr.From(err).Type != "DoesNotExistError" {
		return err
	}
	if note := platformNote(c, doctype); note != "" {
		return noteErr{err, note}
	}
	return err
}

// noteErr is an error with a platform note, which fail prints after it.
type noteErr struct {
	error
	note string
}

func (e noteErr) Unwrap() error { return e.error }

// Space is what every data tool takes besides its own arguments: the space
// the call works in.
type Space struct {
	Tenant *string `json:"tenant,omitempty" jsonschema:"on a site with tenancy: the slug of the tenant to work in; \"\" is the platform space; left out, the tenant the server was started in (ddcore --tenant <slug> mcp), or the platform space"`
}

// space puts the call's tenant in ctx. Left out, ctx is untouched and the
// server's own (Config.EnterTenant) applies; "" names the platform space,
// even on a server started in a tenant.
func (s *server) space(ctx context.Context, sp Space) (context.Context, error) {
	if sp.Tenant == nil {
		return ctx, nil
	}
	t := strings.TrimSpace(*sp.Tenant)
	if t != "" {
		if err := s.e.CheckTenant(ctx, t); err != nil {
			return nil, err
		}
	}
	return engine.WithTenant(ctx, t), nil
}

// maxNotedTenants is how many tenants a platform note names.
const maxNotedTenants = 10

// platformNote says, for a call that ran in the platform space of a site
// with tenancy, that the rows of the tenant-owned DocTypes it read are the
// tenants' and not here — the difference between "0 rows" and "no data",
// which an empty answer alone does not tell (#143). Empty when there is
// nothing to say.
func platformNote(c *engine.Ctx, doctypes ...string) string {
	if c.Tenant != "" || !c.Tenancy() {
		return ""
	}
	var owned []string
	for _, n := range doctypes {
		if d, err := c.St.DocType(n); err == nil && d.TenantOwned && !slices.Contains(owned, d.Name) {
			owned = append(owned, d.Name)
		}
	}
	if len(owned) == 0 {
		return ""
	}
	note := "this ran in the platform space, and the rows of " + strings.Join(owned, ", ") +
		" belong to tenants: pass `tenant` to work in one"
	rows, err := c.GetList(meta.TenantDocType, engine.ListArgs{
		Fields: []string{"id"}, Filters: []any{[]any{"enabled", "=", 1}}, OrderBy: "id asc", Limit: maxNotedTenants + 1,
	})
	if err == nil && len(rows) > 0 {
		var ids []string
		for i, r := range rows {
			if i == maxNotedTenants {
				ids = append(ids, "…")
				break
			}
			ids = append(ids, fmt.Sprint(r["id"]))
		}
		note += " (tenants: " + strings.Join(ids, ", ") + ")"
	}
	return note + "."
}

// tableRe finds the tables a query names: tab_<snake_case>, quoted or not.
var tableRe = regexp.MustCompile(`(?i)\btab_[a-z0-9_]+`)

// queryDoctypes are the DocTypes whose tables a SQL query names.
func queryDoctypes(c *engine.Ctx, query string) []string {
	byTable := map[string]string{}
	for _, n := range c.St.Meta.Names() {
		if d := c.St.Meta.DocTypes[n]; d != nil {
			byTable[d.TableName()] = d.Name
		}
	}
	var out []string
	for _, t := range tableRe.FindAllString(query, -1) {
		if n, ok := byTable[strings.ToLower(t)]; ok && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// writeTypes writes .ddcore/types.d.ts into every app on disk, the core
// (embedded) excepted, and returns the paths it wrote.
func (s *server) writeTypes() ([]string, error) {
	st := s.e.Current()
	var dirs []string
	for _, a := range st.Apps {
		if a.Embedded == nil {
			dirs = append(dirs, a.Dir)
		}
	}
	return typegen.WriteAll(dirs, st.Meta)
}

func testConfig(e *engine.Engine) engine.Config {
	cfg := e.Cfg
	cfg.Test = true
	// run_tests dereferences the test engine's DB: an unreachable test
	// database must fail New, not hand back an engine without one.
	cfg.DeferDB = false
	// the tests run in their own database, in the spaces they enter
	cfg.EnterTenant = ""
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
