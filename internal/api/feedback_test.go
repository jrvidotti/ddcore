package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

type feedbackFile struct{ name, content string }

// sendFeedback posts the desk's Feedback dialog: the fields as JSON in `data`,
// the attachments as `files`.
func (x *env) sendFeedback(auth string, data map[string]any, files ...feedbackFile) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	b, _ := json.Marshal(data)
	mw.WriteField("data", string(b))
	for _, f := range files {
		fw, _ := mw.CreateFormFile("files", f.name)
		fw.Write([]byte(f.content))
	}
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/feedback", &buf)
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

func bug(title string) map[string]any {
	return map[string]any{"feedback_type": "Bug", "title": title, "description": "It breaks",
		"severity": "High", "steps_to_reproduce": "1. open\n2. click", "page_url": "http://localhost/app/x"}
}

// feedbackRows reads Feedback in the platform space, as its System Managers do.
func (x *env) feedbackRows(where string, args ...any) []map[string]any {
	x.t.Helper()
	var rows []map[string]any
	x.asAdmin(func(c *engine.Ctx) error {
		var err error
		rows, err = db.Select(c.Ctx, c.Q(), `SELECT * FROM tab_feedback WHERE `+where, args...)
		return err
	})
	return rows
}

func TestFeedback_SubmitWithFilesAndAudio(t *testing.T) {
	x := setup(t)
	x.e.Cfg.Feedback.To = []string{"dev@example.com"}
	ana := "sid:" + x.sid("ana@x.com")
	r := x.sendFeedback(ana, bug("Save fails"), feedbackFile{"shot.png", "\x89PNG fake"}, feedbackFile{"log.txt", "trace"},
		feedbackFile{"voice-note.webm", "opus bytes"})
	x.expect(r, 200, "")
	id := r.Body["data"].(map[string]any)["id"].(string)

	rows := x.feedbackRows("id = $1", id)
	if len(rows) != 1 {
		t.Fatalf("feedback not written: %v", rows)
	}
	f := rows[0]
	for k, want := range map[string]string{"feedback_type": "Bug", "title": "Save fails", "status": "New", "severity": "High",
		"reported_by": "ana@x.com", "reporter_email": "ana@x.com", "page_url": "http://localhost/app/x"} {
		if db.Str(f[k]) != want {
			t.Fatalf("%s = %q, want %q", k, db.Str(f[k]), want)
		}
	}
	if f["context"] != nil {
		t.Fatalf("context was not sent, so none is stored: %v", f["context"])
	}
	var files, deliveries, inbox []map[string]any
	x.asAdmin(func(c *engine.Ctx) error {
		var err error
		if files, err = db.Select(c.Ctx, c.Q(), `SELECT file_name, is_private FROM tab_file WHERE attached_to_doctype='Feedback' AND attached_to_id=$1`, id); err != nil {
			return err
		}
		if deliveries, err = db.Select(c.Ctx, c.Q(), `SELECT "to", template, attachments FROM tab_email_delivery WHERE template='core.feedback'`); err != nil {
			return err
		}
		inbox, err = db.Select(c.Ctx, c.Q(), `SELECT recipient FROM ddcore_notification WHERE reference_doctype='Feedback' AND reference_id=$1 ORDER BY recipient`, id)
		return err
	})
	if len(files) != 3 {
		t.Fatalf("the 3 files are attached to the feedback: %v", files)
	}
	for _, file := range files {
		if file["is_private"] != true {
			t.Fatalf("feedback files are private: %v", file)
		}
	}
	if len(deliveries) != 1 || db.Str(deliveries[0]["to"]) != "dev@example.com" {
		t.Fatalf("one mail to the feedback address: %v", deliveries)
	}
	got := []string{}
	for _, n := range inbox {
		got = append(got, db.Str(n["recipient"]))
	}
	// the System Managers, Admin among them; ana is not one
	if strings.Join(got, ",") != "Admin,root@x.com" {
		t.Fatalf("inbox recipients: %v", got)
	}

	// the author reads their own, never the DocType
	mine := x.call("GET", "/api/feedback/mine", nil, ana)
	x.expect(mine, 200, "")
	list := mine.Body["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["status"] != "New" || list[0].(map[string]any)["title"] != "Save fails" {
		t.Fatalf("mine: %s", mine.Raw)
	}
	if created, err := time.Parse(time.RFC3339, fmt.Sprint(list[0].(map[string]any)["creation"])); err != nil || time.Since(created) > time.Hour {
		t.Fatalf("mine carries when it was sent: %s", mine.Raw)
	}
	x.expect(x.call("GET", "/api/resource/Feedback", nil, ana), 403, "")
	bia := "sid:" + x.sid("bia@x.com")
	if r := x.call("GET", "/api/feedback/mine", nil, bia); len(r.Body["data"].([]any)) != 0 {
		t.Fatalf("bia sees ana's feedback: %s", r.Raw)
	}
}

