package api

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const (
	alfaAdmin, alfaUser = "chefe@alfa.test", "ana@alfa.test"
	betaAdmin, betaUser = "chefe@beta.test", "bia@beta.test"
)

// setupTenants is a site with tenancy and two tenants, each with a System
// Manager, a Gestor and one Pessoa. The base users of setupSite (ana@x.com,
// root@x.com, …) are in the platform space.
func setupTenants(t *testing.T) *env {
	t.Helper()
	x := setupSite(t, testApp(t), true)
	x.asAdmin(func(c *engine.Ctx) error {
		for _, id := range []string{"alfa", "beta"} {
			d, _ := c.NewDoc("Site Tenant", engine.Doc{"slug": id, "title": strings.ToUpper(id)})
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	seed := map[string][2]string{"alfa": {alfaAdmin, alfaUser}, "beta": {betaAdmin, betaUser}}
	for tenant, users := range seed {
		err := x.e.Run(engine.WithTenant(x.ctx, tenant), "Admin", func(c *engine.Ctx) error {
			for i, u := range users {
				role := []string{"System Manager", "Gestor"}[i]
				d, _ := c.NewDoc("User", engine.Doc{"email": u, "full_name": u, "new_password": "segredo123",
					"roles": []any{map[string]any{"role": role}}})
				if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
					return err
				}
			}
			d, _ := c.NewDoc("Pessoa", engine.Doc{"nome": "Cliente de " + tenant})
			_, err := c.Insert(d, engine.SaveOpts{})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return x
}

func ids(r resp) string {
	var out []string
	rows, _ := r.Body["data"].([]any)
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, fmt.Sprint(m["id"]))
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestTenantAPI_RequestsStayInTheirTenant(t *testing.T) {
	x := setupTenants(t)
	a, b := "sid:"+x.sid(alfaUser), "sid:"+x.sid(betaUser)

	r := x.call("GET", "/api/resource/Pessoa", nil, a)
	x.expect(r, 200, "")
	if got := ids(r); got != "Cliente de alfa" {
		t.Fatalf("alfa lists %q", got)
	}
	x.expect(x.call("GET", "/api/resource/Pessoa/Cliente%20de%20alfa", nil, b), 404, "")
	x.expect(x.call("PUT", "/api/resource/Pessoa/Cliente%20de%20alfa", map[string]any{"tipo": "PJ"}, b), 404, "")
	x.expect(x.call("DELETE", "/api/resource/Pessoa/Cliente%20de%20alfa", nil, b), 404, "")

	// naming another tenant in the header changes nothing for a tenant's user
	r = x.call("GET", "/api/resource/Pessoa", nil, b, "X-DDCore-Tenant", "alfa")
	x.expect(r, 200, "")
	if got := ids(r); got != "Cliente de beta" {
		t.Fatalf("beta, naming alfa, lists %q", got)
	}

	// an API key works in its user's tenant too
	r = x.call("GET", "/api/resource/Pessoa", nil, "token:"+x.apiKey(betaUser))
	x.expect(r, 200, "")
	if got := ids(r); got != "Cliente de beta" {
		t.Fatalf("beta's key lists %q", got)
	}
}

func TestTenantAPI_BootSaysWhereTheUserIs(t *testing.T) {
	x := setupTenants(t)
	tenantOf := func(auth string) map[string]any {
		t.Helper()
		r := x.call("GET", "/api/boot", nil, auth)
		x.expect(r, 200, "")
		data, _ := r.Body["data"].(map[string]any)
		site, _ := data["site"].(map[string]any)
		tb, _ := site["tenant"].(map[string]any)
		if tb == nil {
			t.Fatalf("boot has no tenant block: %s", r.Raw)
		}
		return tb
	}
	tb := tenantOf("sid:" + x.sid(alfaAdmin))
	if tb["id"] != "alfa" || tb["title"] != "ALFA" || tb["platform"] != false || tb["tenants"] != nil {
		t.Fatalf("a tenant's administrator is told %v", tb)
	}
	tb = tenantOf("sid:" + x.sid("Admin"))
	if tb["id"] != "" || tb["platform"] != true || len(tb["tenants"].([]any)) != 2 {
		t.Fatalf("the operator is told %v", tb)
	}
	// a platform user who is not an operator has no tenants to enter
	tb = tenantOf("sid:" + x.sid("ana@x.com"))
	if tb["platform"] != false || tb["tenants"] != nil {
		t.Fatalf("an ordinary platform user is told %v", tb)
	}
}

func TestTenantAPI_TheOperatorEntersATenant(t *testing.T) {
	x := setupTenants(t)
	admin := "sid:" + x.sid("Admin")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, admin)); got != "" {
		t.Fatalf("the platform space lists %q", got)
	}
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "alfa"}, admin), 200, "")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, admin)); got != "Cliente de alfa" {
		t.Fatalf("inside alfa the operator lists %q", got)
	}
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": ""}, admin), 200, "")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, admin)); got != "" {
		t.Fatalf("back in the platform space the operator lists %q", got)
	}
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "nobody"}, admin), 404, "")

	// with an API key, per request
	key := "token:" + x.apiKey("Admin")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, key, "X-DDCore-Tenant", "beta")); got != "Cliente de beta" {
		t.Fatalf("the operator's key, naming beta, lists %q", got)
	}

	// a platform System Manager is an operator and keeps the role inside
	root := "sid:" + x.sid("root@x.com")
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "beta"}, root), 200, "")
	if got := ids(x.call("GET", "/api/resource/User", nil, root)); got != betaUser+","+betaAdmin {
		t.Fatalf("a platform System Manager inside beta lists users %q", got)
	}

	// nobody else enters anything
	for _, u := range []string{alfaAdmin, alfaUser, "ana@x.com"} {
		x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "beta"}, "sid:"+x.sid(u)), 403, "PermissionError")
	}
}

