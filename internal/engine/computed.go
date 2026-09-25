package engine

import (
	"encoding/json"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// dropComputed removes computed fields from a document about to be written,
// parent and child rows alike: they have no column, and a value the desk
// sends back is only what the last onLoad produced.
func (c *Ctx) dropComputed(d *meta.DocType, doc Doc) {
	for _, f := range d.Fields {
		if f.Computed {
			delete(doc, f.Fieldname)
		}
	}
	for _, tf := range d.TableFields() {
		child, err := c.St.DocType(tf.OptionsString())
		if err != nil || child == nil {
			continue
		}
		for _, row := range doc.Children(tf.Fieldname) {
			c.dropComputed(child, row)
		}
	}
}

// LoadComputed runs the controller's onLoad on a copy of the document and
// keeps only the computed fields it set, on the parent and on each child row
// (matched by id). Anything else the hook changes is discarded: onLoad fills
// values for display, it never edits the document.
func (c *Ctx) LoadComputed(doctype string, doc Doc) error {
	d, err := c.St.DocType(doctype)
	if err != nil || doc == nil || !c.hasComputed(d) {
		return err
	}
	var work Doc
	if err := json.Unmarshal(doc.JSON(), &work); err != nil {
		return err
	}
	if err := c.runHook(d, "onLoad", work, nil); err != nil {
		return err
	}
	copyComputed(d, work, doc)
	for _, tf := range d.TableFields() {
		child, err := c.St.DocType(tf.OptionsString())
		if err != nil || child == nil {
			continue
		}
		byID := map[string]Doc{}
		for _, r := range work.Children(tf.Fieldname) {
			byID[r.ID()] = r
		}
		for i, row := range doc.Children(tf.Fieldname) {
			src := byID[row.ID()]
			if src == nil {
				if rows := work.Children(tf.Fieldname); i < len(rows) {
					src = rows[i]
				}
			}
			if src != nil {
				copyComputed(child, src, row)
			}
		}
	}
	return nil
}

func copyComputed(d *meta.DocType, from, to Doc) {
	for _, f := range d.Fields {
		if f.Computed {
			if v, ok := from[f.Fieldname]; ok {
				to[f.Fieldname] = v
			}
		}
	}
}

// hasComputed reports whether the DocType or one of its child tables has a
// computed field, which is what makes running onLoad worth it.
func (c *Ctx) hasComputed(d *meta.DocType) bool {
	for _, f := range d.Fields {
		if f.Computed {
			return true
		}
	}
	for _, tf := range d.TableFields() {
		if child, err := c.St.DocType(tf.OptionsString()); err == nil && child != nil {
			for _, f := range child.Fields {
				if f.Computed {
					return true
				}
			}
		}
	}
	return false
}
