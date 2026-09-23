package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Portals (OPS-10): the HTTP surface a Website User has.
//
// Two layers keep a Website User inside it. confineWebsiteUsers refuses every
// /api route that is not on a short list, so a desk endpoint added tomorrow is
// closed to them by default. And the engine decides every read and write they
// make from the portals' pages rather than from roles (engine/portal.go), so
// the few routes that stay open — uploads, files, portal methods — cannot
// reach anything a page does not grant. The endpoints below add a third,
// narrower filter: a page's own field list.

// websiteUserRoutes are the /api paths a Website User may call. Everything
// else answers 403, including a whitelisted method not marked portal: true.
var websiteUserRoutes = []string{
	"/api/login", "/api/logout", "/api/boot", "/api/translations",
	"/api/upload", "/api/file-info", "/api/health", "/api/ready",
}

var websiteUserPrefixes = []string{"/api/auth/", "/api/portal/", "/api/health/", "/api/ready/"}

func websiteUserRoute(path string) bool {
	if path == "/api/portal" {
		return true
	}
	for _, p := range websiteUserRoutes {
		if path == p {
			return true
		}
	}
	for _, p := range websiteUserPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// isWebsiteUser reports whether a signed-in user is a Website User.
func (s *Server) isWebsiteUser(r *http.Request, u string) bool {
	if u == "Guest" || u == "Admin" || u == "" {
		return false
	}
	return s.E.NewCtx(r.Context(), u).IsWebsiteUser()
}

// confineWebsiteUsers keeps a Website User on the routes a portal needs.
func (s *Server) confineWebsiteUsers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := user(r)
		if !s.isWebsiteUser(r, u) || websiteUserRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/method/") {
			path := strings.TrimPrefix(r.URL.Path, "/api/method/")
			if opts, ok := s.E.Whitelisted(path); ok && opts["portal"] == true {
				next.ServeHTTP(w, r)
				return
			}
		}
		s.writeErr(w, r, cerr.Permission("This area is for system users"))
	})
}

// homeFor is where a user lands after signing in: a Website User always in a
// portal, whatever they asked for; anyone else where they asked, or the desk.
func (s *Server) homeFor(r *http.Request, u, redirect string) string {
	if s.isWebsiteUser(r, u) {
		if redirect == "/portal" || strings.HasPrefix(redirect, "/portal/") {
			return redirect
		}
		return "/portal"
	}
	if redirect != "" {
		return redirect
	}
	return "/app"
}

// ------------------------------------------------------------------ limits

// portalLimiter is a per-user sliding window over the last hour. It lives in
// this process only: a site served by several processes allows each of them
// the full budget.
type portalLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newPortalLimiter() *portalLimiter { return &portalLimiter{hits: map[string][]time.Time{}} }

// allow records one hit for key and reports whether it is within limit, and
// otherwise how long until the oldest hit leaves the window.
func (l *portalLimiter) allow(key string, limit int, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-time.Hour)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false, kept[0].Add(time.Hour).Sub(now)
	}
	l.hits[key] = append(kept, now)
	return true, 0
}

// portalLimit refuses a Website User's write past the hourly budget.
func (s *Server) portalLimit(w http.ResponseWriter, r *http.Request, kind string, limit int) bool {
	u := user(r)
	if !s.isWebsiteUser(r, u) {
		return true
	}
	ok, wait := s.limiter.allow(kind+":"+u, limit, time.Now())
	if !ok {
		s.writeErr(w, r, cerr.TooMany("Too many changes in a short time. Try again later.").WithRetryAfter(int(wait.Seconds())+1))
	}
	return ok
}

// ------------------------------------------------------------------ endpoints

func (s *Server) portalRoutes(r chi.Router) {
	r.Get("/portal", s.portalList)
	r.Get("/portal/{portal}/{page}", s.portalPage)
	r.Get("/portal/{portal}/{page}/list", s.portalRows)
	r.Get("/portal/{portal}/{page}/new", s.portalNew)
	r.Get("/portal/{portal}/{page}/doc", s.portalGet)
	r.Get("/portal/{portal}/{page}/doc/{id}", s.portalGet)
	r.Post("/portal/{portal}/{page}/doc", s.portalCreate)
	r.Put("/portal/{portal}/{page}/doc/{id}", s.portalUpdate)
	r.Get("/portal/{portal}/{page}/search/{field}", s.portalSearch)
}

