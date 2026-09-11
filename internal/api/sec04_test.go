package api

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// badLogin tenta entrar com a senha errada pela rota real.
func (x *env) badLogin(usr string) resp {
	x.t.Helper()
	return x.call("POST", "/api/login", map[string]any{"usr": usr, "pwd": "nao-e-a-senha"}, "")
}

func (x *env) goodLogin(usr, pwd string) resp {
	x.t.Helper()
	return x.call("POST", "/api/login", map[string]any{"usr": usr, "pwd": pwd}, "")
}

// countAttempts conta as tentativas registradas para uma identidade.
func (x *env) countAttempts(identity string) int {
	x.t.Helper()
	rows, err := db.Select(x.ctx, x.e.DB.Pool,
		`SELECT count(*) AS n FROM ddcore_login_attempt WHERE identity = $1`, identity)
	if err != nil {
		x.t.Fatal(err)
	}
	n, _ := rows[0]["n"].(int64)
	return int(n)
}

// Depois de maxLoginAttempts erros a conta trava, e a resposta diz por quanto
// tempo — inclusive no header, que é onde um cliente HTTP procura.
func TestSEC04_LoginLockout(t *testing.T) {
	x := setup(t)
	limit := x.e.Cfg.Auth.MaxLoginAttempts

	for i := 0; i < limit; i++ {
		x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	}
	r := x.badLogin("ana@x.com")
	x.expect(r, 429, "TooManyRequestsError")

	if ra := r.Header.Get("Retry-After"); ra == "" {
		t.Error("um 429 tem de dizer Retry-After")
	} else if n, err := strconv.Atoi(ra); err != nil || n <= 0 {
		t.Errorf("Retry-After devia ser um número de segundos, veio %q", ra)
	}
	if e, ok := r.Body["error"].(map[string]any); ok {
		if extra, ok := e["extra"].(map[string]any); !ok || extra["retryAfter"] == nil {
			t.Errorf("o corpo também tem de carregar retryAfter, veio %s", r.Raw)
		}
	}

	// A senha certa não é uma saída do lockout: se fosse, bastaria acertar
	// depois de esgotar as tentativas para a trava nunca ter existido.
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 429, "TooManyRequestsError")

	// Outra conta segue entrando: a trava é da identidade, não do servidor.
	x.expect(x.goodLogin("bia@x.com", "segredo123"), 200, "")
}

// A regressão que a arquitetura pede: Engine.Login roda dentro de Ctx.Run, que
// faz rollback no erro. Se a tentativa fosse gravada na transação, o próprio
// erro que ela conta a apagaria — e o lockout nunca contaria nada.
func TestSEC04_FailedLoginSurvivesRollback(t *testing.T) {
	x := setup(t)
	before := x.countAttempts("login:ana@x.com")
	x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	if after := x.countAttempts("login:ana@x.com"); after != before+1 {
		t.Fatalf("a tentativa falha tem de sobreviver ao rollback: %d → %d", before, after)
	}
}

// Um endereço que não existe tem de responder igual a um que existe, e travar
// igual: senão o próprio lockout vira o oráculo que ele deveria fechar.
func TestSEC04_LockoutDoesNotEnumerate(t *testing.T) {
	x := setup(t)
	known := x.badLogin("ana@x.com")
	unknown := x.badLogin("ninguem@x.com")

	if known.Status != unknown.Status || known.errType() != unknown.errType() {
		t.Errorf("status/tipo diferentes: conhecido %d %s, desconhecido %d %s",
			known.Status, known.errType(), unknown.Status, unknown.errType())
	}
	if msg(known) != msg(unknown) {
		t.Errorf("mensagens diferentes: %q vs %q", msg(known), msg(unknown))
	}

	// e o desconhecido também tranca
	for i := 1; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ninguem@x.com")
	}
	x.expect(x.badLogin("ninguem@x.com"), 429, "TooManyRequestsError")
}

// Acertar a senha limpa o contador: quem finalmente lembrou não fica cumprindo
// uma trava que não chegou a existir.
func TestSEC04_SuccessClearsTheCounter(t *testing.T) {
	x := setup(t)
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts-1; i++ {
		x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
	}
	x.expect(x.goodLogin("ana@x.com", "segredo123"), 200, "")

	// o contador zerou: um novo erro não pode cair direto no 429
	x.expect(x.badLogin("ana@x.com"), 401, "AuthenticationError")
}

