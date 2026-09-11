package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// JobAction is what one administrative operation did to one job. Status is the
// job's status after the call, except for a running job asked to stop, which
// reports "cancelling": the worker has been told, and has not answered yet.
type JobAction struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// NewID is the job a retry created, zero for every other operation.
	NewID int64 `json:"newId,omitempty"`
}

// CancelJob stops a job, if it can still be stopped.
//
// It is one statement and not two on purpose. Reading the status in Go and then
// branching on it loses the cancellation whenever it races the claim: the
// "cancel a queued job" update blocks on the row lock runOneJob holds, re-reads
// after that transaction commits, finds "running", and changes nothing — while
// the operator is told the job was already over and the job runs on, unmarked,
// until its timeout. Deciding inside one statement means the CASE sees the row
// as it was, whichever side won the race.
//
// COALESCE, rather than plain assignment, keeps the first requester: asking
// twice is not an error, and the first person to ask is the one worth recording.
func (e *Engine) CancelJob(ctx context.Context, id int64, user string) (JobAction, error) {
	const q = `WITH t AS (SELECT id, status FROM ddcore_job WHERE id = $1 FOR UPDATE),
	 u AS (
	   UPDATE ddcore_job j SET
	     cancel_requested = COALESCE(j.cancel_requested, now()),
	     cancelled_by     = COALESCE(j.cancelled_by, $2),
	     status      = CASE WHEN j.status = 'queued' THEN 'cancelled' ELSE j.status END,
	     finished    = CASE WHEN j.status = 'queued' THEN now()       ELSE j.finished END,
	     lease_until = CASE WHEN j.status = 'queued' THEN NULL        ELSE j.lease_until END
	   FROM t WHERE j.id = t.id AND t.status IN ('queued', 'running')
	   RETURNING j.status)
	 SELECT t.status, (SELECT status FROM u) FROM t`

	var was string
	var now *string
	if err := e.DB.Pool.QueryRow(ctx, q, id, user).Scan(&was, &now); err != nil {
		if err == pgx.ErrNoRows {
			return JobAction{}, cerr.NotFound("Job {0} does not exist", id)
		}
		return JobAction{}, err
	}
	if now == nil {
		// Terminal already. Say which, because "could not cancel" leaves the
		// operator not knowing whether the work happened.
		return JobAction{ID: id, Status: was},
			cerr.Validation("Job {0} is already {1} and cannot be cancelled", id, was)
	}
	if *now == "cancelled" {
		return JobAction{ID: id, Status: "cancelled"}, nil
	}
	// Running: the worker picks the request up on its next poll. Anything
	// stronger than "cancelling" would be a claim about work that may be
	// committing in this very instant.
	return JobAction{ID: id, Status: "cancelling"}, nil
}

// RetryJob queues the work of a failed job again, as a new job.
//
// It copies rather than rewinds. The failed row is the record of what happened
// — its error, its attempt count, when it ran — and runOneJob mints the Error
// Log handle as "job:<id>" precisely so that a handle names one execution;
// re-running the same row would leave two executions behind one handle and
// overwrite the evidence of the first. The copy also needs no concurrency guard
// of its own: a row that has just been inserted cannot be held by a worker that
// thinks it still owns it.
//
// A second retry is refused unless forced, so that pressing the button twice
// does not quietly fan one failure out into several jobs.
func (e *Engine) RetryJob(ctx context.Context, id int64, force bool) (JobAction, error) {
	const q = `INSERT INTO ddcore_job (method, args, queue, "user", timeout_seconds, max_attempts, request_id, retry_of)
	 SELECT method, args, queue, "user", timeout_seconds, max_attempts, request_id, id
	   FROM ddcore_job
	  WHERE id = $1 AND status IN ('failed', 'cancelled') AND ($2 OR retried_as IS NULL)
	 RETURNING id`
	var newID int64
	if err := e.DB.Pool.QueryRow(ctx, q, id, force).Scan(&newID); err != nil {
		if err == pgx.ErrNoRows {
			return JobAction{}, e.explainRetryRefusal(ctx, id)
		}
		return JobAction{}, err
	}
	if _, err := e.DB.Pool.Exec(ctx,
		`UPDATE ddcore_job SET retried_as = $2 WHERE id = $1`, id, newID); err != nil {
		return JobAction{}, err
	}
	return JobAction{ID: id, Status: "retried", NewID: newID}, nil
}

// explainRetryRefusal turns "no rows" into the reason, which is the only thing
// the operator can act on.
func (e *Engine) explainRetryRefusal(ctx context.Context, id int64) error {
	var status string
	var retried *int64
	err := e.DB.Pool.QueryRow(ctx,
		`SELECT status, retried_as FROM ddcore_job WHERE id = $1`, id).Scan(&status, &retried)
	switch {
	case err == pgx.ErrNoRows:
		return cerr.NotFound("Job {0} does not exist", id)
	case err != nil:
		return err
	case retried != nil:
		return cerr.Validation("Job {0} was already retried as {1}", id, *retried)
	default:
		return cerr.Validation("Job {0} is {1}; only a failed or cancelled job can be retried", id, status)
	}
}

