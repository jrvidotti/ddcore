package engine

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Portals (OPS-10).
//
// A portal is a declared, self-service view of a few DocTypes for people who
// are not desk users. Its declaration is the whole of what such a person may
// reach: a Website User holds no role grant anywhere, so every read and write
// they make is decided here, inside HasPermission and permissionFilters, and
// the portal's HTTP surface is never the only guard.
//
// "Their own records" is expressed with an identity and a match. The identity
// is the row that stands for the signed-in person — an Employee whose `user`
// is them, say — and each page matches its DocType's fields against that row:
// `{ employee: "id" }` means "documents whose employee is my Employee". The
// identity is read from live data on every request, so linking or unlinking a
// user takes effect at once and nothing has to be kept in sync.

// Portal is a declared portal, as Go sees it.
type Portal struct {
	Name       string         `json:"name"`
	Title      string         `json:"title"`
	App        string         `json:"app"`
	SourceFile string         `json:"sourceFile"`
	Roles      []string       `json:"roles"`
	Identity   PortalIdentity `json:"identity"`
	Pages      []PortalPage   `json:"pages"`
}

// PortalIdentity is how the signed-in user's own record is found.
type PortalIdentity struct {
	Doctype   string         `json:"doctype"`
	UserField string         `json:"userField"`
	Filters   map[string]any `json:"filters,omitempty"`
}

// PortalPage is one screen of a portal, over one DocType.
type PortalPage struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Doctype     string            `json:"doctype"`
	Kind        string            `json:"kind"`
	Match       map[string]string `json:"match"`
	Fields      []string          `json:"fields"`
	ListFields  []string          `json:"listFields,omitempty"`
	Editable    []string          `json:"editable,omitempty"`
	Create      bool              `json:"create,omitempty"`
	Write       bool              `json:"write,omitempty"`
	// DefaultsMethod names a `portal: true` whitelisted method whose result
	// prefills a new document. It is a convenience: the values still pass
	// through the editable list and the document's own validation.
	DefaultsMethod string         `json:"defaultsMethod,omitempty"`
	Actions        []PortalAction `json:"actions,omitempty"`
}

// PortalAction is a link from one page to another page of the same portal.
type PortalAction struct {
	Label string `json:"label"`
	Page  string `json:"page"`
	// New opens the target page's creation form instead of its list.
	New bool `json:"new,omitempty"`
}

const (
	PortalKindList   = "list"
	PortalKindRecord = "record"
)

var portalNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// portalFieldTypesRefused are the fieldtypes a page may not show: a Table has
// no portal editor yet, and a secret never leaves the server.
var portalFieldTypesRefused = map[string]bool{"Table": true, "Table MultiSelect": true, "Password": true, "Vault": true}

// Page returns the page called name, or nil.
func (p *Portal) Page(name string) *PortalPage {
	for i := range p.Pages {
		if p.Pages[i].Name == name {
			return &p.Pages[i]
		}
	}
	return nil
}

// Slug is the portal's name as it appears in a URL.
func (p *Portal) Slug() string { return strings.ReplaceAll(meta.Snake(p.Name), "_", "-") }

// EditableField reports whether the page lets its user type into field.
func (pg *PortalPage) EditableField(field string) bool {
	for _, f := range pg.Editable {
		if f == field {
			return true
		}
	}
	return false
}

// ShowsField reports whether the page shows field.
func (pg *PortalPage) ShowsField(field string) bool {
	for _, f := range pg.Fields {
		if f == field {
			return true
		}
	}
	return false
}

// grants reports whether the page carries ptype.
func (pg *PortalPage) grants(ptype string) bool {
	switch ptype {
	case "read", "report":
		return true
	case "create":
		return pg.Create
	case "write":
		return pg.Write
	}
	// delete, submit, cancel, amend, export, import and share are never a
	// portal's to give
	return false
}

