package engine

import (
	"context"
	"testing"
	"time"
)

// job plants a row directly, because what is under test is the aggregate over
// ddcore_job and not the worker that fills it. Enqueue cannot produce a stalled
// lease or a failure in the past, which are exactly the states worth reporting.
func job(t *testing.T, e *Engine, status string, cols map[string]any) {
	t.Helper()
	sql := `INSERT INTO ddcore_job (method, status, queue, run_after, enqueued, started, finished, lease_until)
		VALUES ('demo.noop', $1, 'default',
		        COALESCE($2, now()), COALESCE($3, now()), $4, $5, $6)`
	_, err := e.DB.Pool.Exec(context.Background(), sql,
		status, cols["run_after"], cols["enqueued"], cols["started"], cols["finished"], cols["lease_until"])
	if err != nil {
		t.Fatal(err)
	}
}

func queue(t *testing.T, e *Engine) *QueueHealth {
	t.Helper()
	q, err := e.QueueHealth(context.Background(), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestPRD03_QueueHealthCountsBacklogAndFailures(t *testing.T) {
	e := setup(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		job(t, e, "queued", nil)
	}
	job(t, e, "failed", map[string]any{"finished": now.Add(-time.Minute)})
	job(t, e, "failed", map[string]any{"finished": now.Add(-2 * time.Hour)})
	job(t, e, "done", map[string]any{"finished": now.Add(-time.Minute)})

	q := queue(t, e)
	if q.Queued != 3 || q.Runnable != 3 {
		t.Fatalf("expected 3 queued and runnable, got %d/%d", q.Queued, q.Runnable)
	}
	// The window is what keeps an old failure from alarming forever.
	if q.FailedInWindow != 1 {
		t.Fatalf("expected 1 failure inside the window, got %d", q.FailedInWindow)
	}
	if q.DoneInWindow != 1 {
		t.Fatalf("expected 1 completion inside the window, got %d", q.DoneInWindow)
	}
	if q.OldestQueuedSeconds < 0 {
		t.Fatalf("negative age: %v", q.OldestQueuedSeconds)
	}
}

// A job deliberately scheduled for tomorrow is not a backlog. Without the
// run_after filter it would hold the age alarm on from the moment it was
// created, and the alarm would never mean anything again.
func TestPRD03_OldestQueuedIgnoresAScheduledJob(t *testing.T) {
	e := setup(t)
	job(t, e, "queued", map[string]any{
		"run_after": time.Now().Add(24 * time.Hour),
		"enqueued":  time.Now().Add(-48 * time.Hour),
	})
	q := queue(t, e)
	if q.Queued != 1 {
		t.Fatalf("a scheduled job is still queued: got %d", q.Queued)
	}
	if q.Runnable != 0 {
		t.Fatalf("a job scheduled for tomorrow must not count as runnable: got %d", q.Runnable)
	}
	if q.OldestQueuedSeconds != 0 {
		t.Fatalf("a scheduled job must not age the queue: got %v", q.OldestQueuedSeconds)
	}
}

// Stalled uses the same predicate requeueStale acts on, so the number reported
// is the number a live worker would put back: it is the signal that a worker
// died holding work.
func TestPRD03_StalledJobIsReported(t *testing.T) {
	e := setup(t)
	job(t, e, "running", map[string]any{
		"started":     time.Now().Add(-10 * time.Minute),
		"lease_until": time.Now().Add(-time.Minute),
	})
	job(t, e, "running", map[string]any{
		"started":     time.Now().Add(-time.Minute),
		"lease_until": time.Now().Add(time.Minute),
	})
	q := queue(t, e)
	if q.Running != 2 {
		t.Fatalf("expected 2 running, got %d", q.Running)
	}
	if q.Stalled != 1 {
		t.Fatalf("expected exactly the expired lease to be stalled, got %d", q.Stalled)
	}
	if q.LongestRunningSeconds < 300 {
		t.Fatalf("expected the oldest run to be minutes old, got %v", q.LongestRunningSeconds)
	}
}

func TestPRD03_ErrorHealthCountsTheWindowAndCarriesTheRequestID(t *testing.T) {
	e := setup(t)
	ctx := WithRequestID(context.Background(), "abcdefgh1234")
	e.LogError(ctx, "test.source", context.Canceled)

	h, err := e.ErrorHealth(context.Background(), 15*time.Minute, 5)
	if err != nil {
		t.Fatal(err)
	}
	if h.InWindow != 1 {
		t.Fatalf("expected 1 entry in the window, got %d", h.InWindow)
	}
	if len(h.Latest) != 1 || h.Latest[0].RequestID != "abcdefgh1234" {
		t.Fatalf("the row lost its request id: %+v", h.Latest)
	}
	// An older failure is outside the window, and the sample follows the window
	// rather than the table: printing a months-old row underneath a count that
	// says "in the last 15 minutes" reads as though it were inside it.
	if _, err := e.DB.Pool.Exec(context.Background(),
		`UPDATE tab_error_log SET creation = now() - interval '2 hours'`); err != nil {
		t.Fatal(err)
	}
	old, err := e.ErrorHealth(context.Background(), 15*time.Minute, 5)
	if err != nil {
		t.Fatal(err)
	}
	if old.InWindow != 0 || len(old.Latest) != 0 {
		t.Fatalf("a row outside the window was still reported: %+v", old)
	}
}

// The context of a failed request is very often already cancelled — the client
// hung up, or a timeout is what failed it. Without detaching, the row that says
// why is the one thing that gets lost.
func TestPRD03_LogErrorSurvivesACancelledContext(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithCancel(WithRequestID(context.Background(), "cancelled123"))
	cancel()
	e.LogError(ctx, "test.cancelled", context.Canceled)

	h, err := e.ErrorHealth(context.Background(), 15*time.Minute, 5)
	if err != nil {
		t.Fatal(err)
	}
	if h.InWindow != 1 {
		t.Fatalf("the Error Log row was lost with the cancelled context")
	}
}

func TestPRD03_ReadyIsMemoisedForASecond(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if h := e.Ready(ctx); !h.OK {
		t.Fatalf("a live database must be ready: %+v", h)
	}
	// Closing the pool would make a fresh probe fail; within the TTL the cached
	// answer stands. That second of staleness is what buys the probe its
	// protection from being used as an amplifier.
	e.DB.Pool.Close()
	if h := e.Ready(ctx); !h.OK {
		t.Fatalf("the probe was not memoised: %+v", h)
	}
	time.Sleep(readyTTL + 200*time.Millisecond)
	if h := e.Ready(ctx); h.OK {
		t.Fatalf("the probe never expired: %+v", h)
	}
}