// runPortal runs fn in portal mode, with the portal and page the URL names.
func (s *Server) runPortal(w http.ResponseWriter, r *http.Request, fn func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error)) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if c.User == "Guest" {
			return nil, cerr.Auth("Sign in to continue")
		}
		c.SetPortalMode(true)
		p := c.PortalBySlug(urlParam(r, "portal"))
		if p == nil {
			return nil, cerr.NotFound("Portal {0} does not exist", urlParam(r, "portal"))
		}
		pg := p.Page(urlParam(r, "page"))
		if pg == nil {
			return nil, cerr.NotFound("Page {0} does not exist", urlParam(r, "page"))
		}
		return fn(c, p, pg)
	})
}

// portalSummary is a portal as the portal's navigation shows it.
func portalSummary(c *engine.Ctx, p *engine.Portal) map[string]any {
	pages := make([]map[string]any, 0, len(p.Pages))
	for _, pg := range p.Pages {
		pages = append(pages, map[string]any{
			"name": pg.Name, "label": c.T(pg.Label), "description": c.T(pg.Description),
			"kind": pg.Kind, "create": pg.Create,
		})
	}
	return map[string]any{"name": p.Name, "slug": p.Slug(), "title": c.T(p.Title), "pages": pages}
}

// portalsFor lists the portals the user reaches.
func portalsFor(c *engine.Ctx) []map[string]any {
	out := []map[string]any{}
	for _, p := range c.Portals() {
		out = append(out, portalSummary(c, p))
	}
	return out
}

func (s *Server) portalList(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if c.User == "Guest" {
			return nil, cerr.Auth("Sign in to continue")
		}
		c.SetPortalMode(true)
		return portalsFor(c), nil
	})
}

// portalPage describes a page: its fields, translated, with everything the
// user may not type into marked read-only.
func (s *Server) portalPage(w http.ResponseWriter, r *http.Request) {
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		d, err := c.St.DocType(pg.Doctype)
		if err != nil {
			return nil, err
		}
		td := c.St.TranslateDocType(d, c.Lang)
		fields := make([]*meta.Field, 0, len(pg.Fields))
		for _, name := range pg.Fields {
			f := td.Field(name)
			if f == nil {
				continue
			}
			cp := *f
			if !pg.EditableField(name) {
				cp.ReadOnly = true
			}
			// only the fields a page shows exist here, so a condition naming
			// any other is unanswerable: the field stays visible
			cp.DependsOn, cp.ReadOnlyDependsOn, cp.MandatoryDependsOn = "", "", ""
			fields = append(fields, &cp)
		}
		actions := make([]map[string]any, 0, len(pg.Actions))
		for _, a := range pg.Actions {
			actions = append(actions, map[string]any{"label": c.T(a.Label), "page": a.Page, "new": a.New})
		}
		state := ""
		if wf := c.WorkflowFor(d.Name); wf != nil {
			state = wf.StateField
		}
		return map[string]any{
			"portal": portalSummary(c, p),
			"page": map[string]any{
				"name": pg.Name, "label": c.T(pg.Label), "description": c.T(pg.Description), "kind": pg.Kind,
				"doctype": d.Name, "doctypeLabel": c.T(d.Label), "create": pg.Create, "write": pg.Write,
				"listFields": pg.ListFields, "editable": pg.Editable, "actions": actions,
				"stateField": state, "titleField": d.TitleField,
			},
			"fields": fields,
		}, nil
	})
}

// portalProject keeps what a page shows of a document, plus the columns the
// portal needs to address and describe it.
func portalProject(c *engine.Ctx, pg *engine.PortalPage, doc engine.Doc) engine.Doc {
	out := engine.Doc{}
	for _, k := range []string{"id", "modified", "creation", "docstatus"} {
		if v, ok := doc[k]; ok {
			out[k] = v
		}
	}
	for _, f := range pg.Fields {
		if v, ok := doc[f]; ok {
			out[f] = v
		}
	}
	if wf := c.WorkflowFor(pg.Doctype); wf != nil {
		out[wf.StateField] = doc[wf.StateField]
	}
	if d, err := c.St.DocType(pg.Doctype); err == nil && d.TitleField != "" {
		if v, ok := doc[d.TitleField]; ok {
			out[d.TitleField] = v
		}
	}
	return out
}

