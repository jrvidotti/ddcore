package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Roles returns the roles of the current user (cached per ctx).
func (c *Ctx) Roles() ([]string, error) {
	if c.roles != nil {
		return c.roles, nil
	}
	r, err := c.RolesOf(c.User)
	if err != nil {
		return nil, err
	}
	c.roles = r
	return r, nil
}

func (c *Ctx) RolesOf(user string) ([]string, error) {
	if user == "Administrator" {
		return []string{"Administrator", "System Manager", "All"}, nil
	}
	if user == "" || user == "Guest" {
		return []string{"Guest"}, nil
	}
	if v, ok := c.E.Cache.Get("roles:" + user); ok {
		return v.([]string), nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT role FROM tab_has_role WHERE parent = $1 AND parenttype = 'User'`, user)
	if err != nil {
		return nil, err
	}
	roles := []string{"All"}
	for _, r := range rows {
		roles = append(roles, db.Str(r["role"]))
	}
	c.E.Cache.Set("roles:"+user, roles, 0)
	return roles, nil
}

func (c *Ctx) HasRole(role string) bool {
	roles, _ := c.Roles()
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission decides whether the user may perform ptype on doctype/doc.
func (c *Ctx) HasPermission(doctype, ptype string, doc Doc) (bool, error) {
	if c.User == "Administrator" || c.IgnorePermissions() {
		return true, nil
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, err
	}
	if d.IsChild {
		return c.childPermission(d, ptype, doc)
	}
	roles, err := c.Roles()
	if err != nil {
		return false, err
	}
	allowed, ownerOnly := false, true
	for _, p := range d.Permissions {
		if !contains(roles, p.Role) || !p.Has(ptype) {
			continue
		}
		allowed = true
		if !p.IfOwner {
			ownerOnly = false
		}
	}
	if ptype == "read" || ptype == "write" {
		// report/export imply read for listing purposes
	}
	if !allowed && ptype == "amend" {
		return false, nil
	}
	if !allowed {
		return false, nil
	}
	if ownerOnly && doc != nil && doc.Str("owner") != c.User {
		return false, nil
	}
	if d.HasController() {
		rt, err := c.RT()
		if err != nil {
			return false, err
		}
		var dj json.RawMessage
		if doc != nil {
			dj = doc.JSON()
		}
		r, err := rt.HasPermission(doctype, dj, ptype, c.User)
		if err != nil {
			return false, err
		}
		if r == "false" {
			return false, nil
		}
	}
	return true, nil
}

// childParent describes one Table field that embeds a child doctype.
type childParent struct {
	Parent *meta.DocType
	Field  *meta.Field
}

// ParentsOf lists the (doctype, Table field) pairs that embed `child`.
func (c *Ctx) ParentsOf(child string) []childParent {
	var out []childParent
	for _, d := range c.E.Meta.DocTypes {
		if d.IsChild {
			continue
		}
		for _, tf := range d.TableFields() {
			if strings.EqualFold(tf.OptionsString(), child) {
				out = append(out, childParent{Parent: d, Field: tf})
			}
		}
	}
	return out
}

// readIsOwnerOnly reports whether the user's read access to d is limited to
// the documents he owns (every matching permission has ifOwner).
func (c *Ctx) readIsOwnerOnly(d *meta.DocType) bool {
	roles, err := c.Roles()
	if err != nil {
		return true
	}
	for _, p := range d.Permissions {
		if contains(roles, p.Role) && (p.Read || p.Report) && !p.IfOwner {
			return false
		}
	}
	return true
}

// ReadableParentsOf returns the names of the doctypes embedding `child` that
// the user may read in full — the whitelist used to scope a child listing
// (B02). A doctype the user only reads as owner is left out: a row filter on
// the child table cannot express the parent's ownership, so listing rows of
// such a doctype is refused instead of leaked.
func (c *Ctx) ReadableParentsOf(child string) ([]string, error) {
	var out []string
	for _, cp := range c.ParentsOf(child) {
		if c.readIsOwnerOnly(cp.Parent) {
			continue
		}
		ok, err := c.HasPermission(cp.Parent.Name, "read", nil)
		if err != nil {
			return nil, err
		}
		if ok && !contains(out, cp.Parent.Name) {
			out = append(out, cp.Parent.Name)
		}
	}
	return out, nil
}

// childPermission resolves a child row's permission through its parent
// document: read follows the parent's read, every mutation follows the
// parent's write and the parent's docstatus (B02). A child row is never
// authorised on its own.
func (c *Ctx) childPermission(d *meta.DocType, ptype string, doc Doc) (bool, error) {
	var parentPtype string
	switch ptype {
	case "read", "report", "export":
		parentPtype = "read"
	case "write", "create", "delete":
		parentPtype = "write"
	default: // submit/cancel/amend never apply to a row on its own
		return false, nil
	}
	pt, pn, pf := doc.Str("parenttype"), doc.Str("parent"), doc.Str("parentfield")
	if pt == "" || pn == "" || pf == "" {
		if doc == nil || doc.Name() == "" {
			// doctype-level check (listing, meta): allow when at least one
			// embedding doctype is allowed; the listing is filtered by
			// permissionFilters.
			if parentPtype == "read" {
				parents, err := c.ReadableParentsOf(d.Name)
				return len(parents) > 0, err
			}
			for _, cp := range c.ParentsOf(d.Name) {
				if c.readIsOwnerOnly(cp.Parent) {
					continue
				}
				ok, err := c.HasPermission(cp.Parent.Name, parentPtype, nil)
				if err != nil {
					return false, err
				}
				if ok {
					return true, nil
				}
			}
			return false, nil
		}
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf(`SELECT parenttype, parent, parentfield FROM %s WHERE name = $1`, db.Ident(d.TableName())), doc.Name())
		if err != nil || len(rows) == 0 {
			return false, err
		}
		pt, pn, pf = db.Str(rows[0]["parenttype"]), db.Str(rows[0]["parent"]), db.Str(rows[0]["parentfield"])
		if pt == "" || pn == "" || pf == "" {
			return false, nil
		}
	}
	parent, ok := c.E.Meta.Get(pt)
	if !ok || parent.IsChild {
		return false, nil
	}
	f := parent.Field(pf)
	if f == nil || f.Fieldtype != "Table" || !strings.EqualFold(f.OptionsString(), d.Name) {
		return false, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf(`SELECT * FROM %s WHERE name = $1`, db.Ident(parent.TableName())), pn)
	if err != nil || len(rows) == 0 {
		return false, err
	}
	pdoc := Doc(rows[0])
	pdoc["doctype"] = parent.Name
	if parentPtype == "write" {
		switch pdoc.Docstatus() {
		case 1:
			if !f.AllowOnSubmit {
				return false, nil
			}
		case 2:
			return false, nil
		}
	}
	return c.HasPermission(parent.Name, parentPtype, pdoc)
}

// permissionFilters adds ifOwner and controller permissionQuery filters.
func (c *Ctx) permissionFilters(d *meta.DocType) ([]db.Filter, error) {
	if c.User == "Administrator" {
		return nil, nil
	}
	if d.IsChild {
		// child rows are only visible through the doctypes that embed them
		parents, err := c.ReadableParentsOf(d.Name)
		if err != nil {
			return nil, err
		}
		vals := make([]any, 0, len(parents))
		for _, p := range parents {
			vals = append(vals, p)
		}
		return []db.Filter{{Field: "parenttype", Op: "in", Value: vals}}, nil
	}
	roles, err := c.Roles()
	if err != nil {
		return nil, err
	}
	ownerOnly := true
	for _, p := range d.Permissions {
		if contains(roles, p.Role) && (p.Read || p.Report) && !p.IfOwner {
			ownerOnly = false
		}
	}
	var out []db.Filter
	if ownerOnly {
		out = append(out, db.Filter{Field: "owner", Op: "=", Value: c.User})
	}
	if d.HasController() {
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		raw, err := rt.PermissionQuery(d.Name, c.User)
		if err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			var v any
			json.Unmarshal(raw, &v)
			f, err := db.ParseFilters(v)
			if err != nil {
				return nil, err
			}
			out = append(out, f...)
		}
	}
	return out, nil
}

// Permissions summarises what the user can do with a doctype (for the desk).
func (c *Ctx) Permissions(d *meta.DocType) map[string]bool {
	out := map[string]bool{}
	for _, p := range []string{"read", "write", "create", "delete", "submit", "cancel", "amend", "report", "export"} {
		ok, _ := c.HasPermission(d.Name, p, nil)
		out[p] = ok
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
