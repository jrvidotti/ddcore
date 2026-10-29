package api

import (
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func (x *env) bootOf(auth string) map[string]any {
	x.t.Helper()
	r := x.call("GET", "/api/boot", nil, auth)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	return data
}

func (x *env) count(sql string, args ...any) int {
	x.t.Helper()
	var n int
	if err := x.e.DB.Pool.QueryRow(x.ctx, sql, args...).Scan(&n); err != nil {
		x.t.Fatal(err)
	}
	return n
}

func impersonator(boot map[string]any) string {
	by, _ := boot["impersonatedBy"].(map[string]any)
	if by == nil {
		return ""
	}
	return by["id"].(string)
}

func TestImpersonate_TheAdministratorWorksAsTheUser(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	r := x.call("POST", "/api/impersonate", map[string]any{"user": "ana@x.com"}, admin)
	x.expect(r, 200, "")
	if home := r.Body["data"].(map[string]any)["home"]; home != "/app" {
		t.Fatalf("a System User lands on %v", home)
	}
	b := x.bootOf(admin)
	if b["user"] != "ana@x.com" || impersonator(b) != "Admin" {
		t.Fatalf("the impersonated boot says user %v, impersonated by %v", b["user"], b["impersonatedBy"])
	}

	// what is written is ana's, and says who really wrote it
	x.expect(x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Feita por Admin"}, admin), 200, "")
	x.expect(x.call("PUT", "/api/resource/Pessoa/Feita por Admin", map[string]any{"tipo": "PJ"}, admin), 200, "")
	if n := x.count(`SELECT count(*) FROM tab_pessoa WHERE id = 'Feita por Admin' AND owner = 'ana@x.com' AND modified_by = 'ana@x.com'`); n != 1 {
		t.Fatal("the row is not ana's")
	}
	if n := x.count(`SELECT count(*) FROM tab_version WHERE doc_id = 'Feita por Admin' AND owner = 'ana@x.com' AND data->>'impersonated_by' = 'Admin'`); n == 0 {
		t.Fatal("the Version does not name the administrator")
	}
	if n := x.count(`SELECT count(*) FROM tab_audit_event WHERE action = 'account.impersonate' AND actor = 'Admin' AND target_id = 'ana@x.com'`); n != 1 {
		t.Fatal("starting is not audited")
	}

	// the user's own sessions and last login are untouched
	if n := x.count(`SELECT count(*) FROM ddcore_session WHERE "user" = 'ana@x.com'`); n != 0 {
		t.Fatalf("ana has %d sessions", n)
	}

	x.expect(x.call("POST", "/api/impersonate/stop", nil, admin), 200, "")
	b = x.bootOf(admin)
	if b["user"] != "Admin" || b["impersonatedBy"] != nil {
		t.Fatalf("after stopping the boot says %v, %v", b["user"], b["impersonatedBy"])
	}
	if n := x.count(`SELECT count(*) FROM tab_audit_event WHERE action = 'account.impersonate_stop' AND actor = 'Admin' AND target_id = 'ana@x.com'`); n != 1 {
		t.Fatal("stopping is not audited")
	}
	x.expect(x.call("POST", "/api/impersonate/stop", nil, admin), 417, "ValidationError")
}

func TestImpersonate_Refusals(t *testing.T) {
	x := setup(t)
	root := "sid:" + x.sid("root@x.com")
	imp := func(auth, target string) resp {
		return x.call("POST", "/api/impersonate", map[string]any{"user": target}, auth)
	}
	x.expect(imp("sid:"+x.sid("ana@x.com"), "ze@x.com"), 403, "PermissionError")
	x.expect(imp(root, "Admin"), 403, "PermissionError")
	x.expect(imp(root, "root@x.com"), 417, "ValidationError")
	x.expect(imp(root, "nobody@x.com"), 404, "")
	x.expect(imp("token:"+x.apiKey("Admin"), "ze@x.com"), 417, "ValidationError")
	x.asAdmin(func(c *engine.Ctx) error { return c.SetValue("User", "bia@x.com", engine.Doc{"enabled": false}) })
	x.expect(imp(root, "bia@x.com"), 403, "PermissionError")
	if n := x.count(`SELECT count(*) FROM tab_audit_event WHERE action = 'account.impersonate' AND outcome = 'Denied' AND actor = 'ana@x.com'`); n != 1 {
		t.Fatal("a refused attempt is not audited")
	}

	// no impersonation inside another: Admin as root, a System Manager, may
	// not go on to impersonate ze
	admin := "sid:" + x.sid("Admin")
	x.expect(imp(admin, "root@x.com"), 200, "")
	x.expect(imp(admin, "ze@x.com"), 417, "ValidationError")

	// an API key of the same administrator is not impersonating anyone
	if b := x.bootOf("token:" + x.apiKey("Admin")); b["user"] != "Admin" {
		t.Fatalf("an API key works as %v", b["user"])
	}
}

func TestImpersonate_NothingOutlivesIt(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": "ze@x.com"}, admin), 200, "")
	x.expect(x.call("POST", "/api/method/core.services.api_keys.createMyAPIKey", map[string]any{"label": "x"}, admin), 403, "PermissionError")
	x.expect(x.call("POST", "/api/method/core.services.profile.changeMyPassword",
		map[string]any{"current": "segredo123", "password": "outrasenha123"}, admin), 403, "PermissionError")
	x.expect(x.call("POST", "/api/method/core.services.sessions.revokeMyOtherSessions", nil, admin), 403, "PermissionError")
}

func TestImpersonate_EndsWithTheAccountOrTheRole(t *testing.T) {
	x := setup(t)
	root := "sid:" + x.sid("root@x.com")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": "ze@x.com"}, root), 200, "")
	if b := x.bootOf(root); b["user"] != "ze@x.com" {
		t.Fatalf("root works as %v", b["user"])
	}
	// disabling the user ends every impersonation of them
	key := "token:" + x.apiKey("Admin")
	x.expect(x.call("PUT", "/api/resource/User/ze@x.com", map[string]any{"enabled": 0}, key), 200, "")
	if b := x.bootOf(root); b["user"] != "root@x.com" {
		t.Fatalf("after ze is disabled root works as %v", b["user"])
	}

	// losing the role ends it too
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": "ana@x.com"}, root), 200, "")
	x.expect(x.call("PUT", "/api/resource/User/root@x.com", map[string]any{"roles": []any{}}, key), 200, "")
	if b := x.bootOf(root); b["user"] != "root@x.com" || b["impersonatedBy"] != nil {
		t.Fatalf("after losing the role root works as %v", b["user"])
	}
	if n := x.count(`SELECT count(*) FROM ddcore_session WHERE "user" = 'root@x.com' AND data ? 'impersonate'`); n != 0 {
		t.Fatal("the session still holds the impersonation")
	}
}

func TestImpersonate_AWebsiteUserLandsOnThePortal(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "web@x.com", "full_name": "Web", "user_type": "Website User", "new_password": "segredo123"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	admin := "sid:" + x.sid("Admin")
	r := x.call("POST", "/api/impersonate", map[string]any{"user": "web@x.com"}, admin)
	x.expect(r, 200, "")
	if home := r.Body["data"].(map[string]any)["home"]; home != "/portal" {
		t.Fatalf("a Website User lands on %v", home)
	}
	b := x.bootOf(admin)
	if b["website"] != true || impersonator(b) != "Admin" {
		t.Fatalf("the portal boot is %v", b)
	}
	x.expect(x.call("GET", "/api/resource/Pessoa", nil, admin), 403, "PermissionError")
	x.expect(x.call("POST", "/api/impersonate/stop", nil, admin), 200, "")
}

func TestImpersonate_StaysInsideTheSpaces(t *testing.T) {
	x := setupTenants(t)
	chefe := "sid:" + x.sid(alfaAdmin)
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": betaUser}, chefe), 404, "")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": alfaUser}, chefe), 200, "")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, chefe)); got != "Cliente de alfa" {
		t.Fatalf("as alfa's user the administrator lists %q", got)
	}

	// the operator reaches any tenant's user, from the platform space or
	// from inside the tenant it entered
	root := "sid:" + x.sid("root@x.com")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": betaUser}, root), 200, "")
	if got := ids(x.call("GET", "/api/resource/Pessoa", nil, root)); got != "Cliente de beta" {
		t.Fatalf("as beta's user the operator lists %q", got)
	}
	x.expect(x.call("POST", "/api/impersonate/stop", nil, root), 200, "")
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "alfa"}, root), 200, "")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": betaUser}, root), 404, "")
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": alfaUser}, root), 200, "")
	if b := x.bootOf(root); b["user"] != alfaUser {
		t.Fatalf("root works as %v", b["user"])
	}

	// a platform user who is not an operator reaches no tenant
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": alfaUser}, "sid:"+x.sid("ana@x.com")), 403, "PermissionError")
}