func TestFeedback_Validation(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	for name, data := range map[string]map[string]any{
		"unknown type":           {"feedback_type": "Rant", "title": "x", "description": "y"},
		"missing title":          {"feedback_type": "Bug", "description": "y"},
		"missing description":    {"feedback_type": "Improvement", "title": "x"},
		"field of another type":  {"feedback_type": "Improvement", "title": "x", "description": "y", "severity": "High"},
		"unknown field":          {"feedback_type": "Bug", "title": "x", "description": "y", "owner": "root@x.com"},
		"bad severity":           {"feedback_type": "Bug", "title": "x", "description": "y", "severity": "Meh"},
		"context is not object":  {"feedback_type": "Bug", "title": "x", "description": "y", "context": "secret"},
		"title is not text":      {"feedback_type": "Bug", "title": 3, "description": "y"},
		"title too long":         {"feedback_type": "Bug", "title": strings.Repeat("x", 201), "description": "y"},
		"status set by the user": {"feedback_type": "Bug", "title": "x", "description": "y", "status": "Done"},
	} {
		if r := x.sendFeedback(ana, data); r.Status != 417 && r.Status != 400 && r.Status != 422 {
			t.Fatalf("%s: expected a validation error, got %d %s", name, r.Status, r.Raw)
		}
	}
	files := make([]feedbackFile, engine.FeedbackMaxFiles+1)
	for i := range files {
		files[i] = feedbackFile{"f.txt", "x"}
	}
	if r := x.sendFeedback(ana, bug("too many"), files...); r.Status < 400 || r.Status >= 500 {
		t.Fatalf("more than %d files: %d %s", engine.FeedbackMaxFiles, r.Status, r.Raw)
	}
	if rows := x.feedbackRows("true"); len(rows) != 0 {
		t.Fatalf("a refused feedback writes nothing: %v", rows)
	}

	// a feature request keeps its own fields and context
	ok := x.sendFeedback(ana, map[string]any{"feedback_type": "Feature Request", "title": "Export", "description": "PDF",
		"problem": "No PDF", "expected_benefit": "Less typing", "context": map[string]any{"viewport": "390x700"}})
	x.expect(ok, 200, "")
	rows := x.feedbackRows("title = 'Export'")
	if len(rows) != 1 || db.Str(rows[0]["problem"]) != "No PDF" || !strings.Contains(db.Str(rows[0]["context"]), "390x700") {
		t.Fatalf("feature request: %v", rows)
	}
}

