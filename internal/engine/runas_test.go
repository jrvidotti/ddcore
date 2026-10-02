package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/js"
)

const (
	runAsAlfa = "alfa@x.com"
	runAsBeta = "beta@x.com"
)

// runAsService is the app code under test: a job body that switches user, and
// the plain bodies and callback a job queued with runAs runs.
const runAsService = `function titles() {
  return ddcore.db.getList("Test Record", { fields: ["title"], orderBy: "title asc" }).map((r: any) => r.title);
}
export function report(args: any) {
  const out: any = { outer: ddcore.session.user, all: titles() };
  out.inner = ddcore.runAs(args.user, () => {
    const seen = titles();
    ddcore.newDoc("Test Record", { title: "From RunAs", company: "Alfa" }).insert();
    let refused = false;
    try { ddcore.newDoc("Test Record", { title: "Leak", company: "Beta" }).insert(); } catch (e) { refused = true; }
    ddcore.audit("tenant.sync", "Test Record", "From RunAs");
    const nested = ddcore.runAs("Admin", () => ddcore.session.user);
    return { user: ddcore.session.user, seen, refused, nested, isJob: ddcore.isJob() };
  });
  out.after = ddcore.session.user;
  try { ddcore.runAs(args.user, () => { throw new Error("boom"); }); } catch (e) { out.thrown = String(e); }
  out.afterThrow = ddcore.session.user;
  out.allAfter = titles().length;
  return out;
}
export function visible() { return { user: ddcore.session.user, titles: titles() }; }
export function started() { ddcore.newDoc("Test Record", { title: "Started", company: "Alfa" }).insert(); }
export function leak() { ddcore.newDoc("Test Record", { title: "Leak", company: "Beta" }).insert(); }
export function spin(args: any) { ddcore.runAs(args.user, () => { let n = 0; while (true) { n++ } }); }
export function tick() { return ddcore.session.user; }`

