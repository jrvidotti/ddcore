package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CORS lets a page on another origin call a whitelisted method (#68) — and
// only one that opted in with `cors: true`, from an origin the site lists in
// cors.origins. Everything else under /api answers as if CORS did not exist:
// a browser then refuses to hand the response to the other page, which is the
// protection every other route keeps.
//
// Credentials are never allowed. A cross-origin caller authenticates with an
// API key or comes as Guest; a session cookie riding along on a request the
// visitor did not mean to make is exactly what CORS exists to stop.

const (
	corsAllowHeaders = "Authorization, Content-Type, X-Tenant"
	corsMaxAge       = "600"
)

// corsMethod is the method a CORS request is about, if it opted in and the
// request can reach it. The name is the route's {path}, decoded as the method
// handler decodes it; a sub-path counts only for a method that takes one.
func (s *Server) corsMethod(r *http.Request) (map[string]any, bool) {
	opts, ok := s.E.Whitelisted(urlParam(r, "path"))
	if !ok || opts["cors"] != true {
		return nil, false
	}
	if urlParam(r, "*") != "" && opts["pathTail"] != true {
		return nil, false
	}
	return opts, true
}

// crossOrigin is the Origin of a request that is CORS at all: one that names
// an origin other than the one it was sent to.
func crossOrigin(r *http.Request) string {
	o := r.Header.Get("Origin")
	if o == "" {
		return ""
	}
	if u, err := url.Parse(o); err == nil && strings.EqualFold(u.Host, r.Host) {
		return ""
	}
	return o
}

// corsPreflight answers the browser's OPTIONS before a cross-origin call.
func (s *Server) corsPreflight(w http.ResponseWriter, r *http.Request) {
	origin := crossOrigin(r)
	opts, ok := s.corsMethod(r)
	if origin == "" || r.Header.Get("Access-Control-Request-Method") == "" || !ok || !originAllowed(s.E.Cfg.CORS.Origins, origin) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Methods", corsMethods(opts))
	h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
	h.Set("Access-Control-Max-Age", corsMaxAge)
	h.Add("Vary", "Origin")
	w.WriteHeader(http.StatusNoContent)
}

// cors marks the actual call's response as readable by the calling origin.
// It is set before the handler runs, so an error — a 405, a validation
// message — reaches the other page as readably as a result does.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := crossOrigin(r); origin != "" {
			if _, ok := s.corsMethod(r); ok && originAllowed(s.E.Cfg.CORS.Origins, origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Expose-Headers", requestIDHeader)
				h.Add("Vary", "Origin")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// corsMethods is the method's own `methods`, which the handler enforces
// anyway, or the two verbs a whitelisted method answers.
func corsMethods(opts map[string]any) string {
	ms, _ := opts["methods"].([]any)
	if len(ms) == 0 {
		return "GET, POST"
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, strings.ToUpper(fmt.Sprint(m)))
	}
	return strings.Join(out, ", ")
}

// originAllowed matches an Origin against cors.origins: "*" is any origin,
// "https://*.example.com" any subdomain of example.com over https (not
// example.com itself), and anything else the same origin, ignoring case.
func originAllowed(allowed []string, origin string) bool {
	o := strings.ToLower(origin)
	for _, a := range allowed {
		a = strings.ToLower(a)
		if a == "*" || a == o {
			return true
		}
		scheme, domain, ok := strings.Cut(a, "://*.")
		if !ok {
			continue
		}
		host, ok := strings.CutPrefix(o, scheme+"://")
		if sub, ok2 := strings.CutSuffix(host, "."+domain); ok && ok2 && sub != "" && !strings.ContainsAny(sub, "/:@") {
			return true
		}
	}
	return false
}
