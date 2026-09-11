package api

import (
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func (x *env) callAs(user, method string, args any) resp {
	x.t.Helper()
	return x.call("POST", "/api/method/"+method, args, "sid:"+x.sid(user))
}

func TestSEC04_GetMyProfile(t *testing.T) {
	x := setup(t)
	r := x.callAs("ana@x.com", "core.services.profile.getMyProfile", nil)
	x.expect(r, 200, "")
	d, _ := r.Body["data"].(map[string]any)
	if d == nil || d["email"] != "ana@x.com" {
		t.Fatalf("unexpected profile: %s", r.Raw)
	}
	roles, _ := d["roles"].([]any)
	found := false
	for _, ro := range roles {
		if ro == "Gestor" {
			found = true
		}
		if ro == "All" {
			t.Error("implicit role All is irrelevant on screen")
		}
	}
	if !found {
		t.Errorf("expected role Gestor, got %v", roles)
	}
	// secrets are never exposed here
	if _, ok := d["password_hash"]; ok {
		t.Error("password_hash must not appear in profile")
	}
}

// The entire point of enumerating fields instead of spreading args: a caller cannot
// escalate privileges by writing roles or enabled via self-service.
func TestSEC04_SelfServiceCannotEscalate(t *testing.T) {
	x := setup(t)
	r := x.callAs("ze@x.com", "core.services.profile.updateMyProfile", map[string]any{
		"fullName":  "New Ze",
		"roles":     []any{map[string]any{"role": "System Manager"}},
		"enabled":   false,
		"email":     "outro@x.com",
		"user_type": "System User",
	})
	x.expect(r, 200, "")

	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.GetDoc("User", "ze@x.com")
		if err != nil {
			t.Fatal(err)
		}
		if d.Str("full_name") != "New Ze" {
			t.Errorf("full name should have changed, got %q", d.Str("full_name"))
		}
		if en, _ := d["enabled"].(bool); !en {
			t.Error("enabled should not have been modified by self-service")
		}
		if len(d.Children("roles")) != 0 {
			t.Error("self-service cannot grant roles")
		}
		return nil
	})
	// and the user still exists under the same name
	if err := x.e.Run(x.ctx, "Administrator", func(c *engine.Ctx) error {
		_, err := c.GetDoc("User", "ze@x.com")
		return err
	}); err != nil {
		t.Errorf("email (name itself) should not have changed: %v", err)
	}
}

// Changing password requires proving current password: an unlocked laptop must not
// be a path to hijack an account from its owner.
func TestSEC04_ChangeMyPasswordNeedsTheCurrentOne(t *testing.T) {
	x := setup(t)
	r := x.callAs("ana@x.com", "core.services.profile.changeMyPassword", map[string]any{
		"current": "not-the-password", "password": "newpassword123",
	})
	if r.Status == 200 {
		t.Fatal("wrong current password should be rejected")
	}
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	ok := x.callAs("ana@x.com", "core.services.profile.changeMyPassword", map[string]any{
		"current": "segredo123", "password": "newpassword123",
	})
	x.expect(ok, 200, "")
	x.expect(x.goodLogin("ana@x.com", "newpassword123"), 200, "")
}

// The session of the user who changes password survives; the others do not.
func TestSEC04_ChangeMyPasswordSparesTheCallersSession(t *testing.T) {
	x := setup(t)
	other := x.sid("ana@x.com")
	mine := x.sid("ana@x.com")

	r := x.call("POST", "/api/method/core.services.profile.changeMyPassword",
		map[string]any{"current": "segredo123", "password": "newpassword123"}, "sid:"+mine)
	x.expect(r, 200, "")

	x.expect(x.call("GET", "/api/boot", nil, "sid:"+mine), 200, "")
	if u, _ := x.call("GET", "/api/boot", nil, "sid:"+other).Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("the other session should have been dropped")
	}
}

