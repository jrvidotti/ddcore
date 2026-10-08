package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// credentialApp is the test app with a credential sign-in provider "ext"
// (#115). verify accepts the password "ok-<username in lower case>", throws
// for the username "boom", and hands a token to afterSignIn, which keeps it
// on the User — or throws, for the username "failafter". A tenant is opted
// out by a Pessoa named "no-ext".
func credentialApp(t *testing.T) string {
	t.Helper()
	dir := testApp(t)
	w := func(rel, src string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"],
  auth: { providers: { ext: {
    label: "External",
    userField: "ext_username",
    enabled() { return !ddcore.db.exists("Pessoa", "no-ext"); },
    verify({ username, password }) {
      if (username === "boom") throw new Error("the other system is down");
      if (password !== "ok-" + username.toLowerCase()) return { ok: false, reason: "wrong password" };
      return { ok: true, subject: username.toUpperCase(), data: { token: "t-" + username.toLowerCase() } };
    },
    afterSignIn({ user, subject, data }) {
      if (subject === "FAILAFTER") throw new Error("could not keep the token");
      ddcore.db.setValue("User", user, "ext_token", data.token);
    },
  } } } });`)
	w("extensions/user.extend.ts", `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("User", { fields: [
  { fieldname: "ext_username", fieldtype: "Data", label: "External username" },
  { fieldname: "ext_token", fieldtype: "Data", label: "External token" },
] });`)
	return dir
}

// addExtUser creates a User linked to an external username in a space.
func (x *env) addExtUser(tenant, email, username string, enabled bool) {
	x.t.Helper()
	ctx := x.ctx
	if tenant != "" {
		ctx = engine.WithTenant(ctx, tenant)
	}
	err := x.e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": email, "full_name": email, "ext_username": username,
			"enabled": enabled, "roles": []any{map[string]any{"role": "Gestor"}}})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err != nil {
		x.t.Fatal(err)
	}
}

func (x *env) credLogin(tenant, usr, pwd string) resp {
	x.t.Helper()
	return x.call("POST", "/api/auth/credentials/ext/login", map[string]any{"tenant": tenant, "usr": usr, "pwd": pwd}, "")
}

func sidOf(r resp) string {
	for _, c := range r.Header.Values("Set-Cookie") {
		if v, ok := strings.CutPrefix(c, "sid="); ok {
			v, _, _ = strings.Cut(v, ";")
			return v
		}
	}
	return ""
}

// lastCredentialAudit is the newest account.login_credentials event in a space.
func (x *env) lastCredentialAudit(tenant string) map[string]any {
	x.t.Helper()
	var row map[string]any
	err := x.e.InSpace(x.ctx, tenant, func(q db.Querier) error {
		rows, err := db.Select(x.ctx, q, `SELECT actor, outcome, target_id, detail FROM tab_audit_event
			WHERE action = 'account.login_credentials' ORDER BY creation DESC LIMIT 1`)
		if len(rows) > 0 {
			row = rows[0]
		}
		return err
	})
	if err != nil {
		x.t.Fatal(err)
	}
	if row == nil {
		x.t.Fatalf("no account.login_credentials event in %q", tenant)
	}
	return row
}

func auditDetail(row map[string]any) map[string]any {
	var d map[string]any
	switch v := row["detail"].(type) {
	case string:
		json.Unmarshal([]byte(v), &d)
	case []byte:
		json.Unmarshal(v, &d)
	case map[string]any:
		d = v
	}
	return d
}

func TestCredentialLogin_SignsInTheLinkedUserOfTheChosenTenant(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	x.addExtUser("beta", "joao@beta.invalid", "JOAO", true)

	// boot tells the sign-in screen about the provider, label translated
	r := x.call("GET", "/api/boot", nil, "")
	x.expect(r, 200, "")
	login := r.Body["data"].(map[string]any)["site"].(map[string]any)["login"].(map[string]any)
	creds, _ := login["credentials"].([]any)
	if len(creds) != 1 || creds[0].(map[string]any)["id"] != "ext" || creds[0].(map[string]any)["label"] != "External" {
		t.Fatalf("boot offers %v", login["credentials"])
	}

	r = x.credLogin("beta", "joao", "ok-joao")
	x.expect(r, 200, "")
	sid := sidOf(r)
	if sid == "" {
		t.Fatalf("no session cookie: %v", r.Header)
	}
	b := x.call("GET", "/api/boot", nil, "sid:"+sid)
	x.expect(b, 200, "")
	data := b.Body["data"].(map[string]any)
	if u := data["user"]; u != "joao@beta.invalid" {
		t.Fatalf("signed in as %v", u)
	}
	if tb := data["site"].(map[string]any)["tenant"].(map[string]any); tb["id"] != "beta" {
		t.Fatalf("signed in to %v", tb)
	}
	// afterSignIn kept what verify returned, in the user's own tenant
	var token string
	if err := x.e.InSpace(x.ctx, "beta", func(q db.Querier) error {
		return q.QueryRow(x.ctx, `SELECT coalesce(ext_token, '') FROM tab_user WHERE id = 'joao@beta.invalid'`).Scan(&token)
	}); err != nil || token != "t-joao" {
		t.Fatalf("afterSignIn left %q (%v)", token, err)
	}
	ev := x.lastCredentialAudit("beta")
	d := auditDetail(ev)
	if ev["outcome"] != "Allowed" || ev["actor"] != "joao@beta.invalid" || d["tenant"] != "beta" || d["username"] != "joao" || d["subject"] != "JOAO" {
		t.Fatalf("audit %v %v", ev, d)
	}
	for k := range d {
		if strings.Contains(strings.ToLower(k), "pass") || k == "data" || k == "token" {
			t.Fatalf("the audit detail carries %q: %v", k, d)
		}
	}
}

func TestCredentialLogin_Refusals(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	x.addExtUser("alfa", "off@alfa.invalid", "OFF", false)
	x.addExtUser("alfa", "failafter@alfa.invalid", "FAILAFTER", true)

	x.expect(x.credLogin("alfa", "joao", "wrong"), 401, "AuthenticationError")
	if d := auditDetail(x.lastCredentialAudit("alfa")); d["reason"] != "invalid: wrong password" {
		t.Fatalf("a wrong password is audited as %v", d)
	}
	// the right password, but nobody here is linked to that username
	r := x.credLogin("alfa", "maria", "ok-maria")
	x.expect(r, 401, "AuthenticationError")
	if !strings.Contains(r.Raw, "Ask an administrator") {
		t.Fatalf("no account answers %s", r.Raw)
	}
	x.expect(x.credLogin("alfa", "off", "ok-off"), 401, "AuthenticationError")
	if d := auditDetail(x.lastCredentialAudit("alfa")); d["reason"] != "disabled" {
		t.Fatalf("a disabled user is audited as %v", d)
	}
	// the other system is down: unavailable, and the account is not counted
	x.expect(x.credLogin("alfa", "boom", "x"), 503, "")
	// afterSignIn throwing refuses the sign-in and leaves no session behind
	r = x.credLogin("alfa", "failafter", "ok-failafter")
	x.expect(r, 503, "")
	if sidOf(r) != "" {
		t.Fatal("a refused sign-in set a session cookie")
	}
	var n int
	x.e.System(x.ctx, func(q db.Querier) error {
		return q.QueryRow(x.ctx, `SELECT count(*) FROM ddcore_session WHERE "user" = 'failafter@alfa.invalid'`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("%d session(s) left for a refused sign-in", n)
	}
	// one username linked to two accounts of the tenant: refused, not guessed
	x.addExtUser("alfa", "dup1@alfa.invalid", "DUP", true)
	x.addExtUser("alfa", "dup2@alfa.invalid", "dup", true)
	x.expect(x.credLogin("alfa", "dup", "ok-dup"), 401, "AuthenticationError")
	if d := auditDetail(x.lastCredentialAudit("alfa")); d["reason"] != "ambiguous" {
		t.Fatalf("an ambiguous link is audited as %v", d)
	}
	// a tenant that does not exist answers like a wrong password, and is
	// audited in the platform space, where somebody reads it
	x.expect(x.credLogin("gama", "joao", "ok-joao"), 401, "AuthenticationError")
	if d := auditDetail(x.lastCredentialAudit("")); d["reason"] != "tenant" || d["tenant"] != "gama" {
		t.Fatalf("an unknown tenant is audited as %v", d)
	}
	// tenancy on: the tenant is required
	x.expect(x.credLogin("", "joao", "ok-joao"), 417, "ValidationError")
	x.expect(x.call("POST", "/api/auth/credentials/nope/login", map[string]any{"tenant": "alfa", "usr": "a", "pwd": "b"}, ""), 404, "")
	x.expect(x.call("GET", "/api/auth/credentials/nope/tenants", nil, ""), 404, "")
}

func TestCredentialLogin_ThrottleComesBeforeVerify(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.expect(x.credLogin("alfa", "joao", "wrong"), 401, "AuthenticationError")
	}
	// locked: even the right password is refused, without asking verify
	x.expect(x.credLogin("alfa", "joao", "ok-joao"), 429, "")
	// the same username in another tenant is another account
	x.addExtUser("beta", "joao@beta.invalid", "JOAO", true)
	x.e.ClearAttempts(x.ctx, "loginip:127.0.0.1")
	x.expect(x.credLogin("beta", "joao", "ok-joao"), 200, "")

	// unlocking the user clears the provider's counter too
	admin := "sid:" + x.sid(alfaAdmin)
	x.expect(x.call("POST", "/api/method/core.services.users.unlockUser", map[string]any{"user": "joao@alfa.invalid"}, admin), 200, "")
	x.expect(x.credLogin("alfa", "joao", "ok-joao"), 200, "")
}

// tenantTitle asks for one organization's sign-in page.
func (x *env) tenantTitle(tenant string) resp {
	x.t.Helper()
	return x.call("GET", "/api/auth/credentials/ext/tenants/"+tenant, nil, "")
}

func TestCredentialLogin_TenantsAreAskedForNotListed(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("beta", "joao@beta.invalid", "JOAO", true)
	// the list endpoint lists nothing (#117), it only says there are tenants
	r := x.call("GET", "/api/auth/credentials/ext/tenants", nil, "")
	x.expect(r, 200, "")
	if got := ids(r); got != "" || r.Body["tenancy"] != true {
		t.Fatalf("the list endpoint answers %s", r.Raw)
	}
	// boot tells the sign-in screen to ask for the organization
	b := x.call("GET", "/api/boot", nil, "")
	if login := b.Body["data"].(map[string]any)["site"].(map[string]any)["login"].(map[string]any); login["tenancy"] != true {
		t.Fatalf("boot login %v", login)
	}
	// one organization, by its id, typed in any case
	r = x.tenantTitle("ALFA")
	x.expect(r, 200, "")
	if d := r.Body["data"].(map[string]any); d["id"] != "alfa" || d["title"] != "ALFA" {
		t.Fatalf("alfa answers %s", r.Raw)
	}
	x.expect(x.tenantTitle("gama"), 404, "")
	x.expect(x.tenantTitle("not a tenant!"), 404, "")

	// beta opts out: its page is a 404 like a missing tenant, and its sign-in refused
	if err := x.e.Run(engine.WithTenant(x.ctx, "beta"), "Admin", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("Pessoa", engine.Doc{"nome": "no-ext"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	x.e.Cache.Clear()
	x.expect(x.tenantTitle("beta"), 404, "")
	x.expect(x.credLogin("beta", "joao", "ok-joao"), 401, "AuthenticationError")
	if d := auditDetail(x.lastCredentialAudit("beta")); d["reason"] != "not_offered" {
		t.Fatalf("an opted-out tenant is audited as %v", d)
	}
	// a disabled tenant is not found either
	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `UPDATE tab_site_tenant SET enabled = false WHERE id = 'alfa'`)
		return err
	})
	x.e.Cache.Clear()
	x.expect(x.tenantTitle("alfa"), 404, "")
}

func TestCredentialLogin_GuessingTenantsIsThrottled(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts*5; i++ {
		x.expect(x.tenantTitle(fmt.Sprintf("guess%d", i)), 404, "")
	}
	// even a real one, once the address has guessed too much
	x.expect(x.tenantTitle("alfa"), 429, "")
	// a sign-in with the password is not held by the guesses
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	x.expect(x.credLogin("alfa", "joao", "ok-joao"), 200, "")
}

func TestCredentialLogin_PasswordLoginOffDoesNotBlockIt(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	off := false
	x.e.Cfg.Auth.PasswordLogin = &off
	x.expect(x.credLogin("alfa", "joao", "ok-joao"), 200, "")
}

func TestCredentialLogin_WithoutTenancy(t *testing.T) {
	x := setupSite(t, credentialApp(t), false)
	x.addExtUser("", "joao@site.invalid", "JOAO", true)
	r := x.call("GET", "/api/auth/credentials/ext/tenants", nil, "")
	x.expect(r, 200, "")
	if got := ids(r); got != "" || r.Body["tenancy"] != false {
		t.Fatalf("without tenancy the list is %q: %s", got, r.Raw)
	}
	r = x.tenantTitle("alfa")
	x.expect(r, 200, "")
	if r.Body["data"] != nil || r.Body["tenancy"] != false {
		t.Fatalf("without tenancy a tenant answers %s", r.Raw)
	}
	x.expect(x.credLogin("", "joao", "ok-joao"), 200, "")
	if ev := x.lastCredentialAudit(""); ev["outcome"] != "Allowed" {
		t.Fatalf("audit %v", ev)
	}
}

func TestCredentialLogin_PlaceholderAddressGetsNoMail(t *testing.T) {
	x := setupTenantsWith(t, credentialApp(t))
	x.addExtUser("alfa", "joao@alfa.invalid", "JOAO", true)
	// the password recovery of a placeholder address sends nothing, and says
	// the same as for any other address
	x.expect(x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": "joao@alfa.invalid"}, ""), 200, "")
	var n int
	x.e.InSpace(x.ctx, "alfa", func(q db.Querier) error {
		return q.QueryRow(x.ctx, `SELECT count(*) FROM tab_email_delivery WHERE "to" LIKE '%.invalid%'`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("%d message(s) queued to a placeholder address", n)
	}
	// mail to placeholder addresses only is skipped, not refused
	err := x.e.Run(engine.WithTenant(x.ctx, "alfa"), "Admin", func(c *engine.Ctx) error {
		out, err := c.QueueMail(engine.MailRequest{Template: engine.MailTemplateReset, To: []string{"joao@alfa.invalid"}})
		if err == nil && out["skipped"] != true {
			t.Errorf("queued %v", out)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// nor can it be invited
	admin := "sid:" + x.sid(alfaAdmin)
	r := x.call("POST", "/api/method/core.services.users.invite",
		map[string]any{"email": "maria@alfa.invalid", "fullName": "Maria"}, admin)
	x.expect(r, 417, "ValidationError")
}
