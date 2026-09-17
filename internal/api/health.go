package api

import (
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// The probes. Two questions an orchestrator asks separately, and which this
// server answered as one until now:
//
//	liveness  — is this process still serving? Restart it if not.
//	readiness — can it do useful work right now? Stop sending it traffic if not.
//
// Conflating them is how a site with a dead database keeps receiving requests:
// the old /api/health returned {"ok": true} from a literal and never once
// looked at Postgres.
func isProbePath(p string) bool {
	// Exact equality, never a prefix. This list is an authentication bypass,
	// and a prefix match on "/api/health" would hand /api/health/report — the
	// one that carries queue depth and error counts — to anyone.
	switch p {
	case "/healthz", "/readyz", "/api/health", "/api/ready",
		// The trailing-slash spellings are registered too, and must be listed
		// here for the same reason the others are.
		"/healthz/", "/readyz/", "/api/health/", "/api/ready/":
		return true
	}
	return false
}

// liveness never touches the database. A process that can run this handler is
// a process worth leaving alone; killing it because Postgres is down would only
// mean that a database outage restarts every application server as well.
func (s *Server) liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "alive"})
}

// readiness answers with a boolean and nothing else.
//
// The temptation is to return the latency, the version and the queue depth,
// because they are already at hand. Each one is a gift to someone probing the
// site: latency is a feedback channel that lets an attacker measure their own
// effect and tune it, queue depth leaks business volume and says when the site
// is under load, the version says which CVEs apply, and a pg error string
// carries host, port, user and database name. An orchestrator reads the status
// code and nothing else, so the disclosure buys nobody anything.
func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	if h := s.E.Ready(r.Context()); !h.OK {
		// The reason is a closed vocabulary, not the error text.
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok": false, "status": "unready", "reason": "database"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ready"})
}

// healthReport is the same picture with the numbers in it, for someone who can
// already read the Error Log.
func (s *Server) healthReport(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if !c.HasRole("System Manager") {
			return nil, cerr.Permission("This report requires the System Manager role")
		}
		h := s.E.Health(r.Context(), engine.HealthOpts{Queue: true, Errors: true})
		if st := s.E.Maintenance(r.Context()); st.Enabled {
			h.Maintenance = &st
		}
		return h, nil
	})
}
