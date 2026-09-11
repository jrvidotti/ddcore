package mail

import (
	"strings"
	"testing"
)

func TestRenderParagraphsAndHeading(t *testing.T) {
	text, html := Render([]Block{
		{Type: "h", Text: "Your order"},
		{Type: "p", Text: "Hi Ana,"},
		{Type: "p", Text: "We received it."},
	})

	if !strings.Contains(text, "Your order\n==========") {
		t.Errorf("heading is not underlined in text:\n%s", text)
	}
	if !strings.Contains(text, "Hi Ana,\n\nWe received it.") {
		t.Errorf("paragraphs are not separated by a blank line:\n%s", text)
	}
	for _, want := range []string{"<h2", ">Your order</h2>", "<p", ">Hi Ana,</p>"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

// Every string in a block arrives already translated from app code, so it is
// data and never markup. The renderer is the only thing standing between a
// customer's name and the reader's mail client.
func TestRenderEscapesEveryString(t *testing.T) {
	text, html := Render([]Block{
		{Type: "p", Text: `Tom & "Jerry" <script>`},
		{Type: "table", Head: []string{"<th>"}, Rows: [][]string{{"<td>"}}},
		{Type: "button", Text: "<b>go</b>", URL: "https://x.com/?a=1&b=2"},
	})

	if strings.Contains(html, "<script>") || strings.Contains(html, "<b>go</b>") || strings.Contains(html, "<th>") {
		t.Errorf("unescaped markup survived into the HTML part:\n%s", html)
	}
	if !strings.Contains(html, "Tom &amp; &quot;Jerry&quot; &lt;script&gt;") {
		t.Errorf("expected escaped text:\n%s", html)
	}
	if !strings.Contains(html, `href="https://x.com/?a=1&amp;b=2"`) {
		t.Errorf("expected an escaped href:\n%s", html)
	}
	// The text part is not markup, so it keeps the characters as written.
	if !strings.Contains(text, `Tom & "Jerry" <script>`) {
		t.Errorf("text part should not be escaped:\n%s", text)
	}
}

// A mail client will not follow javascript:, but the framework should not be
// the thing that put it there.
func TestRenderRefusesAnUnsafeButtonScheme(t *testing.T) {
	_, html := Render([]Block{{Type: "button", Text: "click", URL: "javascript:alert(1)"}})
	if strings.Contains(html, "javascript:") {
		t.Errorf("unsafe scheme survived:\n%s", html)
	}
	if strings.Contains(html, "<a ") {
		t.Errorf("an unsafe URL must not become a link:\n%s", html)
	}
}

func TestRenderAlignsTableColumnsInText(t *testing.T) {
	text, html := Render([]Block{{
		Type: "table",
		Head: []string{"Item", "Qty"},
		Rows: [][]string{{"Cadeira", "2"}, {"Pá", "10"}},
	}})

	// Not TrimSpace: the last cell is padded, and trimming it would hide the
	// very thing this test is about.
	lines := strings.Split(text, "\n")
	if len(lines) != 4 { // head, rule, two rows
		t.Fatalf("expected four lines, got %d:\n%s", len(lines), text)
	}
	width := len([]rune(lines[0]))
	for i, l := range lines {
		if got := len([]rune(l)); got != width {
			t.Errorf("line %d is %d runes wide, want %d:\n%s", i, got, width, text)
		}
	}
	// "Pá" is two runes and three bytes: padding must count runes.
	if !strings.Contains(text, "Pá      | 10") {
		t.Errorf("accented cell is padded by bytes, not runes:\n%s", text)
	}
	if !strings.Contains(html, "<table") || !strings.Contains(html, "<th") || !strings.Contains(html, "<td") {
		t.Errorf("expected a real table in the HTML part:\n%s", html)
	}
}

func TestRenderRuleAndUnknownBlock(t *testing.T) {
	text, html := Render([]Block{
		{Type: "p", Text: "before"},
		{Type: "rule"},
		{Type: "nonsense", Text: "dropped"},
		{Type: "p", Text: "after"},
	})
	if !strings.Contains(text, "before") || !strings.Contains(text, "after") {
		t.Errorf("a block the renderer does not know must not swallow its neighbours:\n%s", text)
	}
	if strings.Contains(text, "dropped") || strings.Contains(html, "dropped") {
		t.Errorf("an unknown block should render nothing:\n%s\n%s", text, html)
	}
	if !strings.Contains(html, "<hr") {
		t.Errorf("expected a rule in the HTML part:\n%s", html)
	}
}

func TestRenderEmptyIsEmpty(t *testing.T) {
	text, html := Render(nil)
	if text != "" || html != "" {
		t.Errorf("nothing in, nothing out; got %q / %q", text, html)
	}
}
