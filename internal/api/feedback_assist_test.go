package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
)

type assistFile struct{ name, contentType, content string }

// sendAssist posts the dialog's "Fill in with AI".
func (x *env) sendAssist(auth string, data map[string]any, files ...assistFile) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	b, _ := json.Marshal(data)
	mw.WriteField("data", string(b))
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="files"; filename="`+f.name+`"`)
		h.Set("Content-Type", f.contentType)
		fw, _ := mw.CreatePart(h)
		fw.Write([]byte(f.content))
	}
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/feedback/assist", &buf)
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

// fakeAIProvider answers transcriptions with "transcript of <file>" and
// responses with answer, recording what it was sent.
type fakeAIProvider struct {
	mu          sync.Mutex
	transcribed []string
	prompts     []map[string]any
	answer      map[string]any
}

func (p *fakeAIProvider) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		switch r.URL.Path {
		case "/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			_, h, _ := r.FormFile("file")
			p.transcribed = append(p.transcribed, h.Filename)
			json.NewEncoder(w).Encode(map[string]any{"text": "transcript of " + h.Filename})
		case "/responses":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			p.prompts = append(p.prompts, body)
			text, _ := json.Marshal(p.answer)
			json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{
				map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(text)}}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fullAnswer(kind string) map[string]any {
	return map[string]any{"feedback_type": kind, "title": "  Saving an invoice fails  ", "description": "The save button shows an error.",
		"severity": "Urgent", "steps_to_reproduce": "1. Open an invoice\n2. Click Save", "expected_result": "It saves",
		"actual_result": "Error 500", "current_behavior": "ignored", "suggested_improvement": nil, "problem": nil, "expected_benefit": nil}
}

func TestFeedbackAssist_FillsFields(t *testing.T) {
	x := setup(t)
	p := &fakeAIProvider{answer: fullAnswer("Bug")}
	srv := p.serve(t)
	x.e.Cfg.AI = config.AI{APIKey: "k", BaseURL: srv.URL, Model: "m", TranscriptionModel: "tx"}
	ana := "sid:" + x.sid("ana@x.com")

	boot := x.call("GET", "/api/boot", nil, ana)
	if boot.Body["data"].(map[string]any)["site"].(map[string]any)["feedbackAssist"] != true {
		t.Fatalf("the boot offers the assistant: %s", boot.Raw)
	}
	r := x.sendAssist(ana, map[string]any{
		"feedback_type": "Improvement", "title": "invoice", "page_url": "http://localhost/app/x",
		"context":     map[string]any{"user": "ana@x.com", "lang": "pt-BR"},
		"transcripts": map[string]any{"voice-note-1.webm": "already known"},
	},
		assistFile{"voice-note-1.webm", "audio/webm", "opus 1"},
		assistFile{"voice-note-2.webm", "audio/webm", "opus 2"},
		assistFile{"shot.png", "image/png", "\x89PNG"},
		assistFile{"log.txt", "text/plain", "trace"},
	)
	x.expect(r, 200, "")
	data := r.Body["data"].(map[string]any)
	if data["feedback_type"] != "Bug" {
		t.Fatalf("the model may move the report to another type: %s", r.Raw)
	}
	fields := data["fields"].(map[string]any)
	if fields["title"] != "Saving an invoice fails" || fields["actual_result"] != "Error 500" {
		t.Fatalf("fields: %v", fields)
	}
	if _, ok := fields["severity"]; ok {
		t.Fatalf("a severity the Select does not offer is dropped: %v", fields)
	}
	if _, ok := fields["current_behavior"]; ok {
		t.Fatalf("a field of another type is dropped: %v", fields)
	}
	if len(p.transcribed) != 1 || p.transcribed[0] != "voice-note-2.webm" {
		t.Fatalf("only the audio without a transcript is uploaded: %v", p.transcribed)
	}
	tr := data["transcripts"].([]any)
	if len(tr) != 2 || tr[0].(map[string]any)["text"] != "already known" || tr[1].(map[string]any)["text"] != "transcript of voice-note-2.webm" {
		t.Fatalf("transcripts: %v", tr)
	}
	prompt, _ := json.Marshal(p.prompts[0])
	for _, want := range []string{"already known", "transcript of voice-note-2.webm", "data:image/png;base64,", "log.txt", "http://localhost/app/x",
		`\"user\":\"ana@x.com\"`, `"json_schema"`, "Improvement"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt carries %s: %s", want, prompt)
		}
	}
}

func TestFeedbackAssist_Off(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	if r := x.sendAssist(ana, map[string]any{"feedback_type": "Bug"}); r.Status != 404 {
		t.Fatalf("without a key the endpoint is not there: %d %s", r.Status, r.Raw)
	}
	boot := x.call("GET", "/api/boot", nil, ana)
	if boot.Body["data"].(map[string]any)["site"].(map[string]any)["feedbackAssist"] != false {
		t.Fatalf("no assistant without the AI: %s", boot.Raw)
	}
	x.e.Cfg.AI = config.AI{APIKey: "k", Model: "m"}
	off := false
	x.e.Cfg.Feedback.Assist = &off
	if r := x.sendAssist(ana, map[string]any{"feedback_type": "Bug"}); r.Status != 404 {
		t.Fatalf("feedback.assist false turns it off: %d", r.Status)
	}
}

func TestFeedbackAssist_ValidationAndGuest(t *testing.T) {
	x := setup(t)
	p := &fakeAIProvider{answer: fullAnswer("Bug")}
	x.e.Cfg.AI = config.AI{APIKey: "k", BaseURL: p.serve(t).URL, Model: "m"}
	ana := "sid:" + x.sid("ana@x.com")
	x.expect(x.sendAssist(ana, map[string]any{"feedback_type": "Rant"}), 417, "ValidationError")
	x.expect(x.sendAssist(ana, map[string]any{"feedback_type": "Bug", "owner": "x"}), 417, "ValidationError")
	x.expect(x.sendAssist("sid:nope", map[string]any{"feedback_type": "Bug"}), 401, "")
	if len(p.prompts) != 0 {
		t.Fatalf("nothing reached the provider: %d", len(p.prompts))
	}
}

func TestFeedbackAssist_RateLimit(t *testing.T) {
	x := setup(t)
	p := &fakeAIProvider{answer: fullAnswer("Bug")}
	x.e.Cfg.AI = config.AI{APIKey: "k", BaseURL: p.serve(t).URL, Model: "m"}
	ana := "sid:" + x.sid("ana@x.com")
	for i := 0; i < 10; i++ {
		x.expect(x.sendAssist(ana, map[string]any{"feedback_type": "Bug", "title": "x"}), 200, "")
	}
	x.expect(x.sendAssist(ana, map[string]any{"feedback_type": "Bug", "title": "x"}), 429, "")
}

func TestFeedback_SubmitKeepsTranscript(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	data := bug("Audio")
	data["transcript"] = "voice-note-1.webm:\nit breaks when I save"
	r := x.sendFeedback(ana, data)
	x.expect(r, 200, "")
	rows := x.feedbackRows("id = $1", r.Body["data"].(map[string]any)["id"])
	if db.Str(rows[0]["transcript"]) != "voice-note-1.webm:\nit breaks when I save" {
		t.Fatalf("transcript: %v", rows[0]["transcript"])
	}
}
