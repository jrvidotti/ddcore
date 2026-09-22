package print

import (
	"fmt"
	"html"
	"strings"
)

// LetterHead contains the branding and letterhead configuration.
type LetterHead struct {
	Name       string `json:"name"`
	HeaderHTML string `json:"header_html"`
	FooterHTML string `json:"footer_html"`
	Align      string `json:"align"`
	Image      string `json:"image"`
	Disabled   bool   `json:"disabled"`
	IsDefault  bool   `json:"is_default"`
}

// AssembleHTML wraps the rendered body HTML in a self-contained HTML document
// with print stylesheet, letterhead, and document title. page sets the @page
// size and orientation.
func AssembleHTML(bodyHTML string, letterhead *LetterHead, title string, lang string, page PDFOptions) string {
	if lang == "" {
		lang = "en"
	}
	if title == "" {
		title = "Document"
	}

	// An image in a printed document is a path on this site (`/files/…`). A PDF
	// is rendered from a temporary file, so without a base the path would mean
	// the renderer's filesystem root and every image would be a blank box.
	// A private file still needs a session the renderer does not have.
	var baseTag string
	if page.SiteURL != "" {
		baseTag = fmt.Sprintf(`<base href="%s/">`, html.EscapeString(strings.TrimRight(page.SiteURL, "/")))
	}

	var headerSection string
	var footerSection string

	if letterhead != nil && !letterhead.Disabled {
		align := strings.ToLower(letterhead.Align)
		if align == "" {
			align = "left"
		}

		var hContent strings.Builder
		if letterhead.Image != "" {
			hContent.WriteString(fmt.Sprintf(`<div class="letterhead-logo"><img src="%s" alt="Logo" style="max-height: 60px; object-fit: contain;"></div>`, html.EscapeString(letterhead.Image)))
		}
		if letterhead.HeaderHTML != "" {
			hContent.WriteString(fmt.Sprintf(`<div class="letterhead-html">%s</div>`, letterhead.HeaderHTML))
		}
		if hContent.Len() > 0 {
			headerSection = fmt.Sprintf(`<header class="letterhead-header align-%s">%s</header>`, align, hContent.String())
		}

		if letterhead.FooterHTML != "" {
			footerSection = fmt.Sprintf(`<footer class="letterhead-footer align-%s">%s</footer>`, align, letterhead.FooterHTML)
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
  <meta charset="utf-8">
  %s
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    /* CSS Paged Media */
    @page {
      size: %s;
      margin: 15mm 15mm 20mm 15mm;
    }
    * {
      box-sizing: border-box;
    }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      font-size: 10pt;
      line-height: 1.45;
      color: #0f172a;
      background: #ffffff;
      margin: 0;
      padding: 0;
      -webkit-font-smoothing: antialiased;
      -moz-osx-font-smoothing: grayscale;
    }
    .print-container {
      width: 100%%;
      max-width: 100%%;
    }
    .letterhead-header {
      margin-bottom: 24px;
      border-bottom: 1px solid #cbd5e1;
      padding-bottom: 12px;
    }
    .letterhead-footer {
      margin-top: 32px;
      border-top: 1px solid #cbd5e1;
      padding-top: 12px;
      font-size: 8.5pt;
      color: #64748b;
    }
    .align-left { text-align: left; }
    .align-center { text-align: center; }
    .align-right { text-align: right; }

    .print-header {
      display: flex;
      justify-content: space-between;
      align-items: baseline;
      margin-bottom: 20px;
      border-bottom: 2px solid #0f172a;
      padding-bottom: 8px;
    }
    .print-header h1 {
      margin: 0;
      font-size: 18pt;
      font-weight: 700;
      color: #0f172a;
      letter-spacing: -0.02em;
    }
    .print-header .subtitle {
      font-size: 10.5pt;
      color: #64748b;
      margin-top: 2px;
    }
    .print-header .badge {
      display: inline-block;
      padding: 2px 8px;
      font-size: 8.5pt;
      font-weight: 600;
      border-radius: 4px;
      border: 1px solid currentColor;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    .badge.green { color: #16a34a; background: #f0fdf4; }
    .badge.red { color: #dc2626; background: #fef2f2; }
    .badge.blue { color: #2563eb; background: #eff6ff; }
    .badge.orange { color: #ea580c; background: #fff7ed; }
    .badge.gray { color: #64748b; background: #f8fafc; }

    .print-grid {
      display: grid;
      gap: 12px 24px;
      margin-bottom: 20px;
    }
    .print-grid.cols-1 { grid-template-columns: 1fr; }
    .print-grid.cols-2 { grid-template-columns: 1fr 1fr; }
    .print-grid.cols-3 { grid-template-columns: 1fr 1fr 1fr; }
    .print-grid.cols-4 { grid-template-columns: 1fr 1fr 1fr 1fr; }
    .print-columns {
      display: grid;
      gap: 12px 24px;
      margin-bottom: 20px;
    }
    .print-column {
      min-width: 0;
    }
    .grid-item .label {
      font-size: 8pt;
      font-weight: 600;
      text-transform: uppercase;
      color: #64748b;
      margin-bottom: 2px;
      letter-spacing: 0.02em;
    }
    .grid-item .val {
      font-size: 9.5pt;
      font-weight: 500;
      color: #0f172a;
      word-break: break-word;
    }

    .print-heading {
      margin: 16px 0 8px;
      color: #1e293b;
      font-weight: 600;
    }
    h2.print-heading { font-size: 14pt; }
    h3.print-heading { font-size: 12pt; border-bottom: 1px solid #e2e8f0; padding-bottom: 4px; margin-top: 20px; }
    h4.print-heading { font-size: 10.5pt; margin-top: 16px; }

    .print-p {
      margin: 0 0 10px;
      color: #334155;
    }

    .print-field {
      margin-bottom: 16px;
    }
    .print-field > .label {
      font-size: 8pt;
      font-weight: 600;
      text-transform: uppercase;
      color: #64748b;
      margin-bottom: 4px;
      letter-spacing: 0.02em;
    }
    .print-richtext {
      color: #1e293b;
      font-size: 9.5pt;
    }
    .print-richtext > :first-child { margin-top: 0; }
    .print-richtext > :last-child { margin-bottom: 0; }
    .print-richtext p { margin: 0 0 8px; }
    .print-richtext h1 { font-size: 14pt; }
    .print-richtext h2 { font-size: 12.5pt; }
    .print-richtext h3, .print-richtext h4 { font-size: 11pt; }
    .print-richtext ul, .print-richtext ol { margin: 0 0 8px; padding-left: 20px; }
    .print-richtext blockquote {
      margin: 0 0 8px;
      padding-left: 10px;
      border-left: 3px solid #e2e8f0;
      color: #475569;
    }
    .print-richtext img { max-width: 100%%; }
    .print-richtext table {
      width: 100%%;
      border-collapse: collapse;
      margin-bottom: 8px;
    }
    .print-richtext th, .print-richtext td {
      border: 1px solid #e2e8f0;
      padding: 4px 8px;
      text-align: left;
    }
    .print-pre {
      margin: 0;
      padding: 8px 10px;
      background: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 3px;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 8.5pt;
      color: #0f172a;
      white-space: pre-wrap;
      word-break: break-word;
    }

    table.print-table {
      width: 100%%;
      border-collapse: collapse;
      margin-bottom: 20px;
      font-size: 9pt;
    }
    table.print-table thead {
      display: table-header-group;
    }
    table.print-table th {
      background: #f8fafc;
      color: #475569;
      font-weight: 600;
      text-align: left;
      padding: 6px 10px;
      border-bottom: 2px solid #cbd5e1;
      border-top: 1px solid #e2e8f0;
      letter-spacing: 0.01em;
    }
    table.print-table td {
      padding: 6px 10px;
      border-bottom: 1px solid #e2e8f0;
      vertical-align: top;
      color: #1e293b;
    }
    table.print-table tr {
      page-break-inside: avoid;
      break-inside: avoid;
    }

    .print-totals-wrapper {
      display: flex;
      justify-content: flex-end;
      margin-bottom: 20px;
      page-break-inside: avoid;
      break-inside: avoid;
    }
    table.print-totals {
      border-collapse: collapse;
      width: 280px;
      font-size: 9pt;
    }
    table.print-totals td {
      padding: 4px 8px;
    }
    table.print-totals td.label {
      font-weight: 500;
      color: #64748b;
      text-align: right;
    }
    table.print-totals td.val {
      font-weight: 600;
      color: #0f172a;
      text-align: right;
    }
    table.print-totals tr:last-child td {
      border-top: 2px solid #0f172a;
      font-size: 11pt;
      font-weight: 700;
      padding-top: 8px;
    }

    .print-hr {
      border: 0;
      border-top: 1px solid #e2e8f0;
      margin: 16px 0;
    }
    .page-break {
      page-break-after: always;
      break-after: page;
      height: 0;
    }
    .avoid-break {
      page-break-inside: avoid;
      break-inside: avoid;
    }

    @media print {
      body {
        font-size: 9.5pt;
      }
      .no-print {
        display: none !important;
      }
    }
  </style>
</head>
<body>
  <div class="print-container">
    %s
    <main class="print-body">
      %s
    </main>
    %s
  </div>
</body>
</html>`,
		html.EscapeString(lang),
		baseTag,
		html.EscapeString(title),
		page.PageSize(),
		headerSection,
		bodyHTML,
		footerSection,
	)
}
