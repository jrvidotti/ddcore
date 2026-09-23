package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/desk"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// TestPortal: the fixture's Assignee portal (OPS-10). A Website User signs in,
// lands on /portal, lists the tasks assigned to them and nothing else, and is
// refused the desk's API.
func TestPortal(t *testing.T) {
	e := setup(t, "_portal")
	ctx := context.Background()
	const member, other = "portal@exemplo.com", "outro@exemplo.com"
	const pwd = "senhaportal123"

	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		for _, u := range []string{member, other} {
			d, err := c.NewDoc("User", engine.Doc{"email": u, "full_name": u, "new_password": pwd, "user_type": "Website User",
				"roles": []any{map[string]any{"role": "Project Contributor"}}})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		p, err := c.NewDoc("Project", engine.Doc{"code": "PRT", "title": "Portal", "start_date": "2026-01-01", "assignee": "Admin"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(p, engine.SaveOpts{}); err != nil {
			return err
		}
		for code, who := range map[string]string{"PRT-1": member, "PRT-2": other} {
			d, err := c.NewDoc("Task", engine.Doc{"code": code, "project": "PRT", "title": code, "assignee": who, "due_date": "2026-12-31"})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	srv, _ := server(t, e)
	do := func(method, path string, body any, sid *http.Cookie) (int, map[string]any) {
		t.Helper()
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "acceptance")
		if sid != nil {
			req.AddCookie(sid)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		if method == "POST" && path == "/api/login" {
			for _, c := range res.Cookies() {
				if c.Name == "sid" {
					out["__sid"] = c
				}
			}
		}
		return res.StatusCode, out
	}

	status, login := do("POST", "/api/login", map[string]any{"usr": member, "pwd": pwd}, nil)
	if status != 200 {
		t.Fatalf("login = %d %v", status, login)
	}
	if home := login["data"].(map[string]any)["home"]; home != "/portal" {
		t.Fatalf("a Website User lands on %v", home)
	}
	sid, _ := login["__sid"].(*http.Cookie)
	if sid == nil {
		t.Fatal("no session cookie")
	}

	_, boot := do("GET", "/api/boot", nil, sid)
	b := boot["data"].(map[string]any)
	portals, _ := b["portals"].([]any)
	if len(portals) != 1 || portals[0].(map[string]any)["slug"] != "assignee" {
		t.Fatalf("boot portals = %v", b["portals"])
	}
	if dts, _ := b["doctypes"].(map[string]any); len(dts) != 0 {
		t.Fatalf("a Website User's boot lists DocTypes: %v", b["doctypes"])
	}

	status, list := do("GET", "/api/portal/assignee/tasks/list", nil, sid)
	if status != 200 {
		t.Fatalf("portal list = %d %v", status, list)
	}
	rows, _ := list["data"].(map[string]any)["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["code"] != "PRT-1" {
		t.Fatalf("the portal lists %v, want only PRT-1", rows)
	}
	if status, _ := do("GET", "/api/portal/assignee/tasks/doc/PRT-2", nil, sid); status != 403 {
		t.Fatalf("another assignee's task answered %d", status)
	}
	if status, _ := do("GET", "/api/resource/Task", nil, sid); status != 403 {
		t.Fatalf("the desk API answered %d to a Website User", status)
	}

	if desk.FS() == nil {
		return
	}
	res, err := srv.Client().Get(srv.URL + "/portal")
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4096)
	n, _ := res.Body.Read(body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(strings.ToLower(string(body[:n])), "<!doctype html") {
		t.Fatalf("GET /portal = %d %.120q", res.StatusCode, body[:n])
	}
}