// pageFilters narrows a list to the page's own rows. The engine already limits
// the user to rows some page matches; this keeps two pages over one DocType
// from showing each other's.
func pageFilters(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) ([]db.Filter, error) {
	groups, err := c.PortalMatchFilters(p, pg)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []db.Filter{{Field: "id", Op: "in", Value: []any{}}}, nil
	}
	return []db.Filter{{Any: groups}}, nil
}

func (s *Server) portalRows(w http.ResponseWriter, r *http.Request) {
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		start, _ := strconv.Atoi(q.Get("start"))
		order := "creation desc"
		if ob := strings.TrimSpace(q.Get("order_by")); ob != "" {
			// ordering by a field the page does not show would reveal it
			field := strings.Fields(ob)[0]
			if !pg.ShowsField(field) && field != "creation" && field != "modified" {
				return nil, cerr.Validation("Cannot sort by {0}", field)
			}
			order = ob
		}
		filters, err := pageFilters(c, p, pg)
		if err != nil {
			return nil, err
		}
		fields := append([]string{"id", "modified", "creation", "docstatus"}, pg.Fields...)
		d, _ := c.St.DocType(pg.Doctype)
		if wf := c.WorkflowFor(pg.Doctype); wf != nil {
			fields = append(fields, wf.StateField)
		}
		if d.TitleField != "" && d.HasColumn(d.TitleField) && !containsFold(fields, d.TitleField) {
			fields = append(fields, d.TitleField)
		}
		rows, err := c.GetList(pg.Doctype, engine.ListArgs{Filters: filters, Fields: fields, OrderBy: order, Limit: limit + 1, Start: start})
		if err != nil {
			return nil, err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		docs := make([]engine.Doc, 0, len(rows))
		for _, row := range rows {
			docs = append(docs, portalProject(c, pg, engine.Doc(row)))
		}
		titles := c.ResolveLinkTitles(pg.Doctype, docs...)
		return map[string]any{"rows": docs, "titles": titles, "more": more}, nil
	})
}

// portalRecord reads one document through a page: the one the URL names, or,
// on a record page with no id, the user's own.
func (s *Server) portalRecord(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage, id string) (engine.Doc, error) {
	if id == "" {
		if pg.Kind != engine.PortalKindRecord {
			return nil, cerr.NotFound("{0} not found", c.T(pg.Label))
		}
		filters, err := pageFilters(c, p, pg)
		if err != nil {
			return nil, err
		}
		rows, err := c.GetList(pg.Doctype, engine.ListArgs{Filters: filters, Fields: []string{"id"}, Limit: 1, OrderBy: "creation asc"})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, cerr.NotFound("{0} not found", c.T(pg.Label))
		}
		id = db.Str(rows[0]["id"])
	}
	doc, err := c.GetDoc(pg.Doctype, id)
	if err != nil {
		return nil, err
	}
	// the engine let the user read it through some page; this page must be
	// the one that matches
	if ok, err := c.PortalMatches(p, pg, doc); err != nil {
		return nil, err
	} else if !ok {
		return nil, cerr.Permission("No permission to read {0} {1}", c.T(pg.Label), id)
	}
	return doc, nil
}

// portalResponse is a document as a page shows it.
func (s *Server) portalResponse(c *engine.Ctx, pg *engine.PortalPage, doc engine.Doc) engine.Doc {
	out := portalProject(c, pg, c.RedactDoc(pg.Doctype, doc))
	c.ResolveLinkTitles(pg.Doctype, out)
	return out
}

func (s *Server) portalGet(w http.ResponseWriter, r *http.Request) {
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		doc, err := s.portalRecord(c, p, pg, urlParam(r, "id"))
		if err != nil {
			return nil, err
		}
		return s.portalResponse(c, pg, doc), nil
	})
}

// portalValues keeps the editable values of a request body and refuses
// anything else: a field the page does not let the user type into is an
// error, not something to drop quietly, so a client that tries is told.
func portalValues(pg *engine.PortalPage, body map[string]any) (engine.Doc, error) {
	out := engine.Doc{}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "id", "modified", "doctype":
			continue
		}
		if !pg.EditableField(k) {
			return nil, cerr.Validation("{0} cannot be changed here", k)
		}
		out[k] = body[k]
	}
	return out, nil
}