// PurgeOpts is one retention pass. A window of zero or less means "keep this
// class forever" — the caller has already resolved what the site configured, so
// there is exactly one meaning here and no sentinel to misread.
type PurgeOpts struct {
	DoneDays   int
	FailedDays int
	DryRun     bool
}

// PurgeCounts is what one pass removed, or would have removed on a dry run.
type PurgeCounts struct {
	Done   int `json:"done"`
	Failed int `json:"failed"`
}

// purgeBatch bounds one DELETE. The first purge of a table that has never had
// retention can be millions of rows; taking them at once means one enormous WAL
// burst and a long-held set of locks.
const purgeBatch = 5000

// PurgeJobs deletes terminal jobs past their retention window.
//
// The status whitelist is the guard that matters, not the age. `finished` is
// not trustworthy on its own: until this release the requeue branch stamped
// `finished = now()` on jobs going back to the *queue*, so a site upgrading into
// this code still holds queued rows carrying a finish time, and an age-only
// predicate would delete work that has never run.
func (e *Engine) PurgeJobs(ctx context.Context, o PurgeOpts) (PurgeCounts, error) {
	var n PurgeCounts
	var err error
	if n.Done, err = e.purge(ctx, []string{"done"}, o.DoneDays, o.DryRun); err != nil {
		return n, err
	}
	// Cancelled jobs keep the failures' window: both are the record of something
	// that did not complete, and both are read long after the fact.
	n.Failed, err = e.purge(ctx, []string{"failed", "cancelled"}, o.FailedDays, o.DryRun)
	return n, err
}

// QueueStat is one queue's standing. The site-wide aggregate in the health
// report cannot say which queue is behind, and with a mail queue and a
// scheduler queue sharing one worker pool that is the first thing anyone asks.
type QueueStat struct {
	Queue     string `json:"queue"`
	Queued    int64  `json:"queued"`
	Running   int64  `json:"running"`
	Done      int64  `json:"done"`
	Failed    int64  `json:"failed"`
	Cancelled int64  `json:"cancelled"`
}

// MethodStat is one method's behaviour in the window. Duration is the point:
// it is how a job that is merely slow is told from one that is stuck.
type MethodStat struct {
	Method     string  `json:"method"`
	Runs       int64   `json:"runs"`
	Failures   int64   `json:"failures"`
	AvgSeconds float64 `json:"avgSeconds"`
	MaxSeconds float64 `json:"maxSeconds"`
}

// JobStats is the queue seen from two directions.
type JobStats struct {
	WindowMinutes int          `json:"windowMinutes"`
	Queues        []QueueStat  `json:"queues"`
	Methods       []MethodStat `json:"methods"`
}

// statsMethodLimit keeps the per-method list to what a person will read. A site
// with hundreds of job methods wants the worst of them, not all of them.
const statsMethodLimit = 20

// JobStats breaks the queue down by queue and by method.
//
// The queue rollup counts every row, because a backlog is a backlog however old
// the job is; the method rollup is bounded by the window, because "how long does
// this usually take" is only meaningful over recent runs.
func (e *Engine) JobStats(ctx context.Context, window time.Duration) (*JobStats, error) {
	out := &JobStats{WindowMinutes: int(window.Minutes())}

	qRows, err := db.Select(ctx, e.DB.Pool, `SELECT queue,
		  count(*) FILTER (WHERE status = 'queued')    AS queued,
		  count(*) FILTER (WHERE status = 'running')   AS running,
		  count(*) FILTER (WHERE status = 'done')      AS done,
		  count(*) FILTER (WHERE status = 'failed')    AS failed,
		  count(*) FILTER (WHERE status = 'cancelled') AS cancelled
		FROM ddcore_job GROUP BY queue ORDER BY queue`)
	if err != nil {
		return nil, err
	}
	for _, r := range qRows {
		out.Queues = append(out.Queues, QueueStat{
			Queue:   db.Str(r["queue"]),
			Queued:  int64(toFloat(r["queued"])),
			Running: int64(toFloat(r["running"])),
			Done:    int64(toFloat(r["done"])),
			Failed:  int64(toFloat(r["failed"])),

			Cancelled: int64(toFloat(r["cancelled"])),
		})
	}

	mRows, err := db.Select(ctx, e.DB.Pool, `SELECT method,
		  count(*)                                        AS runs,
		  count(*) FILTER (WHERE status = 'failed')       AS failures,
		  COALESCE(AVG(EXTRACT(EPOCH FROM (finished - started))), 0) AS avg_seconds,
		  COALESCE(MAX(EXTRACT(EPOCH FROM (finished - started))), 0) AS max_seconds
		FROM ddcore_job
		WHERE finished IS NOT NULL AND started IS NOT NULL
		  AND finished > now() - make_interval(secs => $1)
		GROUP BY method ORDER BY failures DESC, avg_seconds DESC LIMIT $2`,
		window.Seconds(), statsMethodLimit)
	if err != nil {
		return nil, err
	}
	for _, r := range mRows {
		out.Methods = append(out.Methods, MethodStat{
			Method:     db.Str(r["method"]),
			Runs:       int64(toFloat(r["runs"])),
			Failures:   int64(toFloat(r["failures"])),
			AvgSeconds: toFloat(r["avg_seconds"]),
			MaxSeconds: toFloat(r["max_seconds"]),
		})
	}
	return out, nil
}

