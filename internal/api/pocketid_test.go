package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// fakePocketID is the part of PocketID's admin API that provisioning uses,
// served next to the fake OpenID provider so that both share one issuer. Its
// PUT /api/users/{id} replaces the whole user, as the real one does: a body
// without a username is refused.
type fakePocketID struct {
	mu       sync.Mutex
	users    map[string]map[string]any
	groups   map[string]string // id → name
	member   map[string][]string
	ttls     []float64
	puts     []map[string]any
	failNext int // status to answer the next POST /api/users with
	// friendly holds the friendlyName of each group created through the API;
	// failGroup is the status to answer the next POST /api/user-groups with.
	friendly  map[string]string
	failGroup int
	calls     int
}

var fakeUsernameOK = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_.@-]*[a-zA-Z0-9])?$`)

func newFakePocketID(t *testing.T, idp *fakeIdP) *fakePocketID {
	f := &fakePocketID{users: map[string]map[string]any{}, member: map[string][]string{}, friendly: map[string]string{},
		groups: map[string]string{"g-gestores": "erp-gestores", "g-sm": "erp-sm", "g-outros": "outros"}}
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			f.calls++
			f.mu.Unlock()
			if r.Header.Get("X-API-Key") != "pk-test" {
				w.WriteHeader(401)
				json.NewEncoder(w).Encode(map[string]any{"error": "You are not signed in"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	m := idp.mux
	m.HandleFunc("GET /api/users", auth(func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("search"))
		var out []any
		for _, u := range f.users {
			if strings.Contains(strings.ToLower(fmt.Sprint(u["email"])), q) {
				out = append(out, f.view(u))
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": out})
	}))
	m.HandleFunc("GET /api/users/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		u := f.users[r.PathValue("id")]
		if u == nil {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(f.view(u))
	}))
	m.HandleFunc("POST /api/users", auth(func(w http.ResponseWriter, r *http.Request) {
		if f.failNext != 0 {
			w.WriteHeader(f.failNext)
			json.NewEncoder(w).Encode(map[string]any{"error": "Email is already in use"})
			f.failNext = 0
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if !fakeUsernameOK.MatchString(fmt.Sprint(b["username"])) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{"error": "invalid username"})
			return
		}
		b["id"] = fmt.Sprintf("pid-%d", len(f.users)+1)
		f.users[b["id"].(string)] = b
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(f.view(b))
	}))
	m.HandleFunc("PUT /api/users/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if f.users[id] == nil {
			w.WriteHeader(404)
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if b["username"] == nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{"error": "username is required"})
			return
		}
		f.puts = append(f.puts, b)
		b["id"] = id
		f.users[id] = b
		json.NewEncoder(w).Encode(f.view(b))
	}))
	m.HandleFunc("PUT /api/users/{id}/user-groups", auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			IDs []string `json:"userGroupIds"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		f.member[r.PathValue("id")] = b.IDs
		json.NewEncoder(w).Encode(f.view(f.users[r.PathValue("id")]))
	}))
	m.HandleFunc("POST /api/users/{id}/one-time-access-token", auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			TTL float64 `json:"ttl"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		f.ttls = append(f.ttls, b.TTL)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"token": "tok-" + r.PathValue("id")})
	}))
	m.HandleFunc("GET /api/user-groups", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		var out []any
		for id, name := range f.groups {
			if strings.Contains(name, q) {
				out = append(out, map[string]any{"id": id, "name": name, "friendlyName": name})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": out})
	}))
	m.HandleFunc("POST /api/user-groups", auth(func(w http.ResponseWriter, r *http.Request) {
		if f.failGroup != 0 {
			w.WriteHeader(f.failGroup)
			f.failGroup = 0
			json.NewEncoder(w).Encode(map[string]any{"error": "You don't have permission"})
			return
		}
		var b struct {
			Name         string `json:"name"`
			FriendlyName string `json:"friendlyName"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		id := "g-" + b.Name
		f.groups[id], f.friendly[b.Name] = b.Name, b.FriendlyName
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": id, "name": b.Name, "friendlyName": b.FriendlyName})
	}))
	return f
}

func (f *fakePocketID) view(u map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range u {
		out[k] = v
	}
	var gs []any
	for _, id := range f.member[fmt.Sprint(u["id"])] {
		gs = append(gs, map[string]any{"id": id, "name": f.groups[id]})
	}
	out["userGroups"] = gs
	return out
}

func (f *fakePocketID) byEmail(email string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u["email"] == email {
			return u
		}
	}
	return nil
}

