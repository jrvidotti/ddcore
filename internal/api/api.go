// Package api serves the HTTP API, the desk and app client bundles.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

type Server struct {
	E      *engine.Engine
	Desk   fs.FS // built desk (index.html + assets), may be nil
	Router chi.Router
	// MCPHandler is mounted at /mcp when set.
	MCPHandler http.Handler
	// MaxUpload caps the body of /api/upload (default 50 MB).
	MaxUpload int64
}

type ctxKey int

const userKey ctxKey = 1

func New(e *engine.Engine, desk fs.FS) *Server {
	s := &Server{E: e, Desk: desk, MaxUpload: 50 << 20}
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer, middleware.Compress(5))
	r.Use(s.auth)
	r.Route("/api", func(r chi.Router) {
		r.Post("/login", s.login)
		r.Post("/logout", s.logout)
		r.Get("/boot", s.boot)
		r.Get("/meta/{doctype}", s.getMeta)
		r.Get("/translations", s.translations)
		r.Post("/upload", s.upload)
		// endpoints que nunca respondem a visitantes anônimos (B05)
		r.Group(func(r chi.Router) {
			r.Use(s.requireLogin)
			r.Get("/events", s.events)
			r.Get("/search/link", s.linkSearch)
			r.Get("/search/link-titles", s.linkTitles)
			r.Post("/search/link-titles", s.linkTitles)
			r.Get("/report/{name}", s.report)
			r.Get("/workspace/{name}/card/{card}", s.numberCard)
			r.Get("/workspace/{name}/chart/{chart}", s.chart)
			r.Get("/comments/{doctype}/{name}", s.comments)
			r.Get("/versions/{doctype}/{name}", s.versions)
		})
		r.Get("/count/{doctype}", s.count)
		r.Get("/resource/{doctype}", s.list)
		r.Post("/resource/{doctype}", s.create)
		r.Get("/resource/{doctype}/{name}", s.get)
		r.Put("/resource/{doctype}/{name}", s.update)
		r.Delete("/resource/{doctype}/{name}", s.remove)
		r.Post("/resource/{doctype}/{name}/{method}", s.docMethod)
		r.Post("/method/{path}", s.method)
		r.Get("/method/{path}", s.method)
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })
	})
	r.Get("/assets/apps/{app}/*", s.appAsset)
	r.Get("/files/*", s.file)
	r.Get("/private/files/*", s.privateFile)
	r.NotFound(s.deskHandler)
	s.Router = r
	return s
}

// ------------------------------------------------------------------ helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	e := cerr.From(err)
	status := e.Status
	if status == 0 {
		status = 500
	}
	writeJSON(w, status, map[string]any{"error": e})
}

type response struct {
	Data     any              `json:"data"`
	Messages []engine.Message `json:"messages,omitempty"`
}

func user(r *http.Request) string {
	if u, ok := r.Context().Value(userKey).(string); ok && u != "" {
		return u
	}
	return "Guest"
}

// run executes fn in a transaction as the request's user and writes the result.
func (s *Server) run(w http.ResponseWriter, r *http.Request, fn func(c *engine.Ctx) (any, error)) {
	var out any
	c := s.E.NewCtx(r.Context(), user(r))
	c.Request = map[string]any{"method": r.Method, "path": r.URL.Path, "ip": r.RemoteAddr}
	if l := r.Header.Get("X-Lang"); l != "" {
		c.Lang = l
	}
	err := c.Run(func(c *engine.Ctx) error {
		var e error
		out, e = fn(c)
		return e
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, response{Data: out, Messages: c.Messages})
}

// runCached is `run` with an ETag: the desk asks for the same meta on every
// navigation, so an unchanged payload answers 304.
func (s *Server) runCached(w http.ResponseWriter, r *http.Request, fn func(c *engine.Ctx) (any, error)) {
	var out any
	c := s.E.NewCtx(r.Context(), user(r))
	if l := r.Header.Get("X-Lang"); l != "" {
		c.Lang = l
	}
	err := c.Run(func(c *engine.Ctx) error {
		var e error
		out, e = fn(c)
		return e
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	body, err := json.Marshal(response{Data: out, Messages: c.Messages})
	if err != nil {
		writeErr(w, err)
		return
	}
	etag := fmt.Sprintf("%q", "sha256-"+hex.EncodeToString(sha256Sum(body)))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	for _, m := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m), "W/")) == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write(body)
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return cerr.Validation("JSON inválido: %v", err)
	}
	return nil
}

