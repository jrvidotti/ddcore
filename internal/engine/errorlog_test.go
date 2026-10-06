package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// errorLogFiles plants, next to the Job Trace DocType of jobHookFiles, the
// steps of a job that catches its failures and files them: a trace row is the
// witness of which writes committed, and an Error Log row of what was filed.
var errorLogFiles = map[string]string{
	"doctypes/job_trace/job_trace.doctype.ts": jobHookFiles["doctypes/job_trace/job_trace.doctype.ts"],
	"services/steps.ts": `function trace(hook: string) { ddcore.newDoc("Job Trace", { hook, job: "{}" }).insert(); }
export function recordThenThrow() {
  trace("body");
  ddcore.errorLog.record(new Error("step failed"), { context: { step: 1 } });
  throw new Error("body failed");
}
export function recordAndGoOn() {
  trace("kept");
  try {
    ddcore.db.savepoint(() => { trace("undone"); ddcore.throw("gateway down", { type: "ValidationError", title: "Reconcile" }); });
  } catch (e) {
    ddcore.errorLog.record(e, { context: { charge: "C-1", tries: 2 } });
  }
  try {
    ddcore.db.savepoint(() => { trace("undone"); ddcore.errorLog.record("filed inside a savepoint", { method: "steps.inner" }); throw new Error("x"); });
  } catch (_) {}
  return { ok: true };
}`,
}

type errorLogRow struct{ id, method, text, requestID string }

