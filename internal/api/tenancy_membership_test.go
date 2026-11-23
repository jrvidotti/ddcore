package api

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// One person in several tenants (#167), over HTTP.

// setupMembers is setupTenants with a third tenant, gama, and alfa's Gestor
// added to beta as its System Manager (and a Gestor) — the issue's example.
func setupMembers(t *testing.T) *env {
	t.Helper()
	x := setupTenants(t)
	x.asAdmin(func(c *engine.Ctx) error {
		d, _ := c.NewDoc("Site Tenant", engine.Doc{"slug": "gama", "title": "GAMA"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err := x.e.Run(engine.WithTenant(x.ctx, "beta"), "Admin", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": alfaUser, "full_name": "Ana",
			"roles": []any{map[string]any{"role": "System Manager"}, map[string]any{"role": "Gestor"}}})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return x
}

func (x *env) tenantBoot(auth string) map[string]any {
	x.t.Helper()
	r := x.call("GET", "/api/boot", nil, auth)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	site, _ := data["site"].(map[string]any)
	tb, _ := site["tenant"].(map[string]any)
	if tb == nil {
		x.t.Fatalf("boot has no tenant block: %s", r.Raw)
	}
	return tb
}

func tenantIDs(tb map[string]any) string {
	var out []string
	list, _ := tb["tenants"].([]any)
	for _, t := range list {
		m, _ := t.(map[string]any)
		out = append(out, m["id"].(string))
	}
	return strings.Join(out, ",")
}

func TestMembershipAPI_SignInLandsAndSwitches(t *testing.T) {
	x := setupMembers(t)
	// the issue's example: the second `user add` is not "User already exists"
	sid := "sid:" + x.sid(alfaUser)
	tb := x.tenantBoot(sid)
	if tb["id"] != "alfa" || tb["platform"] != false || tenantIDs(tb) != "alfa,beta" {
		t.Fatalf("first sign-in is told %v", tb)
	}
	r := x.call("GET", "/api/resource/Pessoa", nil, sid)
	if got := ids(r); got != "Cliente de alfa" {
		t.Fatalf("alfa lists %q", got)
	}
	// alfa's Gestor cannot read Users; beta's System Manager can
	x.expect(x.call("GET", "/api/resource/User", nil, sid), 403, "")
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "beta"}, sid), 200, "")
	if tb := x.tenantBoot(sid); tb["id"] != "beta" {
		t.Fatalf("after entering beta the boot says %v", tb)
	}
	r = x.call("GET", "/api/resource/Pessoa", nil, sid)
	if got := ids(r); got != "Cliente de beta" {
		t.Fatalf("in beta she lists %q", got)
	}
	x.expect(x.call("GET", "/api/resource/User", nil, sid), 200, "")
	// the next sign-in lands where she last worked
	if tb := x.tenantBoot("sid:" + x.sid(alfaUser)); tb["id"] != "beta" {
		t.Fatalf("a new sign-in lands in %v", tb["id"])
	}
}

func TestMembershipAPI_SwitcherListsOwnTenantsOnly(t *testing.T) {
	x := setupMembers(t)
	sid := "sid:" + x.sid(alfaUser)
	foreign := x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "gama"}, sid)
	missing := x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "zzz"}, sid)
	x.expect(foreign, 403, "")
	x.expect(missing, 403, "")
	if strings.ReplaceAll(msg(foreign), "gama", "?") != strings.ReplaceAll(msg(missing), "zzz", "?") {
		t.Fatalf("a tenant she is not in and one that does not exist are told apart: %q vs %q", msg(foreign), msg(missing))
	}
	// a person in one tenant has no list at all
	if tb := x.tenantBoot("sid:" + x.sid(betaUser)); tb["tenants"] != nil {
		t.Fatalf("a person in one tenant is told %v", tb)
	}
	// X-Tenant moves nobody but the operator
	r := x.call("GET", "/api/resource/Pessoa", nil, sid, "X-Tenant", "beta")
	if got := ids(r); got != "Cliente de alfa" {
		t.Fatalf("the header moved her: %q", got)
	}
}

