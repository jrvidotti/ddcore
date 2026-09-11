package api

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// Usuário desativado:
// - Com senha errada: não revela que a conta existe nem que está desativada (401 "Invalid username or password").
// - Com a senha certa: 401 "User is disabled" (checagem só depois do Argon2).
// - Tentativas contam para o lockout e trancam com 429 como qualquer outra conta.
func TestSEC04_DisabledUserAbuse(t *testing.T) {
	x := setup(t)

	// Desativa ze@x.com
	x.asAdmin(func(c *engine.Ctx) error {
		return c.SetValue("User", "ze@x.com", engine.Doc{"enabled": false})
	})

	// 1. Senha errada: resposta genérica idêntica a conta inexistente
	bad := x.badLogin("ze@x.com")
	unknown := x.badLogin("ninguem@x.com")
	x.expect(bad, 401, "AuthenticationError")
	if bad.Status != unknown.Status || bad.errType() != unknown.errType() || msg(bad) != msg(unknown) {
		t.Errorf("senha errada em usuário desativado tem de ser idêntica a usuário inexistente: %q vs %q", msg(bad), msg(unknown))
	}

	// 2. Senha certa: só agora informa que está desativada
	good := x.goodLogin("ze@x.com", "segredo123")
	x.expect(good, 401, "AuthenticationError")
	if msg(good) == msg(bad) {
		t.Errorf("senha certa devia diferenciar que o usuário está desativado, veio a mesma mensagem: %q", msg(good))
	}
	if !strings.Contains(msg(good), "desativado") && !strings.Contains(msg(good), "disabled") {
		t.Errorf("senha certa devia indicar usuário desativado, veio %q", msg(good))
	}

	// 3. Tentativas repetidas trancam a identidade com 429
	for i := 2; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ze@x.com")
	}
	r := x.badLogin("ze@x.com")
	x.expect(r, 429, "TooManyRequestsError")
	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("o lockout de usuário desativado também tem de responder Retry-After")
	}
}

// Desativar um usuário derruba suas sessões e chaves imediatamente, sem esperar
// expiração nem TTL de cache.
func TestSEC04_DisabledUserSessionsAndKeysRevoked(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	key := x.apiKey("ana@x.com")

	// Sessão e chave funcionam antes da desativação
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+sid), 200, "")
	x.expect(x.call("GET", "/api/boot", nil, "token:"+key), 200, "")

	// Administrador desativa a usuária via formulário/controller
	x.asAdmin(func(c *engine.Ctx) error {
		u, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		u["enabled"] = false
		_, err = c.Save(u, engine.SaveOpts{})
		return err
	})

	// A sessão caiu imediatamente do banco e do cache
	rBoot := x.call("GET", "/api/boot", nil, "sid:"+sid)
	x.expect(rBoot, 200, "")
	if u, _ := rBoot.Body["data"].(map[string]any); u != nil && u["user"] != "Guest" {
		t.Errorf("a sessão devia ter sido invalidada, veio user=%v", u["user"])
	}

	// A chave de API para de autenticar imediatamente
	x.expect(x.call("GET", "/api/boot", nil, "token:"+key), 401, "AuthenticationError")
}

// Brute-force em segredo de chave de API:
// Como a validação de segredo roda Argon2 a cada requisição, 20 palpites errados
// disparam o freio no cache de processo (`apikeyfail:key`) evitando DoS de memória.
func TestSEC04_APIKeyBruteForceBrake(t *testing.T) {
	x := setup(t)
	token := x.apiKey("ana@x.com")
	keyName := splitKey(token)

	for i := 0; i < 20; i++ {
		r := x.call("GET", "/api/boot", nil, "token:"+keyName+":segredoerrado")
		x.expect(r, 401, "AuthenticationError")
	}

	// O freio em cache atingiu o limiar
	v, ok := x.e.Cache.Get("apikeyfail:" + keyName)
	if !ok || v.(int) < 20 {
		t.Fatalf("o freio apikeyfail devia estar ativo no cache: ok=%v val=%v", ok, v)
	}

	// Próxima requisição falha imediatamente antes do Argon2
	r := x.call("GET", "/api/boot", nil, "token:"+keyName+":outrosegedoerrado")
	x.expect(r, 401, "AuthenticationError")
}

// Token kind mismatch:
// Um token de recuperação não pode ser usado em accept-invite, e vice-versa.
func TestSEC04_TokenKindMismatch(t *testing.T) {
	x := setup(t)

	// Token de reset tentando ser usado como invite
	resetTok := x.issueFor("ana@x.com", engine.TokenReset)
	r1 := x.call("POST", "/api/auth/accept-invite",
		map[string]any{"token": resetTok, "password": "senhanovaok1"}, "")
	x.expect(r1, 417, "ValidationError")

	// Token de invite tentando ser usado como reset
	inviteTok := x.issueFor("ana@x.com", engine.TokenInvite)
	r2 := x.call("POST", "/api/auth/reset-password",
		map[string]any{"token": inviteTok, "password": "senhanovaok1"}, "")
	x.expect(r2, 417, "ValidationError")
}

// IP throttle on spraying logins:
// Um atacante tentando múltiplos logins com contas diferentes a partir do mesmo IP
// é barrado pela chave loginip:<ip> após MaxLoginAttempts * 5 erros.
func TestSEC04_IPThrottleSpraying(t *testing.T) {
	x := setup(t)
	limit := x.e.Cfg.Auth.MaxLoginAttempts * 5

	for i := 0; i < limit; i++ {
		usr := fmt.Sprintf("spray%d@x.com", i)
		x.expect(x.badLogin(usr), 401, "AuthenticationError")
	}

	// O IP agora está trancado, mesmo para uma conta que nunca foi tentada
	r := x.badLogin("conta_virgem@x.com")
	x.expect(r, 429, "TooManyRequestsError")
	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("o throttle de IP tem de responder com Retry-After")
	} else if n, err := strconv.Atoi(ra); err != nil || n <= 0 {
		t.Errorf("Retry-After inválido: %q", ra)
	}
}
