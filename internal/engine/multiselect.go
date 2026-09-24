package engine

import (
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// normalizeMultiSelect lets a Table MultiSelect be written as a plain list of
// ids (`["A", "B"]`) as well as rows: each id becomes a row holding it in the
// child's Link field. A value already chosen in `before` keeps that row, id
// included, so re-sending the same list rewrites nothing. It runs before any
// comparison with `before` (field levels, allow-on-submit), which read rows.
func (c *Ctx) normalizeMultiSelect(d *meta.DocType, doc, before Doc) {
	for _, f := range d.Fields {
		if f.Fieldtype != "Table MultiSelect" {
			continue
		}
		child, err := c.St.DocType(f.OptionsString())
		if err != nil || child == nil {
			continue
		}
		link := child.MultiSelectLinkField()
		if link == nil {
			continue
		}
		var items []any
		switch v := doc[f.Fieldname].(type) {
		case []any:
			items = v
		case []string:
			for _, s := range v {
				items = append(items, s)
			}
		default:
			continue
		}
		var existing map[string]Doc
		if before != nil {
			existing = map[string]Doc{}
			for _, r := range before.Children(f.Fieldname) {
				if k := r.Str(link.Fieldname); k != "" {
					existing[k] = r
				}
			}
		}
		rows := make([]any, 0, len(items))
		for _, it := range items {
			s, ok := it.(string)
			if !ok {
				rows = append(rows, it)
				continue
			}
			if r, ok := existing[s]; ok {
				rows = append(rows, map[string]any(r.Clone()))
				continue
			}
			rows = append(rows, map[string]any{link.Fieldname: s})
		}
		doc[f.Fieldname] = rows
	}
}

// checkMultiSelect refuses an empty or repeated value in a Table
// MultiSelect: each row is one choice, and choosing the same one twice is a
// mistake, never a quantity.
func (c *Ctx) checkMultiSelect(tf *meta.Field, child *meta.DocType, rows []Doc) error {
	link := child.MultiSelectLinkField()
	if link == nil {
		return nil
	}
	seen := map[string]bool{}
	for i, row := range rows {
		v := row.Str(link.Fieldname)
		if v == "" {
			return cerr.Validation("{0}, row {1}: {2} is required", c.T(tf.Label), i+1, c.T(link.Label)).WithTitleKey("Required fields")
		}
		if seen[v] {
			return cerr.Validation("{0}: \"{1}\" is chosen more than once", c.T(tf.Label), v).WithTitleKey("Duplicate value")
		}
		seen[v] = true
	}
	return nil
}
