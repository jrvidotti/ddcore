package api

import (
	"net/http"
	"strings"
	"testing"
)

// Rich text is cleaned where every write passes, so it does not matter which
// door the markup came through: what the API returns is what the database
// holds, and neither carries a script.
func TestRichTextIsCleanedOnTheWayIn(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{
		"nome": "Ana", "bio": `<p>hello<img src=x onerror=alert(1)></p><script>alert(1)</script>`,
	}, sid)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	bio, _ := data["bio"].(string)
	if strings.Contains(bio, "onerror") || strings.Contains(bio, "<script") || strings.Contains(bio, "<img") {
		t.Fatalf("the response carries the attack: %s", bio)
	}
	if !strings.Contains(bio, "hello") {
		t.Fatalf("the content was lost with the attack: %s", bio)
	}
	// and what was stored is what was returned
	id, _ := data["id"].(string)
	got, _ := x.call("GET", "/api/resource/Pessoa/"+id, nil, sid).Body["data"].(map[string]any)
	if got["bio"] != bio {
		t.Fatalf("stored %v, returned %q", got["bio"], bio)
	}
}

// A value written as plain text before rich text existed is read as text, not
// as broken markup: the `<` in "a < b" is part of the sentence.
func TestPlainTextThroughTheAPIKeepsItsAngleBracket(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Bia", "bio": "a < b and b > c"}, sid)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	if got := data["bio"]; got != "<p>a &lt; b and b &gt; c</p>" {
		t.Fatalf("plain text came back as %v", got)
	}
}

func TestMarkdownPreviewNeedsALoginAndHasALimit(t *testing.T) {
	x := setup(t)
	x.expect(x.call("POST", "/api/richtext/markdown", map[string]any{"text": "# hi"}, ""), http.StatusUnauthorized, "")

	sid := "sid:" + x.sid("ze@x.com") // any signed-in user: it reads no document
	r := x.call("POST", "/api/richtext/markdown", map[string]any{"text": "# hi\n\n<script>alert(1)</script>"}, sid)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	html, _ := data["html"].(string)
	if !strings.Contains(html, "<h1>hi</h1>") || strings.Contains(html, "<script") {
		t.Fatalf("preview rendered %q", html)
	}
	long := x.call("POST", "/api/richtext/markdown", map[string]any{"text": strings.Repeat("x", 300<<10)}, sid)
	x.expect(long, http.StatusExpectationFailed, "ValidationError")
}

// An Attach Image field refuses a file it would refuse as a value, before the
// bytes are stored rather than after.
func TestUploadIntoAnImageFieldRefusesOtherFiles(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")
	x.expect(x.uploadTo(sid, "report.pdf", "%PDF-1.4", "Pessoa", "foto"), http.StatusExpectationFailed, "ValidationError")
	x.expect(x.uploadTo(sid, "logo.svg", "<svg onload=alert(1)></svg>", "Pessoa", "foto"), http.StatusExpectationFailed, "ValidationError")
	x.expect(x.uploadTo(sid, "photo.png", "\x89PNG\r\n", "Pessoa", "foto"), 200, "")
	// a plain Attach still takes anything
	x.expect(x.uploadTo(sid, "report.pdf", "%PDF-1.4", "Pessoa", "contatos"), 200, "")
}