func (f *fakePocketID) groupsOf(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, g := range f.member[id] {
		out = append(out, f.groups[g])
	}
	sort.Strings(out)
	return out
}

// pocketIDEnv is a site whose provider "fake" is a PocketID with an admin API
// key, and whose groups erp-gestores and erp-sm map to Gestor and System
// Manager.
func pocketIDEnv(t *testing.T) (*env, *fakeIdP, *fakePocketID) {
	x, idp := ssoEnv(t)
	pid := newFakePocketID(t, idp)
	x.e.Cfg.OIDC[0].Kind = config.OIDCKindPocketID
	x.e.Cfg.OIDC[0].APIKey = "pk-test"
	x.e.Cfg.OIDC[0].Scopes = append(x.e.Cfg.OIDC[0].Scopes, "groups")
	x.e.Cfg.Auth.SSO = map[string]config.SSOPolicy{"fake": {GroupRoles: map[string][]string{
		"erp-gestores": {"Gestor"}, "erp-sm": {"System Manager"},
	}}}
	return x, idp, pid
}

func (x *env) rolesOf(user string) []string {
	x.t.Helper()
	var out []string
	x.asAdmin(func(c *engine.Ctx) error {
		rows, err := c.SQL(`SELECT role FROM tab_has_role WHERE parent = $1 AND parenttype = 'User' ORDER BY role`, []any{user})
		for _, r := range rows {
			out = append(out, fmt.Sprint(r["role"]))
		}
		return err
	})
	return out
}

func (x *env) idpJobs(user string) int {
	return x.countSQL(`SELECT count(*) FROM ddcore_job WHERE method = 'core.services.idp.sync' AND args->>'user' = $1`, user)
}

func TestPocketID_InviteCreatesTheAccountAndMailsItsLink(t *testing.T) {
	x, idp, pid := pocketIDEnv(t)
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "nova@x.com", "fullName": "Nova Pessoa", "roles": []string{"Gestor"}})
	x.expect(r, 200, "")
	d, _ := r.Body["data"].(map[string]any)
	u := pid.byEmail("nova@x.com")
	if u == nil {
		t.Fatalf("no account in PocketID: %s", r.Raw)
	}
	if u["username"] != "nova@x.com" || u["emailVerified"] != true || u["firstName"] != "Nova" || u["lastName"] != "Pessoa" || u["displayName"] != "Nova Pessoa" {
		t.Errorf("account: %v", u)
	}
	if g := pid.groupsOf(u["id"].(string)); strings.Join(g, ",") != "erp-gestores" {
		t.Errorf("groups: %v", g)
	}
	// the log transport hands the link back: PocketID's, not a password link
	if d == nil || d["link"] != idp.srv.URL+"/lc/tok-"+u["id"].(string) {
		t.Errorf("link: %s", r.Raw)
	}
	if len(pid.ttls) != 1 || pid.ttls[0] != 72*3600 {
		t.Errorf("ttl: %v", pid.ttls)
	}
	if n := x.countTokens("nova@x.com"); n != 0 {
		t.Errorf("a password token was issued as well: %d", n)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.invite' AND detail->>'provider' = 'fake'`); n != 1 {
		t.Errorf("invite audit: %d", n)
	}
	// the invitation's own role insert has nothing left to push
	x.expect(x.callAs("root@x.com", "core.services.users.resendInvite", map[string]any{"user": "nova@x.com"}), 200, "")
	if len(pid.users) != 1 || len(pid.ttls) != 2 {
		t.Errorf("resend made a second account or no new code: %d users, %v", len(pid.users), pid.ttls)
	}
}

func TestPocketID_InviteReusesAnAccountAndKeepsItsOtherGroups(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	pid.users["pid-9"] = map[string]any{"id": "pid-9", "username": "velha", "email": "velha@x.com", "disabled": true, "emailVerified": true}
	pid.member["pid-9"] = []string{"g-outros", "g-sm"}
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "velha@x.com", "fullName": "Velha", "roles": []string{"Gestor"}})
	x.expect(r, 200, "")
	if len(pid.users) != 1 {
		t.Fatalf("a second account was made: %v", pid.users)
	}
	u := pid.users["pid-9"]
	if u["disabled"] != false || u["username"] != "velha" {
		t.Errorf("the account was not re-enabled as it was: %v", u)
	}
	// erp-sm is mapped and the User is no System Manager: removed; outros is
	// not mapped: kept
	if g := pid.groupsOf("pid-9"); strings.Join(g, ",") != "erp-gestores,outros" {
		t.Errorf("groups: %v", g)
	}
}

