package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Job execution limits. The lease is renewed by heartbeat while the worker
// is alive; if the process crashes, the job returns to the queue when the lease
// expires (B15).
const defaultJobTimeout = 300 // seconds

// Vars and not consts so the tests can shrink them: a cancellation test that
// waits five real seconds per case is a test that gets skipped.
var (
	jobLease = 2 * time.Minute
	// Renewal is a write against an indexed column, so it is not a HOT update:
	// every tick churns ddcore_job_lease and leaves a dead tuple behind. A third
	// of the lease tolerates two consecutive failures before another worker may
	// steal the job, which is all the margin this needs.
	jobLeaseRenew = jobLease / 3
	// The cancellation check is a primary-key read and can afford to be frequent.
	// Keeping it separate from the renewal is the whole point: one interval per
	// question, instead of paying the write cost at the read's frequency.
	jobCancelPoll = 5 * time.Second
	// How long an idle or paused worker waits before it looks at the queue again.
	jobIdlePoll = 2 * time.Second
	// How long Workers.Stop waits, once it has interrupted the jobs still running
	// after the grace, for them to write their attempt back. The interrupt
	// reaches the VM and every outbound call a job makes; what it does not reach
	// — a slow query — is left to the lease sweep after this.
	jobGiveBackWait = 10 * time.Second
)

// How long a failed job waits before it runs again. Fixed is the historical
// thirty seconds, right for work that fails because of something local and
// short-lived. Exponential doubles that per attempt up to an hour, which is what
// a job talking to somebody else's server needs: a receiver that is down for
// twenty minutes should not exhaust six attempts in three.
const (
	BackoffFixed       = "fixed"
	BackoffExponential = "exponential"
)

// retryDelaySQL is the run_after of a job going back to the queue, in terms of
// the row's own backoff and attempts columns. One expression, shared by the
// failure branch and the stale-lease sweep, so the two cannot disagree.
const retryDelaySQL = `now() + CASE WHEN backoff = 'exponential'
	THEN make_interval(secs => least(30 * power(2, greatest(attempts - 1, 0)), 3600))
	ELSE interval '30 seconds' END`

// Enqueue stores a job in ddcore_job; workers pick it with SKIP LOCKED.
func (c *Ctx) Enqueue(method string, args map[string]any, opts map[string]any) (int64, error) {
	if method == "" {
		return 0, cerr.Validation("enqueue: provide the method")
	}
	if err := c.checkWritable(""); err != nil {
		return 0, err
	}
	queue := "default"
	if q, ok := opts["queue"].(string); ok && q != "" {
		queue = q
	}
	runAfter := time.Now()
	if ra, ok := opts["runAfter"].(string); ok && ra != "" {
		if t := parseTime(ra, c.E.Location()); !t.IsZero() {
			runAfter = t
		}
	}
	timeout := defaultJobTimeout
	if t, ok := opts["timeout"]; ok {
		if n := int(toFloat(t)); n > 0 {
			timeout = n
		}
	}
	// max_attempts had a default and no way to set it, so a job whose failure is
	// permanent was retried three times regardless.
	maxAttempts := 3
	if m, ok := opts["maxAttempts"]; ok {
		if n := int(toFloat(m)); n > 0 {
			maxAttempts = n
		}
	}
	backoff := BackoffFixed
	if v, ok := opts["backoff"].(string); ok && v != "" {
		if v != BackoffFixed && v != BackoffExponential {
			return 0, cerr.Validation("enqueue: backoff must be {0} or {1}", BackoffFixed, BackoffExponential)
		}
		backoff = v
	}
	// Lifecycle callbacks, each a method path like the job's own. They are
	// resolved when they run, as the job's method is: a typo surfaces as the
	// attempt's error rather than as a refusal to queue.
	hooks := map[string]string{}
	for _, k := range []string{"onStart", "onFailure"} {
		v, ok := opts[k]
		if !ok || v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			return 0, cerr.Validation("enqueue: {0} must be a method path", k)
		}
		hooks[k] = str
	}
	uniqueKey := ""
	if v, ok := opts["uniqueKey"]; ok && v != nil {
		str, ok := v.(string)
		if !ok || str == "" {
			return 0, cerr.Validation("enqueue: uniqueKey must be a non-empty string")
		}
		uniqueKey = str
	}
	// the key is the tenant's: two tenants queue the same keyed job side by side
	conflict := "unique_key"
	if c.Tenancy() {
		conflict = "tenant, unique_key"
	}
	// The user the job acts as. Checked here so a typo is refused where it was
	// written, and again when the job runs, because a user can be disabled in
	// between.
	runAs := ""
	if v, ok := opts["runAs"]; ok && v != nil {
		str, ok := v.(string)
		if !ok || str == "" {
			return 0, cerr.Validation("enqueue: runAs must be a user")
		}
		if err := c.checkActingUser(str); err != nil {
			return 0, err
		}
		runAs = str
	}
	b, _ := json.Marshal(args)
	// A keyed enqueue whose key is already queued inserts nothing and returns the
	// queued job's id. The job can be claimed between the conflict and the lookup,
	// freeing the key, so the pair is retried instead of assuming either outcome.
	for attempt := 0; ; attempt++ {
		var id int64
		// The request id travels with the job, so the work a request queued can be
		// found from the request, and the other way round.
		err := c.Q().QueryRow(c.Ctx, `INSERT INTO ddcore_job
			(method, args, queue, "user", run_after, timeout_seconds, max_attempts, request_id, backoff,
			 on_start, on_failure, unique_key, run_as)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''),
			        NULLIF($13, ''))
			ON CONFLICT (`+conflict+`) WHERE unique_key IS NOT NULL AND status = 'queued' DO NOTHING
			RETURNING id`,
			method, string(b), queue, c.User, runAfter, timeout, maxAttempts, RequestIDFrom(c.Ctx), backoff,
			hooks["onStart"], hooks["onFailure"], uniqueKey, runAs).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		err = c.Q().QueryRow(c.Ctx, `SELECT id FROM ddcore_job WHERE unique_key = $1 AND status = 'queued'`,
			uniqueKey).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) || attempt >= 4 {
			return 0, err
		}
	}
}

