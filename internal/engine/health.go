package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
)

// Health is one operational picture of the site, assembled from parts that cost
// very different amounts. The caller says which parts it wants: a readiness
// probe answered thousands of times an hour takes only the ping; a report a
// person opened takes the counts as well.
type Health struct {
	Status    string       `json:"status"` // "ok" | "warn" | "down"
	Version   string       `json:"version"`
	Site      string       `json:"site,omitempty"`
	Time      time.Time    `json:"time"`
	Database  db.Health    `json:"database"`
	Queue     *QueueHealth `json:"queue,omitempty"`
	Errors    *ErrorHealth `json:"errors,omitempty"`
	Scheduler SchedHealth  `json:"scheduler"`
	// Maintenance is reported, never turned into a status: a paused site is a
	// decision, and readiness failing would make an orchestrator undo it.
	Maintenance *MaintenanceState `json:"maintenance,omitempty"`
	// Warnings are rendered English sentences, not keys: they are read by an
	// operator in a terminal or a JSON payload, never by the desk.
	Warnings []string `json:"warnings,omitempty"`
}

// QueueHealth separates two things the single word "queued" hides: jobs waiting
// to run, and jobs deliberately scheduled for later.
type QueueHealth struct {
	Queued int64 `json:"queued"`
	// Runnable is Queued minus everything whose run_after is still in the
	// future. It is the only one of the two that means "behind".
	Runnable int64 `json:"runnable"`
	Running  int64 `json:"running"`
	// Stalled is running jobs whose lease expired: the worker holding them is
	// gone. It is the same predicate requeueStale acts on, so this is exactly
	// the number a live worker would put back.
	Stalled        int64 `json:"stalled"`
	FailedInWindow int64 `json:"failedInWindow"`
	DoneInWindow   int64 `json:"doneInWindow"`
	// CancelledInWindow separates work somebody stopped from work that broke.
	// Counting the two together would have an operator hunting a fault that was
	// in fact an administrative decision.
	CancelledInWindow     int64   `json:"cancelledInWindow"`
	OldestQueuedSeconds   float64 `json:"oldestQueuedSeconds"`
	LongestRunningSeconds float64 `json:"longestRunningSeconds"`
	WindowMinutes         int     `json:"windowMinutes"`
}

type ErrorHealth struct {
	InWindow      int64      `json:"inWindow"`
	WindowMinutes int        `json:"windowMinutes"`
	Latest        []ErrorRef `json:"latest,omitempty"`
}

// ErrorRef carries the request id so a report and a user's complaint can be
// joined without opening the desk.
type ErrorRef struct {
	ID        string `json:"id"`
	Method    string `json:"method"`
	RequestID string `json:"requestId,omitempty"`
	Creation  string `json:"creation"`
}

type SchedHealth struct {
	Enabled bool `json:"enabled"`
	Entries int  `json:"entries"`
}

// HealthOpts says which of the expensive parts to gather.
type HealthOpts struct {
	Queue  bool
	Errors bool
}

// readyCache holds the last database probe. A probe endpoint is unauthenticated
// by necessity, and one that opens a round trip per hit is an amplifier anyone
// can point at the database. No orchestrator needs sub-second resolution, so a
// second of staleness buys the rate limit that would otherwise be needed.
type readyCache struct {
	at time.Time
	h  db.Health
}

const readyTTL = time.Second

// Ready answers the readiness probe: does the database respond now.
//
// Only the ping. The queue aggregate is a scan of a table with no retention
// (that is PRD-04), and it has no business on a path a stranger can call.
func (e *Engine) Ready(ctx context.Context) db.Health {
	if c := e.ready.Load(); c != nil && time.Since(c.at) < readyTTL {
		return c.h
	}
	h := e.DB.Check(ctx, e.Cfg.Ops.ReadyTimeout())
	e.ready.Store(&readyCache{at: time.Now(), h: h})
	return h
}

// Health composes the picture and decides what to call it.
func (e *Engine) Health(ctx context.Context, o HealthOpts) Health {
	ops := e.Cfg.Ops
	h := Health{
		Version: Version, Site: e.SiteTitle(), Time: time.Now(),
		Database: e.Ready(ctx), Scheduler: e.SchedulerHealth(),
	}
	if !h.Database.OK {
		// Nothing else can be trusted, and asking would only add its own
		// failure to the report.
		h.Status = "down"
		return h
	}
	h.Status = "ok"
	if w := e.backupWarning(ctx); w != "" {
		h.Warnings = append(h.Warnings, w)
	}
	if o.Queue {
		q, err := e.QueueHealth(ctx, ops.Window())
		if err != nil {
			// Through the redactor: this warning lands in a doctor report, and
			// a pgx error quotes the connection string back at you.
			h.Warnings = append(h.Warnings, "queue could not be read: "+db.RedactError(err))
		} else {
			h.Queue = q
			if q.Runnable > int64(ops.QueueBacklog) {
				h.Warnings = append(h.Warnings, fmt.Sprintf("queue backlog: %d runnable jobs (limit %d)", q.Runnable, ops.QueueBacklog))
			}
			if q.OldestQueuedSeconds > float64(ops.QueueAgeSeconds) {
				h.Warnings = append(h.Warnings, fmt.Sprintf("oldest runnable job has waited %.0fs (limit %ds)", q.OldestQueuedSeconds, ops.QueueAgeSeconds))
			}
			if q.Stalled > 0 {
				h.Warnings = append(h.Warnings, fmt.Sprintf("%d job(s) held by a worker that is gone", q.Stalled))
			}
			if q.FailedInWindow > int64(ops.JobFailures) {
				h.Warnings = append(h.Warnings, fmt.Sprintf("%d job failure(s) in %dm (limit %d)", q.FailedInWindow, ops.WindowMinutes, ops.JobFailures))
			}
		}
	}
	if o.Errors {
		er, err := e.ErrorHealth(ctx, ops.Window(), 5)
		if err != nil {
			h.Warnings = append(h.Warnings, "error log could not be read: "+db.RedactError(err))
		} else {
			h.Errors = er
			if er.InWindow > int64(ops.ErrorLogEntries) {
				h.Warnings = append(h.Warnings, fmt.Sprintf("%d Error Log entries in %dm (limit %d)", er.InWindow, ops.WindowMinutes, ops.ErrorLogEntries))
			}
		}
	}
	if len(h.Warnings) > 0 {
		h.Status = "warn"
	}
	return h
}

