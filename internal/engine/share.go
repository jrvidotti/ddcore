package engine

import (
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// shareDoctype holds one user's grant on one document (SEC-03).
const shareDoctype = "Document Share"

// DocShare is one user's access to one document. Read is implied by Write and
// Share; OverrideScope lets the share reach the user through their User
// Permission scopes, for exactly the rights the share grants.
type DocShare struct {
	ID            string `json:"id"`
	User          string `json:"user"`
	Doctype       string `json:"share_doctype"`
	DocID         string `json:"share_id"`
	Read          bool   `json:"read"`
	Write         bool   `json:"write"`
	Share         bool   `json:"share"`
	OverrideScope bool   `json:"override_scope"`
	Owner         string `json:"owner"`
}

// ShareRights is what a sharer asks to grant.
type ShareRights struct {
	Read          bool `json:"read"`
	Write         bool `json:"write"`
	Share         bool `json:"share"`
	OverrideScope bool `json:"overrideScope"`
}

// DocShares answers the form's sharing panel.
type DocShares struct {
	Shares           []DocShare `json:"shares"`
	CanShare         bool       `json:"canShare"`
	CanOverrideScope bool       `json:"canOverrideScope"`
}

// grants reports whether s gives ptype on its document.
func (s *DocShare) grants(ptype string) bool {
	switch ptype {
	case "read":
		return s.Read || s.Write || s.Share
	case "write":
		return s.Write
	case "share":
		return s.Share
	}
	return false
}

// shareable reports whether a share can ever answer ptype.
func shareable(ptype string) bool {
	return ptype == "read" || ptype == "write" || ptype == "share"
}

func (c *Ctx) loadShares(user string) ([]DocShare, error) {
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, "user", share_doctype, share_id, "read", "write", "share", override_scope, owner
		FROM tab_document_share WHERE "user" = $1 ORDER BY share_doctype, share_id`, user)
	if err != nil {
		return nil, err
	}
	out := make([]DocShare, 0, len(rows))
	for _, r := range rows {
		out = append(out, shareFromRow(r))
	}
	return out, nil
}

func shareFromRow(r map[string]any) DocShare {
	return DocShare{
		ID: db.Str(r["id"]), User: db.Str(r["user"]),
		Doctype: db.Str(r["share_doctype"]), DocID: db.Str(r["share_id"]),
		Read: r["read"] == true, Write: r["write"] == true, Share: r["share"] == true,
		OverrideScope: r["override_scope"] == true, Owner: db.Str(r["owner"]),
	}
}

// Shares returns the documents shared with the current user. Admin and
// elevated contexts need none. The process cache is bypassed once this ctx has
// written a share, so its own transaction sees the change before commit.
func (c *Ctx) Shares() ([]DocShare, error) {
	if c.User == "Admin" || c.User == "Guest" || c.User == "" || c.IgnorePermissions() {
		return nil, nil
	}
	if c.sharesLoaded {
		return c.shares, nil
	}
	key := "shares:" + c.User
	if !c.sharesDirty {
		if v, ok := c.E.Cache.Get(key); ok {
			c.shares, c.sharesLoaded = v.([]DocShare), true
			return c.shares, nil
		}
	}
	shares, err := c.loadShares(c.User)
	if err != nil {
		return nil, err
	}
	c.shares, c.sharesLoaded = shares, true
	if !c.sharesDirty {
		c.E.Cache.Set(key, shares, 0)
	}
	return shares, nil
}

// shareOn returns the current user's share on one document, if any.
func (c *Ctx) shareOn(doctype, name string) (*DocShare, error) {
	if name == "" {
		return nil, nil
	}
	shares, err := c.Shares()
	if err != nil {
		return nil, err
	}
	for i := range shares {
		if shares[i].DocID == name && strings.EqualFold(shares[i].Doctype, doctype) {
			return &shares[i], nil
		}
	}
	return nil, nil
}

// sharedWithDoctype reports whether any share gives ptype on a doctype's
// documents: the doctype-level answer a listing or the Desk asks for.
func (c *Ctx) sharedWithDoctype(doctype, ptype string) (bool, error) {
	shares, err := c.Shares()
	if err != nil {
		return false, err
	}
	for i := range shares {
		if strings.EqualFold(shares[i].Doctype, doctype) && shares[i].grants(ptype) {
			return true, nil
		}
	}
	return false, nil
}

// sharedNames splits the readable shares on a doctype into those still bound
// by the user's scopes and those that override them.
func (c *Ctx) sharedNames(doctype string) (scoped, override []any, err error) {
	shares, err := c.Shares()
	if err != nil {
		return nil, nil, err
	}
	for i := range shares {
		s := &shares[i]
		if !strings.EqualFold(s.Doctype, doctype) || !s.grants("read") {
			continue
		}
		if s.OverrideScope {
			override = append(override, s.DocID)
		} else {
			scoped = append(scoped, s.DocID)
		}
	}
	return scoped, override, nil
}

// scopeOverridden reports whether a share lifts the user's scopes for ptype on
// one document. A doctype closed to scoped users stays closed.
func (c *Ctx) scopeOverridden(doctype, name, ptype string) (bool, error) {
	if !shareable(ptype) || unscopedOnlyDoctypes[doctype] {
		return false, nil
	}
	s, err := c.shareOn(doctype, name)
	if err != nil || s == nil {
		return false, err
	}
	return s.OverrideScope && s.grants(ptype), nil
}

// scopeAllows is the scope check of a read or a write: the user's scopes, or a
// share that overrides them for that right.
func (c *Ctx) scopeAllows(d *meta.DocType, doc Doc, ptype string) (bool, error) {
	ok, err := c.checkUserPermissions(d, doc)
	if err != nil || ok {
		return ok, err
	}
	return c.scopeOverridden(d.Name, doc.ID(), ptype)
}

// sharesChanged drops the cached shares of users, now for this ctx and after
// commit for everyone else. The event authorizer caches reads too.
func (c *Ctx) sharesChanged(users ...string) {
	for _, u := range users {
		if u == c.User {
			c.sharesLoaded, c.sharesDirty = false, true
		}
	}
	c.AfterCommit(func() {
		for _, u := range users {
			if u == "" {
				continue
			}
			c.E.Cache.Del("shares:" + u)
			c.E.Cache.DelPrefix("evperm:" + u + ":")
		}
	})
}

// allSharesChanged is sharesChanged for a rename or a deletion, which moves or
// removes the shares of whoever held one.
func (c *Ctx) allSharesChanged() {
	c.sharesLoaded, c.sharesDirty = false, true
	c.AfterCommit(func() {
		c.E.Cache.DelPrefix("shares:")
		c.E.Cache.DelPrefix("evperm:")
	})
}

// CanOverrideScope reports whether the current user may give a share that
// overrides the recipient's scopes: an admin who is not scoped
// themselves, since the scope is theirs to lift.
func (c *Ctx) CanOverrideScope() (bool, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return true, nil
	}
	perms, err := c.UserPermissions()
	if err != nil || len(perms) > 0 {
		return false, err
	}
	return c.HasRole("System Manager"), nil
}

// shareTarget loads a document the current user can read and that sharing
// applies to.
func (c *Ctx) shareTarget(doctype, name string) (*meta.DocType, Doc, error) {
	if doctype == "" || name == "" {
		return nil, nil, cerr.Validation("doctype and id are required")
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, nil, err
	}
	if d.IsChild || d.IsSingle || unscopedOnlyDoctypes[d.Name] || d.Name == shareDoctype || d.Name == "Audit Event" {
		return nil, nil, cerr.Validation("{0} documents cannot be shared", c.T(d.Label))
	}
	doc, err := c.GetDoc(d.Name, name)
	if err != nil {
		return nil, nil, err
	}
	return d, doc, nil
}

func (c *Ctx) shareRow(user, doctype, name string) (*DocShare, error) {
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, "user", share_doctype, share_id, "read", "write", "share", override_scope, owner
		FROM tab_document_share WHERE "user" = $1 AND share_doctype = $2 AND share_id = $3`, user, doctype, name)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	s := shareFromRow(rows[0])
	return &s, nil
}