func TestPocketID_ARefusalFailsTheInvitation(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	pid.failNext = 400
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "nova@x.com", "fullName": "Nova"})
	x.expect(r, 417, "ValidationError")
	if !strings.Contains(r.Raw, "Email is already in use") {
		t.Errorf("the provider's reason is lost: %s", r.Raw)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_user WHERE id = 'nova@x.com'`); n != 0 {
		t.Errorf("the User survived a failed invitation")
	}
}

func TestPocketID_InviteCreatesAMissingMappedGroup(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	x.e.Cfg.Auth.SSO["fake"].GroupRoles["erp-novo"] = []string{"Gestor", "System Manager"}
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "nova@x.com", "fullName": "Nova", "roles": []string{"Gestor", "System Manager"}})
	x.expect(r, 200, "")
	if f := pid.friendly["erp-novo"]; f != "Gestor, System Manager" {
		t.Fatalf("group not created, or its friendly name is %q: %s", f, r.Raw)
	}
	u := pid.byEmail("nova@x.com")
	if g := pid.groupsOf(u["id"].(string)); strings.Join(g, ",") != "erp-gestores,erp-novo,erp-sm" {
		t.Errorf("groups: %v", g)
	}
	if len(pid.friendly) != 1 {
		t.Errorf("an existing group was created again: %v", pid.friendly)
	}
}

func TestPocketID_AGroupThatCannotBeCreatedFailsTheInvitation(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	x.e.Cfg.Auth.SSO["fake"].GroupRoles["erp-novo"] = []string{"Gestor"}
	pid.failGroup = 403
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "nova@x.com", "fullName": "Nova", "roles": []string{"Gestor"}})
	x.expect(r, 417, "ValidationError")
	for _, want := range []string{"erp-novo", "You don't have permission", "Create the group in PocketID"} {
		if !strings.Contains(r.Raw, want) {
			t.Errorf("the message lacks %q: %s", want, r.Raw)
		}
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_user WHERE id = 'nova@x.com'`); n != 0 {
		t.Errorf("the User survived a failed invitation")
	}
}

func TestPocketID_ProbeListsMappedGroupsToCreate(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	pol := x.e.Cfg.Auth.SSO["fake"]
	pol.GroupRoles["erp-novo"] = []string{"Gestor"}
	probs, absent := engine.ProbePocketID(x.ctx, x.e.Cfg.OIDC[0], pol)
	if len(probs) != 0 || strings.Join(absent, ",") != "erp-novo" {
		t.Errorf("problems %v, absent %v", probs, absent)
	}
	if len(pid.friendly) != 0 {
		t.Errorf("the probe created a group: %v", pid.friendly)
	}
}

func TestPocketID_AWebsiteUserIsInvitedWithAPassword(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	r := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "cliente@x.com", "fullName": "Cliente", "userType": "Website User"})
	x.expect(r, 200, "")
	if pid.calls != 0 || x.countTokens("cliente@x.com") != 1 {
		t.Errorf("a portal user went through PocketID: %d calls", pid.calls)
	}
}

func TestPocketID_MappedRoleChangesArePushedAsGroups(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	pid.users["pid-ana"] = map[string]any{"id": "pid-ana", "username": "ana", "email": "ana@x.com"}
	pid.member["pid-ana"] = []string{"g-gestores", "g-outros"}
	// a change to an unmapped field queues nothing
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ana@x.com", engine.Doc{"full_name": "Ana"})
	})
	if n := x.idpJobs("ana@x.com"); n != 0 {
		t.Fatalf("jobs after an unmapped change: %d", n)
	}
	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		d["roles"] = []any{map[string]any{"role": "System Manager"}}
		_, err = c.Save(d, engine.SaveOpts{})
		return err
	})
	if n := x.idpJobs("ana@x.com"); n != 1 {
		t.Fatalf("jobs after a mapped change: %d", n)
	}
	if _, err := x.e.RunJob(x.ctx, "Admin", "core.services.idp.sync", map[string]any{"user": "ana@x.com"}); err != nil {
		t.Fatal(err)
	}
	if g := pid.groupsOf("pid-ana"); strings.Join(g, ",") != "erp-sm,outros" {
		t.Errorf("groups: %v", g)
	}
	// the sync creates a mapped group PocketID does not have yet
	x.e.Cfg.Auth.SSO["fake"].GroupRoles["erp-novo"] = []string{"System Manager"}
	if _, err := x.e.RunJob(x.ctx, "Admin", "core.services.idp.sync", map[string]any{"user": "ana@x.com"}); err != nil {
		t.Fatal(err)
	}
	if g := pid.groupsOf("pid-ana"); strings.Join(g, ",") != "erp-novo,erp-sm,outros" {
		t.Errorf("groups after a new mapping: %v", g)
	}
}

