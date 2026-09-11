package mail

import (
	"strings"
)

// A Block is one piece of a message body. App code builds a list of these in
// TypeScript, where the strings are already translated into the reader's
// language; the renderer turns the same list into both parts of the multipart
// message. Splitting it this way is what keeps an app from writing HTML: every
// string here is data, and escaping is this file's problem alone.
//
// The vocabulary is deliberately small. A layout that needs more than these
// blocks is a reason to extend the vocabulary once, in the open, rather than a
// reason to let one app hand-roll markup nobody else can restyle.
type Block struct {
	Type string     `json:"type"` // p | h | button | table | rule
	Text string     `json:"text,omitempty"`
	URL  string     `json:"url,omitempty"`
	Head []string   `json:"head,omitempty"`
	Rows [][]string `json:"rows,omitempty"`
}

// Inline styles, because a mail client strips <style> and ignores a stylesheet.
const (
	styleP      = `margin:0 0 16px;font:14px/1.5 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1a1a1a`
	styleH      = `margin:0 0 12px;font:600 18px/1.3 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1a1a1a`
	styleButton = `display:inline-block;padding:10px 18px;border-radius:6px;background:#1a1a1a;color:#ffffff;text-decoration:none;font:600 14px/1 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif`
	styleTable  = `border-collapse:collapse;margin:0 0 16px;font:14px/1.5 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1a1a1a`
	styleTh     = `padding:6px 10px;border-bottom:2px solid #1a1a1a;text-align:left`
	styleTd     = `padding:6px 10px;border-bottom:1px solid #e0e0e0;text-align:left`
	styleRule   = `border:0;border-top:1px solid #e0e0e0;margin:0 0 16px`
)

const textRule = "----------------------------------------"

// Render turns blocks into the plain-text and HTML parts of a message. A block
// whose type is unknown renders nothing at all: a newer app talking to an older
// core loses that block instead of leaking a Go struct into someone's inbox.
func Render(blocks []Block) (text, html string) {
	var textParts, htmlParts []string
	for _, b := range blocks {
		t, h := b.render()
		if t != "" {
			textParts = append(textParts, t)
		}
		if h != "" {
			htmlParts = append(htmlParts, h)
		}
	}
	return strings.Join(textParts, "\n\n"), strings.Join(htmlParts, "\n")
}

func (b Block) render() (text, html string) {
	switch b.Type {
	case "p":
		if b.Text == "" {
			return "", ""
		}
		return b.Text, "<p style=\"" + styleP + "\">" + escapeHTML(b.Text) + "</p>"

	case "h":
		if b.Text == "" {
			return "", ""
		}
		return b.Text + "\n" + strings.Repeat("=", len([]rune(b.Text))),
			"<h2 style=\"" + styleH + "\">" + escapeHTML(b.Text) + "</h2>"

	case "button":
		if b.Text == "" {
			return "", ""
		}
		// The text part always spells the address out: a plain-text reader has
		// no link to click, and this is also what lets `ddcore user invite`
		// work off the log transport.
		if !safeURL(b.URL) {
			return b.Text, "<p style=\"" + styleP + "\">" + escapeHTML(b.Text) + "</p>"
		}
		return b.Text + ": " + b.URL,
			"<p style=\"" + styleP + "\"><a href=\"" + escapeHTML(b.URL) + "\" style=\"" + styleButton + "\">" + escapeHTML(b.Text) + "</a></p>"

	case "table":
		return renderTable(b.Head, b.Rows)

	case "rule":
		return textRule, "<hr style=\"" + styleRule + "\">"
	}
	return "", ""
}

func renderTable(head []string, rows [][]string) (text, html string) {
	grid := make([][]string, 0, len(rows)+1)
	if len(head) > 0 {
		grid = append(grid, head)
	}
	grid = append(grid, rows...)
	if len(grid) == 0 {
		return "", ""
	}

	// Width is counted in runes: an accented cell is more bytes than columns,
	// and padding by bytes is how a table comes out ragged in Portuguese.
	width := 0
	for _, r := range grid {
		if len(r) > width {
			width = len(r)
		}
	}
	widths := make([]int, width)
	for _, r := range grid {
		for i, cell := range r {
			if n := len([]rune(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}

	pad := func(cells []string) string {
		out := make([]string, width)
		for i := range out {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			out[i] = cell + strings.Repeat(" ", widths[i]-len([]rune(cell)))
		}
		return strings.Join(out, " | ")
	}

	lines := make([]string, 0, len(grid)+1)
	var body strings.Builder
	body.WriteString("<table style=\"" + styleTable + "\" cellspacing=\"0\">")
	if len(head) > 0 {
		lines = append(lines, pad(head))
		rule := make([]string, width)
		for i := range rule {
			rule[i] = strings.Repeat("-", widths[i])
		}
		lines = append(lines, strings.Join(rule, "-+-"))

		body.WriteString("<thead><tr>")
		for _, cell := range head {
			body.WriteString("<th style=\"" + styleTh + "\">" + escapeHTML(cell) + "</th>")
		}
		body.WriteString("</tr></thead>")
	}
	body.WriteString("<tbody>")
	for _, r := range rows {
		lines = append(lines, pad(r))
		body.WriteString("<tr>")
		for _, cell := range r {
			body.WriteString("<td style=\"" + styleTd + "\">" + escapeHTML(cell) + "</td>")
		}
		body.WriteString("</tr>")
	}
	body.WriteString("</tbody></table>")
	return strings.Join(lines, "\n"), body.String()
}

// safeURL accepts what belongs in a message: an absolute http(s) address, a
// mailto, or a site-relative path — which is what a link becomes when
// DDCORE_URL is unset, and dropping it there would hide the problem instead of
// showing it. Anything carrying another scheme is rendered as text.
func safeURL(u string) bool {
	if u == "" {
		return false
	}
	if strings.HasPrefix(u, "/") {
		return true
	}
	colon, slash := strings.Index(u, ":"), strings.Index(u, "/")
	if colon < 0 || (slash >= 0 && slash < colon) {
		return false // no scheme at all: not an address we can vouch for
	}
	switch strings.ToLower(u[:colon]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s)
}
