package richtext

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// dangerous covers what an attacker sends and what a word processor pastes.
// Both end up in the same place: a value that must not survive as markup.
var dangerous = []string{
	`<script>alert(1)</script>`,
	`<img src=x onerror=alert(1)>`,
	`<a href="javascript:alert(1)">click</a>`,
	`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">click</a>`,
	`<iframe src="https://evil.example"></iframe>`,
	`<svg><animate onbegin=alert(1) attributeName=x dur=1s>`,
	`<p style="position:fixed;top:0;left:0;width:100vw;height:100vh">cover</p>`,
	`<form action="https://evil.example"><input name="pw"></form>`,
	`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
	`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
	`<img src="https://evil.example/pixel.gif">`,
	`<div onclick="alert(1)">text</div>`,
}

func TestSanitizeDropsDangerousMarkup(t *testing.T) {
	for _, in := range dangerous {
		got := strings.ToLower(Sanitize(in))
		for _, forbidden := range []string{"<script", "<iframe", "<svg", "<form", "<style", "onerror", "onclick", "onbegin", "javascript:", "data:text", "style=", "evil.example"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("Sanitize(%q) kept %q: %s", in, forbidden, got)
			}
		}
	}
}

// Sanitizing twice has to give the same bytes. Every save re-casts the value
// and compares it with itself (readOnly, allowOnSubmit, permlevel), so a
// sanitizer that adds one attribute per pass would make a submitted document
// unsaveable and fill the timeline with empty diffs.
func TestSanitizeIsIdempotent(t *testing.T) {
	cases := append([]string{
		`<p>hello <strong>world</strong></p>`,
		`<a href="https://example.com">out</a>`,
		`<a href="/app/task/T-1">in</a>`,
		`<a href="mailto:someone@example.com" title="write">mail</a>`,
		`<ol start="3"><li>three</li></ol>`,
		`<pre><code class="language-sql">select 1</code></pre>`,
		`<img src="/files/logo.png" alt="Logo" width="120">`,
		`<p>a &lt; b &amp; c</p>`,
		`<p>caf\u00e9 &#233;</p>`,
	}, dangerous...)
	for _, in := range cases {
		once := Sanitize(in)
		if twice := Sanitize(once); twice != once {
			t.Errorf("Sanitize is not idempotent for %q:\n once: %s\ntwice: %s", in, once, twice)
		}
		// Normalize runs on every write, so it has to settle too.
		n1 := Normalize(in)
		if n2 := Normalize(n1); n2 != n1 {
			t.Errorf("Normalize is not idempotent for %q:\n once: %s\ntwice: %s", in, n1, n2)
		}
	}
}

func TestSanitizeKeepsWhatTheEditorWrites(t *testing.T) {
	in := `<h2>Title</h2><p>Some <strong>bold</strong> and <em>italic</em> text.</p>` +
		`<ul><li>one</li><li>two</li></ul>` +
		`<blockquote><p>quoted</p></blockquote>` +
		`<pre><code class="language-ts">const a = 1;</code></pre>` +
		`<img src="/files/diagram.png" alt="Diagram">`
	got := Sanitize(in)
	for _, want := range []string{"<h2>Title</h2>", "<strong>bold</strong>", "<li>one</li>", "<blockquote>", `class="language-ts"`, `src="/files/diagram.png"`} {
		if !strings.Contains(got, want) {
			t.Errorf("Sanitize dropped %q:\n%s", want, got)
		}
	}
}

func TestExternalLinkGetsTargetAndRel(t *testing.T) {
	got := Sanitize(`<a href="https://example.com">out</a>`)
	for _, want := range []string{`target="_blank"`, "noopener", "noreferrer", "nofollow"} {
		if !strings.Contains(got, want) {
			t.Errorf("external link missing %q: %s", want, got)
		}
	}
	if in := Sanitize(`<a href="/app/task/T-1">in</a>`); strings.Contains(in, "target=") {
		t.Errorf("internal link should not open a new tab: %s", in)
	}
}

func TestImagesAreLocalFilesOnly(t *testing.T) {
	if got := Sanitize(`<img src="/private/files/scan.png">`); !strings.Contains(got, "/private/files/scan.png") {
		t.Errorf("local image dropped: %s", got)
	}
	for _, in := range []string{
		`<img src="https://evil.example/p.gif">`,
		`<img src="//evil.example/p.gif">`,
		`<img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">`,
		`<img src="/files/../../etc/passwd">`,
	} {
		if got := Sanitize(in); strings.Contains(got, "src=") {
			t.Errorf("Sanitize(%q) kept a src it should not: %s", in, got)
		}
	}
}