func TestTenantAPI_ATenantAdministratorIsNotTheOperator(t *testing.T) {
	x := setupTenants(t)
	sid := "sid:" + x.sid(alfaAdmin)
	x.expect(x.call("GET", "/api/health/report", nil, sid), 403, "PermissionError")
	if r := x.mcpCall("token:" + x.apiKey(alfaAdmin)); r.Status != 403 {
		t.Fatalf("a tenant's System Manager reached MCP: %d %s", r.Status, r.Raw)
	}
	x.expect(x.call("GET", "/api/resource/Site%20Tenant", nil, sid), 403, "PermissionError")
	// but administers its own tenant: users, and only its own
	r := x.call("GET", "/api/resource/User", nil, sid)
	x.expect(r, 200, "")
	if got := ids(r); got != alfaUser+","+alfaAdmin {
		t.Fatalf("alfa's administrator lists users %q", got)
	}
	// and its own jobs
	if err := x.e.Run(x.ctx, betaUser, func(c *engine.Ctx) error {
		_, err := c.Enqueue("demo.services.x.y", nil, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r = x.call("GET", "/api/jobs", nil, sid)
	x.expect(r, 200, "")
	if strings.Contains(r.Raw, "demo.services.x.y") {
		t.Fatalf("alfa's administrator sees beta's job: %s", r.Raw)
	}
	r = x.call("GET", "/api/jobs", nil, "sid:"+x.sid(betaAdmin))
	if !strings.Contains(r.Raw, "demo.services.x.y") {
		t.Fatalf("beta's administrator does not see beta's job: %s", r.Raw)
	}
}

func TestTenantAPI_EventsReachTheirTenantOnly(t *testing.T) {
	x := setupTenants(t)
	all := func(string, string) bool { return true }
	alfa := x.e.Events.SubscribeIn(alfaUser, "alfa", all)
	beta := x.e.Events.SubscribeIn(betaUser, "beta", all)
	x.expect(x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Novo"}, "sid:"+x.sid(alfaUser)), 200, "")
	if len(alfa) == 0 || len(beta) != 0 {
		t.Fatalf("alfa heard %d events, beta %d", len(alfa), len(beta))
	}
}
