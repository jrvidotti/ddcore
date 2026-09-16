package print

import (
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// StandardFormatOptions configures standard document rendering.
type StandardFormatOptions struct {
	GetChildMeta func(doctype string) *meta.DocType
	FormatValue  func(f *meta.Field, val any) string
	Translate    func(text string) string
	// CanRead hides a field the reader's permission level does not reach
	// (SEC-02). The document already arrives without its value; this keeps its
	// label and column header out too. Nil reads everything.
	CanRead func(f *meta.Field) bool
}

// StandardTemplate builds standard print blocks from a DocType definition and document data.
func StandardTemplate(d *meta.DocType, doc map[string]any, opts StandardFormatOptions) []Block {
	tr := opts.Translate
	if tr == nil {
		tr = func(s string) string { return s }
	}
	fmtVal := opts.FormatValue
	if fmtVal == nil {
		fmtVal = defaultFormatValue
	}

	var blocks []Block

	// 1. Document Header
	docName := str(doc["name"])
	title := tr(d.Label)
	if title == "" {
		title = d.Name
	}
	subtitle := docName
	if d.TitleField != "" && doc[d.TitleField] != nil {
		tVal := str(doc[d.TitleField])
		if tVal != "" {
			title = fmt.Sprintf("%s: %s", title, tVal)
			subtitle = docName
		}
	}

	badge := ""
	badgeColor := "gray"
	if d.Submittable {
		status := intVal(doc["docstatus"])
		switch status {
		case 0:
			badge = tr("Draft")
			badgeColor = "orange"
		case 1:
			badge = tr("Submitted")
			badgeColor = "blue"
		case 2:
			badge = tr("Cancelled")
			badgeColor = "red"
		}
	}

	blocks = append(blocks, Block{
		Type:       "header",
		Title:      title,
		Subtitle:   subtitle,
		Badge:      badge,
		BadgeColor: badgeColor,
	})

	// 2. Sections and Fields
	var currentSectionTitle string
	var currentPairs [][]string

	flushKeyValues := func() {
		if len(currentPairs) > 0 {
			blocks = append(blocks, Block{
				Type:    "keyValues",
				Columns: 2,
				Pairs:   currentPairs,
			})
			currentPairs = nil
		}
	}

	flushSection := func(newTitle string) {
		flushKeyValues()
		currentSectionTitle = newTitle
		if currentSectionTitle != "" {
			blocks = append(blocks, Block{
				Type:  "h",
				Level: 3,
				Text:  tr(currentSectionTitle),
			})
		}
	}

	for _, f := range d.Fields {
		// Layout fields
		if f.Fieldtype == "Section Break" {
			flushSection(f.Label)
			continue
		}
		if meta.LayoutTypes[f.Fieldtype] {
			continue
		}

		// Security: skip password and vault fields unconditionally
		if f.Fieldtype == "Password" || f.Fieldtype == "Vault" {
			continue
		}
		if f.Hidden || (opts.CanRead != nil && !opts.CanRead(f)) {
			continue
		}

		// Child Table
		if f.Fieldtype == "Table" {
			flushKeyValues()
			childTableBlocks := renderChildTable(f, doc[f.Fieldname], opts)
			blocks = append(blocks, childTableBlocks...)
			continue
		}

		// Standard field
		val := doc[f.Fieldname]
		if val == nil || val == "" {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.Fieldname
		}
		valStr := fmtVal(f, val)
		if valStr != "" {
			currentPairs = append(currentPairs, []string{tr(label), valStr})
		}
	}

	flushKeyValues()

	return blocks
}

func renderChildTable(f *meta.Field, val any, opts StandardFormatOptions) []Block {
	rows, ok := val.([]any)
	if !ok || len(rows) == 0 {
		// Also check []map[string]any
		mapRows, okMap := val.([]map[string]any)
		if !okMap || len(mapRows) == 0 {
			return nil
		}
		rows = make([]any, len(mapRows))
		for i, r := range mapRows {
			rows[i] = r
		}
	}

	childDocType := f.OptionsString()
	if childDocType == "" || opts.GetChildMeta == nil {
		return nil
	}
	cd := opts.GetChildMeta(childDocType)
	if cd == nil {
		return nil
	}

	tr := opts.Translate
	if tr == nil {
		tr = func(s string) string { return s }
	}
	fmtVal := opts.FormatValue
	if fmtVal == nil {
		fmtVal = defaultFormatValue
	}

	// Determine visible columns
	var colFields []*meta.Field
	var headers []string
	var aligns []string

	for _, cf := range cd.Fields {
		if cf.Hidden || meta.LayoutTypes[cf.Fieldtype] || cf.Fieldtype == "Password" || cf.Fieldtype == "Vault" ||
			(opts.CanRead != nil && !opts.CanRead(cf)) {
			continue
		}
		// Prefer inListView fields or non-empty fields
		if cf.InListView || len(colFields) < 6 {
			colFields = append(colFields, cf)
			colLabel := cf.Label
			if colLabel == "" {
				colLabel = cf.Fieldname
			}
			headers = append(headers, tr(colLabel))
			switch cf.Fieldtype {
			case "Int", "Float", "Currency", "Percent":
				aligns = append(aligns, "right")
			case "Check":
				aligns = append(aligns, "center")
			default:
				aligns = append(aligns, "left")
			}
		}
	}

	if len(colFields) == 0 {
		return nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		rowMap, ok := r.(map[string]any)
		if !ok {
			continue
		}
		rowVals := make([]string, len(colFields))
		for i, cf := range colFields {
			rowVals[i] = fmtVal(cf, rowMap[cf.Fieldname])
		}
		tableRows = append(tableRows, rowVals)
	}

	var blocks []Block
	tableTitle := f.Label
	if tableTitle == "" {
		tableTitle = f.Fieldname
	}
	blocks = append(blocks, Block{
		Type:  "h",
		Level: 4,
		Text:  tr(tableTitle),
	})
	blocks = append(blocks, Block{
		Type:    "table",
		Headers: headers,
		Rows:    tableRows,
		Aligns:  aligns,
	})

	return blocks
}

func defaultFormatValue(f *meta.Field, val any) string {
	if val == nil {
		return ""
	}
	switch f.Fieldtype {
	case "Check":
		if b, ok := val.(bool); ok && b {
			return "✓"
		}
		if s, ok := val.(string); ok && (s == "1" || strings.EqualFold(s, "true")) {
			return "✓"
		}
		return ""
	default:
		return fmt.Sprint(val)
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func intVal(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