// ValidateTarget checks the portal against the loaded meta. A portal that
// names a field that does not exist would otherwise grant, or hide, something
// nobody meant it to, so the load refuses it instead.
func (p *Portal) ValidateTarget(reg *meta.Registry, whitelisted map[string]map[string]any) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("portal %q: %s", p.Name, fmt.Sprintf(format, args...))
	}
	if !portalNameRe.MatchString(p.Slug()) {
		return fail("name must be letters, digits and spaces")
	}
	if len(p.Roles) == 0 {
		return fail("roles is required: name the roles that reach this portal")
	}
	id, ok := reg.Get(p.Identity.Doctype)
	if !ok {
		return fail("identity DocType %q does not exist", p.Identity.Doctype)
	}
	if id.IsChild || id.IsSingle || id.IsVirtual() {
		return fail("identity DocType %q cannot be a child table, a Single or a virtual DocType", id.Name)
	}
	if id.Name == "User" {
		if p.Identity.UserField != "id" {
			return fail("an identity over User uses userField \"id\"")
		}
	} else {
		f := id.Field(p.Identity.UserField)
		if f == nil || f.Fieldtype != "Link" || f.OptionsString() != "User" {
			return fail("identity userField %q must be a Link to User on %s", p.Identity.UserField, id.Name)
		}
	}
	for k := range p.Identity.Filters {
		if !id.HasColumn(k) {
			return fail("identity filter %q is not a field of %s", k, id.Name)
		}
	}
	if len(p.Pages) == 0 {
		return fail("pages is required")
	}
	seen := map[string]bool{}
	for i := range p.Pages {
		pg := &p.Pages[i]
		pfail := func(format string, args ...any) error {
			return fail("page %q: %s", pg.Name, fmt.Sprintf(format, args...))
		}
		if !portalNameRe.MatchString(pg.Name) {
			return pfail("name must be lowercase letters, digits and dashes")
		}
		if seen[pg.Name] {
			return pfail("defined twice")
		}
		seen[pg.Name] = true
		if pg.Label == "" {
			return pfail("label is required")
		}
		switch pg.Kind {
		case "":
			pg.Kind = PortalKindList
		case PortalKindList, PortalKindRecord:
		default:
			return pfail("kind must be \"list\" or \"record\"")
		}
		d, ok := reg.Get(pg.Doctype)
		if !ok {
			return pfail("DocType %q does not exist", pg.Doctype)
		}
		if d.IsChild || d.IsSingle || d.IsVirtual() {
			return pfail("DocType %q cannot be a child table, a Single or a virtual DocType", d.Name)
		}
		if len(pg.Match) == 0 {
			return pfail("match is required: it is what makes a record the user's own")
		}
		for k, v := range pg.Match {
			if !d.HasColumn(k) {
				return pfail("match field %q is not a field of %s", k, d.Name)
			}
			if !id.HasColumn(v) {
				return pfail("match value %q is not a field of %s", v, id.Name)
			}
			if pg.Create && k == "id" {
				return pfail("a page that creates cannot match on id")
			}
		}
		if pg.Kind == PortalKindRecord && pg.Create {
			return pfail("a record page cannot create")
		}
		if len(pg.Fields) == 0 {
			return pfail("fields is required")
		}
		for _, name := range pg.Fields {
			f := d.Field(name)
			if f == nil || meta.LayoutTypes[f.Fieldtype] {
				return pfail("field %q is not a field of %s", name, d.Name)
			}
			if portalFieldTypesRefused[f.Fieldtype] {
				return pfail("field %q is a %s, which a portal cannot show", name, f.Fieldtype)
			}
			// a Website User reads level 0 only; a restricted field on a page
			// would be a promise the permission model then breaks
			if f.Permlevel > 0 {
				return pfail("field %q has permlevel %d; a portal shows level 0 only", name, f.Permlevel)
			}
		}
		if len(pg.ListFields) == 0 {
			pg.ListFields = pg.Fields
			if len(pg.ListFields) > 4 {
				pg.ListFields = pg.ListFields[:4]
			}
		}
		for _, name := range pg.ListFields {
			if !pg.ShowsField(name) {
				return pfail("list field %q is not in fields", name)
			}
		}
		for _, name := range pg.Editable {
			if !pg.ShowsField(name) {
				return pfail("editable field %q is not in fields", name)
			}
			if _, isMatch := pg.Match[name]; isMatch {
				return pfail("match field %q cannot be editable", name)
			}
			if f := d.Field(name); f.ReadOnly {
				return pfail("editable field %q is read-only", name)
			}
		}
		if (pg.Create || pg.Write) && len(pg.Editable) == 0 {
			return pfail("a page that creates or writes needs editable fields")
		}
		if pg.DefaultsMethod != "" {
			opts, ok := whitelisted[pg.DefaultsMethod]
			if !ok || opts["portal"] != true {
				return pfail("defaultsMethod %q is not whitelisted with portal: true", pg.DefaultsMethod)
			}
		}
	}
	for _, pg := range p.Pages {
		for _, a := range pg.Actions {
			target := p.Page(a.Page)
			if target == nil {
				return fail("page %q: action %q points at page %q, which does not exist", pg.Name, a.Label, a.Page)
			}
			if a.New && !target.Create {
				return fail("page %q: action %q opens a new %q, which does not create", pg.Name, a.Label, a.Page)
			}
		}
	}
	return nil
}

