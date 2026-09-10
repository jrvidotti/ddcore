package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Job execution limits. O lease é renovado por heartbeat enquanto o worker
// estiver vivo; se o processo cair, o job volta para a fila quando o lease
// vencer (B15).
const (
	jobLease          = 2 * time.Minute
	jobHeartbeat      = 30 * time.Second
	defaultJobTimeout = 300 // segundos
)

// Enqueue stores a job in ddcore_job; workers pick it with SKIP LOCKED.
func (c *Ctx) Enqueue(method string, args map[string]any, opts map[string]any) (int64, error) {
	if method == "" {
		return 0, cerr.Validation("enqueue: provide the method")
	}
	queue := "default"
	if q, ok := opts["queue"].(string); ok && q != "" {
		queue = q
	}
	runAfter := time.Now()
	if ra, ok := opts["runAfter"].(string); ok && ra != "" {
		if t := parseTime(ra); !t.IsZero() {
			runAfter = t
		}
	}
	timeout := defaultJobTimeout
	if t, ok := opts["timeout"]; ok {
		if n := int(toFloat(t)); n > 0 {
			timeout = n
		}
	}
	b, _ := json.Marshal(args)
	var id int64
	err := c.Q().QueryRow(c.Ctx, `INSERT INTO ddcore_job (method, args, queue, "user", run_after, timeout_seconds) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		method, string(b), queue, c.User, runAfter, timeout).Scan(&id)
	return id, err
}

// RunJob executes one job payload: a dotted function path with args. O ctx é
// respeitado dentro da VM: um script que não retorna é interrompido quando o
// contexto expira (B15).
func (e *Engine) RunJob(ctx context.Context, user, method string, args map[string]any) (json.RawMessage, error) {
	var out json.RawMessage
	// A transação usa um contexto sem o deadline: o timeout precisa
	// interromper a VM, não estragar o rollback da transação.
	err := e.Run(context.WithoutCancel(ctx), orDefault(user, "Administrator"), func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		rt, err := c.RT()
		if err != nil {
			return err
		}
		b, _ := json.Marshal(args)
		return rt.WithContext(ctx, func() error {
			var callErr error
			out, callErr = rt.CallFunction(method, b)
			return callErr
		})
	})
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, cerr.Validation("job {0} timed out", method)
	}
	return out, err
}

// requeueStale puts back jobs whose worker died: status running com lease
// vencido volta para queued (ou failed, se esgotou as tentativas).
func (e *Engine) requeueStale(ctx context.Context) error {
	_, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job
		SET status = CASE WHEN attempts < max_attempts THEN 'queued' ELSE 'failed' END,
		    lease_until = NULL, finished = CASE WHEN attempts < max_attempts THEN NULL ELSE now() END,
		    error = 'worker interrompido: lease expirou'
		WHERE status = 'running' AND lease_until IS NOT NULL AND lease_until < now()`)
	return err
}

// Worker loops over queued jobs until ctx is cancelled.
func (e *Engine) Worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := e.requeueStale(ctx); err != nil {
			e.Log.Error("requeue", "id", id, "err", err)
		}
		ran, err := e.runOneJob(ctx)
		if err != nil {
			e.Log.Error("worker", "id", id, "err", err)
		}
		if !ran {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (e *Engine) runOneJob(ctx context.Context) (bool, error) {
	tx, err := e.DB.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	rows, err := db.Select(ctx, tx, `SELECT id, method, args, "user", attempts, max_attempts, timeout_seconds FROM ddcore_job
		WHERE status = 'queued' AND run_after <= now() ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`)
	if err != nil || len(rows) == 0 {
		return false, err
	}
	j := rows[0]
	id := int64(toFloat(j["id"]))
	if _, err := tx.Exec(ctx, `UPDATE ddcore_job SET status = 'running', started = now(), attempts = attempts + 1, lease_until = now() + $2::interval WHERE id = $1`,
		id, jobLease.String()); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	var args map[string]any
	if s, ok := j["args"].(string); ok {
		json.Unmarshal([]byte(s), &args)
	} else if m, ok := j["args"].(map[string]any); ok {
		args = m
	}
	method := db.Str(j["method"])
	e.Log.Info("job", "id", id, "method", method)

	timeout := time.Duration(orInt(toFloat(j["timeout_seconds"]), defaultJobTimeout)) * time.Second
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	stopBeat := e.heartbeat(ctx, id)
	res, runErr := e.RunJob(jobCtx, db.Str(j["user"]), method, args)
	stopBeat()
	cancel()
	if runErr != nil {
		attempts, max := int(toFloat(j["attempts"]))+1, int(toFloat(j["max_attempts"]))
		status := "failed"
		if attempts < max {
			status = "queued"
		}
		e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = $2, error = $3, finished = now(), lease_until = NULL, run_after = now() + interval '30 seconds' WHERE id = $1`, id, status, runErr.Error())
		e.LogError(ctx, "job:"+method, runErr)
		e.Events.Publish(Event{Name: "job_done", Payload: map[string]any{"id": id, "method": method, "ok": false, "error": runErr.Error()}, User: db.Str(j["user"])})
		return true, nil
	}
	e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'done', finished = now(), lease_until = NULL, result = $2 WHERE id = $1`, id, string(orJSON(res)))
	e.Events.Publish(Event{Name: "job_done", Payload: map[string]any{"id": id, "method": method, "ok": true}, User: db.Str(j["user"])})
	return true, nil
}

// heartbeat renews the lease while the job runs; returns a stop function.
func (e *Engine) heartbeat(ctx context.Context, id int64) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		t := time.NewTicker(jobHeartbeat)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET lease_until = now() + $2::interval WHERE id = $1 AND status = 'running'`, id, jobLease.String())
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

// LogError writes an Error Log document outside the failed transaction.
func (e *Engine) LogError(ctx context.Context, method string, err error) {
	e.Log.Error(err.Error(), "method", method)
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, _ := c.NewDoc("Error Log", Doc{"method": method, "error": err.Error()})
		_, e := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		return e
	})
}

// StartScheduler registers cron entries from every app's scheduler block.
func (e *Engine) StartScheduler(ctx context.Context) *cron.Cron {
	cr := cron.New()
	add := func(spec string, fns []any) {
		for _, f := range fns {
			method := fmt.Sprint(f)
			cr.AddFunc(spec, func() {
				e.Log.Info("scheduler", "method", method)
				e.Run(ctx, "Administrator", func(c *Ctx) error {
					_, err := c.Enqueue(method, nil, map[string]any{"queue": "scheduler"})
					return err
				})
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
	cr.Start()
	if old := e.sched.Swap(cr); old != nil {
		old.Stop()
	}
	return cr
}

// RestartScheduler rebuilds the cron entries from the current state and stops
// the previous scheduler. Deve ser chamada depois de cada e.Load(): recarregar
// a meta não reinstalava as entradas do scheduler criado uma única vez no
// boot (B08).
func (e *Engine) RestartScheduler(ctx context.Context) *cron.Cron {
	return e.StartScheduler(ctx)
}

// StopScheduler stops the scheduler currently registered, if any.
func (e *Engine) StopScheduler() {
	if old := e.sched.Swap(nil); old != nil {
		old.Stop()
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
					out = append(out, fmt.Sprintf("%s  %s  %v", spec, key, fns))
				}
			case []any:
				out = append(out, fmt.Sprintf("%s  %v", key, x))
			}
		}
	}
	_ = strings.Join
	return out
}
