package js

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeHost struct {
	calls []string
	mode  string
}

func (h *fakeHost) rounding() string {
	if h.mode != "" {
		return h.mode
	}
	return "commercial"
}

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
	case "site":
		return map[string]any{"currency": "USD", "currencyPrecision": 2, "rounding": h.rounding(), "timezone": "UTC"}, nil
	}
	return nil, nil
}

func TestBundleAndRun(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "doctypes/x"), 0o755)
	os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo" });`), 0o644)
	os.WriteFile(filepath.Join(dir, "doctypes/x/x.doctype.ts"), []byte(`import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "X", fields: [{ fieldname: "a", fieldtype: "Int" }] });`), 0o644)
	os.WriteFile(filepath.Join(dir, "doctypes/x/x.controller.ts"), []byte(`import { defineController, whitelisted, _ } from "@ddcore/sdk";
export default defineController("X", {
  validate(doc, ctx) { doc.a = (doc.a || 0) + ddcore.db.getValue("X", "1", "a"); if (doc.a > 100) ddcore.throw(_("Muito grande"), { title: "Limite" }); },
  methods: { dobro(doc, args) { return { v: doc.a * 2, user: ctx_user() } } },
});
function ctx_user() { return ddcore.session.user }
export const hello = whitelisted((args) => "olá " + args.nome);
export function addMonths(a) { return ddcore.utils.addMonths("2026-01-31", 1) }`), 0o644)

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
		t.Fatalf("unexpected meta: %s", m)
	}
	out, err := rt.RunHook("X", "validate", json.RawMessage(`{"doctype":"X","name":"1","a":1}`), nil)
	if err != nil || string(out) != `{"doctype":"X","name":"1","a":43}` {
		t.Fatalf("hook: %v %s", err, out)
	}
	_, err = rt.RunHook("X", "validate", json.RawMessage(`{"doctype":"X","name":"1","a":100}`), nil)
	if err == nil || err.Error() != "Limite: Muito grande" {
		t.Fatalf("expected ValidationError, got %v", err)
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
		if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "`+name+`", title: "`+name+`" });`), 0o644); err != nil {
			t.Fatal(err)
		}
		testSource := `import "@ddcore/sdk/test";
test("` + name + ` test", () => expect(true).toBe(true));`
		if name == "segundo" {
			testSource = `import "@ddcore/sdk/test";
describe("segundo", () => {
  beforeAll(() => { throw new Error("the hook of segundo must not run"); });
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
		t.Fatalf("incorrect app selection: %#v", results)
	}
}

// B08 — a runtime acquired before reload returns to the pool that created it, never
// to the new pool, and the old pool discards whatever it receives after being closed.
func TestB08_ReleaseReturnsToOriginPool(t *testing.T) {
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
	p1.Close() // reload: p1 is the old pool
	// returning through the new pool must not contaminate the new pool
	p2.Release(rt)
	rt2, err := p2.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if rt2 == rt {
		t.Fatalf("old VM entered the new pool")
	}
	p1.mu.Lock()
	n := len(p1.free)
	p1.mu.Unlock()
	if n != 0 {
		t.Fatalf("closed pool retained %d VMs", n)
	}
	// and the old pool still serves VMs for anyone that captured the old meta
	if _, err := p1.Acquire(); err != nil {
		t.Fatalf("closed pool should continue serving: %v", err)
	}
}

