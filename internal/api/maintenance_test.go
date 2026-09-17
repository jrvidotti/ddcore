package api

import (
	"testing"
)

// While the site is paused a server refuses writes at the border with 503,
// keeps reads, sign-in and the probes open, and says so in the boot.
func TestMaintenanceMode(t *testing.T) {
	x := setup(t)
	x.e.Cfg.EnforceMaintenance = true
	admin := "sid:" + x.sid("Administrator")

	if r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Antes"}, admin); r.Status != 200 {
		t.Fatalf("create before pausing: %d %s", r.Status, r.Raw)
	}
	if _, err := x.e.SetMaintenance(x.ctx, true, "Upgrade to 2.0", "tester"); err != nil {
		t.Fatal(err)
	}

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Durante"}, admin)
	if r.Status != 503 || r.errType() != "MaintenanceError" {
		t.Fatalf("create while paused: %d %s", r.Status, r.Raw)
	}
	if r.Header.Get("Retry-After") == "" {
		t.Error("a 503 for maintenance should say when to retry")
	}
	if r := x.call("DELETE", "/api/resource/Pessoa/Antes", nil, admin); r.Status != 503 {
		t.Errorf("delete while paused: %d", r.Status)
	}
	if r := x.call("GET", "/api/resource/Pessoa/Antes", nil, admin); r.Status != 200 {
		t.Errorf("reads stay open: %d %s", r.Status, r.Raw)
	}
	if r := x.call("POST", "/api/login", map[string]any{"usr": "ana@x.com", "pwd": "segredo123"}, ""); r.Status == 503 {
		t.Errorf("sign-in stays open: %s", r.Raw)
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		if r := x.call("GET", p, nil, ""); r.Status != 200 {
			t.Errorf("%s while paused: %d", p, r.Status)
		}
	}
	boot := x.call("GET", "/api/boot", nil, admin)
	site, _ := boot.Body["data"].(map[string]any)["site"].(map[string]any)
	m, _ := site["maintenance"].(map[string]any)
	if m["enabled"] != true || m["reason"] != "Upgrade to 2.0" {
		t.Errorf("boot maintenance: %#v", site["maintenance"])
	}
	var errorLogs int
	x.e.DB.Pool.QueryRow(x.ctx, `SELECT count(*) FROM tab_error_log`).Scan(&errorLogs)
	if errorLogs != 0 {
		t.Errorf("a refusal is not a fault, but %d Error Log row(s) were written", errorLogs)
	}

	if _, err := x.e.SetMaintenance(x.ctx, false, "", "tester"); err != nil {
		t.Fatal(err)
	}
	if r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Depois"}, admin); r.Status != 200 {
		t.Fatalf("create after resuming: %d %s", r.Status, r.Raw)
	}
	boot = x.call("GET", "/api/boot", nil, admin)
	site, _ = boot.Body["data"].(map[string]any)["site"].(map[string]any)
	if site["maintenance"] != nil {
		t.Errorf("boot after resuming: %#v", site["maintenance"])
	}
}
