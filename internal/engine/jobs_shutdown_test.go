package engine

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// shutdownFiles adds to jobHookFiles a job that waits on an outbound HTTP
// call, which is where a job that talks to somebody else's server spends its
// time — and where the interrupt used to be unable to reach it.
var shutdownFiles = func() map[string]string {
	files := maps.Clone(jobHookFiles)
	files["services/slow.ts"] = `export function get(args: any) {
  ddcore.newDoc("Job Trace", { hook: "body", job: "{}" }).insert();
  const r = ddcore.http.get(args.url, { timeout: 600 });
  return { status: r.status };
}`
	return files
}()

// blockingServer answers only once release is called, or never if the client
// gives up first. requests receives one value per request it got.
func blockingServer(t *testing.T) (srv *httptest.Server, requests chan struct{}, release func()) {
	t.Helper()
	requests = make(chan struct{}, 16)
	gate := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		select {
		case <-gate:
			w.Write([]byte("ok"))
		case <-r.Context().Done():
		}
	}))
	// Cleanups run last in, first out: release first, so Close does not wait
	// on a handler a broken implementation left blocked.
	t.Cleanup(srv.Close)
	t.Cleanup(release)
	return srv, requests, release
}

func waitForRequest(t *testing.T, requests chan struct{}) {
	t.Helper()
	select {
	case <-requests:
	case <-time.After(15 * time.Second):
		t.Fatal("the job never made its HTTP call")
	}
}

// The case of #99: a job blocked inside ddcore.http when the process is told
// to stop. After the grace period the call is cut, the job goes back to the
// queue with its attempt given back, and onFailure is not called — all of it
// promptly, and not two minutes later when the lease expires.
func TestWorkersStopGivesBackAJobBlockedInHTTP(t *testing.T) {
	e := setupWith(t, shutdownFiles)
	restore := shortenJobPolling()
	defer restore()
	srv, requests, _ := blockingServer(t)

	id := enqueueWithHooks(t, e, "demo.services.slow.get", map[string]any{"url": srv.URL},
		map[string]any{"maxAttempts": 1})
	before := errorLogCount(t, e)
	w := e.StartWorkers(1)
	waitForStatus(t, e, id, "running")
	waitForRequest(t, requests)

	grace := 300 * time.Millisecond
	start := time.Now()
	w.Stop(grace)
	if took := time.Since(start); took > grace+2*time.Second {
		t.Errorf("Stop took %v; a job blocked in HTTP must be cut right after the grace", took)
	}

	r := readJob(t, e, id)
	if r["status"] != "queued" {
		t.Errorf("status = %v, want queued: a stopped worker gives the job back", r["status"])
	}
	if n := toFloat(r["attempts"]); n != 0 {
		t.Errorf("attempts = %v, want 0: shutdown must not consume an attempt", n)
	}
	if h := hooksOf(traces(t, e)); strings.Contains(h, "failure") {
		t.Errorf("traces = %q; a shutdown is not a failure", h)
	}
	if n := errorLogCount(t, e); n != before {
		t.Errorf("a shutdown wrote %d Error Log row(s); it is not a failure", n-before)
	}
}

// The grace period is for the job to finish: one that does, within it, commits
// as if nothing happened, and Stop does not wait out the rest of the grace.
func TestWorkersStopLetsARunningJobFinishWithinTheGrace(t *testing.T) {
	e := setupWith(t, shutdownFiles)
	restore := shortenJobPolling()
	defer restore()
	srv, requests, release := blockingServer(t)

	id := enqueueWithHooks(t, e, "demo.services.slow.get", map[string]any{"url": srv.URL}, nil)
	w := e.StartWorkers(1)
	waitForStatus(t, e, id, "running")
	waitForRequest(t, requests)

	stopped := make(chan time.Time)
	go func() { w.Stop(10 * time.Second); stopped <- time.Now() }()
	time.Sleep(200 * time.Millisecond)
	released := time.Now()
	release()
	select {
	case at := <-stopped:
		if at.Sub(released) > 5*time.Second {
			t.Errorf("Stop returned %v after the job could finish; it must not wait out the grace", at.Sub(released))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Stop never returned")
	}
	r := readJob(t, e, id)
	if r["status"] != "done" {
		t.Errorf("status = %v, want done: the job finished within the grace", r["status"])
	}
	if n := toFloat(r["attempts"]); n != 1 {
		t.Errorf("attempts = %v, want 1", n)
	}
}

// Once Stop is called nothing new is claimed, even by a worker that is idle
// and even while another one is still finishing its job.
func TestWorkersStopClaimsNothingDuringTheGrace(t *testing.T) {
	e := setupWith(t, shutdownFiles)
	restore := shortenJobPolling()
	defer restore()
	srv, requests, release := blockingServer(t)

	slow := enqueueWithHooks(t, e, "demo.services.slow.get", map[string]any{"url": srv.URL}, nil)
	w := e.StartWorkers(2)
	waitForStatus(t, e, slow, "running")
	waitForRequest(t, requests)

	stopped := make(chan struct{})
	go func() { defer close(stopped); w.Stop(10 * time.Second) }()
	time.Sleep(100 * time.Millisecond)
	late := enqueueWithHooks(t, e, "demo.services.hooks.ok", nil, nil)
	// Long enough for an idle worker that still polled to have claimed it.
	time.Sleep(500 * time.Millisecond)
	release()
	select {
	case <-stopped:
	case <-time.After(15 * time.Second):
		t.Fatal("Stop never returned")
	}
	if s := readJob(t, e, slow)["status"]; s != "done" {
		t.Errorf("the running job ended %v, want done", s)
	}
	r := readJob(t, e, late)
	if r["status"] != "queued" || toFloat(r["attempts"]) != 0 {
		t.Errorf("a job queued after Stop was claimed: status %v, attempts %v", r["status"], r["attempts"])
	}
}

// The job's timeout reaches the HTTP call too: a job blocked in ddcore.http
// fails when its timeout says so, not when the remote server answers.
func TestJobTimeoutCutsABlockedHTTPCall(t *testing.T) {
	e := setupWith(t, shutdownFiles)
	srv, requests, _ := blockingServer(t)

	id := enqueueWithHooks(t, e, "demo.services.slow.get", map[string]any{"url": srv.URL},
		map[string]any{"maxAttempts": 1, "timeout": 1})
	done := make(chan struct{})
	go func() { defer close(done); e.runOneJob(context.Background()) }()
	waitForRequest(t, requests)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the job's timeout did not cut the HTTP call")
	}
	r := readJob(t, e, id)
	if r["status"] != "failed" || !strings.Contains(db2str(r["error"]), "timed out") {
		t.Errorf("status %v, error %v; want failed, timed out", r["status"], r["error"])
	}
	ts := traces(t, e)
	if len(ts) == 0 || ts[len(ts)-1].hook != "failure" || ts[len(ts)-1].job["reason"] != "timeout" {
		t.Errorf("onFailure was not told of the timeout: %v", ts)
	}
	if len(ts) > 0 && toFloat(ts[len(ts)-1].job["starts"]) != 1 {
		t.Errorf("onFailure was told starts %v, want 1", ts[len(ts)-1].job["starts"])
	}
}

func db2str(v any) string {
	s, _ := v.(string)
	return s
}