// The semaphore caps how many VMs can be in use simultaneously.
func TestB08_PoolLimitsConcurrency(t *testing.T) {
	p, err := NewPool(&fakeHost{}, nil, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := p.Acquire()
	b, _ := p.Acquire()
	freeCh := make(chan struct{})
	go func() {
		c, _ := p.Acquire()
		p.Release(c)
		close(freeCh)
	}()
	select {
	case <-freeCh:
		t.Fatalf("Acquire exceeded pool limit")
	case <-time.After(100 * time.Millisecond):
	}
	p.Release(a)
	select {
	case <-freeCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("Acquire did not unblock after Release")
	}
	p.Release(b)
}

// B22 — eval advertises TypeScript: typing must be transpiled beforehand.
func TestB22_EvalTranspilesTS(t *testing.T) {
	rt, err := newRuntime(&fakeHost{}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Eval("const x: number = 1; x")
	if err != nil {
		t.Fatalf("TS eval failed: %v", err)
	}
	if string(out) != "1" {
		t.Fatalf("expected 1, got %s", out)
	}
	if out, err := rt.Eval("interface P { n: string }\nconst p: P = { n: 'ok' }; p.n"); err != nil || string(out) != `"ok"` {
		t.Fatalf("interface: %s %v", out, err)
	}
	if _, err := rt.Eval("const x: = 1"); err == nil {
		t.Fatalf("expected syntax error")
	}
}

// The app runtime rounds money too — a controller that totals a grid has to
// reach the same number the server is about to store. Both implementations are
// asserted against internal/num/testdata/rounding.json rather than against two
// hand-kept tables, because the day the tables drift is the day the form and
// the database disagree about a cent.
func TestRoundToMatchesTheSharedVectors(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "num", "testdata", "rounding.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Why        string  `json:"why"`
			V          float64 `json:"v"`
			P          int     `json:"p"`
			Commercial float64 `json:"commercial"`
			Bankers    float64 `json:"bankers"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("no vectors")
	}
	for _, mode := range []string{"commercial", "bankers"} {
		rt, err := newRuntime(&fakeHost{mode: mode}, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range f.Cases {
			want := c.Commercial
			if mode == "bankers" {
				want = c.Bankers
			}
			out, err := rt.Eval(fmt.Sprintf("ddcore.utils.roundTo(%s, %d)", strconv.FormatFloat(c.V, 'g', -1, 64), c.P))
			if err != nil {
				t.Fatalf("%v @%d %s: %v", c.V, c.P, mode, err)
			}
			got, err := strconv.ParseFloat(string(out), 64)
			if err != nil {
				t.Fatalf("%v @%d %s: roundTo returned %s", c.V, c.P, mode, out)
			}
			if got != want {
				t.Errorf("roundTo(%v, %d) [%s] = %v, want %v — %s", c.V, c.P, mode, got, want, c.Why)
			}
		}
	}
}

// splitAmount exists because rounding each of three thirds of 100.00 gives
// 33.33 three times and the invoice ends up a cent short of itself.
func TestSplitAmountAddsUp(t *testing.T) {
	rt, err := newRuntime(&fakeHost{}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		expr string
		want string
	}{
		{"ddcore.utils.splitAmount(100, 3)", "[33.34,33.33,33.33]"},
		{"ddcore.utils.splitAmount(100, 1)", "[100]"},
		{"ddcore.utils.splitAmount(10, 4)", "[2.5,2.5,2.5,2.5]"},
		{"ddcore.utils.splitAmount(0.05, 10)", "[0.01,0.01,0.01,0.01,0.01,0,0,0,0,0]"},
		{"ddcore.utils.splitAmount(-100, 3)", "[-33.34,-33.33,-33.33]"},
		{"ddcore.utils.splitAmount(0, 3)", "[0,0,0]"},
		{"ddcore.utils.splitAmount(100, 3).reduce((a, b) => a + b, 0)", "100"},
		{"ddcore.utils.splitAmount(-100, 3).reduce((a, b) => a + b, 0)", "-100"},
		{"ddcore.utils.splitAmount(1234.56, 7).reduce((a, b) => a + b, 0)", "1234.56"},
	} {
		out, err := rt.Eval(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if string(out) != c.want {
			t.Errorf("%s = %s, want %s", c.expr, out, c.want)
		}
	}
	if _, err := rt.Eval("ddcore.utils.splitAmount(100, 0)"); err == nil {
		t.Error("splitAmount(100, 0) was accepted")
	}
}

// roundCurrency is what an app calls instead of hardcoding 2, and it has to
// follow the site rather than the habit.
func TestRoundCurrencyFollowsTheSite(t *testing.T) {
	rt, err := newRuntime(&fakeHost{}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for expr, want := range map[string]string{
		"ddcore.utils.roundCurrency(10.005)":   "10.01",
		"ddcore.utils.roundCurrency(-10.005)":  "-10.01",
		"ddcore.utils.roundCurrency('10.005')": "10.01",
		"ddcore.utils.currencyPrecision()":     "2",
	} {
		out, err := rt.Eval(expr)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if string(out) != want {
			t.Errorf("%s = %s, want %s", expr, out, want)
		}
	}
}

// An app's identity is declared in its manifest, not in the path someone
// checked it out to: the same sources have to answer to the same namespace
// under apps/demo, under a repository root, and under a Dockerfile's /app.
func TestAppNameComesFromTheManifest(t *testing.T) {
	cases := []struct {
		name     string
		manifest string // "" writes no ddcore.app.ts at all
		want     string
	}{
		{"declared name wins over the directory", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo" });`, "demo"},

		{"a comment naming something else is not the declaration", `// name: "wrong" — this line is a comment
/* name: "alsowrong" */
import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo" });`, "demo"},

		{"no manifest falls back to the directory", "", "some-dir"},

		{"a name that is not an identifier falls back", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "Not An Ident", title: "x" });`, "some-dir"},

		{"a manifest that does not parse falls back", `export default defineApp({ name: "demo",`, "some-dir"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "some-dir")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if c.manifest != "" {
				if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(c.manifest), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := AppName(dir); got != c.want {
				t.Fatalf("AppName = %q, want %q", got, c.want)
			}
		})
	}
}

// The name is not just reported: it is what every module path is built from,
// which is what makes a moved checkout rename an app's whitelisted methods and
// scheduler targets.
func TestModulePathsUseTheManifestName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ddcore-demo")
	if err := os.MkdirAll(filepath.Join(dir, "services"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo" });`)
	write("services/tasks.ts", `export function markOverdue() { return 1; }`)

	b, err := BuildServer(App{Name: AppName(dir), Dir: dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.App != "demo" {
		t.Fatalf("bundle app = %q, want %q", b.App, "demo")
	}
	if want := `"demo.services.tasks"`; !strings.Contains(b.Code, want) {
		t.Fatalf("bundle does not register %s", want)
	}
	if strings.Contains(b.Code, "ddcore-demo.services") {
		t.Fatal("bundle registered modules under the directory name")
	}
}

func TestPrintTemplateRegistrationAndRender(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "print"), 0o755)
	os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(`import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo" });`), 0o644)
	os.WriteFile(filepath.Join(dir, "print/invoice.print.ts"), []byte(`import { definePrintTemplate, _ } from "@ddcore/sdk";
export default definePrintTemplate({
  name: "demo.invoice",
  doctype: "Sales Invoice",
  label: "Invoice Format",
  body: (doc, b, ctx) => [
    b.header(_("Sales Invoice"), { subtitle: doc.name }),
    b.keyValues([[_("Customer"), doc.customer]]),
    b.table([_("Item"), _("Price")], [[doc.item, ctx.formatCurrency(doc.price)]]),
  ],
});`), 0o644)

	b, err := BuildServer(App{Name: "demo", Dir: dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{}
	pool, err := NewPool(h, []*Bundle{b}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	rt, _ := pool.Acquire()
	defer pool.Release(rt)

	snapJSON, err := rt.Meta()
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		PrintTemplates map[string]struct {
			Name    string `json:"name"`
			Doctype string `json:"doctype"`
			Label   string `json:"label"`
		} `json:"printTemplates"`
	}
	if err := json.Unmarshal(snapJSON, &snap); err != nil {
		t.Fatal(err)
	}
	pt, ok := snap.PrintTemplates["demo.invoice"]
	if !ok {
		t.Fatalf("print template demo.invoice not found in snapshot: %s", snapJSON)
	}
	if pt.Doctype != "Sales Invoice" || pt.Label != "Invoice Format" {
		t.Fatalf("unexpected template metadata: %+v", pt)
	}

	resJSON, err := rt.RenderPrint("demo.invoice", `{"name":"INV-001","customer":"Alice","item":"Book","price":29.99}`, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resJSON, "Sales Invoice") || !strings.Contains(resJSON, "INV-001") {
		t.Fatalf("unexpected render output: %s", resJSON)
	}
}

