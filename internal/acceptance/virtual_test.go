package acceptance

// A virtual DocType over real HTTP (DAT-07): the union a Desk lists, the
// lookups a Link control makes, the writes refused at the border, and a field
// masked by its source's permission level.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func seedWorkItems(t *testing.T, e *engine.Engine) (project, task string) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *engine.Ctx) error {
		u, _ := c.NewDoc("User", engine.Doc{"email": "contrib@x.com", "full_name": "Contributor",
			"roles": []any{map[string]any{"role": "Project Contributor"}}})
		if _, err := c.Insert(u, engine.SaveOpts{}); err != nil {
			return err
		}
		p, _ := c.NewDoc("Project", engine.Doc{"code": "P-1", "title": "Launch", "assignee": "Admin",
			"start_date": "2026-01-01", "budget": 1000})
		p, err := c.Insert(p, engine.SaveOpts{})
		if err != nil {
			return err
		}
		tk, _ := c.NewDoc("Task", engine.Doc{"code": "T-1", "title": "Write the brief", "assignee": "Admin",
			"project": p.ID(), "due_date": "2026-02-01"})
		tk, err = c.Insert(tk, engine.SaveOpts{})
		if err != nil {
			return err
		}
		project, task = p.ID(), tk.ID()
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return "Project:" + project, "Task:" + task
}

func rowsOf(res map[string]any) []map[string]any {
	var out []map[string]any
	list, _ := res["data"].([]any)
	for _, r := range list {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestVirtualDocTypes(t *testing.T) {
	e := setup(t, "virtual")
	srv, tok := server(t, e)
	project, task := seedWorkItems(t, e)

	t.Run("meta and boot carry the union", func(t *testing.T) {
		dt := getJSON(t, srv, tok, "/api/meta/Work%20Item")["data"].(map[string]any)["doctype"].(map[string]any)
		if dt["virtual"] == nil {
			t.Fatalf("meta has no virtual: %v", dt)
		}
		found := false
		for _, f := range dt["fields"].([]any) {
			found = found || f.(map[string]any)["fieldname"] == "source_doctype"
		}
		if !found {
			t.Fatal("source_doctype was not added")
		}
		boot := getJSON(t, srv, tok, "/api/boot")["data"].(map[string]any)
		vs, _ := boot["virtuals"].(map[string]any)
		if src, _ := vs["Work Item"].([]any); len(src) != 2 {
			t.Fatalf("boot virtuals = %v", boot["virtuals"])
		}
	})

	t.Run("the list is the union", func(t *testing.T) {
		rows := rowsOf(getJSON(t, srv, tok, `/api/resource/Work%20Item?fields=["*"]&order_by=id%20asc`))
		if len(rows) != 2 || rows[0]["id"] != project || rows[1]["id"] != task {
			t.Fatalf("rows = %v", rows)
		}
		if rows[0]["title"] != "Launch" || rows[1]["source_doctype"] != "Task" || rows[1]["budget"] != nil {
			t.Fatalf("mapped rows = %v", rows)
		}
		f := url.QueryEscape(`{"source_doctype":"Task"}`)
		if rows := rowsOf(getJSON(t, srv, tok, "/api/resource/Work%20Item?filters="+f)); len(rows) != 1 {
			t.Fatalf("filtered = %v", rows)
		}
		if n := getJSON(t, srv, tok, "/api/count/Work%20Item")["data"]; n != float64(2) {
			t.Fatalf("count = %v", n)
		}
	})

	t.Run("a Link control finds and titles it", func(t *testing.T) {
		hits := rowsOf(getJSON(t, srv, tok, "/api/search/link?doctype=Work%20Item&txt=brief"))
		if len(hits) != 1 || hits[0]["id"] != task {
			t.Fatalf("link search = %v", hits)
		}
		titles := getJSON(t, srv, tok, "/api/search/link-titles?doctype=Work%20Item&ids="+url.QueryEscape(project))
		m := titles["data"].(map[string]any)["Work Item"].(map[string]any)
		if m[project] != "Launch" {
			t.Fatalf("titles = %v", titles)
		}
		doc := getJSON(t, srv, tok, "/api/resource/Work%20Item/"+url.PathEscape(task))["data"].(map[string]any)
		if doc["title"] != "Write the brief" {
			t.Fatalf("get = %v", doc)
		}
	})

	t.Run("writes are refused", func(t *testing.T) {
		refused := func(res *http.Response) {
			t.Helper()
			defer res.Body.Close()
			if res.StatusCode == 200 {
				t.Fatal("a write to a virtual DocType went through")
			}
			var body map[string]any
			json.NewDecoder(res.Body).Decode(&body)
			failure, _ := body["error"].(map[string]any)
			if key, _ := failure["key"].(string); !strings.Contains(key, "virtual DocType") {
				t.Fatalf("refusal = %v", body)
			}
		}
		refused(postJSON(t, srv, tok, "/api/resource/Work%20Item", map[string]any{"title": "x"}))
		for _, method := range []string{"PUT", "DELETE"} {
			req, _ := http.NewRequest(method, srv.URL+"/api/resource/Work%20Item/"+url.PathEscape(project), strings.NewReader(`{"title":"y"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "token "+tok)
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			refused(res)
		}
	})

	t.Run("a source's field level masks the mapped field", func(t *testing.T) {
		ctok, err := e.CreateAPIKey(context.Background(), "contrib@x.com", "acceptance")
		if err != nil {
			t.Fatal(err)
		}
		rows := rowsOf(getJSON(t, srv, ctok, `/api/resource/Work%20Item?fields=["id","budget"]&filters=`+url.QueryEscape(`{"source_doctype":"Project"}`)))
		if len(rows) != 1 || rows[0]["budget"] != nil {
			t.Fatalf("contributor reads %v", rows)
		}
		rows = rowsOf(getJSON(t, srv, tok, `/api/resource/Work%20Item?fields=["id","budget"]&filters=`+url.QueryEscape(`{"source_doctype":"Project"}`)))
		if len(rows) != 1 || rows[0]["budget"] == nil {
			t.Fatalf("admin reads %v", rows)
		}
	})
}
