package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// dataTools registers the tools that read and write documents, and the ones
// that run code against the site.
func (s *server) dataTools(srv *mcp.Server) {
	e := s.e

	addTool(srv, "get_doc", "Reads a document with its child tables.",
		func(ctx context.Context, in struct {
			Doctype string `json:"doctype"`
			ID      string `json:"id"`
		}) (any, error) {
			return s.docOp(ctx, in.Doctype, func(c *engine.Ctx) (engine.Doc, error) {
				return c.GetDoc(in.Doctype, in.ID)
			})
		})

	addTool(srv, "list_docs", "Lists documents. filters: [[field, op, value], ...] or {field: value}; ops: = != > >= < <= like in not in between is set. fields accepts aggregates like \"count(id) as n\".",
		func(ctx context.Context, in struct {
			Doctype string   `json:"doctype"`
			Filters any      `json:"filters,omitempty"`
			Fields  []string `json:"fields,omitempty"`
			OrderBy string   `json:"order_by,omitempty"`
			Limit   int      `json:"limit,omitempty"`
			Start   int      `json:"start,omitempty"`
		}) (any, error) {
			var rows []map[string]any
			if in.Limit == 0 {
				in.Limit = 20
			}
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.GetList(in.Doctype, engine.ListArgs{Filters: in.Filters, Fields: in.Fields, OrderBy: in.OrderBy, Limit: in.Limit, Start: in.Start})
				if d, err := c.St.DocType(in.Doctype); err == nil {
					for _, r := range rows {
						engine.RedactPassword(d, engine.Doc(r))
					}
				}
				return e
			})
			return rows, err
		})

	addTool(srv, "insert_doc", "Creates a document (running validate and the other hooks). Child tables go in their field as arrays.",
		func(ctx context.Context, in struct {
			Doctype string         `json:"doctype"`
			Values  map[string]any `json:"values"`
			Submit  bool           `json:"submit,omitempty" jsonschema:"insert already submitted (docstatus 1)"`
		}) (any, error) {
			return s.docOp(ctx, in.Doctype, func(c *engine.Ctx) (engine.Doc, error) {
				d, err := c.NewDoc(in.Doctype, in.Values)
				if err != nil {
					return nil, err
				}
				if in.Submit {
					d["docstatus"] = 1
				}
				return c.Insert(d, engine.SaveOpts{})
			})
		})

	addTool(srv, "update_doc", "Updates fields of a document and saves (runs validate).",
		func(ctx context.Context, in struct {
			Doctype string         `json:"doctype"`
			ID      string         `json:"id"`
			Values  map[string]any `json:"values"`
		}) (any, error) {
			return s.docOp(ctx, in.Doctype, func(c *engine.Ctx) (engine.Doc, error) {
				d, err := c.GetDoc(in.Doctype, in.ID)
				if err != nil {
					return nil, err
				}
				for k, v := range in.Values {
					if !engine.IsInternalKey(k) {
						d[k] = v
					}
				}
				return c.Save(d, engine.SaveOpts{})
			})
		})

	addTool(srv, "delete_doc", "Deletes a document (fails if linked, unless force=true).",
		func(ctx context.Context, in struct {
			Doctype string `json:"doctype"`
			ID      string `json:"id"`
			Force   bool   `json:"force,omitempty"`
		}) (any, error) {
			err := s.run(ctx, func(c *engine.Ctx) error { return c.Delete(in.Doctype, in.ID, true, in.Force) })
			return "deleted", err
		})

	addTool(srv, "submit_doc", "Submits (docstatus 1) a submittable document.",
		func(ctx context.Context, in struct {
			Doctype string `json:"doctype"`
			ID      string `json:"id"`
		}) (any, error) {
			return s.docOp(ctx, in.Doctype, func(c *engine.Ctx) (engine.Doc, error) {
				d, err := c.GetDoc(in.Doctype, in.ID)
				if err != nil {
					return nil, err
				}
				return c.Submit(d)
			})
		})

	addTool(srv, "cancel_doc", "Cancels (docstatus 2) a submitted document.",
		func(ctx context.Context, in struct {
			Doctype string `json:"doctype"`
			ID      string `json:"id"`
		}) (any, error) {
			return s.docOp(ctx, in.Doctype, func(c *engine.Ctx) (engine.Doc, error) {
				d, err := c.GetDoc(in.Doctype, in.ID)
				if err != nil {
					return nil, err
				}
				return c.Cancel(d)
			})
		})

	addTool(srv, "call_method", "Calls a document controller method (doctype+id+method) or a whitelisted function (path app.dir.file.fn).",
		func(ctx context.Context, in struct {
			Doctype string         `json:"doctype,omitempty"`
			ID      string         `json:"id,omitempty"`
			Method  string         `json:"method" jsonschema:"controller method name, or the function's full path"`
			Args    map[string]any `json:"args,omitempty"`
		}) (any, error) {
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
					doc, err := c.GetDoc(in.Doctype, in.ID)
					if err != nil {
						return err
					}
					res, err := rt.RunMethod(in.Doctype, in.Method, doc.JSON(), b)
					if err != nil {
						return err
					}
					var rdoc engine.Doc
					if json.Unmarshal(res.Doc, &rdoc) == nil && rdoc != nil {
						out = map[string]any{"result": res.Result, "doc": c.RedactDoc(in.Doctype, rdoc)}
						return nil
					}
					out = map[string]any{"result": res.Result, "doc": res.Doc}
					return nil
				}
				res, err := rt.CallFunction(in.Method, b)
				out = res
				return err
			})
			return out, err
		})

	addTool(srv, "sql_query", "Read-only SELECT on Postgres (tables tab_<snake_case>; child tables have parent/parenttype/parentfield/idx). Params via $1, $2.",
		func(ctx context.Context, in struct {
			Query  string `json:"query"`
			Params []any  `json:"params,omitempty"`
		}) (any, error) {
			var rows []map[string]any
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.SQL(in.Query, in.Params)
				return e
			})
			return rows, err
		})

	// eval alone builds its CallToolResult: a failure also carries the logs.
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

	addTool(srv, "run_tests", "Runs the apps' *.test.ts (filter = a regex over name/file). Each test runs in a rolled-back transaction.",
		func(ctx context.Context, in struct {
			Filter string `json:"filter,omitempty"`
		}) (any, error) {
			te, err := engine.New(ctx, testConfig(e))
			if err != nil {
				return nil, err
			}
			defer te.DB.Close()
			if _, err := te.Migrate(ctx, false); err != nil {
				return nil, err
			}
			results, err := te.RunTests(ctx, in.Filter, "")
			if err != nil {
				return nil, err
			}
			failed := 0
			for _, r := range results {
				if !r.OK {
					failed++
				}
			}
			return map[string]any{"total": len(results), "failed": failed, "results": results}, nil
		})

	addTool(srv, "get_logs", "The latest Error Log rows (job, scheduler and server errors).",
		func(ctx context.Context, in struct {
			Limit int `json:"limit,omitempty"`
		}) (any, error) {
			if in.Limit == 0 {
				in.Limit = 20
			}
			var rows []map[string]any
			err := s.run(ctx, func(c *engine.Ctx) error {
				var e error
				rows, e = c.GetList("Error Log", engine.ListArgs{Fields: []string{"id", "creation", "method", "error", "request_id"}, OrderBy: "creation desc", Limit: in.Limit})
				return e
			})
			return rows, err
		})
}
