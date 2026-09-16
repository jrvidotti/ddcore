package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/print"
)

// PrintFormatInfo provides metadata about an available print format for a DocType.
type PrintFormatInfo struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// ListPrintFormats returns all valid print formats for the given DocType.
func (c *Ctx) ListPrintFormats(doctype string) ([]PrintFormatInfo, error) {
	_, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}

	formats := []PrintFormatInfo{
		{Name: "standard", Label: c.T("Standard"), Default: true},
	}

	if c.St.Snap.PrintTemplates != nil {
		for _, pt := range c.St.Snap.PrintTemplates {
			if pt.Doctype == doctype {
				label := pt.Label
				if label == "" {
					label = pt.Name
				}
				formats = append(formats, PrintFormatInfo{
					Name:  pt.Name,
					Label: c.T(label),
				})
			}
		}
	}

	return formats, nil
}

// PrintDoc renders a document to a complete HTML document with print styling.
func (c *Ctx) PrintDoc(doctype, name, format, letterheadName, lang string) (string, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return "", err
	}

	// 1. Load document (enforces existence and read permission)
	doc, err := c.GetDoc(doctype, name)
	if err != nil {
		return "", err
	}

	// 2. Resolve language
	if lang == "" {
		lang = c.Lang
	}
	if lang == "" {
		lang = c.E.Cfg.Lang
	}
	if lang == "" {
		lang = "en"
	}

	// 3. Resolve letterhead
	var lh *print.LetterHead
	if letterheadName != "none" {
		lh, err = c.resolveLetterHead(letterheadName)
		if err != nil {
			return "", err
		}
	}

	// 4. Sanitize document: strip passwords, vault fields, and restricted fields
	access := c.FieldAccess(d)
	sanitized := c.sanitizeDocForPrint(d, doc, access)

	// 5. Render body
	var bodyHTML string
	if format == "" || format == "standard" {
		opts := print.StandardFormatOptions{
			GetChildMeta: func(dt string) *meta.DocType {
				cd, _ := c.St.DocType(dt)
				return cd
			},
			Translate: func(s string) string {
				return c.St.I18n.T(lang, s)
			},
			FormatValue: func(f *meta.Field, val any) string {
				return c.formatPrintValue(f, val, lang)
			},
			CanRead: access.CanRead,
		}
		blocks := print.StandardTemplate(d, sanitized, opts)
		bodyHTML = print.RenderBlocks(blocks)
	} else {
		if c.St.Snap.PrintTemplates == nil {
			return "", cerr.NotFound("Print template {0} does not exist", format)
		}
		pt, ok := c.St.Snap.PrintTemplates[format]
		if !ok {
			return "", cerr.NotFound("Print template {0} does not exist", format)
		}
		if pt.Doctype != doctype {
			return "", cerr.Validation("Print template {0} is for {1}, not {2}", format, pt.Doctype, doctype)
		}

		rt, err := c.St.Pool.Acquire()
		if err != nil {
			return "", err
		}
		defer c.St.Pool.Release(rt)
		rt.Ctx = c
		rt.SetLang(lang)
		defer func() {
			rt.Ctx = nil
			rt.SetLang("")
		}()

		docBytes, err := json.Marshal(sanitized)
		if err != nil {
			return "", err
		}

		resJSON, err := rt.RenderPrint(format, string(docBytes), lang)
		if err != nil {
			return "", err
		}

		var parsed struct {
			Blocks []print.Block `json:"blocks"`
		}
		if err := json.Unmarshal([]byte(resJSON), &parsed); err != nil {
			return "", fmt.Errorf("invalid print template output: %w", err)
		}
		bodyHTML = print.RenderBlocks(parsed.Blocks)
	}

	// 6. Assemble HTML
	docTitle := doc.Name()
	if d.TitleField != "" && doc[d.TitleField] != nil {
		tVal := db.Str(doc[d.TitleField])
		if tVal != "" {
			docTitle = tVal
		}
	}
	title := fmt.Sprintf("%s - %s", c.St.I18n.T(lang, d.Label), docTitle)
	return print.AssembleHTML(bodyHTML, lh, title, lang), nil
}

