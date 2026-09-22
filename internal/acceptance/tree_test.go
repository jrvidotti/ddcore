package acceptance

// A hierarchy over real HTTP (DAT-07): the meta a Desk reads, the level
// endpoint the tree view calls, the filter operator, and the refusal that
// keeps a branch from losing its middle.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func seedCategories(t *testing.T, e *engine.Engine) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *engine.Ctx) error {
		for _, n := range []struct {
			title, parent string
			group         bool
		}{
			{"All Categories", "", true},
			{"Delivery", "All Categories", true},
			{"Shipping", "Delivery", false},
			{"Support", "All Categories", false},
		} {
			doc, err := c.NewDoc("Task Category", engine.Doc{
				"title": n.title, "parent_task_category": n.parent, "is_group": n.group})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func postJSON(t *testing.T, srv *httptest.Server, tok, path string, body any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+tok)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTrees(t *testing.T) {
	e := setup(t, "tree")
	srv, tok := server(t, e)
	seedCategories(t, e)

	t.Run("meta carries the hierarchy", func(t *testing.T) {
		res := getJSON(t, srv, tok, "/api/meta/Task%20Category")
		data, _ := res["data"].(map[string]any)
		dt, _ := data["doctype"].(map[string]any)
		if dt["isTree"] != true || dt["parentField"] != "parent_task_category" {
			t.Fatalf("meta = %v", dt)
		}
		names := map[string]bool{}
		for _, f := range dt["fields"].([]any) {
			names[f.(map[string]any)["fieldname"].(string)] = true
		}
		if !names["parent_task_category"] || !names["is_group"] {
			t.Fatalf("the injected fields are missing from the meta: %v", names)
		}
	})

	t.Run("one level at a time", func(t *testing.T) {
		res := getJSON(t, srv, tok, "/api/tree/Task%20Category")
		data, _ := res["data"].(map[string]any)
		nodes, _ := data["nodes"].([]any)
		if len(nodes) != 1 {
			t.Fatalf("roots = %v", nodes)
		}
		root := nodes[0].(map[string]any)
		if root["id"] != "All Categories" || root["is_group"] != true || root["children"] != float64(2) {
			t.Fatalf("root = %v", root)
		}
		res = getJSON(t, srv, tok, "/api/tree/Task%20Category?parent=All%20Categories")
		data, _ = res["data"].(map[string]any)
		nodes, _ = data["nodes"].([]any)
		var got []string
		for _, n := range nodes {
			got = append(got, n.(map[string]any)["id"].(string))
		}
		if strings.Join(got, ",") != "Delivery,Support" {
			t.Fatalf("children of the root = %v (groups first)", got)
		}
	})

	t.Run("descendants filter over the list API", func(t *testing.T) {
		res := getJSON(t, srv, tok,
			"/api/resource/Task%20Category?order_by=id%20asc&filters="+
				url.QueryEscape(`[["id","descendants of","All Categories"]]`))
		rows, _ := res["data"].([]any)
		var got []string
		for _, r := range rows {
			got = append(got, r.(map[string]any)["id"].(string))
		}
		if strings.Join(got, ",") != "Delivery,Shipping,Support" {
			t.Fatalf("descendants = %v", got)
		}
	})

	t.Run("a group with children is not deleted", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/api/resource/Task%20Category/Delivery", nil)
		req.Header.Set("Authorization", "token "+tok)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("deleted a node with children")
		}
		var body map[string]any
		json.NewDecoder(res.Body).Decode(&body)
		failure, _ := body["error"].(map[string]any)
		if key, _ := failure["key"].(string); !strings.Contains(key, "child nodes") {
			t.Fatalf("the refusal does not name the children: %v", body)
		}
	})

	t.Run("a leaf cannot take children", func(t *testing.T) {
		res := postJSON(t, srv, tok, "/api/resource/Task%20Category",
			map[string]any{"title": "Returns", "parent_task_category": "Support"})
		defer res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("a leaf was given a child")
		}
	})
}
