package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// callNoCSRF é o que um formulário anônimo faz: nenhum header de CSRF, porque
// não há sessão por trás dele.
func (x *env) callNoCSRF(method, path string, body any) resp {
	x.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, x.ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Header: res.Header}
	json.Unmarshal(raw, &out.Body)
	return out
}

// issueFor pega um token de recuperação direto do motor, como o e-mail faria.
func (x *env) issueFor(user, kind string) string {
	x.t.Helper()
	var token string
	x.asAdmin(func(c *engine.Ctx) error {
		t, _, err := x.e.IssueToken(x.ctx, user, kind, x.e.Cfg.Auth.ResetTTL(), "Administrator", "127.0.0.1")
		token = t
		return err
	})
	return token
}

func (x *env) countTokens(user string) int {
	x.t.Helper()
	var n int
	x.asAdmin(func(c *engine.Ctx) error {
		rows, err := c.SQL(`SELECT count(*) AS n FROM ddcore_auth_token WHERE "user" = $1 AND used IS NULL`, []any{user})
		if err != nil {
			return err
		}
		if v, ok := rows[0]["n"].(int64); ok {
			n = int(v)
		}
		return nil
	})
	return n
}

// Desconhecido, conhecido e desativado respondem igual — status e corpo. Se
// diferissem, o endpoint viraria um verificador de endereços de e-mail.
func TestSEC04_ForgotPasswordIsGeneric(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ze@x.com", engine.Doc{"enabled": false})
	})

	var bodies []string
	for _, usr := range []string{"ana@x.com", "ninguem@x.com", "ze@x.com"} {
		r := x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": usr}, "")
		x.expect(r, 200, "")
		bodies = append(bodies, r.Raw)
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("as respostas têm de ser idênticas:\n%s\nvs\n%s", bodies[0], bodies[i])
		}
	}

	// só o usuário ativo e existente ganhou um token
	if n := x.countTokens("ana@x.com"); n != 1 {
		t.Errorf("ana devia ter 1 token, veio %d", n)
	}
	if n := x.countTokens("ze@x.com"); n != 0 {
		t.Errorf("um usuário desativado não pode ganhar token, veio %d", n)
	}
}

func TestSEC04_ResetTokenIsSingleUse(t *testing.T) {
	x := setup(t)
	token := x.issueFor("ana@x.com", engine.TokenReset)

	// espiar diz de quem é, sem gastar
	r := x.call("POST", "/api/auth/token", map[string]any{"token": token}, "")
	x.expect(r, 200, "")
	if d, _ := r.Body["data"].(map[string]any); d == nil || d["user"] != "ana@x.com" || d["kind"] != "reset" {
		t.Fatalf("espiar devia dizer usuário e tipo: %s", r.Raw)
	}

	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "senhanovaok1"}, ""), 200, "")

	// reusar é recusado
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "outrasenha12"}, ""), 417, "ValidationError")

	// e a senha nova entra
	x.expect(x.goodLogin("ana@x.com", "senhanovaok1"), 200, "")
}

// A recuperação derruba todas as sessões: o motivo de redefinir pode ser que a
// conta não seja mais de quem a tinha.
func TestSEC04_ResetDropsEverySession(t *testing.T) {
	x := setup(t)
	old := x.sid("ana@x.com")
	token := x.issueFor("ana@x.com", engine.TokenReset)

	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "senhanovaok1"}, ""), 200, "")

	r := x.call("GET", "/api/boot", nil, "sid:"+old)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("a sessão antiga devia ter caído")
	}
}

// A política de senha vale também no fim da recuperação.
func TestSEC04_ResetAppliesThePasswordPolicy(t *testing.T) {
	x := setup(t)
	token := x.issueFor("ana@x.com", engine.TokenReset)
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "curta"}, ""), 417, "ValidationError")
	// e o token não foi gasto por uma senha recusada
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "agoravalida1"}, ""), 200, "")
}

// Um token de convite define a primeira senha — e antes disso não se entra.
func TestSEC04_InviteAcceptSetsPassword(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "novo@x.com", "full_name": "Novo"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})

	// sem senha, ninguém entra
	x.expect(x.goodLogin("novo@x.com", "qualquercoisa"), 401, "AuthenticationError")

	token := x.issueFor("novo@x.com", engine.TokenInvite)

	// um convite não serve como recuperação
	x.expect(x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": token, "password": "primeirasenha1"}, ""), 417, "ValidationError")

	x.expect(x.call("POST", "/api/auth/accept-invite",
		map[string]any{"token": token, "password": "primeirasenha1", "fullName": "Novo Nome"}, ""), 200, "")
	x.expect(x.goodLogin("novo@x.com", "primeirasenha1"), 200, "")
}

func TestSEC04_AuthEndpointsAreThrottled(t *testing.T) {
	x := setup(t)
	var last resp
	for i := 0; i < 6; i++ {
		last = x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": "ana@x.com"}, "")
	}
	if last.Status != 429 {
		t.Fatalf("esperava 429 depois de pedidos repetidos, veio %d: %s", last.Status, last.Raw)
	}
	if last.Header.Get("Retry-After") == "" {
		t.Error("o 429 tem de dizer Retry-After")
	}
}

// Um link inválido responde 404, e nunca diz se o token existiu mas venceu.
func TestSEC04_UnknownTokenIsRefused(t *testing.T) {
	x := setup(t)
	x.expect(x.call("POST", "/api/auth/token",
		map[string]any{"token": "0123456789abcdef0123456789abcdef0123456789abcdef"}, ""), 404, "DoesNotExistError")
}

// As rotas de recuperação são isentas de CSRF porque não há sessão para
// forjar: sem isso, um formulário de "esqueci a senha" nunca funcionaria.
func TestSEC04_AuthRoutesDoNotNeedTheCSRFHeader(t *testing.T) {
	x := setup(t)
	r := x.callNoCSRF("POST", "/api/auth/forgot-password", map[string]any{"usr": "ana@x.com"})
	x.expect(r, 200, "")
}
