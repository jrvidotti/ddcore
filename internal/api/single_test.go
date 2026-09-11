package api

import "testing"

func TestSingleREST(t *testing.T) {
	x := setup(t)
	auth := "sid:" + x.sid("ana@x.com")
	denied := "sid:" + x.sid("ze@x.com")
	path := "/api/resource/Settings/singleton"
	r := x.call("GET", path, nil, denied)
	if r.Status != 403 {
		t.Fatalf("unauthorized defaults: %d %s", r.Status, r.Raw)
	}
	r = x.call("GET", path, nil, auth)
	if r.Status != 200 {
		t.Fatalf("defaults: %d %s", r.Status, r.Raw)
	}
	doc := r.Body["data"].(map[string]any)
	if doc["enabled"] != true || doc["__islocal"] != true {
		t.Fatalf("defaults: %v", doc)
	}
	doc["enabled"], doc["secret"] = false, "private-value"
	r = x.call("PUT", path, doc, auth)
	if r.Status != 200 {
		t.Fatalf("first save without create: %d %s", r.Status, r.Raw)
	}
	r = x.call("GET", path, nil, auth)
	saved := r.Body["data"].(map[string]any)
	if saved["enabled"] != false || saved["secret"] == "private-value" {
		t.Fatalf("reload/redaction: %v", saved)
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		r = x.call(method, path, map[string]any{"enabled": true}, denied)
		if r.Status < 400 {
			t.Fatalf("unauthorized %s: %d", method, r.Status)
		}
	}
	r = x.call("POST", "/api/resource/Settings", map[string]any{}, auth)
	if r.Status < 400 {
		t.Fatal("duplicate created")
	}
	r = x.call("DELETE", path, nil, auth)
	if r.Status < 400 {
		t.Fatal("deleted Single")
	}
}
