package api

import (
	"strings"
	"testing"
)

// The probes exist to be believed by something that is not a person: an
// orchestrator reads the status code and acts on it without asking anyone.
// These tests are about what it is told, and what it is deliberately not.

// Closing the pool is how an unhealthy database is produced here, and there is
// no alternative: an engine pointed at a dead DSN is never built at all,
// because db.Open pings inside engine.New and returns the error instead of an
// engine. It is contained: setup(t) drops and recreates its own database at the
// start of every test and registers its own cleanup, pgxpool.Close is
// idempotent, and no test in this package runs in parallel. Closing early even
// helps the next test's DROP DATABASE, which fails while connections are open.
func TestPRD03_LivenessDoesNotTouchTheDatabase(t *testing.T) {
	x := setup(t)
	x.e.DB.Pool.Close()
	// The trailing-slash spellings answer too. An unmatched path falls through
	// to the desk's SPA handler, which answers 200 with index.html — so a probe
	// configured as "/readyz/" would pass forever, database down included.
	for _, path := range []string{"/api/health", "/healthz", "/api/health/", "/healthz/"} {
		r := x.call("GET", path, nil, "")
		x.expect(r, 200, "")
		if r.Body["ok"] != true || r.Body["status"] != "alive" {
			t.Fatalf("%s: expected an alive process, got %s", path, r.Raw)
		}
	}
}

func TestPRD03_ReadinessAnswersEverySpelling(t *testing.T) {
	x := setup(t)
	x.e.DB.Pool.Close()
	for _, path := range []string{"/api/ready", "/readyz", "/api/ready/", "/readyz/"} {
		r := x.call("GET", path, nil, "")
		if r.Status != 503 {
			t.Fatalf("%s: a dead database must answer 503, got %d: %s", path, r.Status, r.Raw)
		}
	}
}

func TestPRD03_ReadinessFailsWhenTheDatabaseIsDown(t *testing.T) {
	x := setup(t)
	// Closed before the first probe, so the answer is measured and not the
	// second-long memoisation of an earlier one. That staleness is deliberate
	// — see Engine.Ready — and is why this test does not probe twice.
	x.e.DB.Pool.Close()
	r := x.call("GET", "/readyz", nil, "")
	if r.Status != 503 {
		t.Fatalf("a dead database must answer 503, got %d: %s", r.Status, r.Raw)
	}
	if r.Body["ok"] != false || r.Body["status"] != "unready" || r.Body["reason"] != "database" {
		t.Fatalf("unexpected body: %s", r.Raw)
	}
}

// The readiness body is a boolean on purpose. Latency is a feedback channel for
// anyone measuring their own effect on the site, and a pg error string carries
// the host, port, user and database name.
func TestPRD03_ReadinessLeaksNothingAboutTheDatabase(t *testing.T) {
	x := setup(t)
	x.e.DB.Pool.Close()
	raw := strings.ToLower(x.call("GET", "/readyz", nil, "").Raw)
	for _, leak := range []string{"postgres", "latency", "5455", "localhost", "dial", "conns", "password"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("readiness body leaked %q: %s", leak, raw)
		}
	}
}

// An expired key on a monitoring agent must not make a healthy process look
// dead: s.auth answers 401 for a bad token before the handler runs, on every
// other route alike.
func TestPRD03_ProbesIgnoreABadToken(t *testing.T) {
	x := setup(t)
	// A healthy site answers ready; the rest of this test is about the token.
	if r := x.call("GET", "/readyz", nil, ""); r.Status != 200 || r.Body["status"] != "ready" {
		t.Fatalf("a healthy site must answer ready, got %d %s", r.Status, r.Raw)
	}
	const bad = "token:garbage:garbage"
	for _, path := range []string{"/healthz", "/readyz", "/api/health", "/api/ready"} {
		if r := x.call("GET", path, nil, bad); r.Status != 200 {
			t.Fatalf("%s with a bad token: expected 200, got %d %s", path, r.Status, r.Raw)
		}
	}
	// The allowlist must not have grown into anything that tells a stranger
	// something. A bad token on a real route is still a 401.
	x.expect(x.call("GET", "/api/boot", nil, bad), 401, "AuthenticationError")
	x.expect(x.call("GET", "/api/health/report", nil, bad), 401, "AuthenticationError")
}

func TestPRD03_HealthReportRequiresSystemManager(t *testing.T) {
	x := setup(t)
	x.expect(x.call("GET", "/api/health/report", nil, ""), 401, "AuthenticationError")
	x.expect(x.call("GET", "/api/health/report", nil, "sid:"+x.sid("ana@x.com")), 403, "PermissionError")

	r := x.call("GET", "/api/health/report", nil, "sid:"+x.sid("root@x.com"))
	x.expect(r, 200, "")
	data, ok := r.Body["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data in %s", r.Raw)
	}
	if data["status"] != "ok" {
		t.Fatalf("expected a healthy site, got %v: %s", data["status"], r.Raw)
	}
	for _, key := range []string{"database", "queue", "errors", "scheduler"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("report is missing %q: %s", key, r.Raw)
		}
	}
}
