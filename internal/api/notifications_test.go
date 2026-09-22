package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func TestNotificationsHTTP(t *testing.T) {
	dir := testApp(t)
	if err := os.MkdirAll(filepath.Join(dir, "notifications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notifications", "created.notification.ts"), []byte(`
import { defineNotification } from "@ddcore/sdk";
export default defineNotification({
 name: "demo.created", doctype: "Pessoa", event: "on_insert",
 recipients: () => ["ana@x.com", "bia@x.com", "ze@x.com"],
 desk: {title: doc => "Created " + doc.id, message: doc => "Details " + doc.id}
});`), 0o644); err != nil {
		t.Fatal(err)
	}
	x := setupApp(t, dir)
	ana, bia, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("ze@x.com")
	for _, name := range []string{"First", "Second"} {
		x.expect(x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": name}, ana), 200, "")
	}
	page := func(auth, query string) ([]any, float64) {
		t.Helper()
		r := x.call("GET", "/api/notifications"+query, nil, auth)
		x.expect(r, 200, "")
		p := r.Body["data"].(map[string]any)
		return p["data"].([]any), p["total"].(float64)
	}
	rows, total := page(ana, "?limit=1&offset=0")
	if total != 2 || len(rows) != 1 || rows[0].(map[string]any)["reference_id"] != "Second" {
		t.Fatalf("newest first and paginated: %v, %v", rows, total)
	}
	name := rows[0].(map[string]any)["id"].(string)
	if older, total := page(ana, "?limit=1&offset=1"); total != 2 || len(older) != 1 || older[0].(map[string]any)["reference_id"] != "First" {
		t.Fatalf("second page: %v, %v", older, total)
	}
	otherRows, _ := page(bia, "")
	if otherRows[0].(map[string]any)["id"] == name {
		t.Fatal("recipients must have independent occurrences")
	}
	if rows, total := page(ze, ""); len(rows) != 0 || total != 0 {
		t.Fatalf("unauthorized recipient received notifications: %v", rows)
	}
	count := func(auth string, expected float64) {
		t.Helper()
		r := x.call("GET", "/api/notifications/count", nil, auth)
		x.expect(r, 200, "")
		if r.Body["data"] != expected {
			t.Fatalf("unread count: %s", r.Raw)
		}
	}
	count(ana, 2)
	count(bia, 2)
	count(ze, 0)
	foreign := x.call("PATCH", "/api/notifications/"+name, map[string]any{"read": true}, bia)
	if foreign.Status != 404 && foreign.Status != 403 {
		t.Fatalf("another recipient changed notification: %s", foreign.Raw)
	}
	x.expect(x.call("PATCH", "/api/notifications/"+name, map[string]any{"read": true}, ana), 200, "")
	count(ana, 1)
	count(bia, 2)
	count("sid:"+x.sid("ana@x.com"), 1)
	if rows, total := page(ana, "?read=true"); total != 1 || len(rows) != 1 {
		t.Fatalf("read filter: %v", rows)
	}
	x.expect(x.call("PATCH", "/api/notifications/"+name, map[string]any{"read": false}, ana), 200, "")
	count(ana, 2)

	for _, route := range []string{"/api/notifications", "/api/notifications/count"} {
		r := x.call("GET", route, nil, "")
		if r.Status != 401 && r.Status != 403 {
			t.Fatalf("anonymous request: %s", r.Raw)
		}
		x.expect(x.call("GET", route+"?user=ana%40x.com", nil, bia), 417, "ValidationError")
	}
	if r := x.call("PATCH", "/api/notifications/"+name, map[string]any{"read": true}, ""); r.Status != 401 && r.Status != 403 {
		t.Fatalf("anonymous mutation: %s", r.Raw)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=-1", "?read=1", "?read=true&read=false"} {
		x.expect(x.call("GET", "/api/notifications"+query, nil, ana), 417, "ValidationError")
	}
	for _, body := range []any{map[string]any{}, map[string]any{"read": "true"}, map[string]any{"read": true, "recipient": "bia@x.com"}} {
		x.expect(x.call("PATCH", "/api/notifications/"+name, body, ana), 417, "ValidationError")
	}
	for _, route := range []string{"/api/resource/ddcore_notification", "/api/count/ddcore_notification", "/api/export/ddcore_notification", "/api/report/ddcore_notification"} {
		r := x.call("GET", route, nil, "sid:"+x.sid("Admin"))
		if r.Status == 200 || strings.Contains(r.Raw, "Details Second") {
			t.Fatalf("generic access to notification storage: %s %s", route, r.Raw)
		}
	}
	// Revocation must affect both stored pages and counts immediately.
	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		d["roles"] = []any{}
		_, err = c.Save(d, engine.SaveOpts{})
		return err
	})
	// Ana owns both records, so removing the role leaves the owner permission.
	// Changing ownership removes that final permission too.
	x.asAdmin(func(c *engine.Ctx) error {
		for _, n := range []string{"First", "Second"} {
			if _, err := c.DBSet("Pessoa", n, engine.Doc{"owner": "Admin"}, false); err != nil {
				return err
			}
		}
		return nil
	})
	count(ana, 0)
	if rows, total := page(ana, ""); total != 0 || len(rows) != 0 {
		t.Fatalf("revoked page: %v", rows)
	}
	r := x.call("PATCH", "/api/notifications/"+name, map[string]any{"read": true}, ana)
	if r.Status != 404 && r.Status != 403 {
		t.Fatalf("revoked mutation: %s", r.Raw)
	}
}
