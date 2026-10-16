package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// provider is a fake Responses API: each request is recorded and answered by
// the next handler, the last one repeating.
type provider struct {
	srv      *httptest.Server
	calls    atomic.Int32
	bodies   [][]byte
	headers  []http.Header
	handlers []http.HandlerFunc
}

func newProvider(t *testing.T, handlers ...http.HandlerFunc) *provider {
	t.Helper()
	p := &provider{handlers: handlers}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(p.calls.Add(1)) - 1
		b, _ := io.ReadAll(r.Body)
		p.bodies = append(p.bodies, b)
		p.headers = append(p.headers, r.Header.Clone())
		p.handlers[min(n, len(p.handlers)-1)](w, r)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func answer(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

const okText = `{"id":"resp_1","model":"m-2025","status":"completed","output":[
 {"type":"reasoning","id":"rs_1","encrypted_content":"opaque"},
 {"type":"message","content":[{"type":"output_text","text":"Hello"}]},
 {"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":1}"}
],"usage":{"input_tokens":12,"output_tokens":7}}`

func client(p *provider, log *bytes.Buffer) *Client {
	var l *slog.Logger
	if log != nil {
		l = slog.New(slog.NewTextHandler(log, nil))
	}
	c := New(Config{BaseURL: p.srv.URL + "/", APIKey: "sk-secret", Model: "m-default", TranscriptionModel: "t-default", ReasoningEffort: "low"}, l)
	c.sleep = func(time.Duration) {}
	return c
}

func TestRespondBodyAndResult(t *testing.T) {
	p := newProvider(t, answer(200, okText))
	c := client(p, nil)
	input := `[{"role":"user","content":"hi"},{"type":"reasoning","id":"rs_0","encrypted_content":"keep-me"}]`
	r, err := c.Respond(context.Background(), Request{
		Instructions: "be brief", Input: json.RawMessage(input),
		Tools: json.RawMessage(`[{"type":"function","name":"lookup","parameters":{}}]`),
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if r.Text != "Hello" || len(r.Calls) != 1 || r.Calls[0].CallID != "call_1" || r.Usage.InputTokens != 12 || r.Model != "m-2025" {
		t.Errorf("result: %+v", r)
	}
	if len(r.Output) != 3 {
		t.Errorf("output items are kept for replay, got %d", len(r.Output))
	}
	if got := p.headers[0].Get("Authorization"); got != "Bearer sk-secret" {
		t.Errorf("auth header: %q", got)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(p.bodies[0], &body); err != nil {
		t.Fatal(err)
	}
	if string(body["model"]) != `"m-default"` || string(body["store"]) != "false" ||
		string(body["include"]) != `["reasoning.encrypted_content"]` || string(body["reasoning"]) != `{"effort":"low"}` {
		t.Errorf("body: %s", p.bodies[0])
	}
	if !strings.Contains(string(body["input"]), `"encrypted_content":"keep-me"`) {
		t.Errorf("input is passed through, got %s", body["input"])
	}
	if _, ok := body["tools"]; !ok {
		t.Error("tools are sent")
	}
	if _, ok := body["text"]; ok {
		t.Error("no schema, no text.format")
	}
}

func TestRespondSchema(t *testing.T) {
	p := newProvider(t, answer(200, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"title\":\"x\"}"}]}]}`))
	c := client(p, nil)
	f := false
	r, err := c.Respond(context.Background(), Request{
		Model: "m-other", Input: json.RawMessage(`"hi"`), ReasoningEffort: "high", IncludeReasoning: &f,
		Schema: &Schema{Name: "fb", Schema: json.RawMessage(`{"type":"object"}`)}, MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if m, _ := r.JSON.(map[string]any); m["title"] != "x" {
		t.Errorf("JSON parsed, got %#v", r.JSON)
	}
	var body map[string]any
	json.Unmarshal(p.bodies[0], &body)
	format := body["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "fb" || format["strict"] != true {
		t.Errorf("text.format: %#v", format)
	}
	if body["model"] != "m-other" || body["max_output_tokens"] != float64(100) || body["include"] != nil {
		t.Errorf("body: %s", p.bodies[0])
	}
	if body["reasoning"].(map[string]any)["effort"] != "high" {
		t.Errorf("the call's effort wins, got %v", body["reasoning"])
	}
}

func TestRespondRetries(t *testing.T) {
	p := newProvider(t, answer(503, `{}`), answer(429, `{}`), answer(200, okText))
	if _, err := client(p, nil).Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)}); err != nil {
		t.Fatalf("a 503 then a 429 then a 200 succeeds: %v", err)
	}
	if p.calls.Load() != 3 {
		t.Errorf("3 attempts, got %d", p.calls.Load())
	}

	p = newProvider(t, answer(400, `{"error":{"message":"bad input"}}`))
	_, err := client(p, nil).Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400: bad input") || p.calls.Load() != 1 {
		t.Errorf("a 400 is not retried: %v (%d calls)", err, p.calls.Load())
	}
	if e := cerr.From(err); e.Type != "ValidationError" {
		t.Errorf("a 400 is the caller's error, got %s", e.Type)
	}

	p = newProvider(t, answer(500, `<html>secret stack trace</html>`))
	_, err = client(p, nil).Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)})
	if err == nil || p.calls.Load() != 3 || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("a 500 is retried, then worded without its body: %v (%d calls)", err, p.calls.Load())
	}
}

func TestRespondTransportErrorIsRetried(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		answer(200, okText)(w, r)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, nil)
	c.sleep = func(time.Duration) {}
	if _, err := c.Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)}); err != nil {
		t.Fatalf("a dropped connection is retried: %v", err)
	}
}

func TestRespondBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, nil)
	start := time.Now()
	_, err := c.Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`), Timeout: 0.3})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("a slow provider times out: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("the budget covers the retries, took %v", d)
	}
}

