// Package acceptance holds the end-to-end checks of the "definition of done":
// a clean database plus `migrate` has to produce a usable site, and the HTTP
// surface the desk depends on (boot, translations, /app) has to answer.
//
// As checagens rodam contra o demo app versionado em apps/demo: elas
// exercitam migrate → boot → i18n → demo contra um Postgres e um http server
// reais, que a suíte TS (`ddcore test`) não alcança.
package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/desk"
	"github.com/jrvidotti/ddcore/internal/api"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
)

// testDSN names a throwaway database: setup drops and recreates it. The
// database actually used is this name plus "_acc<sufixo>", so sharing
// DDCORE_TEST_DSN with internal/engine (which drops the database it names) is
// safe even when `go test ./internal/...` runs both packages in paralelo.
var testDSN = envOr("DDCORE_TEST_DSN", "postgres://ddcore:ddcore@localhost:5455/ddcore_test?sslmode=disable")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// dsnFor returns the DSN with the database renamed, plus the "postgres" DSN
// used to create/drop it. Each test gets its own database so they can run in
// any order without sharing installed apps.
func dsnFor(suffix string) (dsn, adminDSN, dbName string) {
	u, err := url.Parse(testDSN)
	if err != nil {
		return "", "", ""
	}
	dbName = strings.TrimPrefix(u.Path, "/") + "_acc" + suffix
	u.Path = "/" + dbName
	dsn = u.String()
	u.Path = "/postgres"
	return dsn, u.String(), dbName
}

// demoApp aponta para o app versionado do repositório, em vez de gerar uma
// fixture quase equivalente num diretório temporário.
func demoApp(t *testing.T) js.App {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "apps", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ddcore.app.ts")); err != nil {
		t.Fatalf("demo app não encontrado em %s: %v", dir, err)
	}
	return js.App{Name: "demo", Dir: dir}
}

// setup recreates the database, boots an engine with the checked-in example
// app (plus any extra app) and migrates it — the literal bootstrap flow.
func setup(t *testing.T, suffix string, extra ...js.App) *engine.Engine {
	t.Helper()
	ctx := context.Background()
	dsn, adminDSN, dbName := dsnFor(suffix)
	if dbName == "" {
		t.Fatalf("DDCORE_TEST_DSN inválida: %s", testDSN)
	}
	admin, err := engine.New(ctx, engine.Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres indisponível em DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres indisponível: %v", err)
	}
	admin.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := admin.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	admin.DB.Close()

	apps := append([]js.App{demoApp(t)}, extra...)
	e, err := engine.New(ctx, engine.Config{DSN: dsn, Apps: apps, SiteName: "ddcore", Lang: "pt-BR", Currency: "BRL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// o roteiro de bootstrap roda migrate mais de uma vez: precisa ser estável
	if plan, _ := e.Plan(ctx, false); len(plan) != 0 {
		t.Fatalf("migrate não é idempotente, sobrou DDL: %v", plan)
	}
	return e
}

// server wraps the engine in the same HTTP surface `ddcore start` exposes,
// authenticated as Administrator through an API key.
func server(t *testing.T, e *engine.Engine) (*httptest.Server, string) {
	t.Helper()
	tok, err := e.CreateAPIKey(context.Background(), "Administrator", "acceptance")
	if err != nil {
		t.Fatalf("apikey: %v", err)
	}
	srv := httptest.NewServer(api.New(e, desk.FS()).Router)
	t.Cleanup(srv.Close)
	return srv, tok
}

// getJSON does an authenticated GET and decodes the JSON body.
func getJSON(t *testing.T, srv *httptest.Server, tok, path string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "token "+tok)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET %s = %d", path, res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("GET %s: json inválido: %v", path, err)
	}
	return out
}

// ---------------------------------------------------------------- instalação

