package js

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/dop251/goja"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

//go:embed prelude.js
var prelude string

// Host is implemented by the engine: every `ddcore.*` call from TS lands here
// with a JSON-encoded argument object and returns JSON.
type Host interface {
	HostCall(rt *Runtime, op string, args json.RawMessage) (any, error)
}

// Runtime is one goja VM with the prelude and all app bundles loaded.
// Not goroutine-safe: use Pool.
type Runtime struct {
	vm   *goja.Runtime
	reg  *goja.Object
	host Host
	// pool is the pool that created this VM. Release always goes back to it,
	// nunca ao pool corrente: um reload troca o pool e uma VM com o bundle
	// antigo não pode entrar no pool novo (B08).
	pool *Pool
	// Ctx is the engine's per-request context, set by the engine while the
	// runtime is checked out from the pool.
	Ctx any
	// Test marks runtimes built with test files.
	Test bool
}

func newRuntime(host Host, bundles []*Bundle, test bool) (*Runtime, error) {
	rt := &Runtime{vm: goja.New(), host: host, Test: test}
	rt.vm.SetFieldNameMapper(goja.UncapFieldNameMapper())
	rt.vm.Set("__host", func(op string, args string) (string, error) {
		res, err := host.HostCall(rt, op, json.RawMessage(args))
		if err != nil {
			e := cerr.From(err)
			b, _ := json.Marshal(e)
			return "", errors.New("ddcore:" + string(b))
		}
		if res == nil {
			return "", nil
		}
		if s, ok := res.(string); ok && op == "__raw" {
			return s, nil
		}
		b, err := json.Marshal(res)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
	if test {
		rt.vm.Set("__ddcoreTest", true)
	}
	if _, err := rt.vm.RunScript("prelude.js", prelude); err != nil {
		return nil, fmt.Errorf("prelude: %w", err)
	}
	rt.reg = rt.vm.Get("__ddcore").ToObject(rt.vm)
	for _, b := range bundles {
		if err := rt.load(b); err != nil {
			return nil, err
		}
	}
	return rt, nil
}

func (rt *Runtime) load(b *Bundle) error {
	mod := rt.vm.NewObject()
	exp := rt.vm.NewObject()
	mod.Set("exports", exp)
	fn, err := rt.vm.RunScript(b.App+".bundle.js", "(function(module, exports){\n"+b.Code+"\n})")
	if err != nil {
		return fmt.Errorf("app %s: %w", b.App, err)
	}
	call, _ := goja.AssertFunction(fn)
	if _, err := call(goja.Undefined(), mod, exp); err != nil {
		return fmt.Errorf("app %s: %w", b.App, toGoError(err))
	}
	return nil
}

// toGoError converts a goja exception into a *cerr.Error where possible.
func toGoError(err error) error {
	var exc *goja.Exception
	if !errors.As(err, &exc) {
		return err
	}
	v := exc.Value()
	if obj, ok := v.(*goja.Object); ok {
		if t := obj.Get("ddcoreType"); t != nil && !goja.IsUndefined(t) {
			e := &cerr.Error{Type: t.String(), Message: str(obj.Get("message")), Title: str(obj.Get("title"))}
			e.Status = statusFor(e.Type)
			if x := obj.Get("extra"); x != nil && !goja.IsUndefined(x) {
				e.Extra = x.Export()
			}
			return e
		}
		// A Go error thrown through the bridge: message carries the JSON.
		msg := str(obj.Get("message"))
		if i := strings.Index(msg, "ddcore:{"); i >= 0 {
			var e cerr.Error
			if json.Unmarshal([]byte(msg[i+6:]), &e) == nil {
				e.Status = statusFor(e.Type)
				return &e
			}
		}
		stack := str(obj.Get("stack"))
		if stack != "" {
			return &cerr.Error{Type: "ScriptError", Status: 500, Message: stack}
		}
	}
	return &cerr.Error{Type: "ScriptError", Status: 500, Message: exc.String()}
}

func str(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

func statusFor(t string) int {
	switch t {
	case "ValidationError", "MandatoryError", "LinkExistsError":
		return 417
	case "PermissionError":
		return 403
	case "DoesNotExistError", "NotFound":
		return 404
	case "TimestampMismatchError", "DuplicateEntryError":
		return 409
	case "AuthenticationError":
		return 401
	}
	return 500
}

// callReg invokes __ddcore.<name>(args...) and returns the result string.
func (rt *Runtime) callReg(name string, args ...any) (string, error) {
	fn, ok := goja.AssertFunction(rt.reg.Get(name))
	if !ok {
		return "", fmt.Errorf("__ddcore.%s is not a function", name)
	}
	vals := make([]goja.Value, len(args))
	for i, a := range args {
		vals[i] = rt.vm.ToValue(a)
	}
	res, err := fn(rt.reg, vals...)
	if err != nil {
		return "", toGoError(err)
	}
	if res == nil || goja.IsUndefined(res) || goja.IsNull(res) {
		return "", nil
	}
	return res.String(), nil
}

// Meta returns the registry snapshot JSON.
func (rt *Runtime) Meta() (json.RawMessage, error) {
	s, err := rt.callReg("meta")
	return json.RawMessage(s), err
}

func (rt *Runtime) HasHook(doctype, event string) bool {
	fn, _ := goja.AssertFunction(rt.reg.Get("hasHook"))
	v, err := fn(rt.reg, rt.vm.ToValue(doctype), rt.vm.ToValue(event))
	return err == nil && v.ToBoolean()
}

// RunHook runs a lifecycle event and returns the mutated doc JSON.
func (rt *Runtime) RunHook(doctype, event string, doc, before json.RawMessage) (json.RawMessage, error) {
	s, err := rt.callReg("runHook", doctype, event, string(doc), string(before))
	return json.RawMessage(s), err
}

type MethodResult struct {
	Doc    json.RawMessage `json:"doc"`
	Result json.RawMessage `json:"result"`
}

func (rt *Runtime) RunMethod(doctype, method string, doc, args json.RawMessage) (*MethodResult, error) {
	s, err := rt.callReg("runMethod", doctype, method, string(doc), string(args))
	if err != nil {
		return nil, err
	}
	var r MethodResult
	return &r, json.Unmarshal([]byte(s), &r)
}

// HasPermission asks the controller hook; "" means no opinion.
func (rt *Runtime) HasPermission(doctype string, doc json.RawMessage, ptype, user string) (string, error) {
	return rt.callReg("hasPermission", doctype, string(doc), ptype, user)
}

func (rt *Runtime) PermissionQuery(doctype, user string) (json.RawMessage, error) {
	s, err := rt.callReg("permissionQuery", doctype, user)
	return json.RawMessage(s), err
}

func (rt *Runtime) CallWhitelisted(path string, args json.RawMessage) (json.RawMessage, error) {
	s, err := rt.callReg("callWhitelisted", path, string(args))
	return json.RawMessage(s), err
}

func (rt *Runtime) CallFunction(path string, args json.RawMessage) (json.RawMessage, error) {
	s, err := rt.callReg("callFunction", path, string(args))
	return json.RawMessage(s), err
}

func (rt *Runtime) RunReport(name string, filters json.RawMessage) (json.RawMessage, error) {
	s, err := rt.callReg("runReport", name, string(filters))
	return json.RawMessage(s), err
}

func (rt *Runtime) NumberCard(ws, name string) (json.RawMessage, error) {
	s, err := rt.callReg("numberCard", ws, name)
	return json.RawMessage(s), err
}

func (rt *Runtime) Chart(ws, name string) (json.RawMessage, error) {
	s, err := rt.callReg("chart", ws, name)
	return json.RawMessage(s), err
}

func (rt *Runtime) AppHook(app, hook string) error {
	_, err := rt.callReg("appHook", app, hook)
	return err
}

func (rt *Runtime) RunPatch(path string) error {
	_, err := rt.callReg("runPatch", path)
	return err
}

func (rt *Runtime) Eval(code string) (json.RawMessage, error) {
	src, err := TransformTS(code)
	if err != nil {
		return nil, &cerr.Error{Type: "ValidationError", Status: 417, Message: err.Error()}
	}
	s, err := rt.callReg("eval", src)
	return json.RawMessage(s), err
}

type TestResult struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	App   string `json:"app"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Stack string `json:"stack,omitempty"`
	Ms    int    `json:"ms"`
}

func (rt *Runtime) RunTests(filter, app string) ([]TestResult, error) {
	s, err := rt.callReg("runTests", filter, app)
	if err != nil {
		return nil, err
	}
	var out []TestResult
	return out, json.Unmarshal([]byte(s), &out)
}

// Pool hands out runtimes to goroutines. The semaphore caps how many VMs can
// be checked out at once, so a burst of requests não cria VMs sem limite —
// `free` sozinho só limitava quantas ficavam guardadas (B08).
type Pool struct {
	mu      sync.Mutex
	free    []*Runtime
	host    Host
	bundles []*Bundle
	size    int
	test    bool
	closed  bool
	sem     chan struct{}
}

func NewPool(host Host, bundles []*Bundle, size int, test bool) (*Pool, error) {
	if size < 1 {
		size = 1
	}
	p := &Pool{host: host, bundles: bundles, size: size, test: test, sem: make(chan struct{}, size)}
	// Build one eagerly so load errors surface immediately.
	rt, err := newRuntime(host, bundles, test)
	if err != nil {
		return nil, err
	}
	rt.pool = p
	p.free = append(p.free, rt)
	return p, nil
}

func (p *Pool) Acquire() (*Runtime, error) {
	// Um pool fechado ainda entrega VMs: um ctx que capturou a meta antiga
	// precisa do bundle antigo. Fechado só quer dizer que nada volta para cá.
	p.sem <- struct{}{}
	p.mu.Lock()
	if n := len(p.free); n > 0 {
		rt := p.free[n-1]
		p.free[n-1] = nil
		p.free = p.free[:n-1]
		p.mu.Unlock()
		return rt, nil
	}
	p.mu.Unlock()
	rt, err := newRuntime(p.host, p.bundles, p.test)
	if err != nil {
		<-p.sem
		return nil, err
	}
	rt.pool = p
	return rt, nil
}

// Release returns a runtime to the pool that created it.
func (p *Pool) Release(rt *Runtime) {
	if rt == nil {
		return
	}
	if rt.pool != nil && rt.pool != p {
		rt.pool.Release(rt)
		return
	}
	rt.Ctx = nil
	p.mu.Lock()
	// depois de Close a VM carrega o bundle antigo: descarta em vez de guardar
	if !p.closed && len(p.free) < p.size {
		p.free = append(p.free, rt)
	}
	p.mu.Unlock()
	<-p.sem
}

// Release devolve o runtime ao pool de origem.
// SetLang tells the VM which language this unit of work is in. The prelude
// mirrors that language's catalogue once and interpolates in JS from then on.
func (rt *Runtime) SetLang(lang string) { rt.vm.Set("__ddcoreLang", lang) }

func (rt *Runtime) Release() {
	if rt.pool != nil {
		rt.pool.Release(rt)
	}
}

// Close marks the pool as retired: VMs devolvidas depois disso são
// descartadas e novos Acquire falham.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.free = nil
	p.mu.Unlock()
}

// WithContext runs fn with the VM interruptible by ctx: um script que não
// retorna é abortado quando o contexto expira (B15).
func (rt *Runtime) WithContext(ctx context.Context, fn func() error) error {
	if ctx == nil || ctx.Done() == nil {
		return fn()
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			rt.vm.Interrupt(ctx.Err())
		case <-stop:
		}
	}()
	err := fn()
	close(stop)
	<-stopped
	rt.vm.ClearInterrupt()
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ie
	}
	return err
}

// Bundles returns the loaded bundles (for hot reload comparison).
func (p *Pool) Bundles() []*Bundle { return p.bundles }

// EvalExpr evaluates a dependsOn expression against a doc.
func (rt *Runtime) EvalExpr(expr string, doc json.RawMessage) (bool, error) {
	s, err := rt.callReg("evalExpr", expr, string(doc))
	return s == "true", err
}
