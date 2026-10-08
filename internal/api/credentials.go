package api

import (
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// Sign-in through an app's credential provider (#115): the person picks a
// tenant, types the username and password of the app's system, and the
// answer is the same session cookie /api/login sets. Both endpoints live
// under /api/auth/, so they need no CSRF header (there is no session yet)
// and stay open in maintenance, like the password sign-in.

// credentialLogin signs in through a provider.
func (s *Server) credentialLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tenant string `json:"tenant"`
		Usr    string `json:"usr"`
		Pwd    string `json:"pwd"`
	}
	if err := readJSON(r, &body); err != nil {
		s.writeErr(w, r, err)
		return
	}
	sid, err := s.E.CredentialLogin(r.Context(), urlParam(r, "provider"), body.Tenant, body.Usr, body.Pwd, engine.LoginFrom{
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	home := "/app"
	if u, _ := s.E.UserFromSession(r.Context(), sid); u != "" {
		s.E.Cache.Del("lang:" + u)
		home = s.homeFor(r, u, "")
	}
	http.SetCookie(w, s.sessionCookie(r, sid))
	writeJSON(w, 200, map[string]any{"data": map[string]any{"ok": true, "home": home}})
}

// credentialTenants never lists anything (#117): it only says whether the
// site has tenancy, for a desk built when it did list the tenants.
func (s *Server) credentialTenants(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.E.Current().CredentialProvider(urlParam(r, "provider")); !ok {
		s.writeErr(w, r, cerr.NotFound("Unknown sign-in provider"))
		return
	}
	writeJSON(w, 200, map[string]any{"data": []any{}, "tenancy": s.E.Cfg.Tenancy})
}

// credentialTenant is one organization's sign-in page data: its title, when
// it exists and offers the provider.
func (s *Server) credentialTenant(w http.ResponseWriter, r *http.Request) {
	out, tenancy, err := s.E.CredentialTenant(r.Context(), urlParam(r, "provider"), urlParam(r, "tenant"), clientIP(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": out, "tenancy": tenancy})
}

// credentialProvidersBoot is what the sign-in screen needs of each provider:
// its id and its label in the reader's language.
func credentialProvidersBoot(c *engine.Ctx) []map[string]any {
	out := []map[string]any{}
	for _, p := range c.St.CredentialProviders() {
		out = append(out, map[string]any{"id": p.ID, "label": c.T(p.Label)})
	}
	return out
}