func shareDetail(user string, r ShareRights) map[string]any {
	return map[string]any{"user": user, "read": r.Read, "write": r.Write, "share": r.Share, "override_scope": r.OverrideScope}
}

// ShareDoc grants user access to one document, or changes an existing grant.
// The sharer needs the share right on the document, and may give write only
// while holding it; overriding a scope is for an unscoped System Manager.
func (c *Ctx) ShareDoc(doctype, name, user string, r ShareRights) (DocShare, error) {
	var out DocShare
	if c.User == "Guest" || c.User == "" {
		return out, cerr.Auth("Sign in to continue")
	}
	d, doc, err := c.shareTarget(doctype, name)
	if err != nil {
		return out, err
	}
	user = strings.TrimSpace(user)
	r.Read = true
	detail := shareDetail(user, r)
	if ok, err := c.HasPermission(d.Name, "share", doc); err != nil {
		return out, err
	} else if !ok {
		c.AuditDenied("permission.share_grant", d.Name, name, detail)
		return out, cerr.Permission("No permission ({0}) on {1} {2}", "share", c.T(d.Label), name)
	}
	switch {
	case user == "":
		return out, cerr.Validation("user is required")
	case user == c.User:
		return out, cerr.Validation("You cannot share a document with yourself")
	case user == "Admin" || user == "Guest":
		return out, cerr.Validation("A document cannot be shared with {0}", user)
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT enabled FROM tab_user WHERE id = $1`, user)
	if err != nil {
		return out, err
	}
	if len(rows) == 0 || rows[0]["enabled"] != true {
		return out, cerr.Validation("User {0} does not exist or is disabled", user)
	}
	if r.Write {
		if ok, err := c.HasPermission(d.Name, "write", doc); err != nil {
			return out, err
		} else if !ok {
			c.AuditDenied("permission.share_grant", d.Name, name, detail)
			return out, cerr.Permission("You cannot grant {0} on {1} {2} without holding it", "write", c.T(d.Label), name)
		}
	}
	if r.OverrideScope {
		if ok, err := c.CanOverrideScope(); err != nil {
			return out, err
		} else if !ok {
			c.AuditDenied("permission.share_grant", d.Name, name, detail)
			return out, cerr.Permission("Only a System Manager without access scopes can override a security scope")
		}
	}
	existing, err := c.shareRow(user, d.Name, name)
	if err != nil {
		return out, err
	}
	values := Doc{"user": user, "share_doctype": d.Name, "share_id": name,
		"read": r.Read, "write": r.Write, "share": r.Share, "override_scope": r.OverrideScope}
	var saved Doc
	err = c.WithIgnorePermissions(func() error {
		var row Doc
		if existing != nil {
			row, err = c.GetDoc(shareDoctype, existing.ID)
			if err != nil {
				return err
			}
			for k, v := range values {
				row[k] = v
			}
			delete(row, "modified")
			saved, err = c.Save(row, SaveOpts{})
			return err
		}
		row, err = c.NewDoc(shareDoctype, values)
		if err != nil {
			return err
		}
		saved, err = c.Insert(row, SaveOpts{})
		return err
	})
	if err != nil {
		return out, err
	}
	if existing == nil {
		c.notifyShare(d, name, user)
	}
	return shareFromRow(saved), nil
}

// notifyShare tells the recipient, in their language. Best effort: a failure is
// rolled back to its savepoint and logged, and the share still commits.
func (c *Ctx) notifyShare(d *meta.DocType, name, user string) {
	lang := c.RecipientLang([]string{user})
	label := c.St.I18n.T(lang, d.Label)
	title := c.St.I18n.T(lang, "Shared with you: {0} {1}", label, name)
	msg := c.St.I18n.T(lang, "{0} shared {1} {2} with you", c.User, label, name)
	if err := c.WithSavepoint(func() error {
		return c.notifyUserAs("share", user, d.Name, name, title, msg)
	}); err != nil {
		c.E.Log.Warn("share notification failed", "doctype", d.Name, "id", name, "user", user, "err", err)
	}
}

// UnshareDoc removes user's share on one document. The recipient may drop their
// own share; anyone else needs the share right on the document.
func (c *Ctx) UnshareDoc(doctype, name, user string) error {
	if c.User == "Guest" || c.User == "" {
		return cerr.Auth("Sign in to continue")
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return err
	}
	existing, err := c.shareRow(user, d.Name, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return cerr.NotFound("{0} {1} is not shared with {2}", c.T(d.Label), name, user)
	}
	if user != c.User {
		_, doc, err := c.shareTarget(d.Name, name)
		if err != nil {
			return err
		}
		if ok, err := c.HasPermission(d.Name, "share", doc); err != nil {
			return err
		} else if !ok {
			c.AuditDenied("permission.share_revoke", d.Name, name, map[string]any{"user": user})
			return cerr.Permission("No permission ({0}) on {1} {2}", "share", c.T(d.Label), name)
		}
	}
	return c.WithIgnorePermissions(func() error {
		return c.Delete(shareDoctype, existing.ID, true, false)
	})
}

// ListDocShares lists who a document is shared with. A user who cannot share it
// sees only their own share.
func (c *Ctx) ListDocShares(doctype, name string) (DocShares, error) {
	out := DocShares{Shares: []DocShare{}}
	d, doc, err := c.shareTarget(doctype, name)
	if err != nil {
		return out, err
	}
	if out.CanShare, err = c.HasPermission(d.Name, "share", doc); err != nil {
		return out, err
	}
	if out.CanShare {
		if out.CanOverrideScope, err = c.CanOverrideScope(); err != nil {
			return out, err
		}
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, "user", share_doctype, share_id, "read", "write", "share", override_scope, owner
		FROM tab_document_share WHERE share_doctype = $1 AND share_id = $2 ORDER BY creation, id`, d.Name, name)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		s := shareFromRow(r)
		if out.CanShare || s.User == c.User {
			out.Shares = append(out.Shares, s)
		}
	}
	return out, nil
}

