package acceptance

// A form's grid over real HTTP (issue #18): computed fields filled by the
// controller's onLoad and never stored, and a Report field's report run for
// the document it sits in.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func putJSON(t *testing.T, srv *httptest.Server, tok, path string, body any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", srv.URL+path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+tok)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGridComputedAndReportField(t *testing.T) {
	e := setup(t, "grid")
	srv, tok := server(t, e)
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		p, _ := c.NewDoc("Project", engine.Doc{"code": "P-1", "title": "Launch", "assignee": "Admin", "start_date": "2020-01-01",
			"milestones": []any{
				map[string]any{"title": "Late", "due_date": "2020-02-01"},
				map[string]any{"title": "Later", "due_date": "2999-01-01"},
			}})
		if _, err := c.Insert(p, engine.SaveOpts{}); err != nil {
			return err
		}
		for _, code := range []string{"T-2", "T-1"} {
			tk, _ := c.NewDoc("Task", engine.Doc{"code": code, "title": "Task " + code, "assignee": "Admin", "project": "P-1", "due_date": "2026-0" + code[2:] + "-01"})
			if _, err := c.Insert(tk, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	check := func(t *testing.T, doc map[string]any) {
		t.Helper()
		if doc["open_tasks"] != float64(2) {
			t.Fatalf("open_tasks = %v", doc["open_tasks"])
		}
		overdue := map[string]any{}
		for _, m := range doc["milestones"].([]any) {
			row := m.(map[string]any)
			overdue[row["title"].(string)] = row["overdue"]
		}
		if overdue["Late"] != true || overdue["Later"] != false {
			t.Fatalf("overdue = %v", overdue)
		}
	}

	t.Run("the form load carries the computed values", func(t *testing.T) {
		check(t, getJSON(t, srv, tok, "/api/resource/Project/P-1")["data"].(map[string]any))
	})

	t.Run("a save ignores computed values and answers with fresh ones", func(t *testing.T) {
		doc := getJSON(t, srv, tok, "/api/resource/Project/P-1")["data"].(map[string]any)
		doc["open_tasks"] = 99
		doc["title"] = "Launch 2"
		res := putJSON(t, srv, tok, "/api/resource/Project/P-1", doc)
		defer res.Body.Close()
		if res.StatusCode != 200 {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("PUT = %d: %s", res.StatusCode, b)
		}
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		saved := out["data"].(map[string]any)
		if saved["title"] != "Launch 2" {
			t.Fatalf("title = %v", saved["title"])
		}
		check(t, saved)
	})

	t.Run("a computed field has no column", func(t *testing.T) {
		var n int
		if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name IN ('tab_project', 'tab_project_milestone') AND column_name IN ('open_tasks', 'overdue')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%d computed columns in the database", n)
		}
		req, _ := http.NewRequest("GET", srv.URL+"/api/resource/Project?filters="+url.QueryEscape(`{"open_tasks":2}`), nil)
		req.Header.Set("Authorization", "token "+tok)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("a list filter on a computed field was accepted")
		}
	})

	t.Run("meta carries the Report field and the grid props", func(t *testing.T) {
		fields := getJSON(t, srv, tok, "/api/meta/Project")["data"].(map[string]any)["doctype"].(map[string]any)["fields"].([]any)
		tasks, ms := fieldOf(fields, "tasks"), fieldOf(fields, "milestones")
		if tasks["fieldtype"] != "Report" || tasks["options"] != "Project Tasks" || tasks["reportFilters"].(map[string]any)["project"] != "id" {
			t.Fatalf("tasks = %v", tasks)
		}
		if ms["gridSort"].(map[string]any)["field"] != "due_date" || ms["gridExport"] != true || ms["gridSelect"] != true || ms["gridSortable"] != true {
			t.Fatalf("milestones = %v", ms)
		}
	})

	t.Run("the report runs for the project", func(t *testing.T) {
		res := getJSON(t, srv, tok, "/api/report/Project%20Tasks?filters="+url.QueryEscape(`{"project":"P-1"}`))["data"].(map[string]any)
		rows := res["result"].(map[string]any)["rows"].([]any)
		if len(rows) != 2 || rows[0].(map[string]any)["id"] != "T-1" {
			t.Fatalf("rows = %v", rows)
		}
		if res["meta"].(map[string]any)["canExport"] != true {
			t.Fatalf("meta = %v", res["meta"])
		}
	})
}
