// Package acceptance holds the end-to-end checks of the "definition of done":
// a clean database plus `migrate` has to produce a usable site, and the HTTP
// surface the desk depends on (boot, translations, /app) has to answer.
//
// The checks run against the fixture app versioned in apps/testapp: they
// exercise migrate → boot → i18n → demo against a real Postgres and http server,
// which the TS suite (`ddcore test`) does not reach.
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/desk"
	"github.com/jrvidotti/ddcore/internal/api"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// testDSN names a throwaway database: setup drops and recreates it. The
// database actually used is this name plus "_acc<suffix>", so sharing
// DDCORE_TEST_DSN with internal/engine (which drops the database it names) is
// safe even when `go test ./internal/...` runs both packages in parallel.
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

// testApp points to the versioned fixture app, rather than generating an
// almost equivalent one in a temporary directory.
func testApp(t *testing.T) js.App {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "apps", "testapp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ddcore.app.ts")); err != nil {
		t.Fatalf("fixture app not found at %s: %v", dir, err)
	}
	return js.App{Name: js.AppName(dir), Dir: dir}
}

// setup recreates the database, boots an engine with the checked-in example
// app (plus any extra app) and migrates it — the literal bootstrap flow.
func setup(t *testing.T, suffix string, extra ...js.App) *engine.Engine {
	t.Helper()
	ctx := context.Background()
	dsn, adminDSN, dbName := dsnFor(suffix)
	if dbName == "" {
		t.Fatalf("invalid DDCORE_TEST_DSN: %s", testDSN)
	}
	admin, err := engine.New(ctx, engine.Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	admin.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := admin.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	admin.DB.Close()

	apps := append([]js.App{testApp(t)}, extra...)
	e, err := engine.New(ctx, engine.Config{DSN: dsn, Apps: apps, Lang: "pt-BR", Currency: "BRL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// the bootstrap flow runs migrate more than once: it must be idempotent
	if plan, _ := e.Plan(ctx, false); len(plan) != 0 {
		t.Fatalf("migrate is not idempotent, remaining DDL: %v", plan)
	}
	return e
}

// server wraps the engine in the same HTTP surface `ddcore start` exposes,
// authenticated as Admin through an API key.
func server(t *testing.T, e *engine.Engine) (*httptest.Server, string) {
	t.Helper()
	tok, err := e.CreateAPIKey(context.Background(), "Admin", "acceptance")
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
		t.Fatalf("GET %s: invalid json: %v", path, err)
	}
	return out
}

// ---------------------------------------------------------------- installation

// TestInstalacao: after migrate the site must be usable — core user
// and base role, plus the role and fixture created by the external app.
func TestInstalacao(t *testing.T) {
	e := setup(t, "")
	ctx := context.Background()

	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		exists := func(doctype, name string) bool {
			ok, err := c.Exists(doctype, name)
			if err != nil {
				t.Errorf("exists %s/%s: %v", doctype, name, err)
			}
			return ok
		}
		if !exists("User", "Admin") {
			t.Error("Admin user was not created by core installation")
		}
		if !exists("Role", "System Manager") {
			t.Error("System Manager role was not created by core installation")
		}
		for _, role := range []string{"Project Manager", "Project Contributor"} {
			if !exists("Role", role) {
				t.Errorf("fixture app role %q was not created", role)
			}
		}
		// the fixture app does not create business data on install; what must
		// exist is its meta, migrated to the new database
		for _, doctype := range []string{"Project", "Task", "Project Milestone", "Pedido"} {
			if _, err := c.St.DocType(doctype); err != nil {
				t.Errorf("fixture app DocType %q missing after migrate: %v", doctype, err)
			}
		}
		if n, err := c.Count("Project", nil); err != nil {
			t.Errorf("count Project: %v", err)
		} else if n != 0 {
			t.Errorf("installation created %d Project(s); fixture app only seeds via `ddcore demo`", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBootHome: desk opens at desk.home, which must be a real workspace with
// a sidebar — the sidebar is always explicit, never inferred from DocTypes.
func TestBootHome(t *testing.T) {
	e := setup(t, "_boot")
	srv, tok := server(t, e)
	// /api/* responses come in the {"data": ...} envelope
	boot, ok := getJSON(t, srv, tok, "/api/boot")["data"].(map[string]any)
	if !ok {
		t.Fatal("/api/boot did not return an object in data")
	}

	if boot["user"] != "Admin" {
		t.Fatalf("authenticated boot returned user=%v", boot["user"])
	}

	// external app's desk.home and desk.logo
	home, logo := "", ""
	for _, a := range boot["apps"].([]any) {
		app := a.(map[string]any)
		if app["name"] != "testapp" {
			continue
		}
		d, ok := app["desk"].(map[string]any)
		if !ok {
			t.Fatalf("fixture app has no desk block in boot: %#v", app["desk"])
		}
		home, _ = d["home"].(string)
		logo, _ = d["logo"].(string)
	}
	if home != "Projects" {
		t.Fatalf("desk.home = %q, expected \"Projects\"", home)
	}
	// the desk block is a passthrough: whatever defineApp declared reaches the
	// browser, multi-byte included, so the sidebar can render the app's mark
	if logo != "\U0001F9EA" {
		t.Fatalf("desk.logo = %q, expected the fixture's emoji", logo)
	}

	// the site's name is the fixture app's own title, translated: nothing in
	// ddcore.json names the site any more, and the one string a reader sees is
	// a catalogue key like every other label
	site, _ := boot["site"].(map[string]any)
	if site == nil {
		t.Fatalf("/api/boot returned no site block: %#v", boot["site"])
	}
	if site["name"] != "App de Teste: Projetos" {
		t.Fatalf("site.name = %v, expected the fixture title in pt-BR", site["name"])
	}

	// the workspace pointed to by desk.home must be returned in boot, with sidebar
	var ws map[string]any
	for _, w := range boot["workspaces"].([]any) {
		if m := w.(map[string]any); m["name"] == home {
			ws = m
		}
	}
	if ws == nil {
		t.Fatalf("workspace %q was not returned in Admin boot", home)
	}
	sidebar, _ := ws["sidebar"].([]any)
	if len(sidebar) == 0 {
		t.Fatal("workspace sidebar is empty")
	}
	// and the linked targets must actually exist
	doctypes := boot["doctypes"].(map[string]any)
	reports := boot["reports"].(map[string]any)
	links := 0
	for _, it := range sidebar {
		item := it.(map[string]any)
		if dt, ok := item["doctype"].(string); ok && dt != "" {
			links++
			if _, ok := doctypes[dt]; !ok {
				t.Errorf("sidebar points to DocType %q, missing from boot", dt)
			}
		}
		if rp, ok := item["report"].(string); ok && rp != "" {
			links++
			if _, ok := reports[rp]; !ok {
				t.Errorf("sidebar points to report %q, missing from boot", rp)
			}
		}
	}
	if links == 0 {
		t.Fatal("sidebar has no links to DocType or report")
	}
}

// --------------------------------------------------------------- translations

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
	// "Save" already exists in core as "Salvar": the app must win.
	w("translations/pt-BR.csv", "Save,Gravar\nChave Só Do App,Valor Só Do App\n")
	return js.App{Name: "traducoes", Dir: dir}
}

// TestTraducoes: /api/translations returns the merged core + apps catalog,
// with the app taking precedence over core for the same key.
func TestTraducoes(t *testing.T) {
	e := setup(t, "_i18n", i18nApp(t))
	srv, tok := server(t, e)

	body := getJSON(t, srv, tok, "/api/translations?lang=pt-BR")
	data, ok := body["data"].(map[string]any)
	if !ok || len(data) == 0 {
		t.Fatalf("pt-BR catalog empty: %#v", body["data"])
	}
	// 1. core-only key remains present
	if got := data["Submit"]; got != "Enviar" {
		t.Errorf("core key Submit = %v, expected \"Enviar\"", got)
	}
	// 2. app-only key appears
	if got := data["Chave Só Do App"]; got != "Valor Só Do App" {
		t.Errorf("app key = %v, expected \"Valor Só Do App\"", got)
	}
	// 3. on collision, app wins over core (Save: Salvar → Gravar)
	if got := data["Save"]; got != "Gravar" {
		t.Errorf("precedence: Save = %v, expected \"Gravar\" (app value)", got)
	}

	// 4. fixture app catalog is included
	if got := data["Start"]; got != "Iniciar" {
		t.Errorf("fixture app key Start = %v, expected \"Iniciar\"", got)
	}

	// the same catalog feeds the server's _()
	if got := e.I18n.T("pt-BR", "Save"); got != "Gravar" {
		t.Errorf("I18n.T(Save) = %q, expected \"Gravar\"", got)
	}

	// language without a catalog must not fail: returns 200
	if b := getJSON(t, srv, tok, "/api/translations?lang=xx-XX"); b == nil {
		t.Error("unknown language should respond with 200")
	}

	// without parameter, site language applies
	if b := getJSON(t, srv, tok, "/api/translations"); b["data"].(map[string]any)["Submit"] != "Enviar" {
		t.Errorf("without ?lang, catalog should be for %q", e.Cfg.Lang)
	}

	// interpolation: {0}, {1} and a missing argument
	cases := []struct {
		key  string
		args []any
		want string
	}{
		{"Save", nil, "Gravar"},
		{"{0} {1} not found", []any{"Task", "T-1"}, "Task T-1 não encontrado"},
		{"{0} {1} not found", []any{"Task"}, "Task {1} não encontrado"},
		{"{0} {1} not found", nil, "{0} {1} não encontrado"},
		// a key without translation returns the key itself, interpolated
		{"No such key {0}", []any{"x"}, "No such key x"},
	}
	for _, c := range cases {
		if got := e.I18n.T("pt-BR", c.key, c.args...); got != c.want {
			t.Errorf("T(%q, %v) = %q, expected %q", c.key, c.args, got, c.want)
		}
	}

	// Catalogue returns the live map: the caller must not mutate it by
	// accident and change what everyone reads
	cat := e.I18n.Catalogue("pt-BR")
	cat["Save"] = "ADULTERADO"
	if got := e.I18n.T("pt-BR", "Save"); got != "Gravar" {
		t.Errorf("catalog was mutated by caller: T(Save) = %q", got)
	}
}

// ------------------------------------------------------------------- /app

// TestDeskIndex: /app must serve the compiled desk index (SvelteKit is
// a SPA: any /app/... route lands on the same index).
func TestDeskIndex(t *testing.T) {
	if desk.FS() == nil {
		t.Skip("desk not compiled: run `make desk`")
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
			t.Errorf("GET %s = %d, expected 200", path, res.StatusCode)
			continue
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s: content-type %q", path, ct)
		}
		if !strings.Contains(strings.ToLower(string(body[:n])), "<!doctype html") {
			t.Errorf("GET %s did not return desk index: %.120q", path, body[:n])
		}
	}

	// API routes must not be caught by desk fallback
	res, err := srv.Client().Get(srv.URL + "/api/rota-que-nao-existe")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("GET /api/nonexistent = %d, expected 404", res.StatusCode)
	}
}

// ------------------------------------------------------------------- demo

// TestDemo: `ddcore demo` is the first-run shortcut and must be able to run
// twice without duplicating anything — as promised by the seed service.
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

	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
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

// --------------------------------------------------------- field permissions

// TestFieldPermissions: the fixture's Project.budget is permlevel 1, granted to
// Project Manager only. A contributor reads the project without it and cannot
// filter on or change it; a manager reads and writes it.
func TestFieldPermissions(t *testing.T) {
	e := setup(t, "_fieldperm")
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		for email, role := range map[string]string{"manager@x.com": "Project Manager", "contributor@x.com": "Project Contributor"} {
			u, _ := c.NewDoc("User", engine.Doc{"email": email, "full_name": email, "roles": []any{map[string]any{"role": role}}})
			if _, err := c.Insert(u, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		p, _ := c.NewDoc("Project", engine.Doc{"code": "P-1", "title": "Secret budget", "assignee": "manager@x.com",
			"start_date": "2026-09-01", "budget": 5000})
		_, err := c.Insert(p, engine.SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := server(t, e)
	token := func(user string) string {
		tok, err := e.CreateAPIKey(ctx, user, "acceptance")
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	contributor, manager := token("contributor@x.com"), token("manager@x.com")

	doc := getJSON(t, srv, contributor, "/api/resource/Project/P-1")["data"].(map[string]any)
	if _, ok := doc["budget"]; ok || doc["title"] != "Secret budget" {
		t.Fatalf("contributor document: %v", doc)
	}
	if doc := getJSON(t, srv, manager, "/api/resource/Project/P-1")["data"].(map[string]any); doc["budget"] == nil {
		t.Fatalf("manager must see the budget: %v", doc)
	}
	rows := getJSON(t, srv, contributor, "/api/resource/Project?fields="+url.QueryEscape(`["*"]`))["data"].([]any)
	if _, ok := rows[0].(map[string]any)["budget"]; ok {
		t.Fatalf("list leaked budget: %v", rows)
	}
	do := func(method, path, tok string, body any) int {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "token "+tok)
		req.Header.Set("Content-Type", "application/json")
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if st := do("GET", "/api/resource/Project?filters="+url.QueryEscape(`[["budget",">",1]]`), contributor, nil); st != 403 {
		t.Fatalf("filter on budget = %d, want 403", st)
	}
	if st := do("PUT", "/api/resource/Project/P-1", contributor, map[string]any{"budget": 1}); st != 403 {
		t.Fatalf("contributor budget change = %d, want 403", st)
	}
	if st := do("PUT", "/api/resource/Project/P-1", manager, map[string]any{"budget": 6000}); st != 200 {
		t.Fatalf("manager budget change = %d", st)
	}
	err = e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		v, err := c.GetValue("Project", "P-1", "budget")
		if err != nil {
			return err
		}
		if db.Str(v) != "6000" && !strings.HasPrefix(db.Str(v), "6000.") {
			t.Fatalf("budget = %v, want 6000", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --------------------------------------------------------------- extensions

// extensaoApp extends the fixture app's Task from another app: a Custom Field,
// a property setter, and a form script for a DocType that is not its own.
func extensaoApp(t *testing.T) js.App {
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
export default defineApp({ name: "extras", title: "Extras", requires: ["testapp"] });`)
	w("extensions/task.extend.ts", `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("Task", {
  fields: [{ fieldname: "cost_centre", fieldtype: "Data", label: "Cost centre", insertAfter: "title" }],
  set: { description: { inListView: true } },
});`)
	w("extensions/task.form.ts", `import { defineForm } from "@ddcore/desk-sdk";
defineForm("Task", { refresh() {} });`)
	return js.App{Name: "extras", Dir: dir}
}

// TestExtensao: the field added by another app reaches the desk through the
// same door as the rest — translated meta — and the extending app's form script
// is served alongside the owner's.
func TestExtensao(t *testing.T) {
	e := setup(t, "_ext", extensaoApp(t))
	srv, tok := server(t, e)

	body := getJSON(t, srv, tok, "/api/meta/Task")
	data, _ := body["data"].(map[string]any)
	doctype, _ := data["doctype"].(map[string]any)
	if doctype == nil {
		t.Fatalf("meta without doctype: %#v", body)
	}
	fields, _ := doctype["fields"].([]any)
	var custom map[string]any
	for _, f := range fields {
		m, _ := f.(map[string]any)
		if m != nil && m["fieldname"] == "cost_centre" {
			custom = m
		}
	}
	if custom == nil {
		t.Fatal("extension field did not reach desk meta")
	}
	// label is translated by the catalog of whoever wrote the string: since
	// `extras` translates nothing, canonical English remains
	if custom["label"] != "Cost centre" {
		t.Errorf("label = %v", custom["label"])
	}
	if custom["app"] != "extras" {
		t.Errorf("field should declare its origin: app = %v", custom["app"])
	}
	if desc := fieldOf(fields, "description"); desc == nil || desc["inListView"] != true {
		t.Errorf("property setter did not reach meta: %#v", desc)
	}

	// both form scripts are declared, owner's first
	apps, _ := doctype["formApps"].([]any)
	if len(apps) != 2 || apps[0] != "testapp" || apps[1] != "extras" {
		t.Fatalf("formApps = %v", apps)
	}
	// and both are actually served
	for _, app := range []string{"testapp", "extras"} {
		req, _ := http.NewRequest("GET", srv.URL+"/assets/apps/"+app+"/forms/task.js", nil)
		req.Header.Set("Authorization", "token "+tok)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("form script for %s = %d", app, res.StatusCode)
		}
	}

	// and the field saves: it is a column like any other
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		p, _ := c.NewDoc("Project", engine.Doc{"code": "PRJ-EXT", "title": "Extensão", "assignee": "Admin", "start_date": "2026-01-01"})
		if _, err := c.Insert(p, engine.SaveOpts{}); err != nil {
			return err
		}
		task, _ := c.NewDoc("Task", engine.Doc{
			"code": "T-EXT", "project": p.Name(), "title": "Tarefa", "assignee": "Admin",
			"due_date": "2026-02-01", "cost_centre": "CC-1",
		})
		if _, err := c.Insert(task, engine.SaveOpts{}); err != nil {
			return err
		}
		back, err := c.GetDoc("Task", task.Name())
		if err != nil {
			return err
		}
		if got := back.Str("cost_centre"); got != "CC-1" {
			t.Fatalf("cost_centre = %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fieldOf(fields []any, name string) map[string]any {
	for _, f := range fields {
		if m, _ := f.(map[string]any); m != nil && m["fieldname"] == name {
			return m
		}
	}
	return nil
}

func runDemo(t *testing.T, e *engine.Engine, ctx context.Context) map[string]any {
	t.Helper()
	raw, err := e.RunJob(ctx, "Admin", "testapp.services.demo.generate", nil)
	if err != nil {
		t.Fatalf("run the demo: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("demo response: %v (%s)", err, raw)
	}
	return out
}

// ------------------------------------------------- schema evolution (DAT-04)

// migracaoApp writes a throwaway app with a DocType and one patch of each
// phase, so the whole route runs against a real Postgres in a second install.
func migracaoApp(t *testing.T) js.App {
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
export default defineApp({ name: "migracao", title: "Migração", roles: ["Migrador"] });`)
	w("doctypes/nota/nota.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Nota", naming: { field: "numero" }, fields: [
  { fieldname: "numero", fieldtype: "Data", label: "Number", reqd: true, unique: true },
  { fieldname: "valor_texto", fieldtype: "Data", label: "Amount (text)" },
  { fieldname: "valor", fieldtype: "Currency", label: "Amount" },
], permissions: [{ role: "Migrador", read: true, write: true, create: true }] });`)
	w("patches/0001_marca.ts", `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  phase: "beforeSchema",
  description: "records the shape the DDL is about to change",
  execute(ctx) { ctx.sql("CREATE TABLE IF NOT EXISTS acc_marca (fase text)"); ctx.sql("INSERT INTO acc_marca VALUES ('before')"); },
});`)
	w("patches/0002_backfill.ts", `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  description: "valor_texto → valor",
  execute(ctx) {
    ctx.sql("INSERT INTO acc_marca VALUES ('after')");
    ctx.sql("UPDATE tab_nota SET valor = NULLIF(valor_origem, '')::numeric WHERE valor IS NULL AND valor_origem IS NOT NULL");
  },
});`)
	return js.App{Name: "migracao", Dir: dir}
}

// TestSchemaEvolution: fresh installation records patches without running, declared
// rename preserves data, and both phases run in the right order.
func TestSchemaEvolution(t *testing.T) {
	e := setup(t, "migra", migracaoApp(t))
	ctx := context.Background()

	// Fresh installation: patches were recorded, not executed — a patch
	// describes a data change that a fresh database does not need.
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT name FROM ddcore_patch WHERE app = 'migracao' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("installation should record both patches: %v", rows)
	}
	if _, err := db.Select(ctx, e.DB.Pool, `SELECT 1 FROM acc_marca`); err == nil {
		t.Fatal("fresh installation ran patches instead of recording them")
	}

	if err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		c.Flags["ignorePermissions"] = true
		doc, err := c.NewDoc("Nota", engine.Doc{"numero": "NF-1", "valor_texto": "1250.50"})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, engine.SaveOpts{IgnorePermissions: true})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// One release later: patches are pending again and valor_texto becomes
	// valor_origem by declaration.
	if _, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_patch WHERE app = 'migracao'`); err != nil {
		t.Fatal(err)
	}
	nota, ok := e.Meta.Get("Nota")
	if !ok {
		t.Fatal("Nota is not in meta")
	}
	f := nota.Field("valor_texto")
	f.Fieldname = "valor_origem"
	f.RenamedFrom = meta.Names{"valor_texto"}
	nota.ResetFieldIndex()

	res, err := e.Migrate(ctx, true)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(res.Patches) != 2 {
		t.Fatalf("both patches should run: %v", res.Patches)
	}
	if len(res.Renames) != 1 {
		t.Fatalf("rename should be recorded: %v", res.Renames)
	}

	// Phase order is observable: before runs before DDL, after runs after.
	fases, err := db.Select(ctx, e.DB.Pool, `SELECT fase FROM acc_marca`)
	if err != nil {
		t.Fatal(err)
	}
	if len(fases) != 2 || db.Str(fases[0]["fase"]) != "before" || db.Str(fases[1]["fase"]) != "after" {
		t.Fatalf("phases out of order: %v", fases)
	}

	// Data survived the rename, and the after-patch backfill saw the
	// already-renamed column.
	got, err := db.Select(ctx, e.DB.Pool, `SELECT valor_origem, valor FROM tab_nota WHERE name = 'NF-1'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || db.Str(got[0]["valor_origem"]) != "1250.50" {
		t.Fatalf("rename lost data: %v", got)
	}
	if v := db.Str(got[0]["valor"]); v == "" || v == "0" {
		t.Fatalf("backfill did not run: %v", got)
	}
	if plan, err := e.Plan(ctx, true); err != nil || len(plan) != 0 {
		t.Fatalf("migrate did not remain idempotent: %v %v", plan, err)
	}
}

// TestRecuperacaoDeSenha validates the end-to-end flow on the real router with
// the log transport (the development default):
// forgot → token extracted from log → token inspected → reset with new password →
// login with old password fails → login with new password works → old session is dead.
func TestRecuperacaoDeSenha(t *testing.T) {
	e := setup(t, "recup")
	ctx := context.Background()

	// Capture logs to read the link emitted by the log transport
	var logBuf bytes.Buffer
	e.Log = slog.New(slog.NewTextHandler(&logBuf, nil))

	// Create user with an initial password
	const user = "recup@exemplo.com"
	const oldPwd = "senhaantiga123"
	const newPwd = "senhanova1234"

	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		d, err := c.NewDoc("User", engine.Doc{
			"email":         user,
			"full_name":     "Recuperante",
			"new_password":  oldPwd,
			"user_type":     "System User",
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	srv, _ := server(t, e)

	postJSON := func(path string, body any, cookie *http.Cookie) (*http.Response, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", srv.URL+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		res.Body.Close()
		return res, out
	}

	// 1. Login with old password works and establishes session
	resLogin, _ := postJSON("/api/login", map[string]any{"usr": user, "pwd": oldPwd}, nil)
	if resLogin.StatusCode != 200 {
		t.Fatalf("initial login failed: %d", resLogin.StatusCode)
	}
	var sidCookie *http.Cookie
	for _, c := range resLogin.Cookies() {
		if c.Name == "sid" {
			sidCookie = c
			break
		}
	}
	if sidCookie == nil {
		t.Fatal("expected sid cookie in login response")
	}

	// 2. Session is active in boot
	reqBoot, _ := http.NewRequest("GET", srv.URL+"/api/boot", nil)
	reqBoot.AddCookie(sidCookie)
	resBoot, err := srv.Client().Do(reqBoot)
	if err != nil || resBoot.StatusCode != 200 {
		t.Fatalf("boot with active session failed: %v", err)
	}
	var bootData map[string]any
	json.NewDecoder(resBoot.Body).Decode(&bootData)
	resBoot.Body.Close()
	if d, _ := bootData["data"].(map[string]any); d == nil || d["user"] != user {
		t.Fatalf("boot should recognize user %s, got %v", user, bootData)
	}

	// 3. Request password recovery
	resForgot, outForgot := postJSON("/api/auth/forgot-password", map[string]any{"usr": user}, nil)
	if resForgot.StatusCode != 200 {
		t.Fatalf("forgot-password failed: %d %v", resForgot.StatusCode, outForgot)
	}

	// 4. Extract token from log emitted by log transport
	re := regexp.MustCompile(`token=([0-9a-fA-F]+)`)
	matches := re.FindStringSubmatch(logBuf.String())
	if len(matches) < 2 {
		t.Fatalf("recovery link not found in logs:\n%s", logBuf.String())
	}
	token := matches[1]

	// 5. Peek token via /api/auth/token
	resTok, outTok := postJSON("/api/auth/token", map[string]any{"token": token}, nil)
	if resTok.StatusCode != 200 {
		t.Fatalf("auth/token failed: %d %v", resTok.StatusCode, outTok)
	}
	dTok, _ := outTok["data"].(map[string]any)
	if dTok == nil || dTok["user"] != user || dTok["kind"] != "reset" {
		t.Fatalf("unexpected token data: %v", outTok)
	}

	// 6. Complete recovery with new password
	resReset, outReset := postJSON("/api/auth/reset-password", map[string]any{"token": token, "password": newPwd}, nil)
	if resReset.StatusCode != 200 {
		t.Fatalf("reset-password failed: %d %v", resReset.StatusCode, outReset)
	}

	// 7. Spent token is single-use and is rejected
	resReused, _ := postJSON("/api/auth/reset-password", map[string]any{"token": token, "password": "outrasenha123"}, nil)
	if resReused.StatusCode != 417 {
		t.Fatalf("token reuse should be rejected with 417, got %d", resReused.StatusCode)
	}

	// 8. Previous session died immediately
	reqDead, _ := http.NewRequest("GET", srv.URL+"/api/boot", nil)
	reqDead.AddCookie(sidCookie)
	resDead, _ := srv.Client().Do(reqDead)
	var deadBoot map[string]any
	json.NewDecoder(resDead.Body).Decode(&deadBoot)
	resDead.Body.Close()
	if d, _ := deadBoot["data"].(map[string]any); d != nil && d["user"] != "Guest" {
		t.Errorf("old session should have been terminated on reset, got user=%v", d["user"])
	}

	// 9. Login with old password fails
	resOld, _ := postJSON("/api/login", map[string]any{"usr": user, "pwd": oldPwd}, nil)
	if resOld.StatusCode != 401 {
		t.Errorf("login with old password should fail with 401, got %d", resOld.StatusCode)
	}

	// 10. Login with new password works
	resNew, _ := postJSON("/api/login", map[string]any{"usr": user, "pwd": newPwd}, nil)
	if resNew.StatusCode != 200 {
		t.Errorf("login with new password should succeed with 200, got %d", resNew.StatusCode)
	}
}


// TestCoreCompatRefusesLoad: an app declaring a ddcore range the binary is
// outside of stops the load with an error naming it (PRD-07), while the
// fixture's own range lets the same site load.
func TestCoreCompatRefusesLoad(t *testing.T) {
	setup(t, "_compat")
	dir := t.TempDir()
	src := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "future", title: "Future", ddcore: ">=999.0.0" });`
	if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	dsn, _, _ := dsnFor("_compat")
	e, err := engine.New(context.Background(), engine.Config{
		DSN: dsn, Apps: []js.App{testApp(t), {Name: "future", Dir: dir}}, Lang: "pt-BR", Currency: "BRL",
	})
	if err == nil {
		e.DB.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "app future requires ddcore >=999.0.0") {
		t.Fatalf("expected the incompatible app to refuse the load, got %v", err)
	}
}
