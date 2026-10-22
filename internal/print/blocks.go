package print

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/jrvidotti/ddcore/internal/barcode"
	"github.com/jrvidotti/ddcore/internal/richtext"
	"github.com/jrvidotti/ddcore/internal/signature"
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
	// Symbology is a "barcode" block's: Code128 (the default), EAN-13 or QR.
	Symbology string `json:"symbology,omitempty"`
	// MaxHeight and MaxWidth bound an "image" block, as CSS lengths.
	MaxHeight string `json:"maxHeight,omitempty"`
	MaxWidth  string `json:"maxWidth,omitempty"`
	// Src is an "image" block's picture, a data URL. Only the engine sets it,
	// after reading the File named in Text as the user printing: it is never
	// read from a template's output, so no other URL reaches the <img>.
	Src string `json:"-"`
}

// imageDataURLRe is what an "image" block prints: one of the Attach Image
// types, inlined, since the PDF renderer has no session to fetch a private
// file with. SVG is not among them — it can carry script.
var imageDataURLRe = regexp.MustCompile(`^data:image/(png|jpeg|gif|webp);base64,[A-Za-z0-9+/]+=*$`)

// cssLengthRe is a length an image may be bounded by; anything else is
// dropped, so a template cannot write its own CSS through it.
var cssLengthRe = regexp.MustCompile(`^\d+(\.\d+)?(mm|cm|in|pt|px|%)$`)

// ImageTypes are the content types an "image" block inlines.
var ImageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// Walk calls fn for every block, those nested in sections and columns too.
func Walk(blocks []Block, fn func(*Block)) {
	for i := range blocks {
		fn(&blocks[i])
		Walk(blocks[i].Blocks, fn)
		for _, cell := range blocks[i].Cells {
			Walk(cell, fn)
		}
	}
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

	case "barcode":
		// Drawn here, as vectors, so a PDF prints bars as sharp as the
		// printer can make them. A value its symbology refuses — a template
		// can build this block from anything — prints as the text it is.
		if b.Text == "" {
			return ""
		}
		svg, err := barcode.SVG(b.Symbology, b.Text)
		if err != nil {
			return b.labelled(`<div class="print-p">` + html.EscapeString(b.Text) + `</div>`)
		}
		class := "print-barcode"
		if b.Symbology == barcode.QR {
			class += " qr"
		}
		return b.labelled(`<div class="` + class + `">` + string(svg) + `</div>`)

	case "signature":
		// The rich-text sanitizer forbids data: URLs, so the image is built
		// here — and only after the value is checked again, since a template
		// can build this block from anything: what reaches src is a small
		// PNG data URL or nothing at all.
		v, err := signature.Normalize(b.Text)
		if err != nil || v == "" {
			return ""
		}
		return b.labelled(`<div class="print-signature-box"><img class="print-signature" src="` + html.EscapeString(v) + `" alt=""></div>`)

	case "image":
		if !imageDataURLRe.MatchString(b.Src) {
			return ""
		}
		var style []string
		if cssLengthRe.MatchString(b.MaxHeight) {
			style = append(style, "max-height: "+b.MaxHeight)
		}
		if cssLengthRe.MatchString(b.MaxWidth) {
			style = append(style, "max-width: "+b.MaxWidth)
		}
		attr := ""
		if len(style) > 0 {
			attr = ` style="` + strings.Join(style, "; ") + `"`
		}
		return b.labelled(`<div class="print-image-box"><img class="print-image" src="` + b.Src + `"` + attr + ` alt=""></div>`)

	case "raw":
		// Raw HTML escape hatch — rendered unescaped by deliberate design for custom markup
		return `<div class="raw-html">` + b.HTML + `</div>`
	}
	return ""
}