func TestPocketID_DisableAtTheProviderOnlyWhenAsked(t *testing.T) {
	x, _, pid := pocketIDEnv(t)
	pid.users["pid-ana"] = map[string]any{"id": "pid-ana", "username": "ana", "email": "ana@x.com", "firstName": "Ana", "disabled": false}
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ana@x.com", engine.Doc{"enabled": false})
	})
	if len(pid.puts) != 0 {
		t.Fatalf("disabling here changed PocketID on its own: %v", pid.puts)
	}
	x.expect(x.callAs("ze@x.com", "core.services.users.setProviderDisabled", map[string]any{"user": "bia@x.com", "disabled": true}), 403, "PermissionError")

	r := x.callAs("root@x.com", "core.services.users.setProviderDisabled", map[string]any{"user": "ana@x.com", "disabled": true})
	x.expect(r, 200, "")
	if d, _ := r.Body["data"].(map[string]any); d == nil || d["found"] != true {
		t.Errorf("result: %s", r.Raw)
	}
	// the PUT replaces the whole user: what it did not change must be sent back
	if len(pid.puts) != 1 || pid.puts[0]["disabled"] != true || pid.puts[0]["username"] != "ana" || pid.puts[0]["firstName"] != "Ana" {
		t.Errorf("put: %v", pid.puts)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.idp_disable' AND target_id='ana@x.com'`); n != 1 {
		t.Errorf("audit: %d", n)
	}

	// a deleted User is found by address
	x.asAdmin(func(c *engine.Ctx) error { return c.Delete("User", "ana@x.com", true, false) })
	r = x.callAs("root@x.com", "core.services.users.setProviderDisabled", map[string]any{"email": "ana@x.com", "disabled": false})
	x.expect(r, 200, "")
	if pid.users["pid-ana"]["disabled"] != false {
		t.Errorf("not re-enabled by address: %v", pid.users["pid-ana"])
	}
	r = x.callAs("root@x.com", "core.services.users.setProviderDisabled", map[string]any{"email": "ninguem@x.com", "disabled": true})
	if d, _ := r.Body["data"].(map[string]any); d == nil || d["found"] != false {
		t.Errorf("unknown address: %s", r.Raw)
	}
}

func TestPocketID_SignInSetsTheMappedRolesFromGroups(t *testing.T) {
	x, idp, _ := pocketIDEnv(t)
	x.asAdmin(func(c *engine.Ctx) error {
		r, _ := c.NewDoc("Role", engine.Doc{"role_name": "Extra"})
		if _, err := c.Insert(r, engine.SaveOpts{}); err != nil {
			return err
		}
		d, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		d["roles"] = []any{map[string]any{"role": "Gestor"}, map[string]any{"role": "Extra"}}
		_, err = c.Save(d, engine.SaveOpts{})
		return err
	})
	// the setup above queued its own push; only the sign-in's would count
	if _, err := x.e.DB.Pool.Exec(x.ctx, `DELETE FROM ddcore_job`); err != nil {
		t.Fatal(err)
	}

	if _, sid := x.ssoLogin(idp, "/app", map[string]any{"groups": []string{"erp-sm", "desconhecido"}}); sid == "" {
		t.Fatal("sign-in failed")
	}
	// Gestor is mapped and not granted: gone; Extra is not mapped: kept
	if r := x.rolesOf("ana@x.com"); strings.Join(r, ",") != "Extra,System Manager" {
		t.Errorf("roles: %v", r)
	}
	if n := x.idpJobs("ana@x.com"); n != 0 {
		t.Errorf("the sign-in pushed its own groups back: %d", n)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='role.revoke' AND target_id='ana@x.com' AND detail->>'role'='Gestor'`); n != 1 {
		t.Errorf("revoke audit: %d", n)
	}

	// no claim at all is not "in no group"
	if _, sid := x.ssoLogin(idp, "/app", nil); sid == "" {
		t.Fatal("sign-in failed")
	}
	if r := x.rolesOf("ana@x.com"); strings.Join(r, ",") != "Extra,System Manager" {
		t.Errorf("roles after a sign-in without the claim: %v", r)
	}
	// an empty list is: every mapped role goes
	if _, sid := x.ssoLogin(idp, "/app", map[string]any{"groups": []string{}}); sid == "" {
		t.Fatal("sign-in failed")
	}
	if r := x.rolesOf("ana@x.com"); strings.Join(r, ",") != "Extra" {
		t.Errorf("roles after an empty groups claim: %v", r)
	}
}