// Everything written before rich text existed is plain text, and some of it
// contains `<`. Reading it as markup would delete the rest of the line.
func TestPlainTextIsNotReadAsMarkup(t *testing.T) {
	for in, want := range map[string]string{
		"a < b and b > c":       "<p>a &lt; b and b &gt; c</p>",
		"<not-a-tag> stays":     "<p>&lt;not-a-tag&gt; stays</p>",
		"one\ntwo":              "<p>one<br>two</p>",
		"one\n\ntwo":            "<p>one</p><p>two</p>",
		"trailing\r\nwindows":   "<p>trailing<br>windows</p>",
		"5 < 6 && 7 > 2":        "<p>5 &lt; 6 &amp;&amp; 7 &gt; 2</p>",
		"<p>already markup</p>": "<p>already markup</p>",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEmpty(t *testing.T) {
	for _, in := range []string{"", "<p></p>", "<p><br></p>", "   ", "<p>  </p>"} {
		if !IsEmpty(Sanitize(in)) {
			t.Errorf("IsEmpty(%q) = false", in)
		}
	}
	for _, in := range []string{"<p>x</p>", `<img src="/files/a.png">`} {
		if IsEmpty(Sanitize(in)) {
			t.Errorf("IsEmpty(%q) = true", in)
		}
	}
}

func TestToText(t *testing.T) {
	for in, want := range map[string]string{
		"<p>one</p><p>two</p>":                "one\ntwo",
		"<p>one<br>two</p>":                   "one\ntwo",
		"<p>a &lt; b</p>":                     "a < b",
		"<ul><li>one</li><li>two</li></ul>":   "one\ntwo",
		`<p>see <a href="/x">this</a></p>`:    "see this",
		"<h2>Title</h2><p>body</p>":           "Title\nbody",
		`<p><img src="/files/a.png"> cap</p>`: "cap",
	} {
		if got := ToText(in); got != want {
			t.Errorf("ToText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownRendersAndSanitizes(t *testing.T) {
	got := Markdown("# Title\n\nsome **bold** text\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n<script>alert(1)</script>\n")
	for _, want := range []string{"<h1>Title</h1>", "<strong>bold</strong>", "<table>", "<td>1</td>"} {
		if !strings.Contains(got, want) {
			t.Errorf("Markdown dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "alert(1)") {
		t.Errorf("Markdown kept raw HTML: %s", got)
	}
	if md := Markdown("[x](javascript:alert(1))"); strings.Contains(md, "javascript:") {
		t.Errorf("Markdown kept a javascript: link: %s", md)
	}
}

// The desk runs the same fixture against its own implementation, so the two
// sanitizers cannot drift into disagreeing about what a value means.
func TestSharedFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Normalize []struct{ Name, In, Out string } `json:"normalize"`
		Text      []struct{ Name, In, Out string } `json:"text"`
		Contains  []struct {
			Name, In string
			Present  []string `json:"present"`
		} `json:"contains"`
		Strip []struct {
			Name, In string
			Absent   []string `json:"absent"`
		} `json:"strip"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Normalize {
		if got := Normalize(c.In); got != c.Out {
			t.Errorf("%s: Normalize(%q) = %q, want %q", c.Name, c.In, got, c.Out)
		}
	}
	for _, c := range fixture.Text {
		if got := ToText(c.In); got != c.Out {
			t.Errorf("%s: ToText(%q) = %q, want %q", c.Name, c.In, got, c.Out)
		}
	}
	for _, c := range fixture.Contains {
		got := Normalize(c.In)
		for _, want := range c.Present {
			if !strings.Contains(got, want) {
				t.Errorf("%s: Normalize(%q) is missing %q: %s", c.Name, c.In, want, got)
			}
		}
	}
	for _, c := range fixture.Strip {
		got := strings.ToLower(Sanitize(c.In))
		for _, absent := range c.Absent {
			if strings.Contains(got, strings.ToLower(absent)) {
				t.Errorf("%s: Sanitize(%q) kept %q: %s", c.Name, c.In, absent, got)
			}
		}
	}
}