func shareRightsOf(doc Doc) ShareRights {
	return ShareRights{Read: doc["read"] == true, Write: doc["write"] == true, Share: doc["share"] == true, OverrideScope: doc["override_scope"] == true}
}

func shareTargetChanged(before, after Doc) bool {
	for _, f := range []string{"user", "share_doctype", "share_id"} {
		if before.Str(f) != after.Str(f) {
			return true
		}
	}
	return false
}

// auditShareSaved records a grant row written by Insert, Save or DBSet, and
// drops the cached shares it touches. before is nil for an insert.
func (c *Ctx) auditShareSaved(before, after Doc) error {
	switch {
	case before == nil:
		if err := c.Audit("permission.share_grant", after.Str("share_doctype"), after.Str("share_id"), shareDetail(after.Str("user"), shareRightsOf(after))); err != nil {
			return err
		}
	case shareTargetChanged(before, after):
		if err := c.auditShareDeleted(before); err != nil {
			return err
		}
		return c.auditShareSaved(nil, after)
	case shareRightsOf(before) != shareRightsOf(after):
		if err := c.Audit("permission.share_update", after.Str("share_doctype"), after.Str("share_id"), shareDetail(after.Str("user"), shareRightsOf(after))); err != nil {
			return err
		}
	}
	c.sharesChanged(after.Str("user"))
	return nil
}

func (c *Ctx) auditShareDeleted(doc Doc) error {
	c.sharesChanged(doc.Str("user"))
	return c.Audit("permission.share_revoke", doc.Str("share_doctype"), doc.Str("share_id"), map[string]any{"user": doc.Str("user")})
}
