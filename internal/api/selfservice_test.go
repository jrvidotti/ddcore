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
		t.Fatalf("perfil inesperado: %s", r.Raw)
	}
	roles, _ := d["roles"].([]any)
	found := false
	for _, ro := range roles {
		if ro == "Gestor" {
			found = true
		}
		if ro == "All" {
			t.Error("o papel implícito All não interessa a ninguém na tela")
		}
	}
	if !found {
		t.Errorf("esperava o papel Gestor, veio %v", roles)
	}
	// um segredo nunca sai por aqui
	if _, ok := d["password_hash"]; ok {
		t.Error("password_hash não pode aparecer no perfil")
	}
}

// O ponto inteiro de enumerar os campos em vez de espalhar args: um caller não
// pode se promover escrevendo roles ou enabled pelo autosserviço.
func TestSEC04_SelfServiceCannotEscalate(t *testing.T) {
	x := setup(t)
	r := x.callAs("ze@x.com", "core.services.profile.updateMyProfile", map[string]any{
		"fullName":  "Zé Novo",
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
		if d.Str("full_name") != "Zé Novo" {
			t.Errorf("o nome completo devia ter mudado, veio %q", d.Str("full_name"))
		}
		if en, _ := d["enabled"].(bool); !en {
			t.Error("enabled não podia ter sido alterado pelo autosserviço")
		}
		if len(d.Children("roles")) != 0 {
			t.Error("o autosserviço não pode conceder papéis")
		}
		return nil
	})
	// e o usuário continua existindo sob o mesmo nome
	if err := x.e.Run(x.ctx, "Administrator", func(c *engine.Ctx) error {
		_, err := c.GetDoc("User", "ze@x.com")
		return err
	}); err != nil {
		t.Errorf("o e-mail (o próprio name) não podia ter mudado: %v", err)
	}
}

// Trocar a senha exige provar a atual: um notebook destravado não pode ser um
// caminho para expulsar o dono da própria conta.
func TestSEC04_ChangeMyPasswordNeedsTheCurrentOne(t *testing.T) {
	x := setup(t)
	r := x.callAs("ana@x.com", "core.services.profile.changeMyPassword", map[string]any{
		"current": "nao-e-a-senha", "password": "novasenha123",
	})
	if r.Status == 200 {
		t.Fatal("a senha atual errada tinha de ser recusada")
	}
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	ok := x.callAs("ana@x.com", "core.services.profile.changeMyPassword", map[string]any{
		"current": "segredo123", "password": "novasenha123",
	})
	x.expect(ok, 200, "")
	x.expect(x.goodLogin("ana@x.com", "novasenha123"), 200, "")
}

// A sessão de quem troca a senha sobrevive; as outras, não.
func TestSEC04_ChangeMyPasswordSparesTheCallersSession(t *testing.T) {
	x := setup(t)
	other := x.sid("ana@x.com")
	mine := x.sid("ana@x.com")

	r := x.call("POST", "/api/method/core.services.profile.changeMyPassword",
		map[string]any{"current": "segredo123", "password": "novasenha123"}, "sid:"+mine)
	x.expect(r, 200, "")

	x.expect(x.call("GET", "/api/boot", nil, "sid:"+mine), 200, "")
	if u, _ := x.call("GET", "/api/boot", nil, "sid:"+other).Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("a outra sessão devia ter caído")
	}
}

// A lista de sessões nunca devolve um sid: ele é bearer token, e um XSS que
// lesse esta lista levaria todos os dispositivos.
func TestSEC04_SessionListNeverReturnsASid(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	r := x.call("POST", "/api/method/core.services.sessions.listMySessions", nil, "sid:"+sid)
	x.expect(r, 200, "")
	if containsStr(r.Raw, sid) {
		t.Fatal("o sid cru apareceu na resposta")
	}
	d, _ := r.Body["data"].(map[string]any)
	list, _ := d["sessions"].([]any)
	if len(list) == 0 {
		t.Fatal("esperava ao menos a sessão atual")
	}
	current := 0
	for _, s := range list {
		m, _ := s.(map[string]any)
		if m["current"] == true {
			current++
		}
		if id, _ := m["id"].(string); len(id) != 12 {
			t.Errorf("o id devia ser um handle curto, veio %q", id)
		}
	}
	if current != 1 {
		t.Errorf("exatamente uma sessão é a atual, veio %d", current)
	}
}

// Um handle copiado da lista de outra pessoa não alcança nada.
func TestSEC04_CannotRevokeSomeoneElsesSession(t *testing.T) {
	x := setup(t)
	victim := x.sid("bia@x.com")
	handle := engineHandle(victim)

	r := x.callAs("ana@x.com", "core.services.sessions.revokeMySession", map[string]any{"id": handle})
	x.expect(r, 200, "")
	if d, _ := r.Body["data"].(map[string]any); d != nil && d["revoked"] != float64(0) {
		t.Errorf("não podia ter revogado nada, veio %v", d["revoked"])
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
		t.Fatalf("esperava o token uma vez: %s", created.Raw)
	}
	// e ele funciona
	x.expect(x.call("GET", "/api/boot", nil, "token:"+token), 200, "")

	// a chave de bia não aparece na lista de ana
	biaKey := x.apiKey("bia@x.com")
	list := x.call("POST", "/api/method/core.services.api_keys.listMyAPIKeys", nil, "sid:"+anaSid)
	x.expect(list, 200, "")
	if containsStr(list.Raw, splitKey(biaKey)) {
		t.Error("a chave de outra pessoa apareceu na lista")
	}

	// e ana não revoga a chave de bia
	r := x.call("POST", "/api/method/core.services.api_keys.revokeMyAPIKey",
		map[string]any{"name": splitKey(biaKey)}, "sid:"+anaSid)
	if r.Status == 200 {
		t.Error("revogar a chave de outra pessoa tinha de ser recusado")
	}
	x.expect(x.call("GET", "/api/boot", nil, "token:"+biaKey), 200, "")
}

// Só System Manager administra outras contas.
func TestSEC04_AdminServicesNeedTheRole(t *testing.T) {
	x := setup(t)
	r := x.callAs("ana@x.com", "core.services.users.invite",
		map[string]any{"email": "x@y.com", "fullName": "X"})
	x.expect(r, 403, "PermissionError")

	ok := x.callAs("root@x.com", "core.services.users.invite",
		map[string]any{"email": "x@y.com", "fullName": "X"})
	x.expect(ok, 200, "")
	// sem transporte de e-mail configurado, o link volta para quem convidou
	if d, _ := ok.Body["data"].(map[string]any); d == nil || d["link"] == nil {
		t.Errorf("esperava o link de volta no transporte de log: %s", ok.Raw)
	}
}

// Uma chave vencida não vale mais nada. O cache de 60s é derrubado à mão aqui
// porque é exatamente a folga que o comentário em UserFromAPIKey documenta.
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
