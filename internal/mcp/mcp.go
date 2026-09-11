// Package mcp exposes the development tools to agents over MCP.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/scaffold"
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
		msg += "\nDica: use list_doctypes / list_docs para ver o que existe."
	case "PermissionError":
		msg += "\nDica: o MCP roda como Administrator; verifique o DocType e o campo."
	case "ScriptError":
		msg += "\nDica: erro no TS do app; veja o stack acima e corrija o arquivo."
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
			"Typical flow: get_doctype / scaffold_doctype → migrate → insert_doc / list_docs → run_tests. " +
			"The app's TS files are the source of truth: edit them and the server reloads.",
	})

	// ---- meta
	mcp.AddTool(srv, &mcp.Tool{Name: "list_doctypes", Description: "Lists all loaded DocTypes (name, app, label, isChild, isSingle, submittable, file)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			App string `json:"app,omitempty" jsonschema:"filter by app"`
		}) (*mcp.CallToolResult, any, error) {
			var out []map[string]any
			for _, n := range e.Meta.Names() {
				d := e.Meta.DocTypes[n]
				if in.App != "" && d.App != in.App {
					continue
				}
				out = append(out, map[string]any{"name": n, "app": d.App, "label": d.Label, "isChild": d.IsChild, "isSingle": d.IsSingle, "submittable": d.Submittable, "fields": len(d.Fields), "file": d.SourceFile})
			}
			return text(out), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_doctype", Description: "The full meta of a DocType (fields, permissions, naming, controller methods, source path)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Name string `json:"name" jsonschema:"DocType name"`
		}) (*mcp.CallToolResult, any, error) {
			d, err := e.DocType(in.Name)
			if err != nil {
				return fail(err)
			}
			app := e.App(d.App)
			path := d.SourceFile
			if app.Embedded == nil && path != "" {
				rel := strings.TrimPrefix(path, d.App+".")
				path = filepath.Join(app.Dir, strings.ReplaceAll(rel, ".", "/")+".ts")
				// module path uses dots for dirs AND the ".doctype" suffix
				path = strings.Replace(path, "/doctype.ts", ".doctype.ts", 1)
			}
			return text(map[string]any{"doctype": d, "file": path, "table": d.TableName()}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "scaffold_doctype", Description: "Creates the files for a new DocType in an app (doctype.ts and optionally controller/form/test). Run migrate afterwards. Fieldtypes: Data, Email, Small Text, Text, Text Editor, Int, Float, Currency, Percent, Check, Date, Datetime, Time, Select (options: list), Link (options: DocType), Dynamic Link (options: field with DocType), Table (options: child DocType), Attach, JSON, Section Break, Column Break, Tab Break, HTML."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			App  string               `json:"app" jsonschema:"app name (directory)"`
			Spec scaffold.DoctypeSpec `json:"spec" jsonschema:"DocType definition"`
		}) (*mcp.CallToolResult, any, error) {
			app := e.App(in.App)
			if app.Name == "" || app.Embedded != nil {
				return fail(cerr.NotFound("app {0} does not exist (apps: {1})", in.App, e.AppOrder()))
			}
			files, err := scaffold.Doctype(app.Dir, app.Name, in.Spec)
			if err != nil {
				return fail(err)
			}
			if err := e.Load(); err != nil {
				return fail(fmt.Errorf("files created (%v) but the meta is invalid: %w", files, err))
			}
			return text(map[string]any{"files": files, "next": "run migrate to create the table"}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "validate_meta", Description: "Recompiles apps and validates metadata (without touching the database). Use after editing .ts files."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			if err := e.Load(); err != nil {
				return fail(err)
			}
			return text(map[string]any{"ok": true, "doctypes": len(e.Meta.DocTypes), "whitelisted": e.WhitelistedPaths()}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "migrate", Description: "Applies the pending DDL, installs new apps and runs patches, in one transaction: beforeSchema patches, DDL, fixtures, afterSchema patches, then the drops. Refuses a rename or conversion it cannot make safely rather than guessing. With dry_run it only reports the plan."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			DryRun bool `json:"dry_run,omitempty"`
			Prune  bool `json:"prune,omitempty" jsonschema:"drops the columns and tables the meta no longer declares, and only the empty ones"`
		}) (*mcp.CallToolResult, any, error) {
			if err := e.Load(); err != nil {
				return fail(err)
			}
			if in.DryRun {
				plan, err := e.Plan(ctx, in.Prune)
				if err != nil {
					return fail(err)
				}
				return text(map[string]any{"report": db.Report(plan), "ddl": db.SQL(plan), "patches": e.PendingPatches()}), nil, nil
			}
			res, err := e.Migrate(ctx, in.Prune)
			if err != nil {
				return fail(err)
			}
			s.writeTypes()
			return text(map[string]any{
				"report": db.Report(res.DDL), "ddl": db.SQL(res.DDL), "patches": res.Patches,
				"installed": res.Installed, "renames": res.Renames, "recorded": res.Recorded,
			}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "generate_types", Description: "Generates .ddcore/types.d.ts (TS interfaces per DocType) in each app."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return text(s.writeTypes()), nil, nil
		})

	// ---- data
	mcp.AddTool(srv, &mcp.Tool{Name: "get_doc", Description: "Reads a document with its child tables."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string `json:"doctype"`
			Name    string `json:"name"`
		}) (*mcp.CallToolResult, any, error) {
			var doc engine.Doc
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				doc, e = c.GetDoc(in.Doctype, in.Name)
				return e
			})
			if err != nil {
				return fail(err)
			}
			return text(doc), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_docs", Description: "Lists documents. filters: [[field, op, value], ...] or {field: value}; ops: = != > >= < <= like in not in between is set. fields accepts aggregates like \"count(name) as n\"."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string   `json:"doctype"`
			Filters any      `json:"filters,omitempty"`
			Fields  []string `json:"fields,omitempty"`
			OrderBy string   `json:"order_by,omitempty"`
			Limit   int      `json:"limit,omitempty"`
			Start   int      `json:"start,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			var rows []map[string]any
			if in.Limit == 0 {
				in.Limit = 20
			}
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.GetList(in.Doctype, engine.ListArgs{Filters: in.Filters, Fields: in.Fields, OrderBy: in.OrderBy, Limit: in.Limit, Start: in.Start})
				return e
			})
			if err != nil {
				return fail(err)
			}
			return text(rows), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "insert_doc", Description: "Creates a document (running validate and the other hooks). Child tables go in their field as arrays."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string         `json:"doctype"`
			Values  map[string]any `json:"values"`
			Submit  bool           `json:"submit,omitempty" jsonschema:"insert already submitted (docstatus 1)"`
		}) (*mcp.CallToolResult, any, error) {
			var doc engine.Doc
			err := s.run(ctx, func(c *engine.Ctx) error {
				d, err := c.NewDoc(in.Doctype, in.Values)
				if err != nil {
					return err
				}
				if in.Submit {
					d["docstatus"] = 1
				}
				doc, err = c.Insert(d, engine.SaveOpts{})
				return err
			})
			if err != nil {
				return fail(err)
			}
			return text(doc), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "update_doc", Description: "Updates fields of a document and saves (runs validate)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string         `json:"doctype"`
			Name    string         `json:"name"`
			Values  map[string]any `json:"values"`
		}) (*mcp.CallToolResult, any, error) {
			var doc engine.Doc
			err := s.run(ctx, func(c *engine.Ctx) error {
				d, err := c.GetDoc(in.Doctype, in.Name)
				if err != nil {
					return err
				}
				for k, v := range in.Values {
					d[k] = v
				}
				doc, err = c.Save(d, engine.SaveOpts{})
				return err
			})
			if err != nil {
				return fail(err)
			}
			return text(doc), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "delete_doc", Description: "Deletes a document (fails if linked, unless force=true)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string `json:"doctype"`
			Name    string `json:"name"`
			Force   bool   `json:"force,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			err := s.run(ctx, func(c *engine.Ctx) error { return c.Delete(in.Doctype, in.Name, true, in.Force) })
			if err != nil {
				return fail(err)
			}
			return text("deleted"), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "submit_doc", Description: "Submits (docstatus 1) a submittable document."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string `json:"doctype"`
			Name    string `json:"name"`
		}) (*mcp.CallToolResult, any, error) {
			var doc engine.Doc
			err := s.run(ctx, func(c *engine.Ctx) error {
				d, err := c.GetDoc(in.Doctype, in.Name)
				if err != nil {
					return err
				}
				doc, err = c.Submit(d)
				return err
			})
			if err != nil {
				return fail(err)
			}
			return text(doc), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "cancel_doc", Description: "Cancels (docstatus 2) a submitted document."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string `json:"doctype"`
			Name    string `json:"name"`
		}) (*mcp.CallToolResult, any, error) {
			var doc engine.Doc
			err := s.run(ctx, func(c *engine.Ctx) error {
				d, err := c.GetDoc(in.Doctype, in.Name)
				if err != nil {
					return err
				}
				doc, err = c.Cancel(d)
				return err
			})
			if err != nil {
				return fail(err)
			}
			return text(doc), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "call_method", Description: "Calls a document controller method (doctype+name+method) or a whitelisted function (path app.dir.file.fn)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Doctype string         `json:"doctype,omitempty"`
			Name    string         `json:"name,omitempty"`
			Method  string         `json:"method" jsonschema:"controller method name, or the function's full path"`
			Args    map[string]any `json:"args,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			var out any
			b, _ := json.Marshal(in.Args)
			if in.Args == nil {
				b = []byte("{}")
			}
			err := s.run(ctx, func(c *engine.Ctx) error {
				rt, err := c.RT()
				if err != nil {
					return err
				}
				if in.Doctype != "" {
					doc, err := c.GetDoc(in.Doctype, in.Name)
					if err != nil {
						return err
					}
					res, err := rt.RunMethod(in.Doctype, in.Method, doc.JSON(), b)
					if err != nil {
						return err
					}
					out = map[string]any{"result": res.Result, "doc": res.Doc}
					return nil
				}
				res, err := rt.CallFunction(in.Method, b)
				out = res
				return err
			})
			if err != nil {
				return fail(err)
			}
			return text(out), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "sql_query", Description: "Read-only SELECT on Postgres (tables tab_<snake_case>; child tables have parent/parenttype/parentfield/idx). Params via $1, $2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Query  string `json:"query"`
			Params []any  `json:"params,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			var rows []map[string]any
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.SQL(in.Query, in.Params)
				return e
			})
			if err != nil {
				return fail(err)
			}
			return text(rows), nil, nil
		})

	// ---- dev
	mcp.AddTool(srv, &mcp.Tool{Name: "eval", Description: "Runs TypeScript/JS in the server runtime with the `ddcore` API available (e.g. ddcore.db.count(\"User\")). The transaction is rolled back unless commit=true. Returns the value of the last expression and the logs."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Code   string `json:"code"`
			Commit bool   `json:"commit,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			res, logs, err := e.Eval(ctx, in.Code, in.Commit)
			if err != nil {
				r, _, _ := fail(err)
				if len(logs) > 0 {
					r.Content = append(r.Content, &mcp.TextContent{Text: "logs:\n" + strings.Join(logs, "\n")})
				}
				return r, nil, nil
			}
			return text(map[string]any{"result": res, "logs": logs, "committed": in.Commit}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "run_tests", Description: "Runs the apps' *.test.ts (filter = a regex over name/file). Each test runs in a rolled-back transaction."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Filter string `json:"filter,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			te, err := engine.New(ctx, testConfig(e))
			if err != nil {
				return fail(err)
			}
			defer te.DB.Close()
			if _, err := te.Migrate(ctx, false); err != nil {
				return fail(err)
			}
			results, err := te.RunTests(ctx, in.Filter, "")
			if err != nil {
				return fail(err)
			}
			failed := 0
			for _, r := range results {
				if !r.OK {
					failed++
				}
			}
			return text(map[string]any{"total": len(results), "failed": failed, "results": results}), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_logs", Description: "The latest Error Log rows (job, scheduler and server errors)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Limit int `json:"limit,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			if in.Limit == 0 {
				in.Limit = 20
			}
			var rows []map[string]any
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.GetList("Error Log", engine.ListArgs{Fields: []string{"name", "creation", "method", "error"}, OrderBy: "creation desc", Limit: in.Limit})
				return e
			})
			if err != nil {
				return fail(err)
			}
			return text(rows), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "reload", Description: "Rebuilds and reloads the apps (`ddcore dev` already does this when a file is saved)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			if err := e.Load(); err != nil {
				return fail(err)
			}
			return text("reloaded"), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_apps", Description: "Loaded apps, directories, whitelisted functions, reports, workspaces and scheduler."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			var apps []map[string]any
			for _, n := range e.AppOrder() {
				a := e.Snap.Apps[n]
				apps = append(apps, map[string]any{"name": n, "title": a.Title, "dir": a.Dir, "scheduler": a.Scheduler, "desk": a.Desk})
			}
			var reports, ws []string
			for n := range e.Snap.Reports {
				reports = append(reports, n)
			}
			for n := range e.Snap.Workspaces {
				ws = append(ws, n)
			}
			sort.Strings(reports)
			sort.Strings(ws)
			return text(map[string]any{"apps": apps, "whitelisted": e.WhitelistedPaths(), "reports": reports, "workspaces": ws}), nil, nil
		})

	// ---- resources
	for _, n := range docNames() {
		name := n
		srv.AddResource(&mcp.Resource{URI: "ddcore://docs/" + name, Name: "docs/" + name, MIMEType: "text/markdown", Description: "ddcore reference: " + name},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: Docs(name)}}}, nil
			})
	}
	srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "ddcore://meta/{doctype}", Name: "meta", MIMEType: "application/json", Description: "A DocType's meta"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			name := strings.TrimPrefix(req.Params.URI, "ddcore://meta/")
			d, err := e.DocType(name)
			if err != nil {
				return nil, err
			}
			b, _ := json.MarshalIndent(d, "", "  ")
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(b)}}}, nil
		})
	return srv
}

func (s *server) run(ctx context.Context, fn func(c *engine.Ctx) error) error {
	return s.e.Run(ctx, "Administrator", fn)
}

func (s *server) writeTypes() []string {
	var out []string
	for _, a := range s.e.Apps {
		if a.Embedded != nil {
			continue
		}
		if err := typegen.Write(a.Dir, s.e.Meta); err == nil {
			out = append(out, filepath.Join(a.Dir, ".ddcore/types.d.ts"))
		}
	}
	return out
}

func testConfig(e *engine.Engine) engine.Config {
	cfg := e.Cfg
	cfg.Test = true
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
