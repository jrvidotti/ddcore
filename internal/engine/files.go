package engine

import "github.com/jrvidotti/ddcore/internal/db"

// FilePermFields is what CanReadFile needs to decide. Kept next to the rule so
// a caller cannot select half of it and get a quiet "no".
var FilePermFields = []string{"owner", "attached_to_doctype", "attached_to_name"}

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
	if db.Str(f["owner"]) == c.User || c.HasRole("System Manager") {
		return true
	}
	if dt, dn := db.Str(f["attached_to_doctype"]), db.Str(f["attached_to_name"]); dt != "" && dn != "" {
		if _, err := c.GetDoc(dt, dn); err == nil {
			return true
		}
	}
	return false
}
