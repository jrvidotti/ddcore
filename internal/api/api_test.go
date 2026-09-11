package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/mcp"
)

// The api tests use the database named in DDCORE_TEST_DSN with an "_api"
// suffix so they never collide with the engine package tests; that database
// is dropped and recreated by setup.
func testDSN() (dsn, adminDSN, dbName string) {
	base := os.Getenv("DDCORE_TEST_DSN")
	if base == "" {
		base = "postgres://ddcore:ddcore@localhost:5455/ddcore_test?sslmode=disable"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", "", ""
	}
	dbName = strings.TrimPrefix(u.Path, "/") + "_api"
	u.Path = "/" + dbName
	dsn = u.String()
	u.Path = "/postgres"
	return dsn, u.String(), dbName
}

func testApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"] });`)
	w("doctypes/pessoa/pessoa.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" }, titleField: "nome", trackChanges: true,
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "tipo", fieldtype: "Select", label: "Tipo", options: ["PF", "PJ"], default: "PF" },
    { fieldname: "contatos", fieldtype: "Table", label: "Contatos", options: "Contato Pessoa" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, report: true }, { role: "All", read: true, ifOwner: true }] });`)
	w("doctypes/contato_pessoa/contato_pessoa.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Contato Pessoa", isChild: true, fields: [
  { fieldname: "telefone", fieldtype: "Data", label: "Telefone" } ] });`)
	w("doctypes/item/item.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Item Pedido", isChild: true, fields: [
  { fieldname: "descricao", fieldtype: "Data", label: "Descrição", reqd: true },
  { fieldname: "qtd", fieldtype: "Int", label: "Qtd", default: 1 } ] });`)
	w("doctypes/nota/nota.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Nota Pedido", isChild: true, fields: [
  { fieldname: "texto", fieldtype: "Data", label: "Texto" } ] });`)
	w("doctypes/pedido/pedido.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pedido", naming: { series: "PED-.####" }, submittable: true, trackChanges: true,
  fields: [
    { fieldname: "cliente", fieldtype: "Link", label: "Cliente", options: "Pessoa", reqd: true },
    { fieldname: "obs", fieldtype: "Small Text", label: "Obs", allowOnSubmit: true },
    { fieldname: "itens", fieldtype: "Table", label: "Itens", options: "Item Pedido" },
    { fieldname: "notas", fieldtype: "Table", label: "Notas", options: "Nota Pedido", allowOnSubmit: true },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true, report: true }] });`)
	w("workspaces/demo.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Demo", label: "Demo", roles: ["Gestor"],
  sidebar: [{ label: "Pessoas", doctype: "Pessoa" }],
  numberCards: [
    { name: "pessoas", label: "Pessoas", doctype: "Pessoa" },
    { name: "segredo", label: "Segredo", method() { return { value: 42 }; } },
  ],
  charts: [{ name: "grafico", label: "Gráfico", type: "bar", method() { return { type: "bar", labels: ["a"], datasets: [{ name: "x", values: [1] }] }; } }] });`)
	w("workspaces/aberto.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Aberto", label: "Aberto", sidebar: [],
  numberCards: [{ name: "pedidos", label: "Pedidos", doctype: "Pedido" }] });`)
	w("reports/pessoas.report.ts", `import { defineReport } from "@ddcore/sdk";
export default defineReport({ name: "Pessoas", refDoctype: "Pessoa", roles: ["Gestor"], filters: [],
  execute() { return { columns: [{ fieldname: "name", label: "Nome" }], rows: ddcore.db.getList("Pessoa", { fields: ["name"] }) }; } });`)
	// "Loop" is a key the core catalogue does not have: the app supplies a
	// translation for it, so a test can prove the border does *not* apply it
	// to a message the JS runtime already translated.
	w("translations/en.csv", "Loop,Looped\n")
	w("doctypes/pessoa/pessoa.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Pessoa", {
  validate(doc) { if (doc.nome === "loop") ddcore.throw("Loop"); },
});`)
	w("services/i18n.ts", `import { whitelisted, _ } from "@ddcore/sdk";
export const echo = whitelisted(() => ({ save: _("Save"), n: _("Loop") }));`)
	w("reports/livre.report.ts", `import { defineReport } from "@ddcore/sdk";
export default defineReport({ name: "Livre", refDoctype: "Pedido", filters: [],
  execute() { return { columns: [], rows: [] }; } });`)
	return dir
}

