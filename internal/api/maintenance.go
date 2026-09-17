package api

import (
	"net/http"
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// maintenance is the HTTP half of maintenance mode (PRD-02): while the site is
// paused, a request that can change something is refused before any handler
// runs. Reads stay open so people can still look things up during a cutover,
// and signing in and out stays open so they can get to those reads.
//
// The engine guard behind it is what actually holds: a GET method that writes
// is let through here and refused there.
func (s *Server) maintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.E.Cfg.EnforceMaintenance && !maintenanceExempt(r) {
			if st := s.E.Maintenance(r.Context()); st.Enabled {
				s.writeError(w, r, engine.MaintenanceError(st), false)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func maintenanceExempt(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	p := r.URL.Path
	if isProbePath(p) || strings.HasPrefix(p, "/api/auth/") {
		return true
	}
	// MCP speaks POST for reads too, and its maintenance_set tool is how an
	// operator switches the pause back off: like stdio `ddcore mcp`, its tools
	// meet the engine guard instead
	if p == "/mcp" || strings.HasPrefix(p, "/mcp/") {
		return true
	}
	switch p {
	case "/api/login", "/api/logout", "/api/search/link-titles":
		return true
	}
	return false
}

// maintenanceBoot is the flag as the desk needs it: whether to show the banner
// and why. Nil when the site is open, so the common payload does not grow.
func (s *Server) maintenanceBoot(r *http.Request) any {
	st := s.E.Maintenance(r.Context())
	if !st.Enabled || !s.E.Cfg.EnforceMaintenance {
		return nil
	}
	return map[string]any{"enabled": true, "reason": st.Reason}
}
