package engine

import (
	"context"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
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
		// a file held by a restricted field is that field's value (SEC-02)
		return c.CanReadAttachmentField(dt, db.Str(f["attached_to_field"]))
	}
	if db.Str(f["owner"]) == c.User || c.HasRole("System Manager") {
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