func TestMembershipAPI_TenantAdministratorLearnsNothing(t *testing.T) {
	x := setupMembers(t)
	admin := "sid:" + x.sid(alfaAdmin)
	// alfa's User list and record are alfa's row
	r := x.call("GET", "/api/resource/User/"+alfaUser, nil, admin)
	x.expect(r, 200, "")
	if strings.Contains(r.Raw, "beta") || strings.Contains(r.Raw, "BETA") {
		t.Fatalf("alfa's record of her mentions beta: %s", r.Raw)
	}
	// the form is told she is not alfa's alone, and nothing more
	r = x.call("POST", "/api/method/core.services.users.accountStatus", map[string]any{"user": alfaUser}, admin)
	x.expect(r, 200, "")
	d, _ := r.Body["data"].(map[string]any)
	if d["canManagePassword"] != false || strings.Contains(strings.ToLower(r.Raw), "beta") {
		t.Fatalf("accountStatus: %s", r.Raw)
	}
	r = x.call("POST", "/api/method/core.services.users.sendPasswordReset", map[string]any{"user": alfaUser}, admin)
	x.expect(r, 403, "")
	if strings.Contains(strings.ToLower(r.Raw), "beta") {
		t.Fatalf("the refusal names beta: %s", r.Raw)
	}
	// impersonating her, alfa's administrator stays in alfa and sees no list
	x.expect(x.call("POST", "/api/impersonate", map[string]any{"user": alfaUser}, admin), 200, "")
	tb := x.tenantBoot(admin)
	if tb["id"] != "alfa" || tb["tenants"] != nil {
		t.Fatalf("impersonating her, the boot says %v", tb)
	}
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "beta"}, admin), 403, "")
	// a credential of another tenant's person: nothing to learn
	r = x.call("POST", "/api/method/core.services.users.accountStatus", map[string]any{"user": betaUser}, "sid:"+x.sid(alfaAdmin))
	if d, _ := r.Body["data"].(map[string]any); r.Status == 200 && d["hasPassword"] == true {
		t.Fatalf("alfa learnt beta's user has a password: %s", r.Raw)
	}
}

func TestMembershipAPI_MemberKeyIgnoresXTenant(t *testing.T) {
	x := setupMembers(t)
	var token string
	if err := x.e.Run(engine.WithTenant(x.ctx, "beta"), "Admin", func(c *engine.Ctx) error {
		res, err := x.e.CreateUserAPIKey(c, alfaUser, "ci", 0, "")
		if err == nil {
			token = res["key"].(string) + ":" + res["secret"].(string)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := x.call("GET", "/api/resource/Pessoa", nil, "token:"+token, "X-Tenant", "alfa")
	x.expect(r, 200, "")
	if got := ids(r); got != "Cliente de beta" {
		t.Fatalf("her beta key lists %q", got)
	}
	// beta holds the key: through it, nothing of her other tenants
	if tb := x.tenantBoot("token:" + token); tb["tenants"] != nil {
		t.Fatalf("her beta key is told about her other tenants: %v", tb)
	}
	r = x.call("POST", "/api/method/core.services.sessions.listMySessions", map[string]any{}, "token:"+token)
	if r.Status == 200 && strings.Contains(r.Raw, "lastSeen") {
		t.Fatalf("her beta key lists her sessions elsewhere: %s", r.Raw)
	}
}

func TestMembershipAPI_RevokeSessionsIsTenantScoped(t *testing.T) {
	x := setupMembers(t)
	inAlfa := "sid:" + x.sid(alfaUser)
	inBeta := "sid:" + x.sid(alfaUser)
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "beta"}, inBeta), 200, "")
	// beta's administrator ends her sessions: beta's
	r := x.call("POST", "/api/method/core.services.users.revokeUserSessions", map[string]any{"user": alfaUser}, "sid:"+x.sid(betaAdmin))
	x.expect(r, 200, "")
	x.e.Cache.Clear()
	if tb := x.call("GET", "/api/boot", nil, inBeta); tb.Status == 200 && strings.Contains(tb.Raw, `"user":"`+alfaUser) {
		t.Fatal("her session in beta survived beta revoking it")
	}
	x.expect(x.call("GET", "/api/resource/Pessoa", nil, inAlfa), 200, "")
}
