package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// UserPerm represents an active scope restriction for a user.
type UserPerm struct {
	ID            string
	User          string
	Allow         string
	ForValue      string
	ApplicableFor string
	IsDefault     bool
}

// UserPermissions returns the active scope restrictions for the current user.
// Admin and operations that ignore permissions have no restrictions.
func (c *Ctx) UserPermissions() ([]UserPerm, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return nil, nil
	}
	if c.userPerms != nil {
		return c.userPerms, nil
	}
	key := "user_perms:" + c.User
	if v, ok := c.E.Cache.Get(key); ok {
		c.userPerms = v.([]UserPerm)
		return c.userPerms, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, "user", allow, for_value, applicable_for, is_default
		FROM tab_user_permission WHERE "user" = $1 ORDER BY allow, for_value`, c.User)
	if err != nil {
		return nil, err
	}
	perms := make([]UserPerm, 0, len(rows))
	for _, r := range rows {
		isDefault, _ := r["is_default"].(bool)
		perms = append(perms, UserPerm{
			ID:            db.Str(r["id"]),
			User:          db.Str(r["user"]),
			Allow:         db.Str(r["allow"]),
			ForValue:      db.Str(r["for_value"]),
			ApplicableFor: db.Str(r["applicable_for"]),
			IsDefault:     isDefault,
		})
	}
	c.userPerms = perms
	c.E.Cache.Set(key, perms, 0)
	return perms, nil
}

func (c *Ctx) invalidateUserPermissionCache(docs ...Doc) {
	users := map[string]struct{}{}
	for _, doc := range docs {
		if user := doc.Str("user"); user != "" {
			users[user] = struct{}{}
		}
	}
	if len(users) == 0 {
		return
	}
	c.AfterCommit(func() {
		for user := range users {
			c.E.Cache.Del("user_perms:" + user)
			c.E.Cache.DelPrefix("evperm:" + user + ":")
		}
	})
}

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
	if user == "Admin" {
		return []string{"Admin", "System Manager", "All"}, nil
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

// workflowStateAllowsEdit reports whether the user's roles may edit a
// document in state: an empty allowEdit leaves editing to the DocType's
// write permission.
func (c *Ctx) workflowStateAllowsEdit(state *js.WorkflowState) bool {
	return state.AllowEdit == "" || c.HasRole(state.AllowEdit)
}

// unscopedOnlyDoctypes are administered only by users without access scopes.
// A Webhook sends every document of a DocType to an outside address, and a
// Webhook Delivery's payload names its document in plain Data fields that no
// scope filter applies to, so neither can be limited to a scope. A User
// Permission is the scope itself: a scoped user who could write one could lift
// their own. The refusal is part of the scope, so ignorePermissions does not
// lift it; the framework's own writes raise the context instead, and
// UserPermissions reads the table directly. A Document Share can override a
// scope, so administering one directly is closed the same way; ddcore.share.*
// checks the sharer instead.
var unscopedOnlyDoctypes = map[string]bool{"Webhook": true, "Webhook Delivery": true, "User Permission": true, "Document Share": true}

// refusedToScopedUser reports whether doctype is closed to the current user
// because the user has access scopes.
func (c *Ctx) refusedToScopedUser(doctype string) (bool, error) {
	if !unscopedOnlyDoctypes[doctype] {
		return false, nil
	}
	perms, err := c.UserPermissions()
	return len(perms) > 0, err
}

// HasPermission decides whether the user may perform ptype on doctype/doc.
func (c *Ctx) HasPermission(doctype, ptype string, doc Doc) (bool, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return true, nil
	}
	if refused, err := c.refusedToScopedUser(doctype); err != nil || refused {
		return false, err
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
		// a row above level 0 grants fields, never the document (SEC-02)
		if p.Permlevel > 0 || !contains(roles, p.Role) || !p.Has(ptype) {
			continue
		}
		allowed = true
		if !p.IfOwner {
			ownerOnly = false
		}
	}
	if allowed && ownerOnly && doc != nil && doc.Str("owner") != c.User {
		allowed = false
	}
	if !allowed && shareable(ptype) {
		// a share stands in for the role grant (SEC-03); the workflow, the
		// controller and the scopes below still have their say
		if doc != nil {
			s, err := c.shareOn(d.Name, doc.ID())
			if err != nil {
				return false, err
			}
			allowed = s != nil && s.grants(ptype)
		} else if allowed, err = c.sharedWithDoctype(d.Name, ptype); err != nil {
			return false, err
		}
	}
	if !allowed {
		return false, nil
	}
	if ptype == "write" && doc != nil && !c.inWorkflowTransition {
		if wf := c.WorkflowFor(doctype); wf != nil {
			st := doc.Str(wf.StateField)
			if st == "" {
				st = wf.InitialState
			}
			if state := wf.GetState(st); state != nil && !c.workflowStateAllowsEdit(state) {
				return false, nil
			}
		}
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
	if doc != nil {
		ok, err := c.scopeAllows(d, doc, ptype)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// checkUserPermissions validates whether doc satisfies active scope restrictions.
func (c *Ctx) checkUserPermissions(d *meta.DocType, doc Doc) (bool, error) {
	if c.User == "Admin" || c.IgnorePermissions() || doc == nil {
		return true, nil
	}
	if refused, err := c.refusedToScopedUser(d.Name); err != nil || refused {
		return false, err
	}
	return c.checkUserPermissionsFor(d, doc, d.Name)
}

// checkUserPermissionsFor validates doc and its Table rows using the scope
// applicable to the enclosing document lifecycle.
func (c *Ctx) checkUserPermissionsFor(d *meta.DocType, doc Doc, applicableFor string) (bool, error) {
	perms, err := c.UserPermissions()
	if err != nil || len(perms) == 0 {
		return true, err
	}

	grouped := make(map[string]map[string]bool)
	for _, p := range perms {
		if p.ApplicableFor != "" && !strings.EqualFold(p.ApplicableFor, applicableFor) {
			continue
		}
		if grouped[p.Allow] == nil {
			grouped[p.Allow] = make(map[string]bool)
		}
		grouped[p.Allow][p.ForValue] = true
	}

	for allow, allowedMap := range grouped {
		if len(allowedMap) == 0 {
			continue
		}
		if strings.EqualFold(d.Name, allow) {
			name := doc.ID()
			if name == "" {
				continue
			}
			ok, err := c.withinScope(allow, name, allowedMap)
			if err != nil {
				return false, err
			}
			if !ok && d.IsTree {
				// A document being created is not in the database yet, so the
				// walk upwards starts at the parent the caller is writing. It
				// also refuses a move *out* of scope, because the parent
				// checked is the one being saved.
				ok, err = c.withinScope(allow, doc.Str(d.TreeParentField()), allowedMap)
				if err != nil {
					return false, err
				}
			}
			if !ok {
				return false, nil
			}
			continue
		}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" && strings.EqualFold(f.OptionsString(), allow) {
				val := db.Str(doc[f.Fieldname])
				ok, err := c.withinScope(allow, val, allowedMap)
				if err != nil {
					return false, err
				}
				if val == "" || !ok {
					return false, nil
				}
			}
			if f.Fieldtype == "Dynamic Link" && d.Field(f.OptionsString()) != nil {
				// A Dynamic Link is restricted only when its selector points at the
				// allowed DocType, mirroring scopeFilters' IfField/IfValue semantics.
				if strings.EqualFold(db.Str(doc[f.OptionsString()]), allow) {
					val := db.Str(doc[f.Fieldname])
					ok, err := c.withinScope(allow, val, allowedMap)
					if err != nil {
						return false, err
					}
					if val == "" || !ok {
						return false, nil
					}
				}
			}
		}
	}
	for _, tf := range d.TableFields() {
		child, err := c.St.DocType(tf.OptionsString())
		if err != nil {
			return false, err
		}
		for _, row := range doc.Children(tf.Fieldname) {
			ok, err := c.checkUserPermissionsFor(child, row, applicableFor)
			if err != nil || !ok {
				return false, err
			}
		}
	}
	return true, nil
}

// withinScope answers whether `value` is covered by the allowed values of a
// User Permission rule. For an ordinary DocType that is exact membership; for a
// tree it also holds when one of the value's ancestors is allowed, which is
// what makes a rule over a hierarchy cover the branch below it (DAT-07).
//
// The answer is memoised per request: a list of a hundred rows in three
// territories asks about three values, not a hundred.
func (c *Ctx) withinScope(allow, value string, allowed map[string]bool) (bool, error) {
	if value == "" {
		return false, nil
	}
	if allowed[value] {
		return true, nil
	}
	ad, ok := c.St.Meta.Get(allow)
	if !ok || ad == nil || !ad.IsTree {
		return false, nil
	}
	key := allow + "\x00" + value
	if c.scopeAncestors == nil {
		c.scopeAncestors = map[string][]string{}
	}
	above, seen := c.scopeAncestors[key]
	if !seen {
		var err error
		if above, err = c.treeAncestorsInclusive(ad, value); err != nil {
			return false, err
		}
		c.scopeAncestors[key] = above
	}
	for _, a := range above {
		if allowed[a] {
			return true, nil
		}
	}
	return false, nil
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
		if p.Permlevel == 0 && contains(roles, p.Role) && (p.Read || p.Report) && !p.IfOwner {
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

// scopeFilters builds filters enforcing User Permission rules for doctype d,
// lifted for the documents a share overrides them on.
func (c *Ctx) scopeFilters(d *meta.DocType) ([]db.Filter, error) {
	strict, err := c.strictScopeFilters(d)
	if err != nil || len(strict) == 0 || d.IsChild {
		return strict, err
	}
	_, override, err := c.sharedNames(d.Name)
	if err != nil || len(override) == 0 {
		return strict, err
	}
	return []db.Filter{{Any: [][]db.Filter{strict, {{Field: "id", Op: "in", Value: override}}}}}, nil
}

// strictScopeFilters builds filters enforcing User Permission rules for doctype d.
func (c *Ctx) strictScopeFilters(d *meta.DocType) ([]db.Filter, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return nil, nil
	}
	perms, err := c.UserPermissions()
	if err != nil || len(perms) == 0 {
		return nil, err
	}
	if unscopedOnlyDoctypes[d.Name] {
		// An empty IN renders as FALSE: no row is in scope.
		return []db.Filter{{Field: "id", Op: "in", Value: []any{}}}, nil
	}

	grouped := make(map[string][]any)
	for _, p := range perms {
		if p.ApplicableFor != "" && !strings.EqualFold(p.ApplicableFor, d.Name) {
			continue
		}
		grouped[p.Allow] = append(grouped[p.Allow], p.ForValue)
	}

	var out []db.Filter
	for allow, allowedValues := range grouped {
		if len(allowedValues) == 0 {
			continue
		}
		// A rule over a hierarchy covers what hangs below the value it names:
		// "Territory: Brazil" is about Brazil and everything in it, which is
		// the only reading that makes a tree usable as a scope (DAT-07).
		op, tree := "in", (*db.TreeRef)(nil)
		if ad, ok := c.St.Meta.Get(allow); ok && ad != nil && ad.IsTree {
			op = "descendants of (inclusive)"
			tree = &db.TreeRef{Table: ad.TableName(), ParentCol: ad.TreeParentField()}
		}
		if strings.EqualFold(d.Name, allow) {
			out = append(out, db.Filter{Field: "id", Op: op, Value: allowedValues, Tree: tree})
			continue
		}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" && strings.EqualFold(f.OptionsString(), allow) {
				// An IN filter intentionally excludes null and empty field values.
				out = append(out, db.Filter{Field: f.Fieldname, Op: op, Value: allowedValues, Tree: tree})
			}
			if f.Fieldtype == "Dynamic Link" && d.Field(f.OptionsString()) != nil {
				// A Dynamic Link is restricted only when its selector points at the
				// allowed DocType. Other selector values remain independently scoped.
				out = append(out, db.Filter{
					Field: f.Fieldname, Op: op, Value: allowedValues, Tree: tree,
					IfField: f.OptionsString(), IfValue: allow,
				})
			}
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
		if doc == nil || doc.ID() == "" {
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
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf(`SELECT parenttype, parent, parentfield FROM %s WHERE id = $1`, db.Ident(d.TableName())), doc.ID())
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
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf(`SELECT * FROM %s WHERE id = $1`, db.Ident(parent.TableName())), pn)
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

// permissionFilters adds child, ifOwner, controller permissionQuery, and
// User Permission scope filters, then ORs in the documents shared with the
// user (SEC-03).
func (c *Ctx) permissionFilters(d *meta.DocType) ([]db.Filter, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
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
		sf, err := c.strictScopeFilters(d)
		if err != nil {
			return nil, err
		}
		return append([]db.Filter{{Field: "parenttype", Op: "in", Value: vals}}, sf...), nil
	}
	roles, err := c.Roles()
	if err != nil {
		return nil, err
	}
	roleRead, ownerOnly := false, true
	for _, p := range d.Permissions {
		if p.Permlevel == 0 && contains(roles, p.Role) && (p.Read || p.Report) {
			roleRead = true
			if !p.IfOwner {
				ownerOnly = false
			}
		}
	}
	var query []db.Filter
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
			if query, err = db.ParseFilters(v); err != nil {
				return nil, err
			}
		}
	}
	strict, err := c.strictScopeFilters(d)
	if err != nil {
		return nil, err
	}
	var byRole []db.Filter
	if ownerOnly {
		byRole = append(byRole, db.Filter{Field: "owner", Op: "=", Value: c.User})
	}
	byRole = append(append(byRole, query...), strict...)

	scoped, override, err := c.sharedNames(d.Name)
	if err != nil {
		return nil, err
	}
	if len(scoped) == 0 && len(override) == 0 {
		return byRole, nil
	}
	// a share answers to the controller's permissionQuery as a role grant does,
	// and to the scopes unless it overrides them
	var groups [][]db.Filter
	if roleRead {
		groups = append(groups, byRole)
	}
	if len(scoped) > 0 {
		g := append([]db.Filter{{Field: "id", Op: "in", Value: scoped}}, query...)
		groups = append(groups, append(g, strict...))
	}
	if len(override) > 0 {
		groups = append(groups, append([]db.Filter{{Field: "id", Op: "in", Value: override}}, query...))
	}
	return []db.Filter{{Any: groups}}, nil
}

// Permissions summarises what the user can do with a doctype (for the desk).
func (c *Ctx) Permissions(d *meta.DocType) map[string]bool {
	out := map[string]bool{}
	for _, p := range []string{"read", "write", "create", "delete", "submit", "cancel", "amend", "report", "export", "share"} {
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
