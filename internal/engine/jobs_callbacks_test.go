package engine

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"
	"time"
)

// jobHookFiles plants a Job Trace DocType and a service whose job body and
// callbacks each leave a row in it. A row is the only honest witness here: what
// the tests need to prove is which of those writes committed, and which rolled
// back with the job.
var jobHookFiles = map[string]string{
	"doctypes/job_trace/job_trace.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Job Trace", fields: [
  { fieldname: "hook", fieldtype: "Data", label: "Hook" },
  { fieldname: "job", fieldtype: "Small Text", label: "Job" } ] });`,
	"services/hooks.ts": `function trace(hook: string, job: any) { ddcore.newDoc("Job Trace", { hook, job: JSON.stringify(job) }).insert(); }
export function started(args: any, job: any) { trace("start", job); if (args.startBoom) throw new Error("start failed"); }
export function failed(args: any, job: any) { trace("failure", job); if (args.failureBoom) throw new Error("failure hook failed"); }
export function boom(args: any) { trace("body", {}); throw new Error("body failed"); }
export function spin(args: any) { trace("body", {}); let n = 0; while (true) { n++ } }
export function ok(args: any) { trace("body", {}); return { ok: true } }`,
}

type jobTrace struct {
	hook string
	job  map[string]any
}

// traces returns the committed Job Trace rows, oldest first.
func traces(t *testing.T, e *Engine) []jobTrace {
	t.Helper()
	rows, err := e.DB.Pool.Query(context.Background(),
		`SELECT hook, job FROM tab_job_trace ORDER BY creation, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []jobTrace
	for rows.Next() {
		var hook, job string
		if err := rows.Scan(&hook, &job); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal([]byte(job), &m)
		out = append(out, jobTrace{hook, m})
	}
	return out
}

func hooksOf(ts []jobTrace) string {
	var s []string
	for _, x := range ts {
		s = append(s, x.hook)
	}
	return strings.Join(s, ",")
}

func enqueueWithHooks(t *testing.T, e *Engine, method string, args, opts map[string]any) int64 {
	t.Helper()
	o := map[string]any{"onStart": "demo.services.hooks.started", "onFailure": "demo.services.hooks.failed"}
	maps.Copy(o, opts)
	var id int64
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue(method, args, o)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Enqueue stamps run_after with the process's clock and the claim compares
	// it with the database's, so a claim straight after it can miss by a
	// fraction of a millisecond. A worker simply polls again; a test cannot.
	if _, err := e.DB.Pool.Exec(context.Background(),
		`UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The reason the callbacks exist: what onStart writes is visible while the
// body is still running, and what onFailure writes survives the body's
// rollback.
func TestJobOnStartCommitsBeforeTheBodyAndOnFailureSurvivesACancel(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	ctx := context.Background()
	restore := shortenJobPolling()
	defer restore()

	id := enqueueWithHooks(t, e, "demo.services.hooks.spin", map[string]any{"name": "x"}, nil)
	done := make(chan struct{})
	go func() { defer close(done); e.runOneJob(ctx) }()

	deadline := time.Now().Add(15 * time.Second)
	for hooksOf(traces(t, e)) != "start" {
		if time.Now().After(deadline) {
			t.Fatalf("onStart's write never became visible: %q", hooksOf(traces(t, e)))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s := readJob(t, e, id)["status"]; s != "running" {
		t.Fatalf("status = %v while onStart has committed, want running", s)
	}
	start := traces(t, e)[0].job
	if toFloat(start["id"]) != float64(id) || start["method"] != "demo.services.hooks.spin" ||
		toFloat(start["attempt"]) != 1 || toFloat(start["maxAttempts"]) != 3 || start["queue"] != "default" {
		t.Errorf("onStart got job %v", start)
	}

	if _, err := e.CancelJob(ctx, id, "ana@x.com"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the worker never returned from the cancelled job")
	}
	ts := traces(t, e)
	// The body's own trace rolled back with it.
	if hooksOf(ts) != "start,failure" {
		t.Fatalf("traces = %q, want start,failure", hooksOf(ts))
	}
	f := ts[1].job
	if f["reason"] != "cancelled" || f["final"] != true || !strings.Contains(f["error"].(string), "ana@x.com") {
		t.Errorf("onFailure got %v", f)
	}
}

// onFailure runs once per failed attempt, and final says whether a retry is
// coming, so an app can wait for the last one.
func TestJobOnFailureRunsPerAttemptAndMarksTheFinalOne(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	ctx := context.Background()
	id := enqueueWithHooks(t, e, "demo.services.hooks.boom", nil, map[string]any{"maxAttempts": 2})
	before := errorLogCount(t, e)

	if ran, err := e.runOneJob(ctx); !ran || err != nil {
		t.Fatalf("first attempt: ran=%v err=%v", ran, err)
	}
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if ran, err := e.runOneJob(ctx); !ran || err != nil {
		t.Fatalf("second attempt: ran=%v err=%v", ran, err)
	}

	ts := traces(t, e)
	if hooksOf(ts) != "start,failure,start,failure" {
		t.Fatalf("traces = %q", hooksOf(ts))
	}
	first, last := ts[1].job, ts[3].job
	if first["final"] != false || toFloat(first["attempt"]) != 1 || first["reason"] != "error" {
		t.Errorf("first onFailure got %v", first)
	}
	if last["final"] != true || toFloat(last["attempt"]) != 2 || !strings.Contains(last["error"].(string), "body failed") {
		t.Errorf("last onFailure got %v", last)
	}
	if s := readJob(t, e, id)["status"]; s != "failed" {
		t.Errorf("status = %v, want failed", s)
	}
	// The body's failures are logged as before; the callbacks add nothing.
	if n := errorLogCount(t, e) - before; n != 2 {
		t.Errorf("error log rows = %d, want 2", n)
	}
}

func TestJobOnFailureReportsATimeout(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	enqueueWithHooks(t, e, "demo.services.hooks.spin", nil, map[string]any{"timeout": 1, "maxAttempts": 1})
	if _, err := e.runOneJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	ts := traces(t, e)
	if hooksOf(ts) != "start,failure" {
		t.Fatalf("traces = %q", hooksOf(ts))
	}
	if f := ts[1].job; f["reason"] != "timeout" || f["final"] != true {
		t.Errorf("onFailure got %v", f)
	}
}

// A throwing onStart fails the attempt before the body runs, and the document
// still hears about it.
func TestJobOnStartThatThrowsFailsTheAttempt(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	id := enqueueWithHooks(t, e, "demo.services.hooks.ok", map[string]any{"startBoom": true}, map[string]any{"maxAttempts": 1})
	if _, err := e.runOneJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	// onStart's own trace rolled back with its throw; the body never ran.
	ts := traces(t, e)
	if hooksOf(ts) != "failure" {
		t.Fatalf("traces = %q, want failure", hooksOf(ts))
	}
	if !strings.Contains(ts[0].job["error"].(string), "start failed") {
		t.Errorf("onFailure got %v", ts[0].job)
	}
	r := readJob(t, e, id)
	if r["status"] != "failed" || toFloat(r["attempts"]) != 1 {
		t.Errorf("job = %v %v, want failed after 1 attempt", r["status"], r["attempts"])
	}
}

// A throwing onFailure is a fault of its own, filed as one, and it does not
// change what the job's row says.
func TestJobOnFailureThatThrowsIsLogged(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	id := enqueueWithHooks(t, e, "demo.services.hooks.boom", map[string]any{"failureBoom": true}, map[string]any{"maxAttempts": 1})
	before := errorLogCount(t, e)
	if _, err := e.runOneJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := readJob(t, e, id)["status"]; s != "failed" {
		t.Errorf("status = %v, want failed", s)
	}
	var n int
	e.DB.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM tab_error_log WHERE method LIKE 'job:onFailure:%' AND error LIKE '%failure hook failed%'`).Scan(&n)
	if n != 1 || errorLogCount(t, e)-before != 2 {
		t.Errorf("onFailure rows = %d, total new = %d; want 1 and 2", n, errorLogCount(t, e)-before)
	}
}