func queryJSON(r *http.Request, key string) (any, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, cerr.Validation("%s inválido: %v", key, err)
	}
	return v, nil
}

// ------------------------------------------------------------------ auth

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u string
		if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "token ") {
			u, _ = s.E.UserFromAPIKey(r.Context(), strings.TrimSpace(h[6:]))
			if u == "" {
				writeErr(w, cerr.Auth("Chave de API inválida"))
				return
			}
		} else if ck, err := r.Cookie("sid"); err == nil {
			u, _ = s.E.UserFromSession(r.Context(), ck.Value)
			// CSRF: state-changing requests with cookie auth need the header
			if u != "" && r.Method != "GET" && r.Method != "HEAD" && !strings.HasPrefix(r.URL.Path, "/api/login") {
				if r.Header.Get("X-DDCore-CSRF") == "" && r.Header.Get("X-Requested-With") == "" {
					writeErr(w, cerr.Permission("Requisição sem cabeçalho CSRF"))
					return
				}
			}
		}
		if u == "" {
			u = "Guest"
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// RequireAdminAPIKey guards administrative handlers (the MCP HTTP transport):
// only an `Authorization: token key:secret` header is accepted — never a
// session cookie — and the key must belong to Administrator or a user with
// the System Manager role.
func (s *Server) RequireAdminAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(strings.ToLower(h), "token ") {
			writeErr(w, cerr.Auth("Este endpoint exige uma chave de API (Authorization: token chave:segredo)"))
			return
		}
		u, err := s.E.UserFromAPIKey(r.Context(), strings.TrimSpace(h[6:]))
		if err != nil {
			writeErr(w, err)
			return
		}
		if u == "" || u == "Guest" {
			writeErr(w, cerr.Auth("Chave de API inválida"))
			return
		}
		roles, err := s.E.NewCtx(r.Context(), u).RolesOf(u)
		if err != nil {
			writeErr(w, err)
			return
		}
		if u != "Administrator" && !containsFold(roles, "System Manager") {
			writeErr(w, cerr.Permission("Este endpoint exige o papel System Manager"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// requireLogin rejects anonymous requests: these endpoints expose data of
// workspaces, reports, events and document history (B05).
func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user(r) == "Guest" {
			writeErr(w, cerr.Auth("Faça login para continuar"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Usr string `json:"usr"`
		Pwd string `json:"pwd"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Usr == "" {
		body.Usr, body.Pwd = r.FormValue("usr"), r.FormValue("pwd")
	}
	sid, err := s.E.Login(r.Context(), body.Usr, body.Pwd)
	if err != nil {
		writeErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	writeJSON(w, 200, map[string]any{"data": map[string]any{"ok": true}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie("sid"); err == nil {
		s.E.Logout(r.Context(), ck.Value)
		s.E.Cache.Del("sid:" + ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"data": map[string]any{"ok": true}})
}

// ------------------------------------------------------------------ boot & meta

func (s *Server) boot(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		roles, _ := c.Roles()
		var userDoc map[string]any
		if c.User != "Guest" {
			userDoc, _ = c.GetValues("User", c.User, []string{"name", "full_name", "language", "user_type"})
		}
		var apps []map[string]any
		var workspaces []map[string]any
		for _, name := range s.E.AppOrder() {
			a := s.E.Snap.Apps[name]
			apps = append(apps, map[string]any{"name": a.Name, "title": a.Title, "desk": a.Desk, "hasDeskInclude": len(deskIncludes(a)) > 0})
		}
		for _, ws := range s.E.Snap.Workspaces {
			if allowed(ws["roles"], roles) {
				workspaces = append(workspaces, ws)
			}
		}
		doctypes := map[string]any{}
		for _, n := range s.E.Meta.Names() {
			d := s.E.Meta.DocTypes[n]
			if d.IsChild {
				continue
			}
			if ok, _ := c.HasPermission(n, "read", nil); ok {
				doctypes[n] = map[string]any{"label": d.Label, "app": d.App, "icon": d.Icon, "module": d.Module, "titleField": d.TitleField}
			}
		}
		reports := map[string]any{}
		for n, rep := range s.E.Snap.Reports {
			if allowed(rep["roles"], roles) {
				reports[n] = map[string]any{"label": orStr(rep["label"], n), "refDoctype": rep["refDoctype"], "app": rep["app"]}
			}
		}
		return map[string]any{
			"user": c.User, "roles": roles, "userDoc": userDoc, "lang": c.Lang, "apps": apps,
			"workspaces": workspaces, "doctypes": doctypes, "reports": reports,
			"site":   map[string]any{"name": s.E.Cfg.SiteName, "currency": s.E.Cfg.Currency, "dev": s.E.Cfg.Dev, "scheduler": s.E.Cfg.Scheduler, "version": "0.1.0"},
			"loaded": s.E.Loaded.UnixMilli(),
		}, nil
	})
}

func allowed(rolesAny any, userRoles []string) bool {
	list, ok := rolesAny.([]any)
	if !ok || len(list) == 0 {
		return true
	}
	for _, r := range list {
		rs := fmt.Sprint(r)
		if rs == "*" {
			return true
		}
		for _, ur := range userRoles {
			if ur == rs {
				return true
			}
		}
	}
	return false
}

func orStr(v any, d string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return d
}

func (s *Server) getMeta(w http.ResponseWriter, r *http.Request) {
	s.runCached(w, r, func(c *engine.Ctx) (any, error) {
		d, err := s.E.DocType(chi.URLParam(r, "doctype"))
		if err != nil {
			return nil, err
		}
		if ok, _ := c.HasPermission(d.Name, "read", nil); !ok && !d.IsChild {
			return nil, cerr.Permission("Sem permissão para %s", d.Label)
		}
		childMeta := map[string]*meta.DocType{}
		for _, tf := range d.TableFields() {
			childMeta[tf.OptionsString()], _ = s.E.DocType(tf.OptionsString())
		}
		linkTitles := map[string]string{}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" {
				if t, ok := s.E.Meta.Get(f.OptionsString()); ok && t.TitleField != "" {
					linkTitles[f.OptionsString()] = t.TitleField
				}
			}
		}
		for _, tf := range d.TableFields() {
			if child, ok := s.E.Meta.Get(tf.OptionsString()); ok {
				for _, cf := range child.Fields {
					if cf.Fieldtype == "Link" {
						if t, ok := s.E.Meta.Get(cf.OptionsString()); ok && t.TitleField != "" {
							linkTitles[cf.OptionsString()] = t.TitleField
						}
					}
				}
			}
		}
		return map[string]any{"doctype": d, "children": childMeta, "permissions": c.Permissions(d), "series": engine.SeriesOptions(d), "linkTitles": linkTitles}, nil
	})
}

func (s *Server) translations(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = s.E.Cfg.Lang
	}
	writeJSON(w, 200, map[string]any{"data": s.E.I18n.Catalogue(lang)})
}

// ------------------------------------------------------------------ resources

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		q := r.URL.Query()
		filters, err := queryJSON(r, "filters")
		if err != nil {
			return nil, err
		}
		orFilters, err := queryJSON(r, "or_filters")
		if err != nil {
			return nil, err
		}
		var fields []string
		if fv, err := queryJSON(r, "fields"); err != nil {
			return nil, err
		} else if list, ok := fv.([]any); ok {
			for _, f := range list {
				fields = append(fields, fmt.Sprint(f))
			}
		}
		if err := s.referenceGuard(c, chi.URLParam(r, "doctype"), filters); err != nil {
			return nil, err
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit == 0 {
			limit = 20
		}
		start, _ := strconv.Atoi(q.Get("start"))
		rows, err := c.GetList(chi.URLParam(r, "doctype"), engine.ListArgs{Filters: filters, OrFilters: orFilters, Fields: fields, OrderBy: q.Get("order_by"), Limit: limit, Start: start, GroupBy: q.Get("group_by")})
		if err != nil {
			return nil, err
		}
		var docs []engine.Doc
		for _, r := range rows {
			docs = append(docs, engine.Doc(r))
		}
		titles := c.ResolveLinkTitles(chi.URLParam(r, "doctype"), docs...)
		if q.Get("with_count") != "" {
			n, err := c.Count(chi.URLParam(r, "doctype"), filters, orFilters)
			if err != nil {
				return nil, err
			}
			return map[string]any{"rows": rows, "count": n, "titles": titles}, nil
		}
		if q.Get("with_titles") != "" {
			return map[string]any{"rows": rows, "titles": titles}, nil
		}
		return rows, nil
	})
}

func (s *Server) count(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		filters, err := queryJSON(r, "filters")
		if err != nil {
			return nil, err
		}
		orFilters, err := queryJSON(r, "or_filters")
		if err != nil {
			return nil, err
		}
		if err := s.referenceGuard(c, chi.URLParam(r, "doctype"), filters); err != nil {
			return nil, err
		}
		return c.Count(chi.URLParam(r, "doctype"), filters, orFilters)
	})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		doc, err := c.GetDoc(chi.URLParam(r, "doctype"), chi.URLParam(r, "name"))
		if err != nil {
			return nil, err
		}
		c.ResolveLinkTitles(chi.URLParam(r, "doctype"), doc)
		return doc, nil
	})
}

// childGuard refuses direct mutation of a child doctype through the generic
// resource API (B02): a row only exists inside its parent document and must
// go through the parent's lifecycle, hooks and permissions.
func (s *Server) childGuard(doctype string) error {
	d, err := s.E.DocType(doctype)
	if err != nil {
		return err
	}
	if d.IsChild {
		return cerr.Validation("%s é uma tabela filha: edite pelo documento pai", d.Label)
	}
	return nil
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body engine.Doc
		if err := readJSON(r, &body); err != nil {
			return nil, err
		}
		dt := chi.URLParam(r, "doctype")
		if err := s.childGuard(dt); err != nil {
			return nil, err
		}
		doc, err := c.NewDoc(dt, body)
		if err != nil {
			return nil, err
		}
		return c.Insert(doc, engine.SaveOpts{})
	})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body engine.Doc
		if err := readJSON(r, &body); err != nil {
			return nil, err
		}
		dt, name := chi.URLParam(r, "doctype"), chi.URLParam(r, "name")
		if err := s.childGuard(dt); err != nil {
			return nil, err
		}
		doc, err := c.GetDoc(dt, name)
		if err != nil {
			return nil, err
		}
		for k, v := range body {
			if k == "name" || k == "doctype" || k == "owner" || k == "creation" {
				continue
			}
			doc[k] = v
		}
		return c.Save(doc, engine.SaveOpts{})
	})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := s.childGuard(chi.URLParam(r, "doctype")); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, c.Delete(chi.URLParam(r, "doctype"), chi.URLParam(r, "name"), false, false)
	})
}

// docMethod: submit, cancel, amend, rename or a controller method.
func (s *Server) docMethod(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		dt, name, m := chi.URLParam(r, "doctype"), chi.URLParam(r, "name"), chi.URLParam(r, "method")
		if err := s.childGuard(dt); err != nil {
			return nil, err
		}
		var args map[string]any
		if err := readJSON(r, &args); err != nil {
			return nil, err
		}
		if args == nil {
			args = map[string]any{}
		}
		switch m {
		case "submit", "cancel", "save":
			doc, err := c.GetDoc(dt, name)
			if err != nil {
				return nil, err
			}
			if body, ok := args["doc"].(map[string]any); ok {
				for k, v := range body {
					if k != "name" && k != "doctype" && k != "owner" && k != "creation" {
						doc[k] = v
					}
				}
			}
			switch m {
			case "submit":
				return c.Submit(doc)
			case "cancel":
				return c.Cancel(doc)
			}
			return c.Save(doc, engine.SaveOpts{})
		case "amend":
			return c.Amend(dt, name)
		case "rename":
			nn, _ := args["name"].(string)
			return c.Rename(dt, name, nn)
		case "run_method":
			m, _ = args["method"].(string)
			delete(args, "method")
		}
		d, err := s.E.DocType(dt)
		if err != nil {
			return nil, err
		}
		if !contains(d.Methods, m) {
			return nil, cerr.NotFound("Método %s não existe em %s", m, dt)
		}
		doc, err := c.GetDoc(dt, name)
		if err != nil {
			return nil, err
		}
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(args)
		res, err := rt.RunMethod(dt, m, doc.JSON(), b)
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": res.Result, "doc": res.Doc}, nil
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// method calls a whitelisted function: app.dir.file.fn
func (s *Server) method(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "path")
	opts, ok := s.E.Whitelisted(path)
	if !ok {
		writeErr(w, cerr.NotFound("Método %s não existe ou não é whitelisted", path))
		return
	}
	if user(r) == "Guest" && opts["allowGuest"] != true {
		writeErr(w, cerr.Auth("Faça login para continuar"))
		return
	}
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		args := map[string]any{}
		if r.Method == "GET" {
			for k, v := range r.URL.Query() {
				args[k] = v[0]
			}
		} else if err := readJSON(r, &args); err != nil {
			return nil, err
		}
		if roles, ok := opts["roles"].([]any); ok && len(roles) > 0 {
			has := false
			for _, ro := range roles {
				if c.HasRole(fmt.Sprint(ro)) {
					has = true
				}
			}
			if !has && c.User != "Administrator" {
				return nil, cerr.Permission("Sem permissão para %s", path)
			}
		}
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(args)
		res, err := rt.CallWhitelisted(path, b)
		if err != nil {
			return nil, err
		}
		return res, nil
	})
}

func (s *Server) linkSearch(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		filters, err := queryJSON(r, "filters")
		if err != nil {
			return nil, err
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		return c.LinkSearch(r.URL.Query().Get("doctype"), r.URL.Query().Get("txt"), filters, limit)
	})
}

func (s *Server) linkTitles(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if r.Method == "POST" {
			var body map[string][]string
			if err := readJSON(r, &body); err == nil && len(body) > 0 {
				out := map[string]map[string]string{}
				for dt, names := range body {
					m, err := c.LinkTitles(dt, names)
					if err == nil && len(m) > 0 {
						out[dt] = m
					}
				}
				return out, nil
			}
		}
		dt := r.URL.Query().Get("doctype")
		namesStr := r.URL.Query().Get("names")
		var names []string
		if namesStr != "" {
			names = strings.Split(namesStr, ",")
		}
		m, err := c.LinkTitles(dt, names)
		if err != nil {
			return nil, err
		}
		return map[string]map[string]string{dt: m}, nil
	})
}

// referenceFields maps the doctypes whose rows only describe another
// document — their visibility follows the referenced document (B04).
var referenceFields = map[string][2]string{
	"Comment": {"reference_doctype", "reference_name"},
	"Version": {"ref_doctype", "docname"},
}

// requireDocRead checks the user may read doctype/name, loading only the
// columns the permission rules need.
func (s *Server) requireDocRead(c *engine.Ctx, doctype, name string) error {
	if doctype == "" || name == "" {
		return cerr.Permission("Informe o documento de referência")
	}
	d, err := s.E.DocType(doctype)
	if err != nil {
		return err
	}
	vals, err := c.GetValues(doctype, name, []string{"name", "owner", "docstatus"})
	if err != nil {
		return err
	}
	if vals == nil {
		return cerr.NotFound("%s %s não encontrado", c.T(d.Label), name)
	}
	ok, err := c.HasPermission(doctype, "read", engine.Doc(vals))
	if err != nil {
		return err
	}
	if !ok {
		return cerr.Permission("Sem permissão para ler %s %s", c.T(d.Label), name)
	}
	return nil
}

// referenceGuard refuses a listing of Comment/Version that does not pin a
// single referenced document the user may read (B04).
func (s *Server) referenceGuard(c *engine.Ctx, doctype string, filters any) error {
	pair, ok := referenceFields[doctype]
	if !ok {
		return nil
	}
	if c.User == "Administrator" || c.HasRole("System Manager") {
		return nil
	}
	fs, err := db.ParseFilters(filters)
	if err != nil {
		return cerr.Validation("%s", err)
	}
	var refDoctype, refName string
	for _, f := range fs {
		if f.Op != "=" && f.Op != "==" {
			continue
		}
		switch f.Field {
		case pair[0]:
			refDoctype = fmt.Sprint(f.Value)
		case pair[1]:
			refName = fmt.Sprint(f.Value)
		}
	}
	if refDoctype == "" || refName == "" {
		return cerr.Permission("Filtre %s por %s e %s", doctype, pair[0], pair[1])
	}
	return s.requireDocRead(c, refDoctype, refName)
}

func (s *Server) comments(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := s.requireDocRead(c, chi.URLParam(r, "doctype"), chi.URLParam(r, "name")); err != nil {
			return nil, err
		}
		return c.GetList("Comment", engine.ListArgs{Filters: map[string]any{"reference_doctype": chi.URLParam(r, "doctype"), "reference_name": chi.URLParam(r, "name")}, Fields: []string{"name", "owner", "creation", "content", "comment_type"}, OrderBy: "creation asc", Limit: 200})
	})
}

func (s *Server) versions(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := s.requireDocRead(c, chi.URLParam(r, "doctype"), chi.URLParam(r, "name")); err != nil {
			return nil, err
		}
		return c.GetList("Version", engine.ListArgs{Filters: map[string]any{"ref_doctype": chi.URLParam(r, "doctype"), "docname": chi.URLParam(r, "name")}, Fields: []string{"name", "owner", "creation", "data"}, OrderBy: "creation desc", Limit: 50})
	})
}

// ------------------------------------------------------------------ reports & workspace

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		name := chi.URLParam(r, "name")
		rep, ok := s.E.Snap.Reports[name]
		if !ok {
			return nil, cerr.NotFound("Relatório %s não existe", name)
		}
		roles, _ := c.Roles()
		if !allowed(rep["roles"], roles) && c.User != "Administrator" {
			return nil, cerr.Permission("Sem permissão para o relatório %s", name)
		}
		if ref, _ := rep["refDoctype"].(string); ref != "" {
			if ok, err := c.HasPermission(ref, "report", nil); err != nil {
				return nil, err
			} else if !ok {
				return nil, cerr.Permission("Sem permissão de relatório em %s", ref)
			}
		}
		filters, err := queryJSON(r, "filters")
		if err != nil {
			return nil, err
		}
		if filters == nil {
			filters = map[string]any{}
		}
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(filters)
		res, err := rt.RunReport(name, b)
		if err != nil {
			return nil, err
		}
		return map[string]any{"meta": rep, "result": res}, nil
	})
}

func (s *Server) numberCard(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		wsName, cardName := chi.URLParam(r, "name"), chi.URLParam(r, "card")
		ws, err := s.workspace(c, wsName)
		if err != nil {
			return nil, err
		}
		cards, _ := ws["numberCards"].([]any)
		for _, cAny := range cards {
			card, _ := cAny.(map[string]any)
			if card["name"] != cardName {
				continue
			}
			if dt, ok := card["doctype"].(string); ok && dt != "" {
				if ok, err := c.HasPermission(dt, "read", nil); err != nil {
					return nil, err
				} else if !ok {
					return nil, cerr.Permission("Sem permissão para %s", dt)
				}
				agg := orStr(card["aggregate"], "count")
				field := "count(*) as value"
				if strings.HasPrefix(agg, "sum:") {
					field = "sum(" + strings.TrimPrefix(agg, "sum:") + ") as value"
				}
				rows, err := c.GetList(dt, engine.ListArgs{Filters: card["filters"], Fields: []string{field}})
				if err != nil {
					return nil, err
				}
				var v any = 0
				if len(rows) > 0 && rows[0]["value"] != nil {
					v = rows[0]["value"]
				}
				return map[string]any{"value": v, "aggregate": agg}, nil
			}
			rt, err := c.RT()
			if err != nil {
				return nil, err
			}
			return rt.NumberCard(wsName, cardName)
		}
		return nil, cerr.NotFound("Card %s não existe", cardName)
	})
}

func (s *Server) chart(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		wsName := chi.URLParam(r, "name")
		if _, err := s.workspace(c, wsName); err != nil {
			return nil, err
		}
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		return rt.Chart(wsName, chi.URLParam(r, "chart"))
	})
}

// workspace resolves a workspace and checks the user holds one of its roles
// (B05): hiding the link in the boot payload is not authorisation.
func (s *Server) workspace(c *engine.Ctx, name string) (map[string]any, error) {
	ws, ok := s.E.Snap.Workspaces[name]
	if !ok {
		return nil, cerr.NotFound("Workspace %s não existe", name)
	}
	roles, err := c.Roles()
	if err != nil {
		return nil, err
	}
	if !allowed(ws["roles"], roles) && c.User != "Administrator" {
		return nil, cerr.Permission("Sem permissão para o workspace %s", name)
	}
	return ws, nil
}

// ------------------------------------------------------------------ events (SSE)

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE não suportado", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.E.Events.Subscribe(user(r), s.eventAuthorizer(r.Context(), user(r)))
	defer s.E.Events.Unsubscribe(ch)
	fmt.Fprintf(w, "event: hello\ndata: {}\n\n")
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev.Payload)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, b)
			fl.Flush()
		}
	}
}

// eventAuthorizer decides whether an SSE subscriber may see events about a
// doctype (B20). The answer is cached for a minute per user/doctype because
// it is consulted on every published event.
func (s *Server) eventAuthorizer(ctx context.Context, u string) engine.Authorizer {
	return func(doctype, name string) bool {
		key := "evperm:" + u + ":" + doctype
		if v, ok := s.E.Cache.Get(key); ok {
			return v.(bool)
		}
		allowed := false
		if err := s.E.Run(ctx, u, func(c *engine.Ctx) error {
			ok, err := c.HasPermission(doctype, "read", nil)
			allowed = ok
			return err
		}); err != nil {
			return false
		}
		s.E.Cache.Set(key, allowed, time.Minute)
		return allowed
	}
}

// ------------------------------------------------------------------ files

func (s *Server) dataDir() string {
	d := s.E.Cfg.DataDir
	if d == "" {
		d = "data"
	}
	return d
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if c.User == "Guest" {
			return nil, cerr.Auth("Faça login para enviar arquivos")
		}
		max := s.MaxUpload
		if max <= 0 {
			max = 50 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, max)
		if err := r.ParseMultipartForm(max); err != nil {
			return nil, cerr.Validation("upload inválido: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			return nil, cerr.Validation("campo file ausente")
		}
		defer f.Close()
		private := r.FormValue("is_private") != "0"
		sub := "public"
		if private {
			sub = "private"
		}
		name := randomFileName(hdr.Filename)
		dir := filepath.Join(s.dataDir(), "files", sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		out, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		defer out.Close()
		n, err := io.Copy(out, f)
		if err != nil {
			return nil, err
		}
		url := "/files/" + name
		if private {
			url = "/private/files/" + name
		}
		doc, _ := c.NewDoc("File", engine.Doc{"file_name": hdr.Filename, "file_url": url, "file_size": n, "content_type": hdr.Header.Get("Content-Type"), "is_private": private,
			"attached_to_doctype": r.FormValue("doctype"), "attached_to_name": r.FormValue("docname"), "attached_to_field": r.FormValue("fieldname")})
		return c.Insert(doc, engine.SaveOpts{IgnorePermissions: true})
	})
}

// Extensions that a browser would execute on our origin are neutralised.
var dangerousExt = map[string]bool{".html": true, ".htm": true, ".svg": true, ".xhtml": true, ".xml": true, ".js": true, ".mjs": true, ".wasm": true, ".shtml": true}

// randomFileName never reuses the uploaded name: it is unguessable and only
// a safe extension survives, so a public file cannot be located by name nor
// served as active content.
func randomFileName(orig string) string {
	ext := strings.ToLower(filepath.Ext(orig))
	for _, r := range ext {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.') {
			ext = ".bin"
			break
		}
	}
	if dangerousExt[ext] || len(ext) > 10 || ext == "." {
		ext = ".bin"
	}
	return engine.RandomToken() + ext
}

// serveUpload hands out user files as downloads, never as active content.
func serveUpload(w http.ResponseWriter, r *http.Request, prefix, dir string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	name := filepath.Base(r.URL.Path)
	ext := strings.ToLower(filepath.Ext(name))
	inline := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".pdf"
	if inline {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", name))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	}
	http.StripPrefix(prefix, http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	serveUpload(w, r, "/files/", filepath.Join(s.dataDir(), "files", "public"))
}

// privateFile serves a private upload only to users who may read the
// document it is attached to (or its owner / System Manager when detached).
func (s *Server) privateFile(w http.ResponseWriter, r *http.Request) {
	if user(r) == "Guest" {
		writeErr(w, cerr.Auth("Faça login"))
		return
	}
	allowed := false
	err := s.E.Run(r.Context(), user(r), func(c *engine.Ctx) error {
		f, err := c.GetValues("File", map[string]any{"file_url": r.URL.Path}, []string{"owner", "attached_to_doctype", "attached_to_name"})
		if err != nil || f == nil {
			return err
		}
		if db.Str(f["owner"]) == c.User || c.HasRole("System Manager") {
			allowed = true
			return nil
		}
		if dt, dn := db.Str(f["attached_to_doctype"]), db.Str(f["attached_to_name"]); dt != "" && dn != "" {
			if _, err := c.GetDoc(dt, dn); err == nil {
				allowed = true
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if !allowed {
		writeErr(w, cerr.Permission("Sem permissão para este arquivo"))
		return
	}
	serveUpload(w, r, "/private/files/", filepath.Join(s.dataDir(), "files", "private"))
}

// ------------------------------------------------------------------ app assets & desk

func deskIncludes(a *engine.AppMeta) []string {
	var out []string
	if a.Desk == nil {
		return nil
	}
	if inc, ok := a.Desk["include"].([]any); ok {
		for _, i := range inc {
			out = append(out, fmt.Sprint(i))
		}
	}
	return out
}

// appAsset serves compiled client code:
//
//	/assets/apps/<app>/forms/<doctype_snake>.js  – form script
//	/assets/apps/<app>/desk.js                    – app-wide includes
//	/assets/apps/<app>/static/*                   – files under <app>/public
func (s *Server) appAsset(w http.ResponseWriter, r *http.Request) {
	appName := chi.URLParam(r, "app")
	rest := chi.URLParam(r, "*")
	app := s.E.App(appName)
	if app.Name == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(rest, "static/") {
		p := filepath.Join(app.Dir, "public", strings.TrimPrefix(rest, "static/"))
		http.ServeFile(w, r, p)
		return
	}
	key := "asset:" + appName + "/" + rest
	if !s.E.Cfg.Dev {
		if v, ok := s.E.Cache.Get(key); ok {
			serveJS(w, v.(string))
			return
		}
	}
	var code string
	var err error
	switch {
	case rest == "desk.js":
		code, err = s.buildDeskInclude(app)
	case strings.HasPrefix(rest, "forms/") && strings.HasSuffix(rest, ".js"):
		snake := strings.TrimSuffix(strings.TrimPrefix(rest, "forms/"), ".js")
		var entry string
		for _, f := range js.ListFiles(app, ".form.ts") {
			if strings.HasSuffix(f, "/"+snake+".form.ts") || f == snake+".form.ts" {
				entry = f
			}
		}
		if entry == "" {
			code = "export {};"
		} else {
			code, err = js.BuildClient(app, entry)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(500)
		fmt.Fprintf(w, "console.error(%q);", err.Error())
		return
	}
	s.E.Cache.Set(key, code, 0)
	serveJS(w, code)
}

func (s *Server) buildDeskInclude(app js.App) (string, error) {
	inc := deskIncludes(s.E.Snap.Apps[app.Name])
	if len(inc) == 0 {
		return "export {};", nil
	}
	// generate an entry importing every include
	var b strings.Builder
	for _, f := range inc {
		fmt.Fprintf(&b, "import %q;\n", "./"+strings.TrimPrefix(f, "./"))
	}
	tmp := filepath.Join(app.Dir, ".ddcore")
	os.MkdirAll(tmp, 0o755)
	entry := filepath.Join(tmp, "desk.entry.ts")
	// includes are relative to app dir, so write the entry as ../
	var b2 strings.Builder
	for _, f := range inc {
		fmt.Fprintf(&b2, "import %q;\n", "../"+strings.TrimPrefix(f, "./"))
	}
	_ = b
	if err := os.WriteFile(entry, []byte(b2.String()), 0o644); err != nil {
		return "", err
	}
	return js.BuildClient(app, ".ddcore/desk.entry.ts")
}

func serveJS(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	io.WriteString(w, code)
}

// deskHandler serves the SPA: static files when they exist, index.html otherwise.
func (s *Server) deskHandler(w http.ResponseWriter, r *http.Request) {
	if s.Desk == nil {
		http.Error(w, "desk não compilado: rode `make desk` ou use a API", 404)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, cerr.NotFound("rota %s não existe", r.URL.Path))
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.Desk.Open(p); err == nil {
		st, _ := f.Stat()
		f.Close()
		if st != nil && !st.IsDir() {
			if strings.HasPrefix(p, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.FileServer(http.FS(s.Desk)).ServeHTTP(w, r)
			return
		}
	}
	data, err := fs.ReadFile(s.Desk, "index.html")
	if err != nil {
		http.Error(w, "index.html ausente", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

var _ = errors.New
var _ = db.Str