// RunJob executes one job payload: a dotted function path with args. The ctx is
// honored inside the VM: a script that does not return is interrupted when the
// context expires (B15).
func (e *Engine) RunJob(ctx context.Context, user, method string, args map[string]any) (json.RawMessage, error) {
	return e.runJobAs(ctx, user, "", method, args)
}

// runJobAs is RunJob for a job queued with runAs: a non-empty runAs is the
// user the job acts as, with permissions enforced.
func (e *Engine) runJobAs(ctx context.Context, user, runAs, method string, args map[string]any) (json.RawMessage, error) {
	if method == notificationSweepMethod {
		return nil, e.SweepNotifications(ctx, time.Now())
	}
	var out json.RawMessage
	b, _ := json.Marshal(args)
	err := e.inJobTx(ctx, user, runAs, method, func(rt *js.Runtime) error {
		var callErr error
		out, callErr = rt.CallFunction(method, b)
		return callErr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// runJobHook runs one of a job's lifecycle callbacks in a transaction of its
// own, which is the whole point of them: what onStart writes commits before the
// body runs, and what onFailure writes survives the body's rollback.
func (e *Engine) runJobHook(ctx context.Context, user, runAs, path string, args, info map[string]any) error {
	b, _ := json.Marshal(args)
	if b == nil || string(b) == "null" {
		b = []byte("{}")
	}
	jb, _ := json.Marshal(info)
	return e.inJobTx(ctx, user, runAs, path, func(rt *js.Runtime) error {
		return rt.CallJobHook(path, b, jb)
	})
}

// inJobTx is the transaction a job's code runs in: the job's user, permissions
// ignored, and a VM that ctx interrupts. A job queued with runAs runs as that
// user instead, with permissions enforced; a user that has since been removed
// or disabled fails the job rather than letting it run unscoped. The
// transaction itself uses a context without the deadline: the timeout needs to
// interrupt the VM, without interfering with the rollback. The outbound calls
// the job makes do take ctx (see callCtx), or a job waiting on a server that
// never answers could be neither timed out, cancelled nor stopped.
func (e *Engine) inJobTx(ctx context.Context, user, runAs, method string, call func(rt *js.Runtime) error) error {
	actor := orDefault(user, "Admin")
	if runAs != "" {
		actor = runAs
	}
	err := e.Run(withCallCtx(context.WithoutCancel(ctx), ctx), actor, func(c *Ctx) error {
		if runAs == "" {
			c.Flags["ignorePermissions"] = true
		} else if err := c.checkActingUser(runAs); err != nil {
			return err
		}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		return rt.WithContext(ctx, func() error { return call(rt) })
	})
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return cerr.Validation("job {0} timed out", method)
	}
	return err
}

type callCtxKey struct{}

// withCallCtx makes call the context the outbound calls of the work under ctx
// run on. It travels as a value, so a ctx that ddcore.runAs or a tenant switch
// derives from the job's still carries it.
func withCallCtx(ctx, call context.Context) context.Context {
	return context.WithValue(ctx, callCtxKey{}, call)
}

// callCtx is the context an outbound call made from app code runs on — an
// HTTP request, a push, a file fetched from a URL. In a job it is the job's,
// which its timeout, a cancellation and a worker shutting down all cancel, so
// they reach a job blocked in the call and not only one running JavaScript.
// Anywhere else it is context.Background(): a request's own calls were never
// tied to the client staying connected, and this does not start now.
func (c *Ctx) callCtx() context.Context { return c.callCtxOr(context.Background()) }

// callCtxOr is callCtx with another context outside a job.
func (c *Ctx) callCtxOr(fallback context.Context) context.Context {
	if c != nil && c.Ctx != nil {
		if call, ok := c.Ctx.Value(callCtxKey{}).(context.Context); ok {
			return call
		}
	}
	return fallback
}

type jobInfoKey struct{}

// withJobInfo names the job the work under ctx belongs to, as the JobInfo its
// callbacks are given. Like the call context it travels as a value, so it
// survives a runAs or a tenant switch inside the job.
func withJobInfo(ctx context.Context, info map[string]any) context.Context {
	return context.WithValue(ctx, jobInfoKey{}, info)
}

// currentJob is ddcore.job.current(): the JobInfo of the job c runs in, or
// nil outside one — a request, a migration, a function run by `jobs run`.
func (c *Ctx) currentJob() map[string]any {
	if c == nil || c.Ctx == nil {
		return nil
	}
	info, _ := c.Ctx.Value(jobInfoKey{}).(map[string]any)
	return info
}

// onFailureTimeout bounds a job's onFailure callback. It runs after the job's
// own deadline, so it cannot share it, and it runs on paths — a worker
// shutting down, an administrative cancel — that must not hang on app code.
var onFailureTimeout = 30 * time.Second

// jobFailure is what onFailure is told about the attempt that ended.
type jobFailure struct {
	id                   int64
	method, user, queue  string
	runAs                string
	hook                 string
	args                 map[string]any
	attempt, maxAttempts int
	starts               int
	reason, err          string
	final                bool
}

// runOnFailure calls the job's onFailure, if it has one. A callback that
// throws is filed in the Error Log against the job and changes nothing else:
// the job has already failed, and that is what its row says.
func (e *Engine) runOnFailure(ctx context.Context, f jobFailure) {
	if f.hook == "" {
		return
	}
	rctx := WithRequestID(context.WithoutCancel(ctx), fmt.Sprintf("job:%d", f.id))
	hctx, cancel := context.WithTimeout(rctx, onFailureTimeout)
	defer cancel()
	info := map[string]any{
		"id": f.id, "method": f.method, "queue": f.queue,
		"attempt": f.attempt, "maxAttempts": f.maxAttempts, "starts": f.starts,
		"error": f.err, "reason": f.reason, "final": f.final,
	}
	if f.runAs != "" {
		info["runAs"] = f.runAs
	}
	if err := e.runJobHook(withJobInfo(hctx, info), f.user, f.runAs, f.hook, f.args, info); err != nil {
		e.LogError(rctx, "job:onFailure:"+f.method, err)
	}
}

// jobArgs decodes the args column, which arrives as text or as a decoded map
// depending on how the row was read.
func jobArgs(v any) map[string]any {
	var args map[string]any
	if s, ok := v.(string); ok {
		json.Unmarshal([]byte(s), &args)
	} else if m, ok := v.(map[string]any); ok {
		args = m
	}
	return args
}

// requeueStale puts back jobs whose worker died: running status with expired
// lease returns to queued (or failed if attempts are exhausted), and a job that
// was cancelled while running stays cancelled rather than being resurrected by
// the very sweep meant to recover crashed workers.
//
// It stays one statement even though it decides three ways. Every worker runs
// it every couple of seconds, so three statements would mean three scans and
// three chances to interleave; one UPDATE ... RETURNING under READ COMMITTED
// hands each concurrent worker only the rows it actually changed, which is what
// makes the publish below exactly-once without any coordination.
//
// The error column is only written on the branch that gives up. It used to be
// set unconditionally, so a job about to run again sat in the queue advertising
// a failure that had not happened yet, and whatever the previous attempt
// actually reported was destroyed.
func (e *Engine) requeueStale(ctx context.Context) error {
	const q = `UPDATE ddcore_job SET
		status = CASE WHEN cancel_requested IS NOT NULL THEN 'cancelled'
		              WHEN attempts < max_attempts      THEN 'queued'
		              ELSE 'failed' END,
		lease_until = NULL,
		finished = CASE WHEN cancel_requested IS NULL AND attempts < max_attempts
		                THEN NULL ELSE now() END,
		started = CASE WHEN cancel_requested IS NULL AND attempts < max_attempts
		               THEN NULL ELSE started END,
		run_after = CASE WHEN cancel_requested IS NULL AND attempts < max_attempts
		                 THEN ` + retryDelaySQL + ` ELSE run_after END,
		error = CASE WHEN cancel_requested IS NOT NULL THEN error
		             WHEN attempts < max_attempts      THEN NULL
		             ELSE 'worker interrupted: lease expired' END
		WHERE status = 'running' AND lease_until IS NOT NULL AND lease_until < now()
		RETURNING id, method, status, "user", args, queue, attempts, max_attempts, starts, on_failure, run_as`
	on, err := e.tenancy(ctx)
	if err != nil {
		return err
	}
	rows, err := db.Select(ctx, e.DB.Sys, q+jobTenantColumn(on))
	if err != nil {
		return err
	}
	// A job that ended here ends without anyone having been told. The enqueuer
	// is waiting on job_done and would otherwise wait for a worker that is gone.
	for _, r := range rows {
		status := db.Str(r["status"])
		// The worker that ran onStart is gone, so nobody else will tell the
		// document the attempt ended. RETURNING hands each row to exactly one
		// sweeping worker, which makes this call exactly-once as well.
		reason := "error"
		if status == "cancelled" {
			reason = "cancelled"
		}
		e.runOnFailure(jobSpace(ctx, r), jobFailure{
			id: int64(toFloat(r["id"])), method: db.Str(r["method"]), user: db.Str(r["user"]),
			runAs: db.Str(r["run_as"]),
			queue: db.Str(r["queue"]), hook: db.Str(r["on_failure"]), args: jobArgs(r["args"]),
			attempt: int(toFloat(r["attempts"])), maxAttempts: int(toFloat(r["max_attempts"])),
			starts: int(toFloat(r["starts"])),
			reason: reason, err: "worker interrupted: lease expired", final: status != "queued",
		})
		if status == "queued" {
			continue
		}
		e.Events.Publish(Event{Name: "job_done", Payload: map[string]any{
			"id": int64(toFloat(r["id"])), "method": db.Str(r["method"]),
			"ok": false, "cancelled": db.Str(r["status"]) == "cancelled",
			"error": "worker interrupted: lease expired",
		}, User: db.Str(r["user"]), Tenant: db.Str(r["tenant"])})
	}
	return nil
}

// Worker loops over queued jobs until ctx is cancelled. Cancelling ctx also
// interrupts the job running at that moment, which gives its attempt back;
// StartWorkers is the shutdown that lets the job finish first.
func (e *Engine) Worker(ctx context.Context, id int) {
	e.work(ctx, ctx, id)
}

// work is the worker loop. claim stops it from taking another job; run is what
// the job it took runs under, and interrupts it. A worker shutting down
// cancels claim first and run only when it has waited long enough.
func (e *Engine) work(claim, run context.Context, id int) {
	idle := func() bool {
		select {
		case <-claim.Done():
			return false
		case <-time.After(jobIdlePoll):
			return true
		}
	}
	for claim.Err() == nil {
		// paused: claim nothing, and leave expired leases alone too — requeueing
		// them is a write, and the job they belong to cannot run anyway
		if e.Paused(claim) {
			if !idle() {
				return
			}
			continue
		}
		if err := e.requeueStale(claim); err != nil && claim.Err() == nil {
			e.Log.Error("requeue", "id", id, "err", err)
		}
		ran, err := e.claimAndRun(claim, run)
		if err != nil && claim.Err() == nil {
			e.Log.Error("worker", "id", id, "err", err)
		}
		if !ran && !idle() {
			return
		}
	}
}

// Workers are the job workers of a process, started by StartWorkers.
type Workers struct {
	e         *Engine
	stopClaim context.CancelFunc
	interrupt context.CancelFunc
	wg        sync.WaitGroup
	stopOnce  sync.Once
}

// StartWorkers starts n job workers. They run until Stop, and nothing else
// stops them: a process's signal context must not reach a running job, or the
// job is interrupted the instant the process is asked to stop instead of being
// given the grace to finish.
func (e *Engine) StartWorkers(n int) *Workers {
	claim, stopClaim := context.WithCancel(context.Background())
	run, interrupt := context.WithCancel(context.Background())
	w := &Workers{e: e, stopClaim: stopClaim, interrupt: interrupt}
	for i := 0; i < n; i++ {
		w.wg.Add(1)
		go func(id int) {
			defer w.wg.Done()
			e.work(claim, run, id)
		}(i)
	}
	return w
}

// Stop shuts the workers down: nothing new is claimed from the moment it is
// called; the jobs already running get up to grace to finish and commit; those
// still running then are interrupted — the VM and every outbound call they are
// waiting on — and go back to the queue with their attempt given back. Stop
// returns once every worker has, or once the give-back has had jobGiveBackWait
// to happen, whichever is first. It is safe to call more than once.
func (w *Workers) Stop(grace time.Duration) {
	w.stopOnce.Do(func() {
		done := make(chan struct{})
		go func() { w.wg.Wait(); close(done) }()
		w.stopClaim()
		if grace > 0 {
			select {
			case <-done:
				w.interrupt()
				return
			case <-time.After(grace):
			}
		}
		select {
		case <-done:
		default:
			w.e.Log.Warn("shutdown: interrupting the jobs still running; they go back to the queue",
				"grace", grace.String())
		}
		w.interrupt()
		select {
		case <-done:
		case <-time.After(jobGiveBackWait):
			w.e.Log.Warn("shutdown: a job did not stop in time; its lease will return it to the queue",
				"waited", jobGiveBackWait.String())
		}
	})
}

// runOneJob claims and runs one job, under a single context that both stops
// the claim and interrupts the job.
func (e *Engine) runOneJob(ctx context.Context) (bool, error) {
	return e.claimAndRun(ctx, ctx)
}

// claimAndRun claims one job unless claim is done, and runs it under run. The
// claim itself is written on run: a claim committed while its context is
// being cancelled is a row left "running" with nobody running it.
func (e *Engine) claimAndRun(claim, ctx context.Context) (bool, error) {
	if claim.Err() != nil {
		return false, nil
	}
	on, err := e.tenancy(ctx)
	if err != nil {
		return false, err
	}
	// The queue is one for the whole site, so the claim is the system pool's.
	// A job's number names it whichever tenant queued it; the tenant comes
	// back with the row and is where the job then runs. A disabled tenant's
	// jobs wait: nothing of it runs until it is enabled again.
	tx, err := e.DB.Sys.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	enabled := ""
	if on {
		enabled = ` AND (tenant = '' OR tenant IN (SELECT id FROM ` + meta.TenantTable + ` WHERE enabled))`
	}
	rows, err := db.Select(ctx, tx, `SELECT id, method, args, "user", attempts, max_attempts, starts, timeout_seconds,
		queue, on_start, on_failure, run_as`+jobTenantColumn(on)+` FROM ddcore_job
		WHERE status = 'queued' AND run_after <= now() AND cancel_requested IS NULL`+enabled+`
		ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`)
	if err != nil || len(rows) == 0 {
		return false, err
	}
	j := rows[0]
	id := int64(toFloat(j["id"]))
	if claim.Err() != nil {
		// stopped while the row was being picked: leave it to the next process
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE ddcore_job SET status = 'running', started = now(), attempts = attempts + 1,
		starts = starts + 1, lease_until = now() + $2::interval WHERE id = $1`,
		id, jobLease.String()); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	args := jobArgs(j["args"])
	method := db.Str(j["method"])
	e.Log.Info("job", "id", id, "method", method)

	attempt := int(toFloat(j["attempts"])) + 1
	starts := int(toFloat(j["starts"])) + 1
	user, runAs := db.Str(j["user"]), db.Str(j["run_as"])
	timeout := time.Duration(orInt(toFloat(j["timeout_seconds"]), defaultJobTimeout)) * time.Second
	tenant := db.Str(j["tenant"])
	spaceCtx := jobSpace(ctx, j)
	jobCtx, cancel := context.WithTimeout(spaceCtx, timeout)
	// The flag, and not the error, is what identifies a cancellation. Both an
	// administrative cancel and a worker shutting down interrupt the VM through
	// a context and surface as context.Canceled, and RunJob rewrites the timeout
	// into a fresh cerr with no Unwrap — so the returned error cannot tell the
	// three apart, and only the callback knows which one happened.
	var cancelled atomic.Bool
	stopBeat := e.heartbeat(ctx, id, attempt, func(reason string) {
		if reason == "cancelled" {
			cancelled.Store(true)
		}
		cancel()
	})
	maxAttempts := int(toFloat(j["max_attempts"]))
	// What onStart is given, and what ddcore.job.current() returns to it and
	// to the body.
	info := map[string]any{"id": id, "method": method, "queue": db.Str(j["queue"]),
		"attempt": attempt, "maxAttempts": maxAttempts, "starts": starts}
	if runAs != "" {
		info["runAs"] = runAs
	}
	jobCtx = withJobInfo(jobCtx, info)
	// onStart commits on its own before the body runs, so the document can say
	// the job is running while it is. If it throws, the attempt has failed and
	// the body does not run: the switch below cannot tell the two apart, and
	// does not need to.
	var res json.RawMessage
	var runErr error
	if hook := db.Str(j["on_start"]); hook != "" {
		runErr = e.runJobHook(WithRequestID(jobCtx, fmt.Sprintf("job:%d", id)), user, runAs, hook, args, info)
	}
	if runErr == nil {
		res, runErr = e.runJobAs(jobCtx, user, runAs, method, args)
	}
	stopBeat() // joins the goroutine, so the flag is visible below
	jobErr := jobCtx.Err()
	cancel()

	// Terminal writes are fenced on the attempt that produced them and run on a
	// context detached from the worker's. Attached, they were skipped outright
	// when the worker was shutting down — and their error was discarded, so the
	// row simply stayed "running" until its lease expired, with nothing said.
	wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer wcancel()
	write := func(sql string, a ...any) {
		if _, err := e.DB.Sys.Exec(wctx, sql, a...); err != nil {
			e.Log.Error("job finalize", "id", id, "err", err)
		}
	}
	failed := func(reason string, final bool, msg string) {
		e.runOnFailure(spaceCtx, jobFailure{
			id: id, method: method, user: user, runAs: runAs, queue: db.Str(j["queue"]),
			hook: db.Str(j["on_failure"]), args: args, attempt: attempt, maxAttempts: maxAttempts,
			starts: starts, reason: reason, err: msg, final: final,
		})
	}
	publish := func(payload map[string]any) {
		payload["id"], payload["method"] = id, method
		e.Events.Publish(Event{Name: "job_done", Payload: payload, User: user, Tenant: tenant})
	}

	switch {
	case runErr == nil:
		// First, deliberately: a cancellation that arrives as the call returns
		// finds work that already committed, and calling that "cancelled" would
		// report a rollback that never happened.
		write(`UPDATE ddcore_job SET status = 'done', finished = now(), lease_until = NULL, result = $2
			WHERE id = $1 AND status = 'running' AND attempts = $3`, id, string(orJSON(res)), attempt)
		publish(map[string]any{"ok": true})

	case cancelled.Load():
		// Asked for by a person. Not retried however many attempts remain, and
		// no Error Log row: a cancellation is not a fault, and logging it as one
		// would bury real errors and push the health report over its threshold.
		write(`UPDATE ddcore_job SET status = 'cancelled', finished = now(), lease_until = NULL,
			error = 'cancelled by ' || COALESCE(cancelled_by, 'an admin')
			WHERE id = $1 AND status = 'running' AND attempts = $2`, id, attempt)
		// Before the event, so whoever refreshes on it reads what onFailure wrote.
		msg := "cancelled"
		e.DB.Sys.QueryRow(wctx, `SELECT COALESCE(error, 'cancelled') FROM ddcore_job WHERE id = $1`, id).Scan(&msg)
		failed("cancelled", true, msg)
		publish(map[string]any{"ok": false, "cancelled": true})

	case isMaintenanceErr(runErr):
		// The site was paused under the job. Like a worker shutting down, that
		// is a decision and not a fault: give the job back without consuming
		// the attempt, and file no Error Log row. The worker claims nothing
		// while the flag is on, so the row simply waits for the window to close.
		write(`UPDATE ddcore_job SET status = 'queued', attempts = attempts - 1, started = NULL,
			lease_until = NULL, error = NULL, run_after = now()
			WHERE id = $1 AND status = 'running' AND attempts = $2`, id, attempt)

	case ctx.Err() != nil && !errors.Is(jobErr, context.DeadlineExceeded):
		// The worker is stopping and the grace ran out, so the job did not fail
		// — it was never allowed to finish. Give it back without consuming the attempt, or a rolling
		// restart would exhaust max_attempts on work nothing is wrong with.
		write(`UPDATE ddcore_job SET status = 'queued', attempts = attempts - 1, started = NULL,
			lease_until = NULL, error = NULL, run_after = now()
			WHERE id = $1 AND status = 'running' AND attempts = $2`, id, attempt)

	default:
		status := "failed"
		if attempt < maxAttempts {
			status = "queued"
		}
		// A job going back to the queue has not finished. Stamping it anyway left
		// live queued rows carrying a finish time, which any retention sweep
		// keyed on `finished` would read as "terminal and old enough to delete".
		write(`UPDATE ddcore_job SET status = $2, error = $3,
			finished = CASE WHEN $2 = 'queued' THEN NULL ELSE now() END,
			lease_until = NULL, run_after = `+retryDelaySQL+`
			WHERE id = $1 AND status = 'running' AND attempts = $4`,
			id, status, runErr.Error(), attempt)
		// A failed run gets a handle of its own, so an Error Log row points at
		// one execution and not merely at a method name.
		e.LogError(WithRequestID(spaceCtx, fmt.Sprintf("job:%d", id)), "job:"+method, runErr)
		reason := "error"
		if errors.Is(jobErr, context.DeadlineExceeded) {
			reason = "timeout"
		}
		failed(reason, status == "failed", runErr.Error())
		publish(map[string]any{"ok": false, "error": runErr.Error()})
	}
	return true, nil
}

// heartbeat renews the lease while the job runs and watches for a cancellation
// request; returns a stop function that joins the goroutine.
//
// The renewal is fenced on the attempt that started it. Without the fence this
// sequence silently loses work: the heartbeat misses enough ticks for the lease
// to expire, another worker requeues and re-claims the job, and the first worker
// — still running — finishes and stamps its own result over the second attempt.
// No rows back means the row is no longer ours, and the right response is to
// stop touching it.
//
// onCancel is called at most once, and the caller decides what it means; the
// reason distinguishes a genuine cancellation from a lost lease, because only
// the former is an administrative act worth recording as one.
func (e *Engine) heartbeat(ctx context.Context, id int64, attempt int, onCancel func(reason string)) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		renew := time.NewTicker(jobLeaseRenew)
		defer renew.Stop()
		poll := time.NewTicker(jobCancelPoll)
		defer poll.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-renew.C:
				var cancelReq *time.Time
				err := e.DB.Sys.QueryRow(ctx, `UPDATE ddcore_job SET lease_until = now() + $2::interval
					WHERE id = $1 AND status = 'running' AND attempts = $3
					RETURNING cancel_requested`, id, jobLease.String(), attempt).Scan(&cancelReq)
				switch {
				case errors.Is(err, pgx.ErrNoRows):
					onCancel("lease lost")
					return
				case err != nil:
					// Not fatal on its own: the lease has room for a missed tick.
					// It used to be discarded, which is how a dying heartbeat
					// became a silently duplicated job.
					e.Log.Warn("heartbeat", "id", id, "err", err)
				case cancelReq != nil:
					onCancel("cancelled")
					return
				}
			case <-poll.C:
				var cancelReq *time.Time
				var status string
				if err := e.DB.Sys.QueryRow(ctx,
					`SELECT status, cancel_requested FROM ddcore_job WHERE id = $1`, id).
					Scan(&status, &cancelReq); err != nil {
					continue
				}
				if cancelReq != nil {
					onCancel("cancelled")
					return
				}
			}
		}
	}()
	return func() { close(done); <-stopped }
}

func orInt(v float64, d int) int {
	if n := int(v); n > 0 {
		return n
	}
	return d
}

func orJSON(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
}

// jobTenantColumn is the tenant of a job row in a select list or a RETURNING.
// The column is there only on a site with tenancy; without it every job is
// the site's.
func jobTenantColumn(tenancy bool) string {
	if tenancy {
		return `, tenant`
	}
	return `, ''::text AS tenant`
}

// jobSpace is ctx naming the tenant a job row belongs to, so that the job's
// code, its callbacks and the Error Log of its failure all run there.
func jobSpace(ctx context.Context, row map[string]any) context.Context {
	if tenant := db.Str(row["tenant"]); tenant != "" {
		return WithTenant(ctx, tenant)
	}
	return ctx
}

// LogError writes an Error Log document outside the failed transaction, in
// the tenant ctx names, if it names one.
func (e *Engine) LogError(ctx context.Context, method string, err error) {
	e.recordError(ctx, method, err.Error(), nil, nil)
}

// scheduler is the running cron and the context it was started with, which a
// reload needs to start its replacement.
type scheduler struct {
	cr  *cron.Cron
	ctx context.Context
}

// StartScheduler registers cron entries from every app's scheduler block and
// replaces the scheduler already running, if any. Once started, every
// successful Load rebuilds it from the reloaded apps.
func (e *Engine) StartScheduler(ctx context.Context) *cron.Cron {
	// the site's midnight, not the process's: "daily" means the start of the
	// day the business is having, and a server in another zone was firing it
	// hours early or late with nothing to show for it
	cr := cron.New(cron.WithLocation(e.Location()))
	add := func(spec string, fns []any) {
		for _, f := range fns {
			method, runAs := scheduledEntry(f)
			if method == "" {
				e.Log.Error("scheduler: entry without a method", "spec", spec, "entry", fmt.Sprint(f))
				continue
			}
			// the ledger key tells apart one method scheduled for several users
			entry, opts := spec+" "+method, map[string]any{"queue": "scheduler"}
			if runAs != "" {
				entry += " as " + runAs
				opts["runAs"] = runAs
			}
			cr.AddFunc(spec, func() {
				if e.Paused(ctx) {
					e.Log.Info("scheduler: skipped, site in maintenance", "method", method)
					return
				}
				if !e.claimTick(ctx, entry) {
					return
				}
				e.Log.Info("scheduler", "method", method, "runAs", runAs)
				// an entry whose runAs user is missing or disabled is refused here
				if err := e.Run(ctx, "Admin", func(c *Ctx) error {
					_, err := c.Enqueue(method, nil, opts)
					return err
				}); err != nil {
					e.Log.Error("scheduler: could not enqueue", "method", method, "runAs", runAs, "err", err)
				}
			})
		}
	}
	for _, app := range e.Current().Snap.Apps {
		for key, v := range app.Scheduler {
			switch key {
			case "cron":
				if m, ok := v.(map[string]any); ok {
					for spec, fns := range m {
						if list, ok := fns.([]any); ok {
							add(spec, list)
						}
					}
				}
			default:
				spec := map[string]string{"all": "*/5 * * * *", "hourly": "0 * * * *", "daily": "0 0 * * *", "weekly": "0 0 * * 1", "monthly": "0 0 1 * *"}[key]
				if list, ok := v.([]any); ok && spec != "" {
					add(spec, list)
				}
			}
		}
	}
	add("*/5 * * * *", []any{notificationSweepMethod})
	cr.Start()
	if old := e.sched.Swap(&scheduler{cr: cr, ctx: ctx}); old != nil {
		old.cr.Stop()
	}
	return cr
}

// scheduledEntry reads one scheduler entry: a method path, or an object with
// the path and the user the method runs as.
func scheduledEntry(f any) (method, runAs string) {
	switch x := f.(type) {
	case string:
		return x, ""
	case map[string]any:
		method, _ = x["method"].(string)
		runAs, _ = x["runAs"].(string)
	}
	return method, runAs
}

// scheduledEntries renders a list of entries for `ddcore jobs scheduled`.
func scheduledEntries(v any) string {
	list, ok := v.([]any)
	if !ok {
		return fmt.Sprint(v)
	}
	out := make([]string, 0, len(list))
	for _, f := range list {
		method, runAs := scheduledEntry(f)
		if runAs != "" {
			method += " (as " + runAs + ")"
		}
		out = append(out, method)
	}
	return "[" + strings.Join(out, " ") + "]"
}

// claimTick reports whether this process is the one to enqueue a cron entry's
// run for the current minute. Every replica runs a scheduler and every one of
// them fires; the first to record the (entry, minute) pair enqueues and the
// rest see the row and stand down, so a job runs once however many replicas
// there are. A database migrated by an older binary has no ledger yet: the
// entry is enqueued then, as it always was.
func (e *Engine) claimTick(ctx context.Context, entry string) bool {
	tick := time.Now().Truncate(time.Minute)
	claimed := true
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		tag, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_scheduler_tick (entry, tick) VALUES ($1, $2) ON CONFLICT DO NOTHING`, entry, tick)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() == 1
		// the ledger only has to outlive the minute; a day is ample
		_, err = c.Tx.Exec(ctx, `DELETE FROM ddcore_scheduler_tick WHERE entry = $1 AND tick < $2`, entry, tick.Add(-24*time.Hour))
		return err
	})
	if err != nil {
		e.Log.Warn("scheduler: could not record the run, enqueuing anyway", "entry", entry, "err", err)
		return true
	}
	return claimed
}

// RestartScheduler rebuilds the cron entries from the current state and stops
// the previous scheduler. Load already does this for a running scheduler.
func (e *Engine) RestartScheduler(ctx context.Context) *cron.Cron {
	return e.StartScheduler(ctx)
}

// StopScheduler stops the scheduler currently registered, if any, and waits
// for the entries already firing to finish enqueuing — at most a few seconds,
// so a process stopping does not exit under one.
func (e *Engine) StopScheduler() {
	if old := e.sched.Swap(nil); old != nil {
		select {
		case <-old.cr.Stop().Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// ScheduledMethods lists everything the scheduler would run (for `ddcore jobs list`).
func (e *Engine) ScheduledMethods() []string {
	var out []string
	for _, app := range e.Current().Snap.Apps {
		for key, v := range app.Scheduler {
			switch x := v.(type) {
			case map[string]any:
				for spec, fns := range x {
					out = append(out, fmt.Sprintf("%s  %s  %s", spec, key, scheduledEntries(fns)))
				}
			case []any:
				out = append(out, fmt.Sprintf("%s  %s", key, scheduledEntries(x)))
			}
		}
	}
	_ = strings.Join
	return out
}