// TestInstalacao: depois de migrate o site tem que estar utilizável — usuário
// e papel base do core, mais o papel e fixture criados pelo app externo.
func TestInstalacao(t *testing.T) {
	e := setup(t, "")
	ctx := context.Background()

	err := e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
		exists := func(doctype, name string) bool {
			ok, err := c.Exists(doctype, name)
			if err != nil {
				t.Errorf("exists %s/%s: %v", doctype, name, err)
			}
			return ok
		}
		if !exists("User", "Administrator") {
			t.Error("usuário Administrator não foi criado pela instalação do core")
		}
		if !exists("Role", "System Manager") {
			t.Error("papel System Manager não foi criado pela instalação do core")
		}
		for _, role := range []string{"Project Manager", "Project Contributor"} {
			if !exists("Role", role) {
				t.Errorf("papel %q do demo app não foi criado", role)
			}
		}
		// o demo app não cria dados de negócio na instalação; o que precisa
		// existir é a meta dele, migrada para o banco novo
		for _, doctype := range []string{"Project", "Task", "Project Milestone"} {
			if _, err := c.St.DocType(doctype); err != nil {
				t.Errorf("DocType %q do demo app ausente depois do migrate: %v", doctype, err)
			}
		}
		if n, err := c.Count("Project", nil); err != nil {
			t.Errorf("count Project: %v", err)
		} else if n != 0 {
			t.Errorf("instalação criou %d Project(s); o demo app semeia só por `ddcore demo`", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBootHome: o desk abre em desk.home, que precisa ser um workspace real e
// com sidebar — a sidebar é sempre explícita, nunca inferida dos DocTypes.
func TestBootHome(t *testing.T) {
	e := setup(t, "_boot")
	srv, tok := server(t, e)
	// as respostas de /api/* vêm no envelope {"data": ...}
	boot, ok := getJSON(t, srv, tok, "/api/boot")["data"].(map[string]any)
	if !ok {
		t.Fatal("/api/boot não devolveu um objeto em data")
	}

	if boot["user"] != "Administrator" {
		t.Fatalf("boot autenticado devolveu user=%v", boot["user"])
	}

	// desk.home do app externo
	home := ""
	for _, a := range boot["apps"].([]any) {
		app := a.(map[string]any)
		if app["name"] != "demo" {
			continue
		}
		d, ok := app["desk"].(map[string]any)
		if !ok {
			t.Fatalf("demo app has no desk block no boot: %#v", app["desk"])
		}
		home, _ = d["home"].(string)
	}
	if home != "Projects" {
		t.Fatalf("desk.home = %q, expected \"Projects\"", home)
	}

	// o workspace apontado por desk.home tem que vir no boot, com sidebar
	var ws map[string]any
	for _, w := range boot["workspaces"].([]any) {
		if m := w.(map[string]any); m["name"] == home {
			ws = m
		}
	}
	if ws == nil {
		t.Fatalf("workspace %q não veio no boot do Administrator", home)
	}
	sidebar, _ := ws["sidebar"].([]any)
	if len(sidebar) == 0 {
		t.Fatal("sidebar do workspace está vazia")
	}
	// e os destinos citados têm que existir de fato
	doctypes := boot["doctypes"].(map[string]any)
	reports := boot["reports"].(map[string]any)
	links := 0
	for _, it := range sidebar {
		item := it.(map[string]any)
		if dt, ok := item["doctype"].(string); ok && dt != "" {
			links++
			if _, ok := doctypes[dt]; !ok {
				t.Errorf("sidebar aponta para o DocType %q, ausente do boot", dt)
			}
		}
		if rp, ok := item["report"].(string); ok && rp != "" {
			links++
			if _, ok := reports[rp]; !ok {
				t.Errorf("sidebar aponta para o relatório %q, ausente do boot", rp)
			}
		}
	}
	if links == 0 {
		t.Fatal("sidebar não tem nenhum link para DocType ou relatório")
	}
}

// --------------------------------------------------------------- traduções

// i18nApp writes a throwaway app whose only content is a pt-BR catalogue that
// overrides one key of the core, to check the merge precedence.
func i18nApp(t *testing.T) js.App {
	t.Helper()
	dir := t.TempDir()
	w := func(rel, src string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "traducoes", title: "Traduções" });`)
	// "Save" já existe no core como "Salvar": o app tem que ganhar.
	w("translations/pt-BR.csv", "Save,Gravar\nChave Só Do App,Valor Só Do App\n")
	return js.App{Name: "traducoes", Dir: dir}
}

// TestTraducoes: /api/translations devolve o catálogo mesclado core + apps,
// com o app tendo precedência sobre o core na mesma chave.
func TestTraducoes(t *testing.T) {
	e := setup(t, "_i18n", i18nApp(t))
	srv, tok := server(t, e)

	body := getJSON(t, srv, tok, "/api/translations?lang=pt-BR")
	data, ok := body["data"].(map[string]any)
	if !ok || len(data) == 0 {
		t.Fatalf("catálogo pt-BR vazio: %#v", body["data"])
	}
	// 1. chave só do core continua presente
	if got := data["Submit"]; got != "Enviar" {
		t.Errorf("chave do core Submit = %v, esperado \"Enviar\"", got)
	}
	// 2. chave só do app aparece
	if got := data["Chave Só Do App"]; got != "Valor Só Do App" {
		t.Errorf("chave do app = %v, esperado \"Valor Só Do App\"", got)
	}
	// 3. na colisão, o app vence o core (Save: Salvar → Gravar)
	if got := data["Save"]; got != "Gravar" {
		t.Errorf("precedência: Save = %v, esperado \"Gravar\" (valor do app)", got)
	}

	// 4. o catálogo do demo app entra junto
	if got := data["Start"]; got != "Iniciar" {
		t.Errorf("chave do demo app Start = %v, esperado \"Iniciar\"", got)
	}

	// o mesmo catálogo alimenta o `_()` do servidor
	if got := e.I18n.T("pt-BR", "Save"); got != "Gravar" {
		t.Errorf("I18n.T(Save) = %q, esperado \"Gravar\"", got)
	}

	// idioma sem catálogo não pode explodir: devolve 200
	if b := getJSON(t, srv, tok, "/api/translations?lang=xx-XX"); b == nil {
		t.Error("idioma desconhecido devia responder 200")
	}

	// sem parâmetro, vale o idioma do site
	if b := getJSON(t, srv, tok, "/api/translations"); b["data"].(map[string]any)["Submit"] != "Enviar" {
		t.Errorf("sem ?lang, o catálogo devia ser o de %q", e.Cfg.Lang)
	}

	// interpolação: {0}, {1} e um argumento ausente
	cases := []struct {
		key  string
		args []any
		want string
	}{
		{"Save", nil, "Gravar"},
		{"{0} {1} not found", []any{"Task", "T-1"}, "Task T-1 não encontrado"},
		{"{0} {1} not found", []any{"Task"}, "Task {1} não encontrado"},
		{"{0} {1} not found", nil, "{0} {1} não encontrado"},
		// uma chave sem tradução devolve a própria chave, interpolada
		{"No such key {0}", []any{"x"}, "No such key x"},
	}
	for _, c := range cases {
		if got := e.I18n.T("pt-BR", c.key, c.args...); got != c.want {
			t.Errorf("T(%q, %v) = %q, esperado %q", c.key, c.args, got, c.want)
		}
	}

	// Catalogue devolve o dicionário vivo: o chamador não pode alterá-lo por
	// acidente e mudar o que todo mundo lê
	cat := e.I18n.Catalogue("pt-BR")
	cat["Save"] = "ADULTERADO"
	if got := e.I18n.T("pt-BR", "Save"); got != "Gravar" {
		t.Errorf("o catálogo foi alterado pelo chamador: T(Save) = %q", got)
	}
}

// ------------------------------------------------------------------- /app

// TestDeskIndex: /app precisa servir o index do desk compilado (o SvelteKit é
// SPA: qualquer rota /app/... cai no mesmo index).
func TestDeskIndex(t *testing.T) {
	if desk.FS() == nil {
		t.Skip("desk não compilado: rode `make desk`")
	}
	e := setup(t, "_desk")
	srv, _ := server(t, e)

	for _, path := range []string{"/app", "/app/Task", "/app/workspace/Projects"} {
		res, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 4096)
		n, _ := res.Body.Read(body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("GET %s = %d, esperado 200", path, res.StatusCode)
			continue
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s: content-type %q", path, ct)
		}
		if !strings.Contains(strings.ToLower(string(body[:n])), "<!doctype html") {
			t.Errorf("GET %s não devolveu o index do desk: %.120q", path, body[:n])
		}
	}

	// a API não pode ser capturada pelo fallback do desk
	res, err := srv.Client().Get(srv.URL + "/api/rota-que-nao-existe")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("GET /api/inexistente = %d, esperado 404", res.StatusCode)
	}
}

// ------------------------------------------------------------------- demo

// TestDemo: `ddcore demo` é o atalho de primeira execução e tem que poder rodar
// duas vezes sem duplicar nada — é o que o demo app promete.
func TestDemo(t *testing.T) {
	e := setup(t, "_demo")
	ctx := context.Background()

	first := runDemo(t, e, ctx)
	if first["count"].(float64) == 0 {
		t.Fatalf("the first run created nothing: %#v", first)
	}
	second := runDemo(t, e, ctx)
	if got := second["count"].(float64); got != 0 {
		t.Errorf("the second run created %v record(s), want 0", got)
	}

	err := e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
		for doctype, want := range map[string]int64{"Project": 1, "Task": 3, "Project Milestone": 3} {
			n, err := c.Count(doctype, nil)
			if err != nil {
				t.Errorf("count %s: %v", doctype, err)
				continue
			}
			if n != want {
				t.Errorf("%s: %d record(s) after two runs, want %d", doctype, n, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runDemo(t *testing.T, e *engine.Engine, ctx context.Context) map[string]any {
	t.Helper()
	raw, err := e.RunJob(ctx, "Administrator", "demo.services.demo.generate", nil)
	if err != nil {
		t.Fatalf("run the demo: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("resposta da demo: %v (%s)", err, raw)
	}
	return out
}