func TestFeedback_GuestAndDisabled(t *testing.T) {
	x := setup(t)
	if r := x.sendFeedback("", bug("anon")); r.Status != 401 && r.Status != 403 {
		t.Fatalf("a guest cannot send feedback: %d %s", r.Status, r.Raw)
	}
	ana := "sid:" + x.sid("ana@x.com")
	boot := x.call("GET", "/api/boot", nil, ana)
	if boot.Body["data"].(map[string]any)["site"].(map[string]any)["feedback"] != true {
		t.Fatalf("on by default: %s", boot.Raw)
	}
	off := false
	x.e.Cfg.Feedback.Enabled = &off
	x.expect(x.sendFeedback(ana, bug("off")), 404, "")
	x.expect(x.call("GET", "/api/feedback/mine", nil, ana), 404, "")
	boot = x.call("GET", "/api/boot", nil, ana)
	if boot.Body["data"].(map[string]any)["site"].(map[string]any)["feedback"] != false {
		t.Fatalf("the boot says it is off: %s", boot.Raw)
	}
}

func TestFeedback_TenantFeedbackLandsInThePlatform(t *testing.T) {
	x := setupTenants(t)
	a := "sid:" + x.sid(alfaUser)
	r := x.sendFeedback(a, bug("From alfa"), feedbackFile{"voice-note.webm", "opus"})
	x.expect(r, 200, "")
	id := r.Body["data"].(map[string]any)["id"].(string)

	rows := x.feedbackRows("id = $1", id)
	if len(rows) != 1 || db.Str(rows[0]["source_tenant"]) != "alfa" || db.Str(rows[0]["reported_by"]) != alfaUser {
		t.Fatalf("the platform has it, stamped with its tenant: %v", rows)
	}
	var files []map[string]any
	x.asAdmin(func(c *engine.Ctx) error {
		var err error
		files, err = db.Select(c.Ctx, c.Q(), `SELECT id FROM tab_file WHERE attached_to_doctype='Feedback' AND attached_to_id=$1`, id)
		return err
	})
	if len(files) != 1 {
		t.Fatalf("its file is in the platform too: %v", files)
	}
	// the tenant's own System Manager does not read it: it is not theirs
	chefe := "sid:" + x.sid(alfaAdmin)
	if r := x.call("GET", "/api/resource/Feedback", nil, chefe); r.Status == 200 && ids(r) != "" {
		t.Fatalf("the tenant reads the platform's feedback: %s", r.Raw)
	}
	mine := x.call("GET", "/api/feedback/mine", nil, a)
	x.expect(mine, 200, "")
	if list := mine.Body["data"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != id {
		t.Fatalf("the author follows it from the tenant: %s", mine.Raw)
	}
	if r := x.call("GET", "/api/feedback/mine", nil, "sid:"+x.sid(betaUser)); len(r.Body["data"].([]any)) != 0 {
		t.Fatalf("another tenant sees it: %s", r.Raw)
	}
}

func TestFeedback_RateLimit(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	for i := 0; i < 20; i++ {
		x.expect(x.sendFeedback(ana, bug("again")), 200, "")
	}
	if r := x.sendFeedback(ana, bug("one too many")); r.Status < 400 || r.Status >= 500 {
		t.Fatalf("the 21st in an hour is refused: %d %s", r.Status, r.Raw)
	}
	// the limit is the author's, not the site's
	x.expect(x.sendFeedback("sid:"+x.sid("bia@x.com"), bug("mine")), 200, "")
}

func TestFeedback_AudioIsServedInPlace(t *testing.T) {
	x := setup(t)
	root := "sid:" + x.sid("root@x.com")
	r := x.sendFeedback("sid:"+x.sid("ana@x.com"), bug("listen"), feedbackFile{"voice-note.webm", "opus"})
	x.expect(r, 200, "")
	var url string
	x.asAdmin(func(c *engine.Ctx) error {
		return c.Q().QueryRow(c.Ctx, `SELECT file_url FROM tab_file WHERE attached_to_id=$1`, r.Body["data"].(map[string]any)["id"]).Scan(&url)
	})
	req, _ := http.NewRequest("GET", x.ts.URL+url, nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(root, "sid:")})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "audio/webm" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") ||
		res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("a voice note plays in place: %d %v", res.StatusCode, res.Header)
	}
}