// portalDefaults are the values a new document starts with: the page's
// defaults method, if it has one, limited to the editable fields.
func portalDefaults(c *engine.Ctx, pg *engine.PortalPage) (engine.Doc, error) {
	out := engine.Doc{}
	if pg.DefaultsMethod == "" {
		return out, nil
	}
	rt, err := c.RT()
	if err != nil {
		return nil, err
	}
	raw, err := rt.CallWhitelisted(pg.DefaultsMethod, json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	var vals map[string]any
	if len(raw) > 0 {
		json.Unmarshal(raw, &vals)
	}
	for k, v := range vals {
		if pg.EditableField(k) {
			out[k] = v
		}
	}
	return out, nil
}

func (s *Server) portalNew(w http.ResponseWriter, r *http.Request) {
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		if !pg.Create {
			return nil, cerr.Permission("No permission to create {0}", c.T(pg.Label))
		}
		out, err := portalDefaults(c, pg)
		if err != nil {
			return nil, err
		}
		c.ResolveLinkTitles(pg.Doctype, out)
		return out, nil
	})
}

func (s *Server) portalCreate(w http.ResponseWriter, r *http.Request) {
	if !s.portalLimit(w, r, "write", s.E.Cfg.Portal.Writes()) {
		return
	}
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		if !pg.Create {
			return nil, cerr.Permission("No permission to create {0}", c.T(pg.Label))
		}
		var body map[string]any
		if err := readJSON(r, &body); err != nil {
			return nil, err
		}
		vals, err := portalValues(pg, body)
		if err != nil {
			return nil, err
		}
		// a new document starts from the defaults, as the form does: a client
		// that leaves a field out keeps its default instead of blanking it
		defaults, err := portalDefaults(c, pg)
		if err != nil {
			return nil, err
		}
		for k, v := range defaults {
			if _, sent := vals[k]; !sent {
				vals[k] = v
			}
		}
		ident, err := c.PortalIdentity(p)
		if err != nil {
			return nil, err
		}
		if len(ident) != 1 {
			// with several identity rows the portal cannot tell whose record
			// this is, and guessing would file it under the wrong one
			return nil, cerr.Permission("No permission to create {0}", c.T(pg.Label))
		}
		for k, v := range pg.Match {
			vals[k] = ident[0][v]
		}
		doc, err := c.NewDoc(pg.Doctype, vals)
		if err != nil {
			return nil, err
		}
		doc, err = c.Insert(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}
		return s.portalResponse(c, pg, doc), nil
	})
}

func (s *Server) portalUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.portalLimit(w, r, "write", s.E.Cfg.Portal.Writes()) {
		return
	}
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		if !pg.Write {
			return nil, cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(pg.Label), urlParam(r, "id"))
		}
		var body map[string]any
		if err := readJSON(r, &body); err != nil {
			return nil, err
		}
		vals, err := portalValues(pg, body)
		if err != nil {
			return nil, err
		}
		doc, err := s.portalRecord(c, p, pg, urlParam(r, "id"))
		if err != nil {
			return nil, err
		}
		for k, v := range vals {
			doc[k] = v
		}
		// the concurrency guard: a stale form must not overwrite a newer save
		if m, ok := body["modified"]; ok {
			doc["modified"] = m
		}
		doc, err = c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}
		return s.portalResponse(c, pg, doc), nil
	})
}

// portalSearch offers the choices of an editable Link field. The target
// DocType is usually no page's (a list of document types, say), so it is read
// with permissions ignored — which is why only an editable field can be
// searched, and why only the id and title come back.
func (s *Server) portalSearch(w http.ResponseWriter, r *http.Request) {
	s.runPortal(w, r, func(c *engine.Ctx, p *engine.Portal, pg *engine.PortalPage) (any, error) {
		field := urlParam(r, "field")
		if !pg.EditableField(field) || !(pg.Create || pg.Write) {
			return nil, cerr.Permission("No permission for {0}", field)
		}
		d, err := c.St.DocType(pg.Doctype)
		if err != nil {
			return nil, err
		}
		f := d.Field(field)
		if f == nil || f.Fieldtype != "Link" {
			return nil, cerr.Validation("{0} is not a link", field)
		}
		target, err := c.St.DocType(f.OptionsString())
		if err != nil {
			return nil, err
		}
		var rows []map[string]any
		err = c.WithIgnorePermissions(func() error {
			var e error
			rows, e = c.LinkSearch(target.Name, r.URL.Query().Get("q"), nil, 20)
			return e
		})
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{"id": row["id"]}
			if target.TitleField != "" {
				item["title"] = row[target.TitleField]
			}
			out = append(out, item)
		}
		return out, nil
	})
}