// A worker shutting down gives the attempt back; nothing failed, so there is
// nothing to tell the document.
func TestJobShutdownDoesNotCallOnFailure(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	restore := shortenJobPolling()
	defer restore()
	id := enqueueWithHooks(t, e, "demo.services.hooks.spin", nil, nil)

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runOneJob(ctx) }()
	waitForStatus(t, e, id, "running")
	stop()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the worker never returned after shutdown")
	}
	if h := hooksOf(traces(t, e)); strings.Contains(h, "failure") {
		t.Errorf("traces = %q; a shutdown is not a failure", h)
	}
}

// The worker that ran onStart died; the sweep that recovers the job is the
// only one left to tell the document.
func TestJobExpiredLeaseCallsOnFailure(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	ctx := context.Background()
	id := enqueueWithHooks(t, e, "demo.services.hooks.ok", nil, map[string]any{"maxAttempts": 1})
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = 1,
		lease_until = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	ts := traces(t, e)
	if hooksOf(ts) != "failure" {
		t.Fatalf("traces = %q, want failure", hooksOf(ts))
	}
	if f := ts[0].job; f["final"] != true || !strings.Contains(f["error"].(string), "lease expired") {
		t.Errorf("onFailure got %v", f)
	}
}

// A job cancelled before any worker claimed it never runs, so no worker will
// call its onFailure: the cancel does.
func TestJobCancelledWhileQueuedCallsOnFailure(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	id := enqueueWithHooks(t, e, "demo.services.hooks.ok", nil, nil)
	if _, err := e.CancelJob(context.Background(), id, "ana@x.com"); err != nil {
		t.Fatal(err)
	}
	ts := traces(t, e)
	if hooksOf(ts) != "failure" {
		t.Fatalf("traces = %q, want failure", hooksOf(ts))
	}
	if f := ts[0].job; f["reason"] != "cancelled" || f["final"] != true || toFloat(f["attempt"]) != 0 {
		t.Errorf("onFailure got %v", f)
	}
}

func TestJobRetryKeepsItsCallbacks(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	ctx := context.Background()
	id := enqueueWithHooks(t, e, "demo.services.hooks.boom", nil, map[string]any{"maxAttempts": 1})
	e.runOneJob(ctx)
	act, err := e.RetryJob(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	j, err := e.GetJob(ctx, act.NewID, false)
	if err != nil {
		t.Fatal(err)
	}
	if j["on_start"] != "demo.services.hooks.started" || j["on_failure"] != "demo.services.hooks.failed" {
		t.Errorf("retried job hooks = %v / %v", j["on_start"], j["on_failure"])
	}
}

func TestEnqueueRejectsANonStringCallback(t *testing.T) {
	e := setup(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		_, err := c.Enqueue("demo.services.loop.ok", nil, map[string]any{"onStart": 42})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "onStart") {
		t.Fatalf("err = %v, want a validation error naming onStart", err)
	}
}
