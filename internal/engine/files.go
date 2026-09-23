package engine

import (
	"context"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// FilePermFields is what CanReadFile needs to decide. Kept next to the rule so
// a caller cannot select half of it and get a quiet "no".
var FilePermFields = []string{"owner", "attached_to_doctype", "attached_to_id", "attached_to_field"}

// CanReadFile decides who may read one File row.
//
// File is owner-only for an ordinary user, which on its own would hide a
// colleague's attachment on a document the reader is perfectly entitled to.
// So the question is delegated: read permission on the document a file is
// attached to is read permission on the file. A detached file has no document
// to ask, and stays with its owner and System Manager.
//
// This is the rule /private/files has always applied inline; it is a function
// because mailing an attachment has to ask exactly the same question, and two
// copies of an authorization rule is one copy too many. (exportFiles asks it a
// third way on purpose: it has already checked the page's documents, so it
// scopes the lookup instead of re-deciding per file.)
func (c *Ctx) CanReadFile(f map[string]any) bool {
	if f == nil {
		return false
	}
	if c.User == "Admin" || c.IgnorePermissions() {
		return true
	}
	if dt, dn := db.Str(f["attached_to_doctype"]), db.Str(f["attached_to_id"]); dt != "" && dn != "" {
		if _, err := c.GetDoc(dt, dn); err != nil {
			return false
		}
		// a portal reaches a document through the fields its pages show, and
		// only those: a file on a field no page shows stays with the desk
		if c.PortalMode() && !c.portalShowsField(dt, db.Str(f["attached_to_field"])) {
			return false
		}
		// a file held by a restricted field is that field's value (SEC-02)
		return c.CanReadAttachmentField(dt, db.Str(f["attached_to_field"]))
	}
	if db.Str(f["owner"]) == c.User || (!c.PortalMode() && c.HasRole("System Manager")) {
		return true
	}
	return false
}

// CanReadAttachmentField reports whether the user may read the field a file is
// attached through. No field, or one the DocType does not declare, is the
// document's own attachment and follows the document.
func (c *Ctx) CanReadAttachmentField(doctype, field string) bool {
	if field == "" {
		return true
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false
	}
	return c.FieldAccess(d).CanRead(d.Field(field))
}

// AttachmentFieldRestricted reports whether a file uploaded to doctype.field
// sits above permission level 0, and whether the user may write that field.
func (c *Ctx) AttachmentFieldRestricted(doctype, field string) (restricted, canWrite bool) {
	if doctype == "" || field == "" {
		return false, true
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, true
	}
	f := d.Field(field)
	if f == nil || f.Permlevel == 0 {
		return false, true
	}
	return true, c.FieldAccess(d).CanWrite(f)
}

// AttachmentFieldWantsImage reports whether the field an upload names is an
// Attach Image, along with its label for the message. The check repeats what
// the field's own validation does, so bytes are never stored for a field that
// would refuse their URL a moment later.
func (c *Ctx) AttachmentFieldWantsImage(doctype, field string) (bool, string) {
	if doctype == "" || field == "" {
		return false, ""
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, ""
	}
	f := d.Field(field)
	if f == nil || f.Fieldtype != "Attach Image" {
		return false, ""
	}
	return true, f.Label
}

// claimAttachments attaches the files a save names that are still detached.
//
// A file picked on a form that has not been saved yet is uploaded before the
// document has an id, so it is stored detached — readable only by whoever
// uploaded it. Once the document is written, each Attach value that points at
// such a file is attached to it, and from then on the file follows the
// document's read permission like any other attachment.
//
// Only files the saving user uploaded are claimed: naming someone else's
// detached file in a field must not be a way to take it over.
func (c *Ctx) claimAttachments(d *meta.DocType, doc Doc) error {
	id := doc.ID()
	if id == "" {
		return nil
	}
	type claim struct{ url, field string }
	var claims []claim
	for _, f := range d.Fields {
		switch f.Fieldtype {
		case "Attach", "Attach Image":
			if u := doc.Str(f.Fieldname); u != "" {
				claims = append(claims, claim{u, f.Fieldname})
			}
		case "Table":
			child, err := c.St.DocType(f.OptionsString())
			if err != nil {
				continue
			}
			for _, row := range doc.Children(f.Fieldname) {
				for _, cf := range child.Fields {
					if cf.Fieldtype != "Attach" && cf.Fieldtype != "Attach Image" {
						continue
					}
					if u := row.Str(cf.Fieldname); u != "" {
						// a row's file answers to the Table field that holds it
						claims = append(claims, claim{u, f.Fieldname})
					}
				}
			}
		}
	}
	for _, cl := range claims {
		if !strings.HasPrefix(cl.url, "/files/") && !strings.HasPrefix(cl.url, "/private/files/") {
			continue
		}
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_file SET attached_to_doctype = $1, attached_to_id = $2, attached_to_field = $3
			WHERE file_url = $4 AND owner = $5 AND COALESCE(attached_to_id, '') = ''
			AND (COALESCE(attached_to_doctype, '') = '' OR attached_to_doctype = $1)`,
			d.Name, id, cl.field, cl.url, c.User); err != nil {
			return err
		}
	}
	return nil
}

// deleteFileBytesAfterCommit removes the stored bytes of deleted File rows once
// the deletion is committed: a rolled-back delete must still find its bytes.
// It is best effort — the rows are already gone, so a storage failure is
// logged as an orphan rather than turned into an error nobody can act on.
func (c *Ctx) deleteFileBytesAfterCommit(fileURLs ...string) {
	if len(fileURLs) == 0 {
		return
	}
	store := c.E.Storage()
	c.AfterCommit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, u := range fileURLs {
			key, ok := storage.KeyFromURL(u)
			if !ok {
				continue
			}
			if err := store.Delete(ctx, key); err != nil {
				c.E.Log.Warn("file bytes left behind after delete", "file_url", u, "backend", store.Backend(), "err", err)
			}
		}
	})
}
