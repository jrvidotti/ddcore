package js

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeHost struct{ calls []string }

func (h *fakeHost) HostCall(rt *Runtime, op string, args json.RawMessage) (any, error) {
	h.calls = append(h.calls, op)
	switch op {
	case "session":
		return map[string]any{"user": "Administrator", "roles": []string{"System Manager"}, "lang": "pt-BR"}, nil
	case "translate":
		var a struct{ Text string }
		json.Unmarshal(args, &a)
		return a.Text, nil
	case "nowdate":
		return "2026-09-09", nil
	case "db.getValue":
		return 42, nil
	}
	return nil, nil
}

func TestBundleAndRun(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "doctypes/x"), 0o755)
	os.WriteFile(filepath.Join(dir, "cerne.app.ts"), []byte(`import { defineApp } from "@cerne/sdk";
export default defineApp({ name: "demo", title: "Demo" });`), 0o644)
	os.WriteFile(filepath.Join(dir, "doctypes/x/x.doctype.ts"), []byte(`import { defineDoctype } from "@cerne/sdk";
export default defineDoctype({ name: "X", fields: [{ fieldname: "a", fieldtype: "Int" }] });`), 0o644)
	os.WriteFile(filepath.Join(dir, "doctypes/x/x.controller.ts"), []byte(`import { defineController, whitelisted, _ } from "@cerne/sdk";
export default defineController("X", {
  validate(doc, ctx) { doc.a = (doc.a || 0) + cerne.db.getValue("X", "1", "a"); if (doc.a > 100) cerne.throw(_("Muito grande"), { title: "Limite" }); },
  methods: { dobro(doc, args) { return { v: doc.a * 2, user: ctx_user() } } },
});
function ctx_user() { return cerne.session.user }
export const hello = whitelisted((args) => "olá " + args.nome);
export function addMonths(a) { return cerne.utils.addMonths("2026-01-31", 1) }`), 0o644)

	b, err := BuildServer(App{Name: "demo", Dir: dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{}
	rt, err := newRuntime(h, []*Bundle{b}, false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := rt.Meta()
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Doctypes    map[string]map[string]any
		Whitelisted []struct{ Path string }
	}
	json.Unmarshal(m, &meta)
	if meta.Doctypes["X"]["app"] != "demo" || len(meta.Whitelisted) != 1 || meta.Whitelisted[0].Path != "demo.doctypes.x.x.controller.hello" {
		t.Fatalf("meta inesperada: %s", m)
	}
	out, err := rt.RunHook("X", "validate", json.RawMessage(`{"doctype":"X","name":"1","a":1}`), nil)
	if err != nil || string(out) != `{"doctype":"X","name":"1","a":43}` {
		t.Fatalf("hook: %v %s", err, out)
	}
	_, err = rt.RunHook("X", "validate", json.RawMessage(`{"doctype":"X","name":"1","a":100}`), nil)
	if err == nil || err.Error() != "Limite: Muito grande" {
		t.Fatalf("esperava ValidationError, veio %v", err)
	}
	r, err := rt.RunMethod("X", "dobro", json.RawMessage(`{"doctype":"X","name":"1","a":5}`), json.RawMessage(`{}`))
	if err != nil || string(r.Result) != `{"v":10,"user":"Administrator"}` {
		t.Fatalf("method: %v %s", err, r.Result)
	}
	w, err := rt.CallWhitelisted("demo.doctypes.x.x.controller.hello", json.RawMessage(`{"nome":"mundo"}`))
	if err != nil || string(w) != `"olá mundo"` {
		t.Fatalf("whitelisted: %v %s", err, w)
	}
	f, _ := rt.CallFunction("demo.doctypes.x.x.controller.addMonths", json.RawMessage(`{}`))
	if string(f) != `"2026-02-28"` {
		t.Fatalf("addMonths: %s", f)
	}
}

func TestRunTestsFiltersByApp(t *testing.T) {
	root := t.TempDir()
	build := func(name string) *Bundle {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cerne.app.ts"), []byte(`import { defineApp } from "@cerne/sdk";
export default defineApp({ name: "`+name+`", title: "`+name+`" });`), 0o644); err != nil {
			t.Fatal(err)
		}
		testSource := `import "@cerne/sdk/test";
test("` + name + ` test", () => expect(true).toBe(true));`
		if name == "segundo" {
			testSource = `import "@cerne/sdk/test";
describe("segundo", () => {
  beforeAll(() => { throw new Error("hook de segundo não deve executar"); });
  test("segundo test", () => expect(true).toBe(true));
});`
		}
		if err := os.WriteFile(filepath.Join(dir, name+`.test.ts`), []byte(testSource), 0o644); err != nil {
			t.Fatal(err)
		}
		bundle, err := BuildServer(App{Name: name, Dir: dir}, true)
		if err != nil {
			t.Fatal(err)
		}
		return bundle
	}

	rt, err := newRuntime(&fakeHost{}, []*Bundle{build("primeiro"), build("segundo")}, true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := rt.RunTests("", "primeiro")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].App != "primeiro" || results[0].Name != "primeiro test" {
		t.Fatalf("seleção de app incorreta: %#v", results)
	}
}

// B08 — um runtime adquirido antes do reload volta ao pool que o criou, nunca
// ao pool novo, e o pool antigo descarta o que recebe depois de fechado.
func TestB08_ReleaseVoltaAoPoolDeOrigem(t *testing.T) {
	h := &fakeHost{}
	p1, err := NewPool(h, nil, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := p1.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	p2, err := NewPool(h, nil, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	p1.Close() // reload: p1 é o pool antigo
	// devolver pelo pool novo não pode contaminar o pool novo
	p2.Release(rt)
	rt2, err := p2.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if rt2 == rt {
		t.Fatalf("VM antiga entrou no pool novo")
	}
	p1.mu.Lock()
	n := len(p1.free)
	p1.mu.Unlock()
	if n != 0 {
		t.Fatalf("pool fechado guardou %d VMs", n)
	}
	// e o pool antigo ainda entrega VMs para quem capturou a meta antiga
	if _, err := p1.Acquire(); err != nil {
		t.Fatalf("pool fechado deveria continuar servindo: %v", err)
	}
}

// O semáforo limita quantas VMs ficam em uso ao mesmo tempo.
func TestB08_PoolLimitaConcorrencia(t *testing.T) {
	p, err := NewPool(&fakeHost{}, nil, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := p.Acquire()
	b, _ := p.Acquire()
	livre := make(chan struct{})
	go func() {
		c, _ := p.Acquire()
		p.Release(c)
		close(livre)
	}()
	select {
	case <-livre:
		t.Fatalf("Acquire passou do limite do pool")
	case <-time.After(100 * time.Millisecond):
	}
	p.Release(a)
	select {
	case <-livre:
	case <-time.After(2 * time.Second):
		t.Fatalf("Acquire não destravou depois do Release")
	}
	p.Release(b)
}

// B22 — eval anuncia TypeScript: tipagem precisa ser transpilada antes.
func TestB22_EvalTranspilaTS(t *testing.T) {
	rt, err := newRuntime(&fakeHost{}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Eval("const x: number = 1; x")
	if err != nil {
		t.Fatalf("eval de TS falhou: %v", err)
	}
	if string(out) != "1" {
		t.Fatalf("esperava 1, veio %s", out)
	}
	if out, err := rt.Eval("interface P { n: string }\nconst p: P = { n: 'ok' }; p.n"); err != nil || string(out) != `"ok"` {
		t.Fatalf("interface: %s %v", out, err)
	}
	if _, err := rt.Eval("const x: = 1"); err == nil {
		t.Fatalf("esperava erro de sintaxe")
	}
}