type env struct {
	t   *testing.T
	e   *engine.Engine
	s   *Server
	ts  *httptest.Server
	ctx context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dsn, adminDSN, dbName := testDSN()
	if dbName == "" {
		t.Fatal("DDCORE_TEST_DSN inválida")
	}
	e0, err := engine.New(ctx, engine.Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres indisponível em DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres indisponível: %v", err)
	}
	e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	e0.DB.Close()
	e, err := engine.New(ctx, engine.Config{DSN: dsn, Apps: []js.App{{Name: "demo", Dir: testApp(t)}}, Test: true, DataDir: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	s := New(e, nil)
	s.MCPHandler = mcp.HTTPHandler(e)
	s.Router.Handle("/mcp", s.RequireAdminAPIKey(s.MCPHandler))
	s.Router.Handle("/mcp/*", s.RequireAdminAPIKey(s.MCPHandler))
	ts := httptest.NewServer(s.Router)
	t.Cleanup(func() { ts.Close(); e.DB.Close() })
	x := &env{t: t, e: e, s: s, ts: ts, ctx: ctx}
	// users: ana (Gestor), ze (sem papel), root (System Manager)
	x.asAdmin(func(c *engine.Ctx) error {
		for _, u := range []struct{ email, role string }{{"ana@x.com", "Gestor"}, {"bia@x.com", "Gestor"}, {"ze@x.com", ""}, {"root@x.com", "System Manager"}} {
			d, _ := c.NewDoc("User", engine.Doc{"email": u.email, "full_name": u.email, "new_password": "segredo"})
			if u.role != "" {
				d["roles"] = []any{map[string]any{"role": u.role}}
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err := e.SetPassword(ctx, "Administrator", "admin"); err != nil {
		t.Fatal(err)
	}
	return x
}

func (x *env) asAdmin(fn func(c *engine.Ctx) error) {
	x.t.Helper()
	if err := x.e.Run(x.ctx, "Administrator", fn); err != nil {
		x.t.Fatal(err)
	}
}

func (x *env) as(user string, fn func(c *engine.Ctx) error) error {
	return x.e.Run(x.ctx, user, fn)
}

// sid logs a user in and returns the session cookie value.
func (x *env) sid(user string) string {
	x.t.Helper()
	pwd := "segredo"
	if user == "Administrator" {
		pwd = "admin"
	}
	sid, err := x.e.Login(x.ctx, user, pwd)
	if err != nil {
		x.t.Fatal(err)
	}
	return sid
}

func (x *env) apiKey(user string) string {
	x.t.Helper()
	k, err := x.e.CreateAPIKey(x.ctx, user, "test")
	if err != nil {
		x.t.Fatal(err)
	}
	return k
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
	Header http.Header
}

func (r resp) errType() string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		return fmt.Sprint(e["type"])
	}
	return ""
}

// call performs a request; auth is "" (Guest), "sid:<sid>" or "token:<key>".
func (x *env) call(method, path string, body any, auth string, hdr ...string) resp {
	x.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, x.ts.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "test")
	if strings.HasPrefix(auth, "sid:") {
		req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
	} else if strings.HasPrefix(auth, "token:") {
		req.Header.Set("Authorization", "token "+strings.TrimPrefix(auth, "token:"))
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
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

func (x *env) expect(r resp, status int, errType string) {
	x.t.Helper()
	if r.Status != status || (errType != "" && r.errType() != errType) {
		x.t.Fatalf("esperava %d %s, veio %d %s: %s", status, errType, r.Status, r.errType(), r.Raw)
	}
}

// ------------------------------------------------------------------ B01

const mcpSQL = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sql_query","arguments":{"query":"SELECT current_user AS db_user"}}}`

func (x *env) mcpCall(auth string) resp {
	x.t.Helper()
	req, _ := http.NewRequest("POST", x.ts.URL+"/mcp", strings.NewReader(mcpSQL))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if strings.HasPrefix(auth, "sid:") {
		req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
		req.Header.Set("X-Requested-With", "test")
	} else if strings.HasPrefix(auth, "token:") {
		req.Header.Set("Authorization", "token "+strings.TrimPrefix(auth, "token:"))
	}
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

func TestB01_MCPRequiresAPIKey(t *testing.T) {
	x := setup(t)
	// sem credencial
	x.expect(x.mcpCall(""), 401, "AuthenticationError")
	// sessão por cookie, mesmo Administrator, não vale para o MCP HTTP
	x.expect(x.mcpCall("sid:"+x.sid("Administrator")), 401, "AuthenticationError")
	// chave inválida
	x.expect(x.mcpCall("token:abc:def"), 401, "AuthenticationError")
	// chave válida de usuário sem papel administrativo
	x.expect(x.mcpCall("token:"+x.apiKey("ze@x.com")), 403, "PermissionError")
	x.expect(x.mcpCall("token:"+x.apiKey("ana@x.com")), 403, "PermissionError")
	// System Manager e Administrator passam
	for _, u := range []string{"root@x.com", "Administrator"} {
		r := x.mcpCall("token:" + x.apiKey(u))
		if r.Status != 200 || !strings.Contains(r.Raw, "db_user") {
			t.Fatalf("%s: esperava 200 com resultado, veio %d: %s", u, r.Status, r.Raw)
		}
	}
}

// ------------------------------------------------------------------ B02

func TestB02_ChildResourceEndpointsFollowParent(t *testing.T) {
	x := setup(t)
	var roleRow string
	x.asAdmin(func(c *engine.Ctx) error {
		u, err := c.GetDoc("User", "Administrator")
		if err != nil {
			return err
		}
		roleRow = u.Children("roles")[0].Name()
		return nil
	})
	// Guest e usuário sem papel não leem a linha de Has Role
	for _, auth := range []string{"", "sid:" + x.sid("ze@x.com")} {
		x.expect(x.call("GET", "/api/resource/Has%20Role/"+roleRow, nil, auth), 403, "PermissionError")
		x.expect(x.call("GET", "/api/resource/Has%20Role", nil, auth), 403, "PermissionError")
		// mutações em filho nunca passam pela API genérica
		x.expect(x.call("PUT", "/api/resource/Has%20Role/"+roleRow, map[string]any{"role": "Guest"}, auth), 417, "ValidationError")
		x.expect(x.call("POST", "/api/resource/Has%20Role", map[string]any{"role": "Guest"}, auth), 417, "ValidationError")
		x.expect(x.call("DELETE", "/api/resource/Has%20Role/"+roleRow, nil, auth), 417, "ValidationError")
		x.expect(x.call("POST", "/api/resource/Has%20Role/"+roleRow+"/save", map[string]any{}, auth), 417, "ValidationError")
	}
	// a linha continua intacta
	x.asAdmin(func(c *engine.Ctx) error {
		v, err := c.GetValue("Has Role", roleRow, "role")
		if err != nil {
			return err
		}
		if v != "System Manager" {
			t.Fatalf("linha alterada: %v", v)
		}
		return nil
	})
	// nem para Administrator: filho se edita pelo pai
	x.expect(x.call("PUT", "/api/resource/Has%20Role/"+roleRow, map[string]any{"role": "Guest"}, "sid:"+x.sid("Administrator")), 417, "ValidationError")

	// Gestor lê linhas do próprio Pedido; usuário sem papel não
	var item string
	x.asAdmin(func(c *engine.Ctx) error {
		pes, _ := c.NewDoc("Pessoa", engine.Doc{"nome": "Cliente"})
		if _, err := c.Insert(pes, engine.SaveOpts{}); err != nil {
			return err
		}
		p, _ := c.NewDoc("Pedido", engine.Doc{"cliente": "Cliente"})
		p["itens"] = []any{map[string]any{"descricao": "a"}}
		saved, err := c.Insert(p, engine.SaveOpts{})
		if err != nil {
			return err
		}
		item = saved.Children("itens")[0].Name()
		return nil
	})
	if r := x.call("GET", "/api/resource/Item%20Pedido/"+item, nil, "sid:"+x.sid("ana@x.com")); r.Status != 200 {
		t.Fatalf("Gestor deveria ler a linha: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", "/api/resource/Item%20Pedido?fields=%5B%22name%22%5D", nil, "sid:"+x.sid("ana@x.com")); r.Status != 200 {
		t.Fatalf("Gestor deveria listar linhas: %d %s", r.Status, r.Raw)
	}
	x.expect(x.call("GET", "/api/resource/Item%20Pedido/"+item, nil, "sid:"+x.sid("ze@x.com")), 403, "PermissionError")
	x.expect(x.call("GET", "/api/resource/Item%20Pedido", nil, ""), 403, "PermissionError")
}

// ------------------------------------------------------------------ B05

func TestB05_WorkspaceAndReportRequirePermission(t *testing.T) {
	x := setup(t)
	guestPaths := []string{
		"/api/workspace/Demo/card/pessoas",
		"/api/workspace/Demo/chart/grafico",
		"/api/report/Pessoas",
		"/api/events",
		"/api/search/link?doctype=Pessoa&txt=a",
		"/api/comments/Pessoa/x",
		"/api/versions/Pessoa/x",
	}
	for _, p := range guestPaths {
		x.expect(x.call("GET", p, nil, ""), 401, "AuthenticationError")
	}
	if r := x.call("GET", "/api/boot", nil, ""); r.Status != 200 {
		t.Fatalf("/api/boot deveria continuar público: %d %s", r.Status, r.Raw)
	}

	ze, ana := "sid:"+x.sid("ze@x.com"), "sid:"+x.sid("ana@x.com")
	// workspace com papel: usuário sem o papel não lê card nem gráfico
	x.expect(x.call("GET", "/api/workspace/Demo/card/segredo", nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/workspace/Demo/chart/grafico", nil, ze), 403, "PermissionError")
	// workspace aberto, mas o card agrega um doctype sem permissão
	x.expect(x.call("GET", "/api/workspace/Aberto/card/pedidos", nil, ze), 403, "PermissionError")
	// relatórios: por papel e por permissão de relatório no refDoctype
	x.expect(x.call("GET", "/api/report/Pessoas", nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/report/Livre", nil, ze), 403, "PermissionError")
	for _, p := range []string{"/api/workspace/Demo/card/segredo", "/api/workspace/Demo/chart/grafico", "/api/report/Pessoas", "/api/report/Livre"} {
		if r := x.call("GET", p, nil, ana); r.Status != 200 {
			t.Fatalf("Gestor deveria acessar %s: %d %s", p, r.Status, r.Raw)
		}
	}
}

// ------------------------------------------------------------------ B04

func TestB04_VersionAndCommentFollowReference(t *testing.T) {
	x := setup(t)
	ana, bia, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("ze@x.com")

	// Pessoa criada e alterada pela Ana gera uma Version
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Reservada"}, ana)
	if r.Status != 200 {
		t.Fatalf("criar Pessoa: %d %s", r.Status, r.Raw)
	}
	if r := x.call("PUT", "/api/resource/Pessoa/Reservada", map[string]any{"tipo": "PJ"}, ana); r.Status != 200 {
		t.Fatalf("alterar Pessoa: %d %s", r.Status, r.Raw)
	}
	var version string
	x.asAdmin(func(c *engine.Ctx) error {
		rows, err := c.GetList("Version", engine.ListArgs{Filters: map[string]any{"ref_doctype": "Pessoa", "docname": "Reservada"}, Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			t.Fatal("nenhuma Version gravada")
		}
		version = fmt.Sprint(rows[0]["name"])
		return nil
	})

	verFilter := `/api/resource/Version?filters=` + url.QueryEscape(`{"ref_doctype":"Pessoa","docname":"Reservada"}`)
	// usuário sem permissão na Pessoa não alcança o histórico por nenhum caminho
	x.expect(x.call("GET", "/api/versions/Pessoa/Reservada", nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/resource/Version", nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", verFilter, nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/resource/Version/"+version, nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/count/Version", nil, ze), 403, "PermissionError")
	// quem lê a Pessoa lê o histórico
	for _, p := range []string{"/api/versions/Pessoa/Reservada", verFilter, "/api/resource/Version/" + version} {
		if r := x.call("GET", p, nil, ana); r.Status != 200 {
			t.Fatalf("Gestor deveria ler %s: %d %s", p, r.Status, r.Raw)
		}
	}

	// comentários seguem o documento comentado; autoria manda na edição
	r = x.call("POST", "/api/resource/Comment", map[string]any{"reference_doctype": "Pessoa", "reference_name": "Reservada", "content": "oi"}, ana)
	if r.Status != 200 {
		t.Fatalf("comentar: %d %s", r.Status, r.Raw)
	}
	comment := fmt.Sprint(r.Body["data"].(map[string]any)["name"])
	x.expect(x.call("GET", "/api/comments/Pessoa/Reservada", nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/resource/Comment/"+comment, nil, ze), 403, "PermissionError")
	x.expect(x.call("GET", "/api/resource/Comment", nil, ze), 403, "PermissionError")
	x.expect(x.call("POST", "/api/resource/Comment", map[string]any{"reference_doctype": "Pessoa", "reference_name": "Reservada", "content": "invasor"}, ze), 403, "PermissionError")
	if r := x.call("GET", "/api/comments/Pessoa/Reservada", nil, bia); r.Status != 200 {
		t.Fatalf("outro Gestor deveria ler comentários: %d %s", r.Status, r.Raw)
	}
	x.expect(x.call("PUT", "/api/resource/Comment/"+comment, map[string]any{"content": "editado por outro"}, bia), 403, "PermissionError")
	x.expect(x.call("DELETE", "/api/resource/Comment/"+comment, nil, bia), 403, "PermissionError")
	if r := x.call("PUT", "/api/resource/Comment/"+comment, map[string]any{"content": "editado pela autora"}, ana); r.Status != 200 {
		t.Fatalf("autora deveria editar: %d %s", r.Status, r.Raw)
	}
	// System Manager continua com acesso administrativo
	if r := x.call("GET", "/api/resource/Version", nil, "sid:"+x.sid("root@x.com")); r.Status != 200 {
		t.Fatalf("System Manager deveria listar versões: %d %s", r.Status, r.Raw)
	}
}

// ------------------------------------------------------------------ B20

func TestB20_EventAuthorizerFollowsPermissions(t *testing.T) {
	x := setup(t)
	ana := x.s.eventAuthorizer(x.ctx, "ana@x.com")
	ze := x.s.eventAuthorizer(x.ctx, "ze@x.com")
	guest := x.s.eventAuthorizer(x.ctx, "Guest")
	if !ana("Pedido", "PED-0001") {
		t.Error("Gestor deveria receber eventos de Pedido")
	}
	if ze("Pedido", "PED-0001") {
		t.Error("usuário sem papel não deveria receber eventos de Pedido")
	}
	if guest("Pedido", "PED-0001") {
		t.Error("Guest não deveria receber eventos de Pedido")
	}
	// a resposta é memorizada por usuário/doctype
	if _, ok := x.e.Cache.Get("evperm:ze@x.com:Pedido"); !ok {
		t.Error("a decisão deveria ficar em cache")
	}
	// e chega ao hub: o evento de um doctype restrito não é entregue
	ch := x.e.Events.Subscribe("ze@x.com", ze)
	defer x.e.Events.Unsubscribe(ch)
	x.e.Events.Publish(engine.Event{Name: "doc_update", Doctype: "Pedido", DocName: "PED-0001"})
	select {
	case ev := <-ch:
		t.Fatalf("evento restrito entregue: %+v", ev)
	default:
	}
}

// events sem sessão não pode sequer abrir o stream SSE.
func TestB20_EventsRequireAuth(t *testing.T) {
	x := setup(t)
	x.expect(x.call("GET", "/api/events", nil, ""), 401, "AuthenticationError")
	// e o visitante anônimo não deixa assinatura pendurada no hub
	x.e.Events.Publish(engine.Event{Name: "ping"})

	req, _ := http.NewRequest("GET", x.ts.URL+"/api/events", nil)
	req.Header.Set("X-Requested-With", "test")
	req.AddCookie(&http.Cookie{Name: "sid", Value: x.sid("ana@x.com")})
	ctx, cancel := context.WithCancel(x.ctx)
	defer cancel()
	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("usuário logado deveria abrir o stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	buf := make([]byte, 64)
	n, _ := res.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: hello") {
		t.Fatalf("esperava o hello do SSE, veio %q", string(buf[:n]))
	}
}

// ------------------------------------------------------------------ lacunas
// (tabela "Outras lacunas e melhorias" de docs/check-v1.md)

// Credenciais de usuário desativado: a chave de API para de valer assim que
// o usuário é desativado, sem esperar o TTL do cache.
func TestLacuna_APIKeyDeUsuarioDesativado(t *testing.T) {
	x := setup(t)
	key := x.apiKey("ana@x.com")
	if r := x.call("GET", "/api/boot", nil, "token:"+key); r.Status != 200 {
		t.Fatalf("chave válida deveria funcionar: %d %s", r.Status, r.Raw)
	}
	x.asAdmin(func(c *engine.Ctx) error {
		u, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		u["enabled"] = false
		_, err = c.Save(u, engine.SaveOpts{})
		return err
	})
	x.expect(x.call("GET", "/api/boot", nil, "token:"+key), 401, "AuthenticationError")
	// as chaves dos demais usuários continuam íntegras
	x.expect(x.mcpCall("token:"+x.apiKey("root@x.com")), 200, "")
}

// Nome com caractere escapado na URL: o chi roteia pelo RawPath quando o
// caminho traz um escape que o Go não produziria sozinho (o "@" de um e-mail
// vira %40), e o parâmetro chega ainda codificado. Todo parâmetro de rota tem
// de ser decodificado antes de virar nome de documento.
func TestLacuna_ParametroDeRotaPercentCodificado(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Administrator")
	for _, p := range []string{
		"/api/resource/User/ana%40x.com",
		"/api/comments/User/ana%40x.com",
		"/api/versions/User/ana%40x.com",
	} {
		if r := x.call("GET", p, nil, admin); r.Status != 200 {
			t.Fatalf("%s: esperava 200, veio %d %s", p, r.Status, r.Raw)
		}
	}
	// e o escape que o Go também produziria (o espaço) continua funcionando
	if r := x.call("GET", "/api/meta/Has%20Role", nil, admin); r.Status != 200 {
		t.Fatalf("meta de doctype com espaço: %d %s", r.Status, r.Raw)
	}
}

// upload envia um arquivo e devolve a resposta.
func (x *env) upload(auth, filename, content string) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write([]byte(content))
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "test")
	req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
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

// Uploads: limite explícito do corpo e nome de arquivo aleatório.
func TestLacuna_UploadLimiteENomeAleatorio(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	for _, tc := range []struct{ name, ext string }{
		{"contrato secreto.PDF", ".pdf"},
		{"payload.html", ".bin"},
		{"../../etc/passwd", ""}, // sem extensão: nada do nome original sobrevive
	} {
		r := x.upload(ana, tc.name, "conteudo")
		if r.Status != 200 {
			t.Fatalf("upload %s: %d %s", tc.name, r.Status, r.Raw)
		}
		u := fmt.Sprint(r.Body["data"].(map[string]any)["file_url"])
		base := strings.TrimPrefix(u, "/private/files/")
		if !strings.HasSuffix(base, tc.ext) {
			t.Errorf("%s: extensão esperada %s, veio %s", tc.name, tc.ext, base)
		}
		stem := strings.TrimSuffix(base, tc.ext)
		if len(stem) < 32 || strings.ContainsAny(stem, "._-/ ") {
			t.Errorf("%s: nome deveria ser aleatório, veio %s", tc.name, base)
		}
	}
	x.s.MaxUpload = 1 << 10
	x.expect(x.upload(ana, "grande.pdf", strings.Repeat("x", 4<<10)), 417, "ValidationError")
}

// ETag: getMeta responde 304 quando a meta não mudou.
func TestLacuna_MetaComETag(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	r := x.call("GET", "/api/meta/Pessoa", nil, ana)
	if r.Status != 200 {
		t.Fatalf("meta: %d %s", r.Status, r.Raw)
	}
	etag := r.Header.Get("ETag")
	if etag == "" {
		t.Fatal("meta deveria devolver ETag")
	}
	r2 := x.call("GET", "/api/meta/Pessoa", nil, ana, "If-None-Match", etag)
	if r2.Status != 304 || r2.Raw != "" {
		t.Fatalf("esperava 304 vazio, veio %d %s", r2.Status, r2.Raw)
	}
	if same := x.call("GET", "/api/meta/Pessoa", nil, ana).Header.Get("ETag"); same != etag {
		t.Fatalf("ETag deveria ser estável: %s != %s", same, etag)
	}
	// outro usuário tem outras permissões, logo outro ETag
	if other := x.call("GET", "/api/meta/Pessoa", nil, "sid:"+x.sid("ze@x.com")).Header.Get("ETag"); other == etag {
		t.Fatal("ETag deveria variar com as permissões do usuário")
	}
}

func TestLinkTitlesAPI(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Administrator")
	// Test GET /api/search/link-titles
	r := x.call("GET", "/api/search/link-titles?doctype=User&names=Administrator", nil, admin)
	if r.Status != 200 {
		t.Fatalf("link-titles GET: %d %s", r.Status, r.Raw)
	}
	var data map[string]any
	json.Unmarshal([]byte(r.Raw), &data)
	d, ok := data["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", data)
	}
	userMap, ok := d["User"].(map[string]any)
	if !ok || userMap["Administrator"] != "Administrator" {
		t.Fatalf("expected User.Administrator title, got %v", d)
	}

	// Test POST /api/search/link-titles
	rPost := x.call("POST", "/api/search/link-titles", map[string]any{"User": []string{"Administrator"}}, admin)
	if rPost.Status != 200 {
		t.Fatalf("link-titles POST: %d %s", rPost.Status, rPost.Raw)
	}

	// Test boot contains titleField
	rBoot := x.call("GET", "/api/boot", nil, admin)
	if rBoot.Status != 200 {
		t.Fatalf("boot: %d", rBoot.Status)
	}
	var bootData map[string]any
	json.Unmarshal([]byte(rBoot.Raw), &bootData)
	bData := bootData["data"].(map[string]any)
	dts := bData["doctypes"].(map[string]any)
	userMeta := dts["User"].(map[string]any)
	if userMeta["titleField"] != "full_name" {
		t.Fatalf("expected User titleField 'full_name', got %v", userMeta["titleField"])
	}
}

func TestLinkFieldSearchAPI(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Administrator")

	// Cria Pessoa e Pedido
	r1 := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Carlos Comércio", "tipo": "PF"}, admin)
	if r1.Status != 200 {
		t.Fatalf("cria pessoa: %d %s", r1.Status, r1.Raw)
	}
	r2 := x.call("POST", "/api/resource/Pedido", map[string]any{"cliente": "Carlos Comércio"}, admin)
	if r2.Status != 200 {
		t.Fatalf("cria pedido: %d %s", r2.Status, r2.Raw)
	}

	// Busca sem acento no campo Link pelo título "Comércio".
	orFilters := url.QueryEscape(`[["cliente","like","%comercio%"]]`)
	rList := x.call("GET", "/api/resource/Pedido?or_filters="+orFilters+"&with_count=true", nil, admin)
	if rList.Status != 200 {
		t.Fatalf("list com or_filters falhou: %d %s", rList.Status, rList.Raw)
	}
	var res map[string]any
	json.Unmarshal([]byte(rList.Raw), &res)
	data := res["data"].(map[string]any)
	rows := data["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("esperava 1 pedido buscando por 'comercio', veio %d (%v)", len(rows), rows)
	}
	if int(data["count"].(float64)) != 1 {
		t.Fatalf("esperava count 1, veio %v", data["count"])
	}

	// Endpoint count com or_filters
	rCount := x.call("GET", "/api/count/Pedido?or_filters="+orFilters, nil, admin)
	if rCount.Status != 200 {
		t.Fatalf("count com or_filters falhou: %d %s", rCount.Status, rCount.Raw)
	}
	var countRes map[string]any
	json.Unmarshal([]byte(rCount.Raw), &countRes)
	if int(countRes["data"].(float64)) != 1 {
		t.Fatalf("esperava count 1, veio %v", countRes["data"])
	}

	// Busca com termo inexistente
	noMatch := url.QueryEscape(`[["cliente","like","%NaoExiste%"]]`)
	rEmpty := x.call("GET", "/api/resource/Pedido?or_filters="+noMatch+"&with_count=true", nil, admin)
	if rEmpty.Status != 200 {
		t.Fatalf("list sem match falhou: %d", rEmpty.Status)
	}
	var emptyRes map[string]any
	json.Unmarshal([]byte(rEmpty.Raw), &emptyRes)
	emptyData := emptyRes["data"].(map[string]any)
	if len(emptyData["rows"].([]any)) != 0 || int(emptyData["count"].(float64)) != 0 {
		t.Fatalf("esperava 0 resultados para termo inexistente, veio %v", emptyData)
	}
}