// The session list never returns a sid: it is a bearer token, and an XSS that
// read this list would compromise all devices.
func TestSEC04_SessionListNeverReturnsASid(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	r := x.call("POST", "/api/method/core.services.sessions.listMySessions", nil, "sid:"+sid)
	x.expect(r, 200, "")
	if containsStr(r.Raw, sid) {
		t.Fatal("raw sid appeared in response")
	}
	d, _ := r.Body["data"].(map[string]any)
	list, _ := d["sessions"].([]any)
	if len(list) == 0 {
		t.Fatal("expected at least the current session")
	}
	current := 0
	for _, s := range list {
		m, _ := s.(map[string]any)
		if m["current"] == true {
			current++
		}
		if id, _ := m["id"].(string); len(id) != 12 {
			t.Errorf("id should be a short handle, got %q", id)
		}
	}
	if current != 1 {
		t.Errorf("exactly one session should be current, got %d", current)
	}
}

// A handle copied from someone else's list affects nothing.
func TestSEC04_CannotRevokeSomeoneElsesSession(t *testing.T) {
	x := setup(t)
	victim := x.sid("bia@x.com")
	handle := engineHandle(victim)

	r := x.callAs("ana@x.com", "core.services.sessions.revokeMySession", map[string]any{"id": handle})
	x.expect(r, 200, "")
	if d, _ := r.Body["data"].(map[string]any); d != nil && d["revoked"] != float64(0) {
		t.Errorf("should not have revoked anything, got %v", d["revoked"])
	}
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+victim), 200, "")
}

func TestSEC04_MyAPIKeysAreMineOnly(t *testing.T) {
	x := setup(t)
	anaSid := x.sid("ana@x.com")

	created := x.call("POST", "/api/method/core.services.api_keys.createMyAPIKey",
		map[string]any{"label": "cli"}, "sid:"+anaSid)
	x.expect(created, 200, "")
	d, _ := created.Body["data"].(map[string]any)
	token, _ := d["token"].(string)
	if token == "" {
		t.Fatalf("expected token once: %s", created.Raw)
	}
	// and it works
	x.expect(x.call("GET", "/api/boot", nil, "token:"+token), 200, "")

	// bia's key does not appear in ana's list
	biaKey := x.apiKey("bia@x.com")
	list := x.call("POST", "/api/method/core.services.api_keys.listMyAPIKeys", nil, "sid:"+anaSid)
	x.expect(list, 200, "")
	if containsStr(list.Raw, splitKey(biaKey)) {
		t.Error("another user's key appeared in list")
	}

	// and ana cannot revoke bia's key
	r := x.call("POST", "/api/method/core.services.api_keys.revokeMyAPIKey",
		map[string]any{"name": splitKey(biaKey)}, "sid:"+anaSid)
	if r.Status == 200 {
		t.Error("revoking another user's key should be rejected")
	}
	x.expect(x.call("GET", "/api/boot", nil, "token:"+biaKey), 200, "")
}

// Only System Manager can manage other accounts.
func TestSEC04_AdminServicesNeedTheRole(t *testing.T) {
	x := setup(t)
	r := x.callAs("ana@x.com", "core.services.users.invite",
		map[string]any{"email": "x@y.com", "fullName": "X"})
	x.expect(r, 403, "PermissionError")

	ok := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "x@y.com", "fullName": "X"})
	x.expect(ok, 200, "")
	// without email transport configured, link returns to inviter
	if d, _ := ok.Body["data"].(map[string]any); d == nil || d["link"] == nil {
		t.Errorf("expected link back in log transport: %s", ok.Raw)
	}
}

// An expired key is no longer valid. The 60s cache is cleared manually here
// because it is exactly the leeway documented in UserFromAPIKey.
func TestSEC04_APIKeyExpires(t *testing.T) {
	x := setup(t)
	token := x.apiKey("ana@x.com")
	name := splitKey(token)
	x.expect(x.call("GET", "/api/boot", nil, "token:"+token), 200, "")

	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("API Key", name, engine.Doc{"expires": "2020-01-01 00:00:00"})
	})
	x.e.Cache.Del("apikey:" + name)

	x.expect(x.call("GET", "/api/boot", nil, "token:"+token), 401, "AuthenticationError")
}

func containsStr(hay, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func splitKey(token string) string {
	for i := 0; i < len(token); i++ {
		if token[i] == ':' {
			return token[:i]
		}
	}
	return token
}

func engineHandle(sid string) string { return engine.TokenHandle(sid) }
