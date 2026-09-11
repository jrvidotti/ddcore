package api

import (
	"context"
	"strings"
	"testing"
)

// plantJob puts one row in ddcore_job directly: the states job administration
// exists for — a failure, a job still queued — are not ones the HTTP surface can
// create, and this suite is about who may read and change them.
func plantJob(t *testing.T, x *env, status, method, args string) int64 {
	t.Helper()
	var id int64
	err := x.e.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO ddcore_job (method, args, queue, status, finished, result)
		 VALUES ($1, $2, 'default', $3, CASE WHEN $3 = 'queued' THEN NULL ELSE now() END, '"done"')
		 RETURNING id`, method, args, status).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Every job route is administration and none of it is for ordinary users: a
// listing names the methods a site runs and the users they run as, and the
// actions stop and re-run work.
func TestJobRoutesRequireSystemManager(t *testing.T) {
	x := setup(t)
	id := plantJob(t, x, "failed", "demo.services.diag.ping", "{}")

	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/jobs", nil},
		{"GET", "/api/jobs/stats", nil},
		{"GET", "/api/jobs/1", nil},
		{"POST", "/api/jobs/1/retry", map[string]any{}},
		{"POST", "/api/jobs/1/cancel", map[string]any{}},
		{"POST", "/api/jobs/purge", map[string]any{"dryRun": true}},
	} {
		// Signed in, but with no role that administers anything.
		r := x.call(c.method, c.path, c.body, "sid:"+x.sid("ze@x.com"))
		x.expect(r, 403, "PermissionError")

		// And anonymous callers never reach the handler at all.
		r = x.call(c.method, c.path, c.body, "")
		if r.Status != 401 && r.Status != 403 {
			t.Errorf("%s %s as a guest: %d %s", c.method, c.path, r.Status, r.Raw)
		}
	}

	// The same routes answer a System Manager.
	root := "sid:" + x.sid("root@x.com")
	x.expect(x.call("GET", "/api/jobs", nil, root), 200, "")
	x.expect(x.call("GET", "/api/jobs/stats", nil, root), 200, "")
	if r := x.call("GET", "/api/jobs/1", nil, root); r.Status != 200 {
		t.Errorf("a System Manager should read job 1: %d %s", r.Status, r.Raw)
	}
	_ = id
}

// The listing is the one job surface that reaches a browser, and ddcore_job
// holds live secrets: the framework's own mail job is enqueued with the rendered
// message, so a queued password-reset carries its recovery link in args.html.
// Payloads are readable on the CLI, where the caller already holds the database,
// and nowhere else.
func TestJobAPINeverReturnsPayloads(t *testing.T) {
	x := setup(t)
	const secret = "https://site/reset?token=SHOULD-NOT-LEAK"
	id := plantJob(t, x, "failed", "core.services.mail.send", `{"html":"`+secret+`"}`)
	root := "sid:" + x.sid("root@x.com")

	for _, path := range []string{"/api/jobs", "/api/jobs/stats"} {
		r := x.call("GET", path, nil, root)
		x.expect(r, 200, "")
		assertNoPayload(t, path, r.Raw, secret)
	}

	r := x.call("GET", "/api/jobs/1", nil, root)
	x.expect(r, 200, "")
	assertNoPayload(t, "/api/jobs/{id}", r.Raw, secret)
	// It must still be useful: the metadata is the whole point of the route.
	if !strings.Contains(r.Raw, "core.services.mail.send") {
		t.Errorf("the job route dropped the metadata too: %s", r.Raw)
	}
	_ = id
}

func assertNoPayload(t *testing.T, where, raw, secret string) {
	t.Helper()
	if strings.Contains(raw, secret) {
		t.Errorf("%s leaked a job payload: %s", where, raw)
	}
	for _, field := range []string{`"args"`, `"result"`} {
		if strings.Contains(raw, field) {
			t.Errorf("%s exposed %s: %s", where, field, raw)
		}
	}
}

// The actions have to work through the API, not merely be reachable.
func TestJobRetryAndCancelOverHTTP(t *testing.T) {
	x := setup(t)
	root := "sid:" + x.sid("root@x.com")
	failed := plantJob(t, x, "failed", "demo.services.diag.ping", "{}")
	queued := plantJob(t, x, "queued", "demo.services.diag.ping", "{}")

	r := x.call("POST", "/api/jobs/1/retry", map[string]any{}, root)
	x.expect(r, 200, "")
	if !strings.Contains(r.Raw, "newId") {
		t.Errorf("retry should report the job it created: %s", r.Raw)
	}

	r = x.call("POST", "/api/jobs/2/cancel", map[string]any{}, root)
	x.expect(r, 200, "")
	if !strings.Contains(r.Raw, "cancelled") {
		t.Errorf("cancelling a queued job should report it cancelled: %s", r.Raw)
	}
	_, _ = failed, queued
}
