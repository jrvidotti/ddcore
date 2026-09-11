package engine

import (
	"context"
	"strings"
	"testing"
	"time"
)

// plantJob writes one ddcore_job row with whatever columns the case needs and
// returns its id. It exists alongside health_test.go's `job` because the states
// worth testing here — a pending cancellation, an exhausted attempt count — are
// ones Enqueue cannot produce, and because the id is the handle every
// administrative operation takes.
func plantJob(t *testing.T, e *Engine, cols map[string]any) int64 {
	t.Helper()
	get := func(k string, d any) any {
		if v, ok := cols[k]; ok {
			return v
		}
		return d
	}
	var id int64
	err := e.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO ddcore_job (method, args, queue, "user", status, run_after, enqueued,
			started, finished, lease_until, attempts, max_attempts, error, cancel_requested)
		 VALUES ($1, $2, $3, $4, $5, COALESCE($6, now()), COALESCE($7, now()),
		         $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
		get("method", "demo.services.loop.ok"), get("args", nil), get("queue", "default"),
		get("user", "Administrator"), get("status", "queued"),
		get("run_after", nil), get("enqueued", nil), get("started", nil), get("finished", nil),
		get("lease_until", nil), get("attempts", 0), get("max_attempts", 3),
		get("error", nil), get("cancel_requested", nil)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// readJob returns the row as a map, for assertions that span several columns.
func readJob(t *testing.T, e *Engine, id int64) map[string]any {
	t.Helper()
	rows, err := e.DB.Pool.Query(context.Background(),
		`SELECT status, error, started, finished, run_after, attempts, cancelled_by,
		        cancel_requested, retry_of, retried_as
		   FROM ddcore_job WHERE id = $1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("job %d not found", id)
	}
	vals, err := rows.Values()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for i, f := range rows.FieldDescriptions() {
		out[string(f.Name)] = vals[i]
	}
	return out
}

// waitForStatus polls the row until it reaches want, or fails the test. Job
// administration is asynchronous by nature — the worker acts on a request it
// reads on its next tick — so the tests assert on state transitions with a
// generous deadline and never on elapsed time, which would make them flaky on a
// loaded machine without proving anything extra.
func waitForStatus(t *testing.T, e *Engine, id int64, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last any
	for time.Now().Before(deadline) {
		last = readJob(t, e, id)["status"]
		if last == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %d stayed at %v, want %s", id, last, want)
}

// shortenJobPolling makes the worker notice a cancellation in milliseconds
// instead of seconds. Without it every cancellation case costs a real poll
// interval, which is how a suite ends up with these tests skipped.
func shortenJobPolling() func() {
	poll, renew := jobCancelPoll, jobLeaseRenew
	jobCancelPoll, jobLeaseRenew = 20*time.Millisecond, 50*time.Millisecond
	return func() { jobCancelPoll, jobLeaseRenew = poll, renew }
}

func errorLogCount(t *testing.T, e *Engine) int {
	t.Helper()
	var n int
	if err := e.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM tab_error_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Cancelling a running job interrupts it where it stands.
//
// The spinner never returns on its own, so there is no race in which the job
// finishes before the cancellation lands, and the timeout is left at its
// default — far longer than this test's patience — so that reaching "cancelled"
// can only mean the cancellation did it, and not the deadline.
func TestCancelRunningJob(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	restore := shortenJobPolling()
	defer restore()

	id := plantJob(t, e, map[string]any{"method": "demo.services.loop.travar"})
	before := errorLogCount(t, e)

	done := make(chan struct{})
	go func() { defer close(done); e.runOneJob(ctx) }()

	waitForStatus(t, e, id, "running")

	act, err := e.CancelJob(ctx, id, "ana@x.com")
	if err != nil {
		t.Fatal(err)
	}
	// A running job is not stopped yet, and saying so would be a lie the caller
	// would act on: the job may even commit in the same instant.
	if act.Status != "cancelling" {
		t.Errorf("cancelling a running job reported %q, want cancelling", act.Status)
	}

	waitForStatus(t, e, id, "cancelled")
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the worker never returned from the cancelled job")
	}

	r := readJob(t, e, id)
	if n := toFloat(r["attempts"]); n != 1 {
		t.Errorf("attempts = %v, want 1: a cancelled job must not be retried", n)
	}
	if r["finished"] == nil {
		t.Error("a cancelled job must be finished")
	}
	if r["cancelled_by"] != "ana@x.com" {
		t.Errorf("cancelled_by = %v, want ana@x.com", r["cancelled_by"])
	}
	// Cancelling is an administrative act, not a fault. Logging it as one would
	// bury real errors and trip the Error Log threshold in the health report.
	if n := errorLogCount(t, e); n != before {
		t.Errorf("a cancellation wrote %d Error Log row(s)", n-before)
	}

	// The pooled VM must survive the interrupt, or the cancellation costs the
	// worker every job after it.
	res, err := e.RunJob(ctx, "Administrator", "demo.services.loop.ok", map[string]any{"x": 7})
	if err != nil || !strings.Contains(string(res), `"x":7`) {
		t.Fatalf("the runtime did not survive the cancel: %s %v", res, err)
	}
}

// Stopping the worker is not cancelling the job, and it is not a failure
// either. The job was never allowed to finish, so it goes back to the queue
// with its attempt returned — otherwise a rolling restart would eat
// max_attempts on work that has nothing wrong with it.
//
// This is the case an implementation that keys off the returned error gets
// wrong: shutdown and an administrative cancel both interrupt the VM through a
// context and both surface as context.Canceled. Only the flag separates them.
func TestWorkerShutdownRequeuesWithoutConsumingTheAttempt(t *testing.T) {
	e := setup(t)
	restore := shortenJobPolling()
	defer restore()

	id := plantJob(t, e, map[string]any{"method": "demo.services.loop.travar"})
	before := errorLogCount(t, e)

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runOneJob(ctx) }()

	waitForStatus(t, e, id, "running")
	stop() // the operator stopped the worker

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the worker never returned after shutdown")
	}

	r := readJob(t, e, id)
	if r["status"] != "queued" {
		t.Errorf("status = %v, want queued: a stopped worker gives the job back", r["status"])
	}
	if n := toFloat(r["attempts"]); n != 0 {
		t.Errorf("attempts = %v, want 0: shutdown must not consume an attempt", n)
	}
	if r["cancelled_by"] != nil {
		t.Errorf("a shutdown is not a cancellation: cancelled_by = %v", r["cancelled_by"])
	}
	if n := errorLogCount(t, e); n != before {
		t.Errorf("a shutdown wrote %d Error Log row(s); it is not a failure", n-before)
	}
}

// A queued job never starts. No worker is involved and the answer is final, so
// the caller is told "cancelled" and not "cancelling".
func TestCancelQueuedJob(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	id := plantJob(t, e, map[string]any{"method": "demo.services.loop.travar"})
	act, err := e.CancelJob(ctx, id, "ana@x.com")
	if err != nil {
		t.Fatal(err)
	}
	if act.Status != "cancelled" {
		t.Errorf("cancelling a queued job reported %q, want cancelled", act.Status)
	}

	ran, err := e.runOneJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("the worker claimed a cancelled job")
	}
}

// Cancelling something already over is not an error to swallow: the operator
// asked to stop work that had in fact completed, and needs to know that.
func TestCancelFinishedJobReportsItsStatus(t *testing.T) {
	e := setup(t)
	id := plantJob(t, e, map[string]any{"status": "done", "finished": time.Now()})

	_, err := e.CancelJob(context.Background(), id, "ana@x.com")
	if err == nil {
		t.Fatal("cancelling a done job should report that it is done")
	}
	if !strings.Contains(err.Error(), "done") {
		t.Errorf("the error should name the status it found: %v", err)
	}
}

// A listing must not carry job payloads. The framework's own mail job is
// enqueued with the rendered message in its args, so a queued password-reset
// job holds a live recovery link — and the listing is the one job surface that
// reaches a browser.
func TestListJobsNeverReturnsPayloads(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	plantJob(t, e, map[string]any{"args": `{"html":"https://site/reset?token=SECRET"}`})

	rows, err := e.ListJobs(ctx, JobFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("listed %d jobs, want 1", len(rows))
	}
	for _, col := range []string{"args", "result"} {
		if _, ok := rows[0][col]; ok {
			t.Errorf("the listing exposed %q", col)
		}
	}
	if rows[0]["method"] == nil {
		t.Error("the listing should still carry the metadata")
	}

	// The CLI, where the caller already holds the database, is the one way in.
	one, err := e.GetJob(ctx, int64(toFloat(rows[0]["id"])), true)
	if err != nil {
		t.Fatal(err)
	}
	if one["args"] == nil {
		t.Error("GetJob with payloads should return args")
	}
	bare, err := e.GetJob(ctx, int64(toFloat(rows[0]["id"])), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bare["args"]; ok {
		t.Error("GetJob without payloads exposed args")
	}
}

func TestListJobsFilters(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	plantJob(t, e, map[string]any{"status": "failed", "queue": "mail", "method": "a.b.c", "finished": time.Now()})
	plantJob(t, e, map[string]any{"status": "done", "queue": "mail", "method": "a.b.c", "finished": time.Now()})
	plantJob(t, e, map[string]any{"status": "failed", "queue": "default", "method": "x.y.z", "finished": time.Now()})

	for _, c := range []struct {
		name string
		f    JobFilter
		want int
	}{
		{"everything", JobFilter{}, 3},
		{"by status", JobFilter{Status: []string{"failed"}}, 2},
		{"by queue", JobFilter{Queue: "mail"}, 2},
		{"by method", JobFilter{Method: "a.b.c"}, 2},
		{"status and queue together", JobFilter{Status: []string{"failed"}, Queue: "mail"}, 1},
		{"nothing matches", JobFilter{Queue: "nope"}, 0},
	} {
		rows, err := e.ListJobs(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(rows) != c.want {
			t.Errorf("%s: got %d jobs, want %d", c.name, len(rows), c.want)
		}
		n, err := e.CountJobs(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if int(n) != c.want {
			t.Errorf("%s: count = %d, want %d", c.name, n, c.want)
		}
	}
}

func jobExists(t *testing.T, e *Engine, id int64) bool {
	t.Helper()
	var n int
	if err := e.DB.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ddcore_job WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// Retention keeps the table from growing forever, and must never touch work
// that has not happened. Failures outlive successes because they are the
// evidence — the same reasoning SweepAuth applies to spent recovery tokens.
func TestPurgeRemovesOnlyOldTerminalJobs(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }
	day := 24 * time.Hour

	oldDone := plantJob(t, e, map[string]any{"status": "done", "finished": ago(10 * day)})
	freshDone := plantJob(t, e, map[string]any{"status": "done", "finished": ago(1 * day)})
	oldFailed := plantJob(t, e, map[string]any{"status": "failed", "finished": ago(40 * day)})
	freshFailed := plantJob(t, e, map[string]any{"status": "failed", "finished": ago(10 * day)})
	oldCancelled := plantJob(t, e, map[string]any{"status": "cancelled", "finished": ago(40 * day)})
	queued := plantJob(t, e, map[string]any{"status": "queued"})
	// A running job finished nothing, but the column used to be stamped on the
	// requeue branch too, so an old row can carry one. Status is what decides.
	running := plantJob(t, e, map[string]any{"status": "running", "finished": ago(90 * day)})

	n, err := e.PurgeJobs(ctx, PurgeOpts{DoneDays: 7, FailedDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if n.Done != 1 || n.Failed != 2 {
		t.Errorf("purged %+v, want 1 done and 2 failed/cancelled", n)
	}

	for _, c := range []struct {
		id   int64
		name string
		want bool
	}{
		{oldDone, "a done job past its window", false},
		{freshDone, "a recent done job", true},
		{oldFailed, "a failed job past its window", false},
		{freshFailed, "a failed job inside its window", true},
		{oldCancelled, "a cancelled job past its window", false},
		{queued, "a queued job", true},
		{running, "a running job", true},
	} {
		if got := jobExists(t, e, c.id); got != c.want {
			t.Errorf("%s: exists = %v, want %v", c.name, got, c.want)
		}
	}
}

// A dry run is what makes the first purge of a long-lived table safe to look at.
func TestPurgeDryRunCountsWithoutDeleting(t *testing.T) {
	e := setup(t)
	id := plantJob(t, e, map[string]any{"status": "done", "finished": time.Now().Add(-240 * time.Hour)})

	n, err := e.PurgeJobs(context.Background(), PurgeOpts{DoneDays: 7, FailedDays: 30, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if n.Done != 1 {
		t.Errorf("a dry run must still report what it would remove: %+v", n)
	}
	if !jobExists(t, e, id) {
		t.Error("a dry run deleted a row")
	}
}

// Zero days means keep forever, and the distinction matters enough to test:
// read as a cutoff instead of a switch, it would delete the entire table.
func TestPurgeKeepsForeverWhenWindowIsZero(t *testing.T) {
	e := setup(t)
	id := plantJob(t, e, map[string]any{"status": "done", "finished": time.Now().Add(-8760 * time.Hour)})

	n, err := e.PurgeJobs(context.Background(), PurgeOpts{DoneDays: 0, FailedDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if n.Done != 0 {
		t.Errorf("purged %+v with retention disabled", n)
	}
	if !jobExists(t, e, id) {
		t.Error("a year-old job was deleted although retention was disabled")
	}
}

// max_attempts is a column with a default that nothing could set, which left
// retry policy unconfigurable: a job whose failure is permanent was retried
// three times regardless, and one worth more tries could not have them.
func TestEnqueueAcceptsMaxAttempts(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	var id int64
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, map[string]any{"maxAttempts": 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var max int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT max_attempts FROM ddcore_job WHERE id = $1`, id).Scan(&max); err != nil {
		t.Fatal(err)
	}
	if max != 1 {
		t.Errorf("max_attempts = %d, want 1", max)
	}

	// Saying nothing still means the shipped default.
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB.Pool.QueryRow(ctx, `SELECT max_attempts FROM ddcore_job WHERE id = $1`, id).Scan(&max); err != nil {
		t.Fatal(err)
	}
	if max != 3 {
		t.Errorf("max_attempts = %d without the option, want the default 3", max)
	}
}

// The aggregate in the health report counts every terminal state but the one
// this release adds, so a queue full of cancelled jobs would read as idle.
func TestQueueHealthCountsCancelledJobs(t *testing.T) {
	e := setup(t)
	plantJob(t, e, map[string]any{"status": "cancelled", "finished": time.Now()})

	q, err := e.QueueHealth(context.Background(), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if q.CancelledInWindow != 1 {
		t.Errorf("cancelledInWindow = %d, want 1", q.CancelledInWindow)
	}
}

// Metrics exist so an operator can tell which queue is behind and which method
// is failing, which the single site-wide aggregate cannot answer.
func TestJobStatsBreakDownByQueueAndMethod(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	now := time.Now()

	plantJob(t, e, map[string]any{"queue": "mail", "status": "failed", "method": "a.b.send", "finished": now, "started": now.Add(-2 * time.Second)})
	plantJob(t, e, map[string]any{"queue": "mail", "status": "done", "method": "a.b.send", "finished": now, "started": now.Add(-4 * time.Second)})
	plantJob(t, e, map[string]any{"queue": "mail", "status": "queued"})
	plantJob(t, e, map[string]any{"queue": "default", "status": "done", "method": "x.y.z", "finished": now, "started": now.Add(-1 * time.Second)})

	s, err := e.JobStats(ctx, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	byQueue := map[string]QueueStat{}
	for _, q := range s.Queues {
		byQueue[q.Queue] = q
	}
	if len(byQueue) != 2 {
		t.Fatalf("got queues %+v, want mail and default", s.Queues)
	}
	if m := byQueue["mail"]; m.Failed != 1 || m.Done != 1 || m.Queued != 1 {
		t.Errorf("mail queue = %+v, want 1 failed, 1 done, 1 queued", m)
	}

	var send *MethodStat
	for i := range s.Methods {
		if s.Methods[i].Method == "a.b.send" {
			send = &s.Methods[i]
		}
	}
	if send == nil {
		t.Fatalf("a.b.send missing from %+v", s.Methods)
	}
	if send.Runs != 2 || send.Failures != 1 {
		t.Errorf("a.b.send = %+v, want 2 runs and 1 failure", *send)
	}
	// Duration is the point of a per-method view: it is how a slow job is found.
	if send.AvgSeconds < 2 || send.AvgSeconds > 4 {
		t.Errorf("avgSeconds = %v, want between 2 and 4", send.AvgSeconds)
	}
}

// The scheduled sweep is reached as app code, through a dotted path and a host
// op, so a typo anywhere along that chain is invisible until a nightly cron
// quietly stops running. Exercise the real path rather than SweepJobs directly.
func TestScheduledSweepRunsAndPurges(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	old := plantJob(t, e, map[string]any{
		"status": "done", "finished": time.Now().Add(-365 * 24 * time.Hour),
	})

	res, err := e.RunJob(ctx, "Administrator", "core.services.jobs.sweep", nil)
	if err != nil {
		t.Fatalf("the scheduler entry does not run: %v", err)
	}
	if !strings.Contains(string(res), `"done":1`) {
		t.Errorf("the sweep reported %s, want 1 done row removed", res)
	}
	if jobExists(t, e, old) {
		t.Error("the sweep left a year-old done job behind")
	}
}

// Retrying makes a new job rather than rewinding the old one.
//
// The failed row is the audit record — its error, its timings — and
// runOneJob mints the Error Log handle as "job:<id>", promising that a handle
// names one execution. Running the same row twice would break both.
func TestRetryCreatesANewJobAndLinksBothWays(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	old := plantJob(t, e, map[string]any{
		"status": "failed", "finished": time.Now(), "attempts": 3,
		"error": "the reason it failed", "queue": "mail",
		"method": "demo.services.loop.ok", "user": "ana@x.com",
	})

	act, err := e.RetryJob(ctx, old, false)
	if err != nil {
		t.Fatal(err)
	}
	if act.NewID == 0 || act.NewID == old {
		t.Fatalf("retry returned id %d; it must be a new job", act.NewID)
	}

	o := readJob(t, e, old)
	if o["status"] != "failed" {
		t.Errorf("the failed job must stay failed as the record: %v", o["status"])
	}
	if o["error"] == nil {
		t.Error("the failed job must keep saying why it failed")
	}
	if toFloat(o["retried_as"]) != float64(act.NewID) {
		t.Errorf("retried_as = %v, want %d", o["retried_as"], act.NewID)
	}

	n := readJob(t, e, act.NewID)
	if n["status"] != "queued" {
		t.Errorf("the new job status = %v, want queued", n["status"])
	}
	if toFloat(n["retry_of"]) != float64(old) {
		t.Errorf("retry_of = %v, want %d", n["retry_of"], old)
	}
	if n["error"] != nil {
		t.Errorf("a job that has not run carries no error: %v", n["error"])
	}
	if toFloat(n["attempts"]) != 0 {
		t.Errorf("the new job starts at attempt 0, got %v", n["attempts"])
	}

	// The copy has to be faithful or the retry runs something else.
	var method, queue, user string
	if err := e.DB.Pool.QueryRow(ctx,
		`SELECT method, queue, "user" FROM ddcore_job WHERE id = $1`, act.NewID).
		Scan(&method, &queue, &user); err != nil {
		t.Fatal(err)
	}
	if method != "demo.services.loop.ok" || queue != "mail" || user != "ana@x.com" {
		t.Errorf("the retry did not copy the job: %s %s %s", method, queue, user)
	}
}

// Pressing retry twice must not fan one failure out into several jobs.
func TestRetryTwiceNeedsForce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := plantJob(t, e, map[string]any{"status": "failed", "finished": time.Now()})

	if _, err := e.RetryJob(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RetryJob(ctx, id, false); err == nil {
		t.Fatal("a second retry without --force should be refused")
	}
	if _, err := e.RetryJob(ctx, id, true); err != nil {
		t.Fatalf("--force should allow it: %v", err)
	}
}

// Only terminal, unsuccessful jobs are retryable. Retrying a queued job would
// duplicate work that is about to run; retrying a done one would repeat effects
// that already happened.
func TestRetryRefusesJobsThatAreNotFailed(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, status := range []string{"queued", "running", "done"} {
		id := plantJob(t, e, map[string]any{"status": status, "finished": time.Now()})
		if _, err := e.RetryJob(ctx, id, false); err == nil {
			t.Errorf("retrying a %s job should be refused", status)
		}
	}
}

// A job that failed but still has attempts left goes back to the queue, and a
// queued job has not finished. It was being stamped `finished = now()` on that
// branch too, which left live queued rows carrying a finish time — and any
// retention sweep keyed on `finished < cutoff` would then delete work that had
// never run.
func TestFailedButRetryableJobIsNotFinished(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	id := plantJob(t, e, map[string]any{
		"method": "demo.services.loop.naoExiste", "max_attempts": 3,
	})
	ran, err := e.runOneJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("the worker claimed nothing")
	}

	r := readJob(t, e, id)
	if r["status"] != "queued" {
		t.Fatalf("status = %v, want queued (2 attempts remain)", r["status"])
	}
	if r["finished"] != nil {
		t.Errorf("a job waiting to run again is not finished: %v", r["finished"])
	}
	if n := toFloat(r["attempts"]); n != 1 {
		t.Errorf("attempts = %v, want 1", n)
	}
}

// requeueStale has three outcomes and they had one CASE between them. A job
// whose worker died goes back to the queue, one that ran out of attempts fails,
// and — new here — one that was cancelled while running stays cancelled instead
// of being resurrected by the very sweep meant to recover crashed workers.
func TestRequeueStaleThreeOutcomes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)

	cancelled := plantJob(t, e, map[string]any{
		"status": "running", "lease_until": past, "started": past,
		"attempts": 1, "cancel_requested": past,
	})
	retried := plantJob(t, e, map[string]any{
		"status": "running", "lease_until": past, "started": past,
		"attempts": 1, "error": "the previous attempt said this",
	})
	exhausted := plantJob(t, e, map[string]any{
		"status": "running", "lease_until": past, "started": past,
		"attempts": 3, "max_attempts": 3,
	})

	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}

	if r := readJob(t, e, cancelled); r["status"] != "cancelled" {
		t.Errorf("a cancelled job whose worker died: status = %v, want cancelled", r["status"])
	} else if r["finished"] == nil {
		t.Error("a cancelled job must be finished")
	}

	r := readJob(t, e, retried)
	if r["status"] != "queued" {
		t.Errorf("a job with attempts left: status = %v, want queued", r["status"])
	}
	// The error of the *previous* attempt must not survive on a row that is
	// about to run again: it advertises a failure that has not happened yet.
	if r["error"] != nil {
		t.Errorf("a requeued job still carries error = %v", r["error"])
	}
	if r["finished"] != nil {
		t.Errorf("a requeued job must not be finished: %v", r["finished"])
	}

	if r := readJob(t, e, exhausted); r["status"] != "failed" {
		t.Errorf("a job out of attempts: status = %v, want failed", r["status"])
	} else if r["error"] == nil {
		t.Error("an exhausted job must say why it stopped")
	}
}