// setupRunAs is the SEC01 fixture outside test mode, because ddcore.runAs is a
// production API: two companies with a record each, and one user scoped to
// each company.
func setupRunAs(t *testing.T) *Engine {
	t.Helper()
	dir := sec01App(t)
	writeAppFile(t, dir, "services/work.ts", runAsService)
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "scope_test", Dir: dir}}})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, company := range []string{"Alfa", "Beta"} {
			if err := insertDoc(c, "Test Company", Doc{"title": company}); err != nil {
				return err
			}
			if err := insertDoc(c, "Test Record", Doc{"title": company + " Record", "company": company}); err != nil {
				return err
			}
			user := strings.ToLower(company) + "@x.com"
			if err := insertDoc(c, "User", Doc{"email": user, "full_name": user,
				"roles": []any{map[string]any{"role": "Scope User"}}}); err != nil {
				return err
			}
			if err := insertDoc(c, "User Permission", Doc{"user": user, "allow": "Test Company", "for_value": company}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func enqueueRunAs(t *testing.T, e *Engine, method string, args, opts map[string]any) int64 {
	t.Helper()
	var id int64
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue(method, args, opts)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(context.Background(),
		`UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func jobRow(t *testing.T, e *Engine, id int64) (status, runAs, errText string, result map[string]any) {
	t.Helper()
	var res []byte
	if err := e.DB.Pool.QueryRow(context.Background(),
		`SELECT status, COALESCE(run_as, ''), COALESCE(error, ''), result FROM ddcore_job WHERE id = $1`, id).
		Scan(&status, &runAs, &errText, &res); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(res, &result)
	return
}

func recordField(t *testing.T, e *Engine, title, field string) string {
	t.Helper()
	var v string
	if err := e.DB.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(`+field+`, '') FROM tab_test_record WHERE title = $1`, title).Scan(&v); err != nil {
		t.Fatalf("record %q: %v", title, err)
	}
	return v
}

// Inside an ordinary job — permissions ignored — ddcore.runAs applies the
// user's roles and scopes to reads and writes, and hands the job back its own
// context afterwards.
func TestRunAsScopesCodeInsideAJob(t *testing.T) {
	e := setupRunAs(t)
	ctx := context.Background()
	raw, err := e.RunJob(ctx, "Admin", "scope_test.services.work.report", map[string]any{"user": runAsAlfa})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Outer, After, AfterThrow, Thrown string
		All                              []string
		AllAfter                         int
		Inner                            struct {
			User, Nested string
			Seen         []string
			Refused      bool
			IsJob        bool
		}
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.All) != 2 {
		t.Errorf("the job itself saw %v, want both records", out.All)
	}
	if out.Inner.User != runAsAlfa {
		t.Errorf("session.user inside runAs = %q", out.Inner.User)
	}
	if len(out.Inner.Seen) != 1 || out.Inner.Seen[0] != "Alfa Record" {
		t.Errorf("runAs saw %v, want only the Alfa record", out.Inner.Seen)
	}
	if !out.Inner.Refused {
		t.Error("an insert outside the user's scope was accepted")
	}
	if out.Inner.Nested != "Admin" {
		t.Errorf("nested runAs ran as %q", out.Inner.Nested)
	}
	if !out.Inner.IsJob {
		t.Error("isJob turned false inside runAs")
	}
	if out.Outer != "Admin" || out.After != "Admin" || out.AfterThrow != "Admin" {
		t.Errorf("caller was %q, then %q, then %q after a throw", out.Outer, out.After, out.AfterThrow)
	}
	if !strings.Contains(out.Thrown, "boom") {
		t.Errorf("the callback's error was %q", out.Thrown)
	}
	if out.AllAfter != 3 {
		t.Errorf("after runAs the job saw %d records, want 3: its own context ignores permissions", out.AllAfter)
	}
	if owner, by := recordField(t, e, "From RunAs", "owner"), recordField(t, e, "From RunAs", "modified_by"); owner != runAsAlfa || by != runAsAlfa {
		t.Errorf("owner = %q, modified_by = %q, want the effective user", owner, by)
	}
	events, err := e.ListAuditEvents(ctx, AuditFilter{Action: "tenant.sync"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0]["actor"] != runAsAlfa {
		t.Errorf("audit events = %v, want one with the effective user as actor", events)
	}
}

func TestRunAsRefusesAMissingOrDisabledUser(t *testing.T) {
	e := setupRunAs(t)
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_user SET enabled = false WHERE id = $1`, runAsBeta); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"nobody@x.com", runAsBeta, ""} {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			if _, err := c.RT(); err != nil {
				return err
			}
			if err := c.runAsEnter(user); err == nil {
				t.Errorf("runAs(%q) was accepted", user)
			}
			if rt, _ := c.RT(); rt.Ctx != any(c) {
				t.Errorf("a refused runAs(%q) left the VM switched", user)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// What a ctx acting as another user registers for after the commit runs when
// the transaction commits: the document events and cache invalidations of a
// write made under runAs depend on it.
func TestRunAsKeepsAfterCommitCallbacks(t *testing.T) {
	e := setupRunAs(t)
	fired, undone := false, false
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		if err := c.runAsEnter(runAsAlfa); err != nil {
			return err
		}
		child := rt.Ctx.(*Ctx)
		child.AfterCommit(func() { fired = true })
		child.WithSavepoint(func() error {
			child.AfterCommit(func() { undone = true })
			return os.ErrInvalid
		})
		if child.restoreUser() != c {
			t.Error("restoreUser did not return the caller")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Error("an after-commit callback registered under runAs never ran")
	}
	if undone {
		t.Error("a callback registered in a rolled-back savepoint ran")
	}
}

// A job queued with runAs runs its body and its callbacks as that user, with
// permissions enforced; "user" stays whoever queued it.
func TestEnqueueRunAsScopesTheWholeJob(t *testing.T) {
	e := setupRunAs(t)
	ctx := context.Background()
	id := enqueueRunAs(t, e, "scope_test.services.work.visible", nil,
		map[string]any{"runAs": runAsAlfa, "onStart": "scope_test.services.work.started"})
	if ran, err := e.runOneJob(ctx); err != nil || !ran {
		t.Fatalf("runOneJob: %v %v", ran, err)
	}
	status, runAs, errText, res := jobRow(t, e, id)
	if status != "done" || runAs != runAsAlfa {
		t.Fatalf("status = %s, run_as = %q, error = %q", status, runAs, errText)
	}
	if res["user"] != runAsAlfa {
		t.Errorf("session.user = %v", res["user"])
	}
	if got := res["titles"]; len(got.([]any)) != 2 || got.([]any)[0] != "Alfa Record" || got.([]any)[1] != "Started" {
		t.Errorf("the job saw %v, want the Alfa record and the one onStart wrote", got)
	}
	if owner := recordField(t, e, "Started", "owner"); owner != runAsAlfa {
		t.Errorf("onStart wrote as %q", owner)
	}
	j, err := e.GetJob(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if j["user"] != "Admin" || j["run_as"] != runAsAlfa {
		t.Errorf("listing shows user = %v, run_as = %v", j["user"], j["run_as"])
	}

	// a write outside the scope fails the job, where a plain job would make it
	id = enqueueRunAs(t, e, "scope_test.services.work.leak", nil, map[string]any{"runAs": runAsAlfa, "maxAttempts": 1})
	if _, err := e.runOneJob(ctx); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := jobRow(t, e, id); status != "failed" {
		t.Errorf("a scoped job writing outside its scope ended %s", status)
	}
	id = enqueueRunAs(t, e, "scope_test.services.work.leak", nil, map[string]any{"maxAttempts": 1})
	if _, err := e.runOneJob(ctx); err != nil {
		t.Fatal(err)
	}
	if status, runAs, errText, _ := jobRow(t, e, id); status != "done" || runAs != "" {
		t.Errorf("a job without runAs ended %s (run_as %q): %s", status, runAs, errText)
	}
}

func TestEnqueueRunAsValidatesTheUser(t *testing.T) {
	e := setupRunAs(t)
	ctx := context.Background()
	for _, v := range []any{"nobody@x.com", "", 7} {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			_, err := c.Enqueue("scope_test.services.work.visible", nil, map[string]any{"runAs": v})
			return err
		})
		if err == nil {
			t.Errorf("enqueue accepted runAs = %v", v)
		}
	}

	// disabled after it was queued: the job fails instead of running unscoped
	id := enqueueRunAs(t, e, "scope_test.services.work.visible", nil, map[string]any{"runAs": runAsBeta, "maxAttempts": 1})
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_user SET enabled = false WHERE id = $1`, runAsBeta); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runOneJob(ctx); err != nil {
		t.Fatal(err)
	}
	status, _, errText, _ := jobRow(t, e, id)
	if status != "failed" || !strings.Contains(errText, "disabled") {
		t.Errorf("status = %s, error = %q", status, errText)
	}

	// and a retry is still a job for that user
	act, err := e.RetryJob(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, runAs, _, _ := jobRow(t, e, act.NewID); runAs != runAsBeta {
		t.Errorf("the retry has run_as = %q", runAs)
	}
}

// A timeout interrupts the VM inside runAs, where the JS finally may not run.
// The pooled VM must come back as whoever acquires it next.
func TestRunAsDoesNotOutliveAnInterruptedJob(t *testing.T) {
	e := setupRunAs(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := e.RunJob(ctx, "Admin", "scope_test.services.work.spin", map[string]any{"user": runAsAlfa}); err == nil {
		t.Fatal("the spinning job returned without an error")
	}
	for i := 0; i < 4; i++ {
		raw, err := e.RunJob(context.Background(), "Admin", "scope_test.services.work.tick", nil)
		if err != nil || string(raw) != `"Admin"` {
			t.Fatalf("after the interrupt a job ran as %s (%v)", raw, err)
		}
	}
}

// A scheduler entry may be an object naming the user its method runs as.
func TestSchedulerEntryRunAs(t *testing.T) {
	e := setupRunAs(t)
	ctx := context.Background()
	app := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "scope_test", title: "Scope Test", roles: ["Scope User"],
  scheduler: { all: ["scope_test.services.work.tick", { method: "scope_test.services.work.visible", runAs: "` + runAsAlfa + `" }] } });`
	if err := os.WriteFile(filepath.Join(e.Cfg.Apps[0].Dir, "ddcore.app.ts"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.ScheduledMethods(), "\n"); !strings.Contains(got, "scope_test.services.work.visible (as "+runAsAlfa+")") {
		t.Errorf("ScheduledMethods = %q", got)
	}
	cr := e.StartScheduler(ctx)
	defer e.StopScheduler()
	for _, entry := range cr.Entries() {
		entry.Job.Run()
	}
	rows, err := e.DB.Pool.Query(ctx, `SELECT method, COALESCE(run_as, ''), "user" FROM ddcore_job WHERE queue = 'scheduler' ORDER BY method`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var method, runAs, user string
		if err := rows.Scan(&method, &runAs, &user); err != nil {
			t.Fatal(err)
		}
		got[method] = user + "/" + runAs
	}
	if got["scope_test.services.work.tick"] != "Admin/" || got["scope_test.services.work.visible"] != "Admin/"+runAsAlfa {
		t.Errorf("scheduled jobs = %v", got)
	}
}
