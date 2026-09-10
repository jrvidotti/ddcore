// Package acceptance holds the end-to-end checks of the "definition of done":
// a clean database plus `migrate` has to produce a usable site, and the HTTP
// surface the desk depends on (boot, translations, /app) has to answer.
//
// The temporary app below keeps these checks independent from any product app:
// they exercise migrate → afterInstall → boot → i18n against a real Postgres
// and a real http server, which the TS suite (`cerne test`) cannot reach.
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

	"github.com/jrvidotti/cerne/desk"
	"github.com/jrvidotti/cerne/internal/api"
	"github.com/jrvidotti/cerne/internal/engine"
	"github.com/jrvidotti/cerne/internal/js"
)

// testDSN names a throwaway database: setup drops and recreates it. The
// database actually used is this name plus "_acc<sufixo>", so sharing
// CERNE_TEST_DSN with internal/engine (which drops the database it names) is
// safe even when `go test ./internal/...` runs both packages in paralelo.
var testDSN = envOr("CERNE_TEST_DSN", "postgres://cerne:cerne@localhost:5455/cerne_test?sslmode=disable")

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

// acceptanceApp creates the smallest external app that exercises installation,
// boot, workspace, report, form bundle and app-level translations.
func acceptanceApp(t *testing.T) js.App {
	t.Helper()
	dir := t.TempDir()
	w := func(rel, src string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("cerne.app.ts", `import { defineApp } from "@cerne/sdk";
export default defineApp({
  name: "acceptance",
  title: "Acceptance",
  roles: ["Acceptance Manager"],
  desk: { home: "Acceptance" },
  afterInstall() {
    cerne.newDoc("Acceptance Item", { codigo: "seed", titulo: "Instalado" }).insert({ ignorePermissions: true });
  },
});`)
	w("doctypes/acceptance_item/acceptance_item.doctype.ts", `import { defineDoctype } from "@cerne/sdk";
export default defineDoctype({
  name: "Acceptance Item",
  label: "Acceptance Item",
  module: "Acceptance",
  naming: { field: "codigo" },
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Código", reqd: true, unique: true },
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true, inListView: true },
  ],
  permissions: [{ role: "Acceptance Manager", read: true, write: true, create: true, delete: true, report: true }],
});`)
	w("doctypes/acceptance_item/acceptance_item.form.ts", `import { defineForm } from "@cerne/desk-sdk";
defineForm("Acceptance Item", { refresh(frm) { frm.addIndicator(frm.doc.titulo || "Item", "blue"); } });`)
	w("reports/acceptance_items.report.ts", `import { defineReport } from "@cerne/sdk";
export default defineReport({
  name: "Acceptance Items",
  label: "Acceptance Items",
  roles: ["System Manager", "Acceptance Manager"],
  execute() { return { columns: [{ fieldname: "titulo", label: "Título", fieldtype: "Data" }], rows: [] }; },
});`)
	w("workspaces/acceptance.workspace.ts", `import { defineWorkspace } from "@cerne/sdk";
export default defineWorkspace({
  name: "Acceptance",
  label: "Acceptance",
  roles: ["System Manager", "Acceptance Manager"],
  sidebar: [
    { label: "Itens", doctype: "Acceptance Item" },
    { label: "Relatório", report: "Acceptance Items" },
  ],
});`)
	w("translations/pt-BR.csv", "Acceptance Item,Item de Aceite\n")
	return js.App{Name: "acceptance", Dir: dir}
}

// setup recreates the database, boots an engine with a temporary external app
// (plus any extra app) and migrates it — the literal bootstrap flow.
func setup(t *testing.T, suffix string, extra ...js.App) *engine.Engine {
	t.Helper()
	ctx := context.Background()
	dsn, adminDSN, dbName := dsnFor(suffix)
	if dbName == "" {
		t.Fatalf("CERNE_TEST_DSN inválida: %s", testDSN)
	}
	admin, err := engine.New(ctx, engine.Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("CERNE_TEST_DSN") != "" {
			t.Fatalf("postgres indisponível em CERNE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres indisponível: %v", err)
	}
	admin.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := admin.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	admin.DB.Close()

	apps := append([]js.App{acceptanceApp(t)}, extra...)
	e, err := engine.New(ctx, engine.Config{DSN: dsn, Apps: apps, SiteName: "cerne", Lang: "pt-BR", Currency: "BRL"})
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

// server wraps the engine in the same HTTP surface `cerne start` exposes,
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
		if !exists("Role", "Acceptance Manager") {
			t.Error("papel do app externo não foi criado")
		}
		if !exists("Acceptance Item", "seed") {
			t.Error("afterInstall do app externo não criou Acceptance Item seed")
		} else {
			item, err := c.GetValues("Acceptance Item", "seed", []string{"titulo"})
			if err != nil {
				t.Errorf("carrega fixture do app externo: %v", err)
			} else if item["titulo"] != "Instalado" {
				t.Errorf("titulo da fixture = %v, esperado Instalado", item["titulo"])
			}
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
		if app["name"] != "acceptance" {
			continue
		}
		d, ok := app["desk"].(map[string]any)
		if !ok {
			t.Fatalf("acceptance sem bloco desk no boot: %#v", app["desk"])
		}
		home, _ = d["home"].(string)
	}
	if home != "Acceptance" {
		t.Fatalf("desk.home = %q, esperado \"Acceptance\"", home)
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
	w("cerne.app.ts", `import { defineApp } from "@cerne/sdk";
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

	// o mesmo catálogo alimenta o `_()` do servidor
	if got := e.I18n.T("pt-BR", "Save"); got != "Gravar" {
		t.Errorf("I18n.T(Save) = %q, esperado \"Gravar\"", got)
	}

	// idioma sem catálogo não pode explodir: devolve 200
	if b := getJSON(t, srv, tok, "/api/translations?lang=xx-XX"); b == nil {
		t.Error("idioma desconhecido devia responder 200")
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

	for _, path := range []string{"/app", "/app/Acceptance%20Item", "/app/workspace/Acceptance"} {
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