func sortedPortalNames(m map[string]Portal) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------------ user type

// IsWebsiteUser reports whether the signed-in user is a Website User: someone
// who reaches the site only through its portals. Guest carries that user type
// too, but Guest is not signed in and is governed by its own role, so it is
// never counted here; neither is Admin.
func (c *Ctx) IsWebsiteUser() bool {
	if c.User == "Admin" || c.User == "Guest" || c.User == "" {
		return false
	}
	if c.userType == "" {
		c.userType = c.E.UserType(c, c.User)
	}
	return c.userType == "Website User"
}

// UserType is User.user_type, cached like the roles: it is asked on every
// permission check a Website User makes.
func (e *Engine) UserType(c *Ctx, user string) string {
	key := "utype:" + user
	if v, ok := e.Cache.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT user_type FROM tab_user WHERE id = $1`, user)
	if err != nil {
		// a failed read is not evidence of desk access; not cached, so the
		// next request asks again
		return "Website User"
	}
	t := ""
	if len(rows) > 0 {
		t = db.Str(rows[0]["user_type"])
	}
	// no row: no session can name this user, so it is trusted code acting as
	// them (a job, a test); the field's own default applies
	if t == "" {
		t = "System User"
	}
	e.Cache.Set(key, t, 0)
	return t
}

// ------------------------------------------------------------------ portal mode

// PortalMode reports whether this request's permissions come from portals
// instead of roles: always for a Website User, and for anyone on the portal's
// own endpoints, so a desk user previewing a portal sees what it grants.
func (c *Ctx) PortalMode() bool {
	return c.portal || c.IsWebsiteUser()
}

// SetPortalMode puts the context in portal mode.
func (c *Ctx) SetPortalMode(v bool) { c.portal = v }

// Portals returns the portals the user's roles reach, in name order.
func (c *Ctx) Portals() []*Portal {
	if c.User == "Guest" || c.User == "" {
		return nil
	}
	roles, err := c.Roles()
	if err != nil {
		return nil
	}
	var out []*Portal
	for _, name := range sortedPortalNames(c.St.Portals) {
		p := c.St.Portals[name]
		for _, r := range p.Roles {
			if contains(roles, r) {
				out = append(out, &p)
				break
			}
		}
	}
	return out
}

// PortalBySlug finds a portal the user reaches by its URL name.
func (c *Ctx) PortalBySlug(slug string) *Portal {
	for _, p := range c.Portals() {
		if p.Slug() == slug {
			return p
		}
	}
	return nil
}

// PortalIdentity returns the identity rows of the signed-in user in p. It is
// read with permissions ignored — the identity is what decides permission, so
// it cannot depend on it — and memoised for the request.
func (c *Ctx) PortalIdentity(p *Portal) ([]Doc, error) {
	if c.portalIdent == nil {
		c.portalIdent = map[string][]Doc{}
	}
	if rows, ok := c.portalIdent[p.Name]; ok {
		return rows, nil
	}
	fields := []string{"id"}
	for _, pg := range p.Pages {
		for _, v := range pg.Match {
			if !containsExact(fields, v) {
				fields = append(fields, v)
			}
		}
	}
	filters := map[string]any{}
	for k, v := range p.Identity.Filters {
		filters[k] = v
	}
	filters[p.Identity.UserField] = c.User
	list, err := c.GetList(p.Identity.Doctype, ListArgs{Filters: filters, Fields: fields, Limit: 50, IgnorePermissions: true, OrderBy: "creation asc"})
	if err != nil {
		return nil, err
	}
	rows := make([]Doc, 0, len(list))
	for _, r := range list {
		rows = append(rows, Doc(r))
	}
	c.portalIdent[p.Name] = rows
	return rows, nil
}

func containsExact(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// PortalMatches reports whether doc is the user's own under page pg: every
// match pair holds against one identity row.
func (c *Ctx) PortalMatches(p *Portal, pg *PortalPage, doc Doc) (bool, error) {
	rows, err := c.PortalIdentity(p)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		ok := true
		for k, v := range pg.Match {
			want := db.Str(row[v])
			if want == "" || db.Str(doc[k]) != want {
				ok = false
				break
			}
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// PortalMatchFilters is the filter that selects page pg's own rows: one group
// per identity row, ORed. With no identity the group list is empty and
// nothing matches.
func (c *Ctx) PortalMatchFilters(p *Portal, pg *PortalPage) ([][]db.Filter, error) {
	rows, err := c.PortalIdentity(p)
	if err != nil {
		return nil, err
	}
	var groups [][]db.Filter
	for _, row := range rows {
		var g []db.Filter
		for k, v := range pg.Match {
			want := db.Str(row[v])
			if want == "" {
				g = nil
				break
			}
			g = append(g, db.Filter{Field: k, Op: "=", Value: want})
		}
		if g != nil {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

// portalAllows is HasPermission's grant step in portal mode: some page of a
// portal the user reaches carries ptype on doctype and, for a document, the
// document matches that page's identity.
func (c *Ctx) portalAllows(doctype, ptype string, doc Doc) (bool, error) {
	for _, p := range c.Portals() {
		for i := range p.Pages {
			pg := &p.Pages[i]
			if pg.Doctype != doctype || !pg.grants(ptype) {
				continue
			}
			if doc == nil {
				return true, nil
			}
			ok, err := c.PortalMatches(p, pg, doc)
			if err != nil || ok {
				return ok, err
			}
		}
	}
	return false, nil
}

// portalFilters is permissionFilters' grant step in portal mode: the rows
// matched by any page that reads doctype.
func (c *Ctx) portalFilters(d *meta.DocType) ([]db.Filter, error) {
	var groups [][]db.Filter
	for _, p := range c.Portals() {
		for i := range p.Pages {
			pg := &p.Pages[i]
			if pg.Doctype != d.Name {
				continue
			}
			g, err := c.PortalMatchFilters(p, pg)
			if err != nil {
				return nil, err
			}
			groups = append(groups, g...)
		}
	}
	if len(groups) == 0 {
		// an empty IN renders as FALSE: nothing is the user's own
		return []db.Filter{{Field: "id", Op: "in", Value: []any{}}}, nil
	}
	return []db.Filter{{Any: groups}}, nil
}

// portalShowsField reports whether some page the user reaches shows
// doctype.field — CanReadFile's extra condition in portal mode.
func (c *Ctx) portalShowsField(doctype, field string) bool {
	for _, p := range c.Portals() {
		for i := range p.Pages {
			pg := &p.Pages[i]
			if pg.Doctype == doctype && pg.ShowsField(field) {
				return true
			}
		}
	}
	return false
}

// PortalUploadAllowed reports whether a portal lets the user put a file into
// doctype.field: the field must be an attachment some page makes editable,
// on a page that creates or writes.
func (c *Ctx) PortalUploadAllowed(doctype, field string) bool {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false
	}
	f := d.Field(field)
	if f == nil || (f.Fieldtype != "Attach" && f.Fieldtype != "Attach Image") {
		return false
	}
	for _, p := range c.Portals() {
		for i := range p.Pages {
			pg := &p.Pages[i]
			if pg.Doctype == doctype && (pg.Create || pg.Write) && pg.EditableField(field) {
				return true
			}
		}
	}
	return false
}
