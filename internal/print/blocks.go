package print

import (
	"fmt"
	"html"
	"strings"

	"github.com/jrvidotti/ddcore/internal/richtext"
)

// Block is one structural piece of a printed document.
type Block struct {
	Type       string     `json:"type"`
	Title      string     `json:"title,omitempty"`
	Subtitle   string     `json:"subtitle,omitempty"`
	Badge      string     `json:"badge,omitempty"`
	BadgeColor string     `json:"badgeColor,omitempty"`
	Columns    int        `json:"columns,omitempty"`
	Pairs      [][]string `json:"pairs,omitempty"`
	Headers    []string   `json:"headers,omitempty"`
	Rows       [][]string `json:"rows,omitempty"`
	Aligns     []string   `json:"aligns,omitempty"`
	Text       string     `json:"text,omitempty"`
	Level      int        `json:"level,omitempty"`
	HTML       string     `json:"html,omitempty"`
	Blocks     []Block    `json:"blocks,omitempty"`
	// Cells are the columns of a "columns" block, each a list of blocks.
	Cells [][]Block `json:"cells,omitempty"`
}

// labelled puts a block's title above it, the way the key/value grid labels an
// ordinary field, so a full-width value is not printed without saying what it
// is.
func (b Block) labelled(body string) string {
	if b.Title == "" {
		return body
	}
	return `<div class="print-field"><div class="label">` + html.EscapeString(b.Title) + `</div>` + body + `</div>`
}

// RenderHTML converts a slice of Blocks into escaped, styled HTML.
func RenderBlocks(blocks []Block) string {
	var sb strings.Builder
	for _, b := range blocks {
		sb.WriteString(b.RenderHTML())
	}
	return sb.String()
}

// RenderHTML renders a single block to HTML.
func (b Block) RenderHTML() string {
	switch b.Type {
	case "header":
		var sb strings.Builder
		sb.WriteString(`<div class="print-header"><div>`)
		if b.Title != "" {
			sb.WriteString(`<h1>` + html.EscapeString(b.Title) + `</h1>`)
		}
		if b.Subtitle != "" {
			sb.WriteString(`<div class="subtitle">` + html.EscapeString(b.Subtitle) + `</div>`)
		}
		sb.WriteString(`</div>`)
		if b.Badge != "" {
			color := "gray"
			if b.BadgeColor != "" {
				color = b.BadgeColor
			}
			sb.WriteString(`<span class="badge ` + html.EscapeString(color) + `">` + html.EscapeString(b.Badge) + `</span>`)
		}
		sb.WriteString(`</div>`)
		return sb.String()

	case "keyValues":
		cols := b.Columns
		if cols < 1 || cols > 4 {
			cols = 2
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`<div class="print-grid cols-%d">`, cols))
		for _, pair := range b.Pairs {
			if len(pair) < 2 {
				continue
			}
			sb.WriteString(`<div class="grid-item">`)
			sb.WriteString(`<div class="label">` + html.EscapeString(pair[0]) + `</div>`)
			sb.WriteString(`<div class="val">` + html.EscapeString(pair[1]) + `</div>`)
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
		return sb.String()

	case "section":
		var sb strings.Builder
		sb.WriteString(`<div class="print-section">`)
		if b.Title != "" {
			sb.WriteString(`<h2>` + html.EscapeString(b.Title) + `</h2>`)
		}
		if len(b.Blocks) > 0 {
			sb.WriteString(RenderBlocks(b.Blocks))
		}
		sb.WriteString(`</div>`)
		return sb.String()

	case "table":
		var sb strings.Builder
		sb.WriteString(`<table class="print-table">`)
		if len(b.Headers) > 0 {
			sb.WriteString(`<thead><tr>`)
			for i, h := range b.Headers {
				align := "left"
				if i < len(b.Aligns) && b.Aligns[i] != "" {
					align = b.Aligns[i]
				}
				sb.WriteString(fmt.Sprintf(`<th style="text-align:%s">%s</th>`, align, html.EscapeString(h)))
			}
			sb.WriteString(`</tr></thead>`)
		}
		sb.WriteString(`<tbody>`)
		for _, row := range b.Rows {
			sb.WriteString(`<tr>`)
			for i, c := range row {
				align := "left"
				if i < len(b.Aligns) && b.Aligns[i] != "" {
					align = b.Aligns[i]
				}
				sb.WriteString(fmt.Sprintf(`<td style="text-align:%s">%s</td>`, align, html.EscapeString(c)))
			}
			sb.WriteString(`</tr>`)
		}
		sb.WriteString(`</tbody></table>`)
		return sb.String()

	case "totals":
		var sb strings.Builder
		sb.WriteString(`<div class="print-totals-wrapper"><table class="print-totals"><tbody>`)
		for _, pair := range b.Pairs {
			if len(pair) < 2 {
				continue
			}
			sb.WriteString(`<tr>`)
			sb.WriteString(`<td class="label">` + html.EscapeString(pair[0]) + `</td>`)
			sb.WriteString(`<td class="val">` + html.EscapeString(pair[1]) + `</td>`)
			sb.WriteString(`</tr>`)
		}
		sb.WriteString(`</tbody></table></div>`)
		return sb.String()

	case "p":
		if b.Text == "" {
			return ""
		}
		return `<p class="print-p">` + html.EscapeString(b.Text) + `</p>`

	case "h":
		lvl := b.Level
		if lvl < 1 || lvl > 6 {
			lvl = 2
		}
		return fmt.Sprintf(`<h%d class="print-heading">%s</h%d>`, lvl, html.EscapeString(b.Text), lvl)

	case "columns":
		if len(b.Cells) == 0 {
			return ""
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`<div class="print-columns" style="grid-template-columns: repeat(%d, 1fr)">`, len(b.Cells)))
		for _, cell := range b.Cells {
			sb.WriteString(`<div class="print-column">`)
			sb.WriteString(RenderBlocks(cell))
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
		return sb.String()

	case "rule":
		return `<hr class="print-hr">`

	case "pageBreak":
		return `<div class="page-break"></div>`

	case "richText":
		// A Text Editor value, already cleaned when it was stored. It is
		// cleaned again here because a template can build this block from
		// anything it reads, and print is the one surface that renders markup
		// instead of escaping it.
		body := richtext.Sanitize(b.HTML)
		if body == "" {
			return ""
		}
		return b.labelled(`<div class="print-richtext">` + body + `</div>`)

	case "markdown":
		body := richtext.Markdown(b.Text)
		if body == "" {
			return ""
		}
		return b.labelled(`<div class="print-richtext">` + body + `</div>`)

	case "pre":
		if b.Text == "" {
			return ""
		}
		return b.labelled(`<pre class="print-pre">` + html.EscapeString(b.Text) + `</pre>`)

	case "raw":
		// Raw HTML escape hatch — rendered unescaped by deliberate design for custom markup
		return `<div class="raw-html">` + b.HTML + `</div>`
	}
	return ""
}