// O cookie de sessão segue a política, e não um número escrito no handler.
func TestSEC04_SessionCookieFollowsPolicy(t *testing.T) {
	x := setup(t)
	r := x.goodLogin("ana@x.com", "segredo123")
	x.expect(r, 200, "")

	want := int(x.e.Cfg.Auth.SessionTTL().Seconds())
	for _, c := range r.Header.Values("Set-Cookie") {
		if !strings.Contains(c, "sid=") {
			continue
		}
		if !strings.Contains(c, "Max-Age="+strconv.Itoa(want)) {
			t.Errorf("Max-Age devia ser %d (a política), veio %q", want, c)
		}
		if !strings.Contains(c, "HttpOnly") {
			t.Errorf("o cookie de sessão tem de ser HttpOnly: %q", c)
		}
		// Secure fica fora: httptest serve http e o site não declarou https.
		if strings.Contains(c, "Secure") {
			t.Errorf("sem TLS e sem url https, Secure tornaria o cookie inútil: %q", c)
		}
	}
}

// Uma sessão revogada morre agora, não daqui a um minuto: DropSessions tem de
// derrubar o cache junto com a linha.
func TestSEC04_DropSessionsInvalidatesTheCache(t *testing.T) {
	x := setup(t)
	sid := x.sid("ana@x.com")
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+sid), 200, "")

	x.asAdmin(func(c *engine.Ctx) error {
		n, err := x.e.DropSessions(context.Background(), c.Tx, "ana@x.com", "")
		if err != nil {
			return err
		}
		if n == 0 {
			t.Error("esperava ao menos uma sessão derrubada")
		}
		return nil
	})

	r := x.call("GET", "/api/boot", nil, "sid:"+sid)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] != "Guest" {
		t.Errorf("a sessão derrubada continuou valendo: %v", u["user"])
	}
}

// exceptSid é o que permite trocar a própria senha sem se deslogar da aba onde
// ela foi digitada.
func TestSEC04_DropSessionsSparesTheCaller(t *testing.T) {
	x := setup(t)
	keep := x.sid("ana@x.com")
	other := x.sid("ana@x.com")

	x.asAdmin(func(c *engine.Ctx) error {
		_, err := x.e.DropSessions(context.Background(), c.Tx, "ana@x.com", keep)
		return err
	})

	x.expect(x.call("GET", "/api/boot", nil, "sid:"+keep), 200, "")
	r := x.call("GET", "/api/boot", nil, "sid:"+other)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("a outra sessão deveria ter caído")
	}
}

// A política vale nos caminhos que definem senha, e não só no formulário: é
// por isso que ela mora no hash, e não em cada chamador.
func TestSEC04_PasswordPolicyOnEveryPath(t *testing.T) {
	x := setup(t)
	curta := "abc"

	// 1. formulário User e `ddcore user add`, via new_password → __hashPassword
	err := x.e.Run(x.ctx, "Administrator", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "nova@x.com", "full_name": "Nova", "new_password": curta})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err == nil {
		t.Error("o formulário User devia recusar uma senha abaixo do mínimo")
	}

	// 2. `ddcore user passwd`, recuperação e convite, via SetPassword
	if err := x.e.SetPassword(x.ctx, "ana@x.com", curta); err == nil {
		t.Error("SetPassword devia recusar uma senha abaixo do mínimo")
	}
	if err := x.e.SetPassword(x.ctx, "ana@x.com", "outrasenha1"); err != nil {
		t.Errorf("uma senha válida devia passar: %v", err)
	}
}

// Trocar a senha derruba as sessões antigas: se a troca foi porque a senha
// vazou, deixar as sessões de pé não teria trocado nada.
func TestSEC04_PasswordChangeRevokesSessions(t *testing.T) {
	x := setup(t)
	old := x.sid("ana@x.com")
	x.expect(x.call("GET", "/api/boot", nil, "sid:"+old), 200, "")

	if err := x.e.SetPassword(x.ctx, "ana@x.com", "senhanova123"); err != nil {
		t.Fatal(err)
	}

	r := x.call("GET", "/api/boot", nil, "sid:"+old)
	if u, _ := r.Body["data"].(map[string]any); u != nil && u["user"] == "ana@x.com" {
		t.Error("a sessão antiga devia ter caído com a troca de senha")
	}
	x.expect(x.goodLogin("ana@x.com", "senhanova123"), 200, "")
}

// Uma senha trocada também destrava a conta: quem acabou de provar que pode
// defini-la não deve continuar cumprindo lockout.
func TestSEC04_PasswordChangeClearsTheLockout(t *testing.T) {
	x := setup(t)
	for i := 0; i < x.e.Cfg.Auth.MaxLoginAttempts; i++ {
		x.badLogin("ana@x.com")
	}
	x.expect(x.badLogin("ana@x.com"), 429, "TooManyRequestsError")

	if err := x.e.SetPassword(x.ctx, "ana@x.com", "destravada123"); err != nil {
		t.Fatal(err)
	}
	x.expect(x.goodLogin("ana@x.com", "destravada123"), 200, "")
}

func msg(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		if m, ok := e["message"].(string); ok {
			return m
		}
	}
	return ""
}