func TestParseFailures(t *testing.T) {
	cases := map[string]struct {
		body     string
		wantJSON bool
		want     string
	}{
		"failed":     {`{"status":"failed","error":{"message":"boom"},"output":[]}`, false, "boom"},
		"no output":  {`{"status":"completed"}`, false, "no output"},
		"incomplete": {`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`, true, "cut short: max_output_tokens"},
		"not json":   {`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"nope"}]}]}`, true, "invalid JSON"},
	}
	for name, tc := range cases {
		if _, err := Parse(200, []byte(tc.body), tc.wantJSON); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if r, err := Parse(200, []byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`), false); err != nil || r.IncompleteReason != "max_output_tokens" {
		t.Errorf("incomplete text is an answer when no JSON was asked: %v %+v", err, r)
	}
	long := strings.Repeat("x", 400)
	_, err := Parse(400, []byte(`{"error":{"message":"`+long+`"}}`), false)
	if err == nil || strings.Contains(err.Error(), long) || !strings.Contains(err.Error(), "…") {
		t.Errorf("a long provider message is cut: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	c := New(Config{Model: "m"}, nil)
	if _, err := c.Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)}); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("no key: %v", err)
	}
	c = New(Config{APIKey: "k"}, nil)
	if c.Enabled() {
		t.Error("no model: not enabled")
	}
	if _, err := c.Respond(context.Background(), Request{Input: json.RawMessage(`"hi"`)}); err == nil || !strings.Contains(err.Error(), "No AI model") {
		t.Errorf("no model: %v", err)
	}
	if _, err := c.Transcribe(context.Background(), TranscribeRequest{Audio: []byte("a"), Filename: "a.ogg"}); err == nil || !strings.Contains(err.Error(), "transcription model") {
		t.Errorf("no transcription model: %v", err)
	}
}

func TestTranscribe(t *testing.T) {
	var fields map[string]string
	var file []byte
	var fileName, fileType string
	p := newProvider(t, answer(502, `{}`), func(w http.ResponseWriter, r *http.Request) {
		answer(200, `{"text":" olá mundo "}`)(w, r)
	})
	c := client(p, nil)
	audio := []byte("OggS-fake-audio")
	got, err := c.Transcribe(context.Background(), TranscribeRequest{
		Base64: base64.StdEncoding.EncodeToString(audio), Filename: "voice-note-1.webm",
		ContentType: "audio/webm", Language: "pt",
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got.Text != "olá mundo" || got.Model != "t-default" {
		t.Errorf("result: %+v", got)
	}
	// read the second (successful) upload back
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(p.bodies[1]))
	req.Header.Set("Content-Type", p.headers[1].Get("Content-Type"))
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	fields = map[string]string{"model": req.FormValue("model"), "language": req.FormValue("language")}
	f, h, err := req.FormFile("file")
	if err != nil {
		t.Fatal(err)
	}
	file, _ = io.ReadAll(f)
	fileName, fileType = h.Filename, h.Header.Get("Content-Type")
	if fields["model"] != "t-default" || fields["language"] != "pt" || string(file) != string(audio) || fileName != "voice-note-1.webm" || fileType != "audio/webm" {
		t.Errorf("multipart: %v %q %q %q", fields, file, fileName, fileType)
	}

	big := TranscribeRequest{Audio: make([]byte, MaxAudioBytes+1), Filename: "a.ogg"}
	if _, err := c.Transcribe(context.Background(), big); err == nil || !strings.Contains(err.Error(), "larger than 25 MB") {
		t.Errorf("an audio over the limit is refused: %v", err)
	}
	if _, err := c.Transcribe(context.Background(), TranscribeRequest{Audio: audio}); err == nil || !strings.Contains(err.Error(), "filename") {
		t.Errorf("a filename is required: %v", err)
	}
}

func TestLogsNeitherPromptNorKey(t *testing.T) {
	p := newProvider(t, answer(200, okText))
	var log bytes.Buffer
	if _, err := client(p, &log).Respond(context.Background(), Request{Instructions: "SECRET-INSTRUCTIONS", Input: json.RawMessage(`"SECRET-PROMPT"`)}); err != nil {
		t.Fatal(err)
	}
	out := log.String()
	if !strings.Contains(out, "ai call") {
		t.Errorf("one line per attempt is logged, got %q", out)
	}
	for _, s := range []string{"SECRET-INSTRUCTIONS", "SECRET-PROMPT", "sk-secret", "Hello"} {
		if strings.Contains(out, s) {
			t.Errorf("the log carries %q: %s", s, out)
		}
	}
}