// queueSQL is one round trip on purpose: six counts and two ages over a table
// that only grows.
//
// The window arrives in seconds through make_interval, not as a duration
// string: Go's Duration.String() is not an interval literal Postgres always
// accepts — it rejects "1ns" outright — and a probe is no place to find that
// out.
//
// The `run_after <= now()` on runnable and on the age is the whole point. A job
// enqueued to run tomorrow is not a backlog, and without the filter it would
// hold the age alarm on permanently from the moment it was created.
const queueSQL = `SELECT
  count(*) FILTER (WHERE status = 'queued')                                      AS queued,
  count(*) FILTER (WHERE status = 'queued'  AND run_after <= now())              AS runnable,
  count(*) FILTER (WHERE status = 'running')                                     AS running,
  count(*) FILTER (WHERE status = 'running' AND lease_until IS NOT NULL
                                            AND lease_until < now())             AS stalled,
  count(*) FILTER (WHERE status = 'failed'  AND finished > now() - make_interval(secs => $1)) AS failed_in_window,
  count(*) FILTER (WHERE status = 'done'    AND finished > now() - make_interval(secs => $1)) AS done_in_window,
  count(*) FILTER (WHERE status = 'cancelled' AND finished > now() - make_interval(secs => $1)) AS cancelled_in_window,
  COALESCE(EXTRACT(EPOCH FROM (now() - min(enqueued)
     FILTER (WHERE status = 'queued' AND run_after <= now()))), 0)               AS oldest_queued_seconds,
  COALESCE(EXTRACT(EPOCH FROM (now() - min(started)
     FILTER (WHERE status = 'running'))), 0)                                     AS longest_running_seconds
FROM ddcore_job`

func (e *Engine) QueueHealth(ctx context.Context, window time.Duration) (*QueueHealth, error) {
	q := &QueueHealth{WindowMinutes: int(window.Minutes())}
	err := e.DB.Pool.QueryRow(ctx, queueSQL, window.Seconds()).Scan(
		&q.Queued, &q.Runnable, &q.Running, &q.Stalled,
		&q.FailedInWindow, &q.DoneInWindow, &q.CancelledInWindow,
		&q.OldestQueuedSeconds, &q.LongestRunningSeconds)
	if err != nil {
		return nil, err
	}
	return q, nil
}

// ErrorHealth counts and samples the Error Log.
//
// It reads the table directly rather than going through GetList: a health
// report has no transaction and no document cache to spend, and whoever is
// asking was already authorised at the border.
func (e *Engine) ErrorHealth(ctx context.Context, window time.Duration, limit int) (*ErrorHealth, error) {
	dt, ok := e.Meta.DocTypes["Error Log"]
	if !ok {
		return nil, fmt.Errorf("Error Log doctype is not loaded")
	}
	table := dt.TableName()
	out := &ErrorHealth{WindowMinutes: int(window.Minutes())}
	if err := e.DB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM `+table+` WHERE creation > now() - make_interval(secs => $1)`, window.Seconds()).Scan(&out.InWindow); err != nil {
		return nil, err
	}
	// The sample is scoped to the same window as the count. Reporting the five
	// newest rows regardless would print months-old failures underneath a line
	// that says "in 15m", which reads as though they were.
	rows, err := db.Select(ctx, e.DB.Pool,
		`SELECT id, method, request_id, creation FROM `+table+`
		 WHERE creation > now() - make_interval(secs => $1) ORDER BY creation DESC LIMIT $2`, window.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Latest = append(out.Latest, ErrorRef{
			ID: db.Str(r["id"]), Method: db.Str(r["method"]),
			RequestID: db.Str(r["request_id"]), Creation: db.Str(r["creation"]),
		})
	}
	return out, nil
}

// SchedulerHealth reports what this build would install, not what is running.
//
// It counts the declarations rather than e.sched, because the process asking is
// usually `ddcore doctor`, which never started a cron. Nothing persists a last
// run today, so "the scheduler is alive" is not a claim this can honestly make.
func (e *Engine) SchedulerHealth() SchedHealth {
	return SchedHealth{Enabled: e.Cfg.Scheduler, Entries: len(e.ScheduledMethods())}
}
