package engine

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"
)

// startsFiles adds a job body that reports what ddcore.job.current() told it,
// and only waits on its URL the first time it starts — the shape of a job that
// guards a non-idempotent call with `starts > 1`.
var startsFiles = func() map[string]string {
	files := maps.Clone(shutdownFiles)
	files["services/starts.ts"] = `export function record(args: any) {
  const job = ddcore.job.current();
  ddcore.newDoc("Job Trace", { hook: "body", job: JSON.stringify(job) }).insert();
  if (args.url && job && job.starts === 1) ddcore.http.get(args.url, { timeout: 600 });
  return job;
}
export function current() { return { job: ddcore.job.current() }; }`
	return files
}()

// starts counts every time a worker began the job; attempt counts the ones
// that were spent. On a first run they agree, and the body sees the same
// JobInfo its onStart was given.
func TestJobStartsIsOneOnTheFirstRun(t *testing.T) {
	e := setupWith(t, startsFiles)
	id := enqueueWithHooks(t, e, "demo.services.starts.record", map[string]any{}, nil)
	if ran, err := e.runOneJob(context.Background()); !ran || err != nil {
		t.Fatalf("runOneJob: %v %v", ran, err)
	}
	if s := readJob(t, e, id)["status"]; s != "done" {
		t.Fatalf("status = %v (%v), want done", s, readJob(t, e, id)["error"])
	}
	ts := traces(t, e)
	if hooksOf(ts) != "start,body" {
		t.Fatalf("traces = %q, want start,body", hooksOf(ts))
	}
	for _, tr := range ts {
		if toFloat(tr.job["starts"]) != 1 || toFloat(tr.job["attempt"]) != 1 || toFloat(tr.job["id"]) != float64(id) {
			t.Errorf("%s was told %v; want starts 1, attempt 1, id %d", tr.hook, tr.job, id)
		}
	}
	if body := ts[1].job; body["method"] != "demo.services.starts.record" || toFloat(body["maxAttempts"]) != 3 {
		t.Errorf("ddcore.job.current() = %v", body)
	}
	var starts int
	if err := e.DB.Pool.QueryRow(context.Background(), `SELECT starts FROM ddcore_job WHERE id = $1`, id).
		Scan(&starts); err != nil || starts != 1 {
		t.Errorf("starts column = %d (%v), want 1", starts, err)
	}
}

// The case of #91: a worker shutting down gave the attempt back, so attempt
// alone cannot tell the second run from the first. starts can.
func TestJobStartsCountsAGivenBackRun(t *testing.T) {
	e := setupWith(t, startsFiles)
	restore := shortenJobPolling()
	defer restore()
	srv, requests, _ := blockingServer(t)

	id := enqueueWithHooks(t, e, "demo.services.starts.record", map[string]any{"url": srv.URL},
		map[string]any{"maxAttempts": 1})
	w := e.StartWorkers(1)
	waitForStatus(t, e, id, "running")
	waitForRequest(t, requests)
	w.Stop(50 * time.Millisecond)
	r := readJob(t, e, id)
	if r["status"] != "queued" || toFloat(r["attempts"]) != 0 {
		t.Fatalf("after shutdown: status %v, attempts %v; want queued, 0", r["status"], r["attempts"])
	}

	if _, err := e.DB.Pool.Exec(context.Background(),
		`UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if ran, err := e.runOneJob(context.Background()); !ran || err != nil {
		t.Fatalf("runOneJob: %v %v", ran, err)
	}
	if s := readJob(t, e, id)["status"]; s != "done" {
		t.Fatalf("second run ended %v, want done", s)
	}
	// The first body rolled back with its trace; both onStarts committed.
	ts := traces(t, e)
	if hooksOf(ts) != "start,start,body" {
		t.Fatalf("traces = %q, want start,start,body", hooksOf(ts))
	}
	if first := ts[0].job; toFloat(first["starts"]) != 1 || toFloat(first["attempt"]) != 1 {
		t.Errorf("first onStart was told %v; want starts 1, attempt 1", first)
	}
	for _, tr := range ts[1:] {
		if toFloat(tr.job["starts"]) != 2 || toFloat(tr.job["attempt"]) != 1 {
			t.Errorf("second run's %s was told %v; want starts 2, attempt 1", tr.hook, tr.job)
		}
	}
}

// Outside a job there is no job to describe: a request, and a function run by
// hand with `ddcore jobs run`, both get null.
func TestJobCurrentIsNullOutsideAJob(t *testing.T) {
	e := setupWith(t, startsFiles)
	ctx := context.Background()
	var out json.RawMessage
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		out, err = rt.CallFunction("demo.services.starts.current", json.RawMessage(`{}`))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"job":null}` {
		t.Errorf("in a request, ddcore.job.current() gave %s, want null", out)
	}
	res, err := e.RunJob(ctx, "Admin", "demo.services.starts.current", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != `{"job":null}` {
		t.Errorf("in `jobs run`, ddcore.job.current() gave %s, want null", res)
	}
}