func (c *Ctx) resolveLetterHead(name string) (*print.LetterHead, error) {
	// If Letter Head DocType doesn't exist yet, return nil gracefully
	if _, err := c.St.DocType("Letter Head"); err != nil {
		return nil, nil
	}

	var rows []map[string]any
	var err error

	if name != "" {
		rows, err = db.Select(c.Ctx, c.Q(), `SELECT letter_head_name, header_html, footer_html, align, image, disabled, is_default FROM tab_letter_head WHERE name = $1 AND NOT disabled`, name)
	} else {
		rows, err = db.Select(c.Ctx, c.Q(), `SELECT letter_head_name, header_html, footer_html, align, image, disabled, is_default FROM tab_letter_head WHERE is_default AND NOT disabled LIMIT 1`)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	r := rows[0]
	return &print.LetterHead{
		Name:       db.Str(r["letter_head_name"]),
		HeaderHTML: db.Str(r["header_html"]),
		FooterHTML: db.Str(r["footer_html"]),
		Align:      db.Str(r["align"]),
		Image:      db.Str(r["image"]),
		Disabled:   db.Str(r["disabled"]) == "1",
		IsDefault:  db.Str(r["is_default"]) == "1",
	}, nil
}

// sanitizeDocForPrint copies doc without secrets and without the fields the
// reader's access cannot read; a child row is judged by the parent's access.
func (c *Ctx) sanitizeDocForPrint(d *meta.DocType, doc Doc, access FieldAccess) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		f := d.Field(k)
		if f != nil {
			if f.Fieldtype == "Password" || f.Fieldtype == "Vault" || !access.CanRead(f) {
				continue
			}
			if f.Fieldtype == "Table" {
				cd, _ := c.St.DocType(f.OptionsString())
				if cd != nil {
					rows := doc.Children(k)
					childOut := make([]any, 0, len(rows))
					for _, cr := range rows {
						childOut = append(childOut, c.sanitizeDocForPrint(cd, cr, access))
					}
					out[k] = childOut
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func (c *Ctx) formatPrintValue(f *meta.Field, val any, lang string) string {
	if val == nil {
		return ""
	}

	switch f.Fieldtype {
	case "Date":
		s := db.Str(val)
		if strings.HasPrefix(strings.ToLower(lang), "pt") {
			parts := strings.Split(s, "-")
			if len(parts) == 3 {
				return fmt.Sprintf("%s/%s/%s", parts[2], parts[1], parts[0])
			}
		}
		return s
	case "Datetime":
		t, err := time.Parse(time.RFC3339Nano, db.Str(val))
		if err != nil {
			t, err = time.Parse(time.RFC3339, db.Str(val))
		}
		if err != nil {
			return db.Str(val)
		}
		loc := c.E.Location()
		if loc != nil {
			t = t.In(loc)
		}
		if strings.HasPrefix(strings.ToLower(lang), "pt") {
			return t.Format("02/01/2006 15:04:05")
		}
		return t.Format("2006-01-02 15:04:05")
	case "Currency":
		numVal := toFloat(val)
		curr := c.E.Cfg.Currency
		if curr == "" {
			curr = "USD"
		}
		isPT := strings.HasPrefix(strings.ToLower(lang), "pt")
		formatted := formatNumber(numVal, 2, isPT)
		if isPT {
			return fmt.Sprintf("R$ %s", formatted)
		}
		return fmt.Sprintf("%s %s", curr, formatted)
	case "Percent":
		numVal := toFloat(val)
		isPT := strings.HasPrefix(strings.ToLower(lang), "pt")
		return fmt.Sprintf("%s%%", formatNumber(numVal, 2, isPT))
	case "Check":
		if db.Str(val) == "1" || val == true {
			return "✓"
		}
		return ""
	case "Select":
		raw := db.Str(val)
		return c.St.I18n.T(lang, raw)
	default:
		return fmt.Sprint(val)
	}
}

func formatNumber(n float64, decimals int, isPT bool) string {
	parts := strings.Split(fmt.Sprintf("%.*f", decimals, n), ".")
	intPart := parts[0]
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	thousandSep := ","
	decimalSep := "."
	if isPT {
		thousandSep = "."
		decimalSep = ","
	}

	sign := ""
	if strings.HasPrefix(intPart, "-") {
		sign = "-"
		intPart = intPart[1:]
	}

	var grouped strings.Builder
	l := len(intPart)
	for i, ch := range intPart {
		if i > 0 && (l-i)%3 == 0 {
			grouped.WriteString(thousandSep)
		}
		grouped.WriteRune(ch)
	}

	if decimals == 0 {
		return sign + grouped.String()
	}
	return fmt.Sprintf("%s%s%s%s", sign, grouped.String(), decimalSep, fracPart)
}
