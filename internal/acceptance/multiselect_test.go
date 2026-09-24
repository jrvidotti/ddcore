package acceptance

// A Table MultiSelect over real HTTP (DAT-08): the ids a client sends come
// back as child rows, and a PUT that leaves one out removes it.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func TestTableMultiSelectOverHTTP(t *testing.T) {
	e := setup(t, "multiselect")
	srv, tok := server(t, e)
	seedCategories(t, e)
	if err := e.Run(context.Background(), "Admin", func(c *engine.Ctx) error {
		p, _ := c.NewDoc("Project", engine.Doc{"code": "PRJ-MS", "title": "Tags", "assignee": "Admin", "start_date": "2026-01-01"})
		_, err := c.Insert(p, engine.SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	tags := func(doc map[string]any) []string {
		var out []string
		rows, _ := doc["tags"].([]any)
		for _, r := range rows {
			row := r.(map[string]any)
			if row["parentfield"] != "tags" || row["id"] == nil {
				t.Fatalf("not a child row: %v", row)
			}
			out = append(out, row["category"].(string))
		}
		return out
	}

	res := postJSON(t, srv, tok, "/api/resource/Task", map[string]any{
		"code": "T-MS", "project": "PRJ-MS", "title": "Tagged", "assignee": "Admin", "due_date": "2026-02-01",
		"tags": []string{"Shipping", "Support"},
	})
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST = %d", res.StatusCode)
	}
	doc := getJSON(t, srv, tok, "/api/resource/Task/T-MS")["data"].(map[string]any)
	if got := strings.Join(tags(doc), ","); got != "Shipping,Support" {
		t.Fatalf("tags = %q", got)
	}

	put := func(body map[string]any) int {
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest("PUT", srv.URL+"/api/resource/Task/T-MS", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "token "+tok)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if status := put(map[string]any{"tags": []string{"Support"}}); status != 200 {
		t.Fatalf("PUT = %d", status)
	}
	doc = getJSON(t, srv, tok, "/api/resource/Task/T-MS")["data"].(map[string]any)
	if got := strings.Join(tags(doc), ","); got != "Support" {
		t.Fatalf("tags after PUT = %q", got)
	}
	if status := put(map[string]any{"tags": []string{"Support", "Support"}}); status == 200 {
		t.Fatal("a repeated value was saved")
	}
	if status := put(map[string]any{"tags": []string{"Nowhere"}}); status == 200 {
		t.Fatal("a value that does not exist was saved")
	}
}