// SweepJobs is the scheduled retention pass, using the windows the site
// configured.
//
// It is hygiene and never correctness. The scheduler only runs where the site
// sets `"scheduler": true`, so nothing may depend on this having run: a job is
// terminal whether or not its row is still here, and if this never ran the
// table would only grow.
func (e *Engine) SweepJobs(ctx context.Context) (PurgeCounts, error) {
	return e.PurgeJobs(ctx, PurgeOpts{
		DoneDays:   e.Cfg.Ops.DoneRetentionDays(),
		FailedDays: e.Cfg.Ops.FailedRetentionDays(),
	})
}

func (e *Engine) purge(ctx context.Context, statuses []string, days int, dry bool) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	const where = `status = ANY($1) AND finished IS NOT NULL
		AND finished < now() - make_interval(days => $2)`
	if dry {
		var n int
		err := e.DB.Pool.QueryRow(ctx,
			`SELECT count(*) FROM ddcore_job WHERE `+where, statuses, days).Scan(&n)
		return n, err
	}
	total := 0
	for {
		// SKIP LOCKED so a purge never blocks a worker writing its own result.
		tag, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_job WHERE id IN (
			SELECT id FROM ddcore_job WHERE `+where+`
			 ORDER BY id LIMIT $3 FOR UPDATE SKIP LOCKED)`, statuses, days, purgeBatch)
		if err != nil {
			return total, err
		}
		total += int(tag.RowsAffected())
		if tag.RowsAffected() < purgeBatch {
			return total, nil
		}
	}
}

// jobColumns is everything a listing shows. args and result are deliberately
// absent: a queued password-reset mail carries its own recovery link in
// args.html, so the payload is readable on the CLI, where whoever is asking
// already holds the database, and never over HTTP.
const jobColumns = `id, method, queue, status, "user", enqueued, run_after, started, finished,
	attempts, max_attempts, timeout_seconds, lease_until, request_id,
	cancel_requested, cancelled_by, retry_of, retried_as, error`

// JobFilter narrows a listing. Every field is optional and they combine with
// AND, which is what an operator reading a queue expects.
type JobFilter struct {
	Status []string
	Queue  string
	Method string
	User   string
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Start  int
}

// where builds the shared predicate for ListJobs and CountJobs, so the two can
// never drift into disagreeing about what matches.
func (f JobFilter) where() (string, []any) {
	var clauses []string
	var args []any
	add := func(sql string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}
	if len(f.Status) > 0 {
		add("status = ANY($%d)", f.Status)
	}
	if f.Queue != "" {
		add("queue = $%d", f.Queue)
	}
	if f.Method != "" {
		add("method = $%d", f.Method)
	}
	if f.User != "" {
		add(`"user" = $%d`, f.User)
	}
	if f.Since != nil {
		add("enqueued >= $%d", *f.Since)
	}
	if f.Until != nil {
		add("enqueued <= $%d", *f.Until)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListJobs reads the queue, newest first.
func (e *Engine) ListJobs(ctx context.Context, f JobFilter) ([]map[string]any, error) {
	where, args := f.where()
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, f.Start)
	q := fmt.Sprintf(`SELECT %s FROM ddcore_job%s ORDER BY id DESC LIMIT $%d OFFSET $%d`,
		jobColumns, where, len(args)-1, len(args))
	return db.Select(ctx, e.DB.Pool, q, args...)
}

// CountJobs is the same predicate without the page, for `with_count`.
func (e *Engine) CountJobs(ctx context.Context, f JobFilter) (int64, error) {
	where, args := f.where()
	var n int64
	err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_job`+where, args...).Scan(&n)
	return n, err
}

// GetJob reads one job. withPayload adds args and result; only the CLI asks.
func (e *Engine) GetJob(ctx context.Context, id int64, withPayload bool) (map[string]any, error) {
	cols := jobColumns
	if withPayload {
		cols += ", args, result"
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT `+cols+` FROM ddcore_job WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.NotFound("Job {0} does not exist", id)
	}
	return rows[0], nil
}
