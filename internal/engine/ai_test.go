package engine

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/js"
)

// aiRuntime is a runtime with no database on an engine whose AI is cfg; test
// turns on the ddcore test mode (ddcore.test, the fake).
func aiRuntime(t *testing.T, cfg config.AI, test bool) *js.Runtime {
	t.Helper()
	pool, err := js.NewPool(&Engine{Cfg: Config{AI: cfg, Test: test}}, nil, 1, test)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Release)
	rt.Ctx = &Ctx{}
	return rt
}

func TestAIRespondFromJS(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		io.WriteString(w, `{"id":"r1","model":"m1","status":"completed","output":[
		  {"type":"message","content":[{"type":"output_text","text":"oi"}]},
		  {"type":"function_call","call_id":"c1","name":"f","arguments":"{}"}],
		  "usage":{"input_tokens":3,"output_tokens":2}}`)
	}))
	defer srv.Close()
	rt := aiRuntime(t, config.AI{APIKey: "k", Model: "m1", BaseURL: srv.URL + "/v1"}, false)
	v, err := rt.Eval(`(() => {
		const r = ddcore.ai.respond({ instructions: "x", input: [{ role: "user", content: [
			{ type: "input_text", text: "look" }, ddcore.ai.image("AAAA", "image/png") ] }] });
		return { enabled: ddcore.ai.enabled(), text: r.text, callId: r.calls[0].callId, out: r.output.length, tokens: r.usage.inputTokens };
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != `{"enabled":true,"text":"oi","callId":"c1","out":2,"tokens":3}` {
		t.Errorf("result: %s", v)
	}
	input := body["input"].([]any)[0].(map[string]any)["content"].([]any)
	if img := input[1].(map[string]any); img["image_url"] != "data:image/png;base64,AAAA" {
		t.Errorf("image part: %v", img)
	}
}

func TestAITranscribeFromJS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.FormValue("model") != "tx" || r.FormValue("language") != "pt" {
			t.Errorf("fields %v", r.MultipartForm.Value)
		}
		io.WriteString(w, `{"text":"bom dia"}`)
	}))
	defer srv.Close()
	rt := aiRuntime(t, config.AI{APIKey: "k", Model: "m", TranscriptionModel: "tx", BaseURL: srv.URL}, false)
	v, err := rt.Eval(`ddcore.ai.transcribe("T2dnUw==", { filename: "a.ogg", contentType: "audio/ogg", language: "pt" }).text`)
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != `"bom dia"` {
		t.Errorf("text: %s", v)
	}
}

func TestAIOffFromJS(t *testing.T) {
	rt := aiRuntime(t, config.AI{Model: "m"}, false)
	v, err := rt.Eval(`ddcore.ai.enabled()`)
	if err != nil || string(v) != "false" {
		t.Errorf("no key: enabled() = %s, %v", v, err)
	}
	if _, err := rt.Eval(`ddcore.ai.respond({ input: "hi" })`); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("no key: %v", err)
	}
	if _, err := rt.Eval(`__ddcore_unused = 1; (function(){ return globalThis.__host("ai.prepare", JSON.stringify({ req: { input: "x" } })) })()`); err == nil {
		t.Error("the fake's ops are refused outside ddcore test")
	}
}

func TestAIFakeInTests(t *testing.T) {
	rt := aiRuntime(t, config.AI{Model: "site-model", TranscriptionModel: "site-tx", APIKey: "real-key-must-not-be-used"}, true)
	if _, err := rt.Eval(`ddcore.ai.respond({ input: "hi" })`); err == nil || !strings.Contains(err.Error(), "fakeAi") {
		t.Errorf("a test without a fake must not reach the provider: %v", err)
	}
	v, err := rt.Eval(`(() => {
		const f = ddcore.test.fakeAi({
			respond: (c) => ({ status: "completed", model: c.body.model, output: [
				{ type: "message", content: [{ type: "output_text", text: JSON.stringify({ ok: c.body.store === false }) }] }] }),
			transcribe: (c) => "heard " + c.filename + " with " + c.model,
		});
		const r = ddcore.ai.respond({ input: "hi", schema: { name: "s", schema: { type: "object" } }, timeout: 10 });
		const t = ddcore.ai.transcribe("AAAA", { filename: "v.webm" });
		return { json: r.json, model: r.model, timeout: f.respondCalls[0].timeout, text: t.text,
			calls: f.transcribeCalls.length, enabled: ddcore.ai.enabled() };
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != `{"json":{"ok":true},"model":"site-model","timeout":10,"text":"heard v.webm with site-tx","calls":1,"enabled":true}` {
		t.Errorf("fake: %s", v)
	}
	// a raw HTTP answer is read like the provider's, error included
	_, err = rt.Eval(`(() => {
		const f = ddcore.test.fakeAi({ respond: () => ({ status: 400, body: JSON.stringify({ error: { message: "Invalid file format." } }) }) });
		try { ddcore.ai.respond({ input: "hi" }); } finally { f.restore(); }
	})()`)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400: Invalid file format.") {
		t.Errorf("raw answer: %v", err)
	}
	// restore put the earlier handlers back
	if v, err := rt.Eval(`ddcore.ai.respond({ input: "hi", schema: { schema: {} } }).json.ok`); err != nil || string(v) != "true" {
		t.Errorf("restore: %s %v", v, err)
	}
}