func errorLogRows(t *testing.T, e *Engine, requestID string) []errorLogRow {
	t.Helper()
	rows, err := e.DB.Sys.Query(context.Background(),
		`SELECT id, method, error, COALESCE(request_id, '') FROM tab_error_log
		WHERE COALESCE(request_id, '') = $1 ORDER BY creation, id`, requestID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []errorLogRow
	for rows.Next() {
		var r errorLogRow
		if err := rows.Scan(&r.id, &r.method, &r.text, &r.requestID); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func enqueuePlain(t *testing.T, e *Engine, method string) int64 {
	t.Helper()
	var id int64
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue(method, nil, map[string]any{"maxAttempts": 1})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// see enqueueWithHooks: the claim compares with the database's clock
	if _, err := e.DB.Pool.Exec(context.Background(),
		`UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The row a job files is on a transaction of its own: the job's throw takes
// back the job's writes and leaves the row, under the job's handle.
func TestErrorLogRecordSurvivesTheJobThrowing(t *testing.T) {
	e := setupWith(t, errorLogFiles)
	id := enqueuePlain(t, e, "demo.services.steps.recordThenThrow")
	if _, err := e.runOneJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := readJob(t, e, id)["status"]; s != "failed" {
		t.Fatalf("status = %v, want failed", s)
	}
	if h := hooksOf(traces(t, e)); h != "" {
		t.Fatalf("traces = %q; the job's own write should have rolled back", h)
	}
	handle := fmt.Sprintf("job:%d", id)
	rows := errorLogRows(t, e, handle)
	if len(rows) != 2 {
		t.Fatalf("rows under %s = %+v, want the recorded one and the failure", handle, rows)
	}
	rec := rows[0]
	if rec.method != "job:demo.services.steps.recordThenThrow" ||
		!strings.HasPrefix(rec.text, "Error: step failed") || !strings.Contains(rec.text, `Context: {"step":1}`) {
		t.Fatalf("recorded row = %+v", rec)
	}
	if !strings.Contains(rows[1].text, "body failed") {
		t.Fatalf("failure row = %+v", rows[1])
	}
}

// The case the API exists for: a job catches a failing step, files it, and
// finishes with the other steps' work committed.
func TestErrorLogRecordLetsTheJobGoOn(t *testing.T) {
	e := setupWith(t, errorLogFiles)
	id := enqueuePlain(t, e, "demo.services.steps.recordAndGoOn")
	if _, err := e.runOneJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := readJob(t, e, id)["status"]; s != "done" {
		t.Fatalf("status = %v, want done", s)
	}
	if h := hooksOf(traces(t, e)); h != "kept" {
		t.Fatalf("traces = %q, want only the step that succeeded", h)
	}
	rows := errorLogRows(t, e, fmt.Sprintf("job:%d", id))
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the two recorded", rows)
	}
	caught := rows[0]
	if caught.method != "job:demo.services.steps.recordAndGoOn" ||
		!strings.HasPrefix(caught.text, "ValidationError: Reconcile — gateway down\n\n") ||
		!strings.Contains(caught.text, "\n\tat services/steps.ts:") || strings.Contains(caught.text, "throw (prelude.js") ||
		!strings.HasSuffix(caught.text, `Context: {"charge":"C-1","tries":2}`) {
		t.Fatalf("caught row = %q %q", caught.method, caught.text)
	}
	// filed inside a savepoint that then rolled back: the row is not the
	// savepoint's, so it stays
	if inner := rows[1]; inner.method != "steps.inner" || inner.text != "filed inside a savepoint" {
		t.Fatalf("inner row = %+v", inner)
	}
	if h, err := e.ErrorHealth(context.Background(), 15*time.Minute, 5); err != nil || h.InWindow < 2 {
		t.Fatalf("doctor does not count the recorded rows: %+v %v", h, err)
	}
}

// In a request the row carries the request's id, and returns the row's id;
// the request rolling back leaves the row.
func TestErrorLogRecordInARequest(t *testing.T) {
	e := setupWith(t, errorLogFiles)
	ctx := WithRequestID(context.Background(), "errlog-req-0001")
	out, _, err := e.Eval(ctx, `ddcore.newDoc("Job Trace", { hook: "req", job: "{}" }).insert();
ddcore.errorLog.record("plain words", { context: { a: 1 } })`, false)
	if err != nil {
		t.Fatal(err)
	}
	rows := errorLogRows(t, e, "errlog-req-0001")
	if len(rows) != 1 || rows[0].method != "app.record" || rows[0].text != "plain words\n\nContext: {\"a\":1}" {
		t.Fatalf("rows = %+v", rows)
	}
	if string(out) != `"`+rows[0].id+`"` {
		t.Fatalf("record returned %s, the row is %s", out, rows[0].id)
	}
	if h := hooksOf(traces(t, e)); h != "" {
		t.Fatalf("traces = %q; the eval rolled back", h)
	}
}

// Outside any request or job — a migration, a CLI command — the row has no
// handle, and a context JSON cannot hold costs the row its context, not the
// caller a throw. Maintenance does not stop it either.
func TestErrorLogRecordWithoutAHandle(t *testing.T) {
	e := setupWith(t, errorLogFiles)
	ctx := context.Background()
	if _, err := e.SetMaintenance(ctx, true, "cutover", "tester"); err != nil {
		t.Fatal(err)
	}
	e.Cfg.EnforceMaintenance = true
	t.Cleanup(func() { e.Cfg.EnforceMaintenance = false })
	out, _, err := e.Eval(ctx, `const o: any = {}; o.self = o;
ddcore.errorLog.record({ message: "an object" }, { context: o })`, false)
	if err != nil {
		t.Fatal(err)
	}
	rows := errorLogRows(t, e, "")
	if len(rows) != 1 || string(out) != `"`+rows[0].id+`"` || rows[0].method != "app.record" ||
		rows[0].text != "an object\n\nContext: [object Object]" {
		t.Fatalf("out = %s, rows = %+v", out, rows)
	}
}

// A tenant's failure is filed in that tenant, where its own operators look.
func TestErrorLogRecordInATenant(t *testing.T) {
	e := setupTenancy(t)
	ctx := WithRequestID(WithTenant(context.Background(), tenantA), "errlog-tenant-01")
	if _, _, err := e.Eval(ctx, `ddcore.errorLog.record(new TypeError("bad"))`, false); err != nil {
		t.Fatal(err)
	}
	var tenant, text string
	if err := e.DB.Sys.QueryRow(context.Background(),
		`SELECT tenant, error FROM tab_error_log WHERE request_id = 'errlog-tenant-01'`).Scan(&tenant, &text); err != nil {
		t.Fatal(err)
	}
	if tenant != tenantA || !strings.HasPrefix(text, "TypeError: bad") {
		t.Fatalf("row in %q: %q", tenant, text)
	}
}

func TestErrorLogTextIsCapped(t *testing.T) {
	big := strings.Repeat("é", errorLogHeadMax)
	got := AppError{Message: big, Context: strings.Repeat("x", 3*errorLogContextMax)}.text()
	if len(got) > errorLogHeadMax+errorLogContextMax+200 {
		t.Fatalf("text is %d bytes", len(got))
	}
	if !strings.Contains(got, "bytes cut]") || !strings.Contains(got, "\n\nContext: xxx") {
		t.Fatalf("cut text lost its shape: %q", got[len(got)-80:])
	}
	if head := strings.SplitN(got, "\n\n", 2)[0]; !strings.HasPrefix(head, "éé") || strings.ContainsRune(head, '�') {
		t.Fatalf("the cut split a rune")
	}
}

// The row's write runs on the caller's VM. Taking a second one from the pool
// would hang every caller at once on a busy site, each holding the slot the
// others wait for.
func TestErrorLogRecordDoesNotTakeASecondVM(t *testing.T) {
	e := setupWith(t, errorLogFiles)
	pool := e.Current().Pool
	for i := 0; i < e.Cfg.Workers+4-1; i++ {
		rt, err := pool.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Release(rt)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := e.Eval(WithRequestID(context.Background(), "errlog-busy-01"), `ddcore.errorLog.record("busy")`, false)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("record waited for a second VM from a full pool")
	}
	if rows := errorLogRows(t, e, "errlog-busy-01"); len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
}
