package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// holdLock takes key in a transaction of its own, in ctx's space, and keeps
// it until the returned release is called.
func holdLock(t *testing.T, e *Engine, ctx context.Context, key string) (release func()) {
	t.Helper()
	held, done := make(chan error, 1), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.Run(ctx, "Admin", func(c *Ctx) error {
			err := c.Lock(key)
			held <- err
			if err != nil {
				return err
			}
			<-done
			return nil
		})
	}()
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release = func() { once.Do(func() { close(done); wg.Wait() }) }
	t.Cleanup(release)
	return release
}

// tryLock asks for key in a transaction of its own, through the JS API.
func tryLock(t *testing.T, e *Engine, ctx context.Context, key string) bool {
	t.Helper()
	var got bool
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		out, err := evalIn(t, c, `ddcore.db.tryLock(`+jsonString(key)+`)`)
		got = out == "true"
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func isValidationErr(err error) bool {
	var ce *cerr.Error
	return errors.As(err, &ce) && ce.Type == "ValidationError"
}

func jsonString(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// #98: tryLock does not wait. It fails at once while another transaction holds
// the key — taken with lock, so the two meet — and takes it once that
// transaction has ended; the key is then held until its own transaction ends.
func TestTryLockFailsWhileHeldAndTakesTheKeyAfter(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	release := holdLock(t, e, ctx, "bot:reply:1")

	start := time.Now()
	if tryLock(t, e, ctx, "bot:reply:1") {
		t.Fatal("tryLock took a key another transaction holds")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("tryLock waited %v; it must answer at once", took)
	}
	if !tryLock(t, e, ctx, "bot:reply:2") {
		t.Error("tryLock refused a key nobody holds")
	}
	release()
	if !tryLock(t, e, ctx, "bot:reply:1") {
		t.Error("tryLock refused a key its holder released")
	}

	// held to the end of the transaction: a second one cannot have it meanwhile
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		if ok, err := c.TryLock("bot:reply:3"); err != nil || !ok {
			t.Fatalf("first tryLock = %v, %v", ok, err)
		}
		if tryLock(t, e, ctx, "bot:reply:3") {
			t.Error("another transaction took a key tryLock holds")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTryLockRefusesABlankKeyAndNoTransaction(t *testing.T) {
	e := setup(t)
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		if _, err := c.TryLock("  "); !isValidationErr(err) {
			t.Errorf("blank key: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c := &Ctx{E: e, Ctx: context.Background(), User: "Admin"}
	if _, err := c.TryLock("k"); !isValidationErr(err) {
		t.Errorf("no transaction: %v", err)
	}
}

// tryLock's key is the tenant's, as lock's is: another tenant and the platform
// take the same key freely, the same tenant does not.
func TestTryLockIsPerTenant(t *testing.T) {
	e := setupTenancy(t)
	holdLock(t, e, WithTenant(context.Background(), tenantA), "charge:1")
	if !tryLock(t, e, WithTenant(context.Background(), tenantB), "charge:1") {
		t.Error("another tenant was refused a tenant's key")
	}
	if !tryLock(t, e, context.Background(), "charge:1") {
		t.Error("the platform was refused a tenant's key")
	}
	if tryLock(t, e, WithTenant(context.Background(), tenantA), "charge:1") {
		t.Error("the same tenant took a key that is held")
	}
}

func enqueueErr(e *Engine, opts map[string]any) (int64, error) {
	var id int64
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.hooks.ok", nil, opts)
		return err
	})
	return id, err
}

// #98: a runAfter that cannot be read used to run the job now — a job meaning
// to wait turned into a hot loop. It is refused, and runAfterSeconds says the
// delay without a clock.
func TestEnqueueRunAfterIsValidated(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	for _, opts := range []map[string]any{
		{"runAfter": "tomorrow"},
		{"runAfter": 10.0},
		{"runAfterSeconds": -1.0},
		{"runAfterSeconds": "10"},
		{"runAfter": "2030-01-01T00:00:00Z", "runAfterSeconds": 10.0},
	} {
		if _, err := enqueueErr(e, opts); !isValidationErr(err) {
			t.Errorf("%v: want a validation error, got %v", opts, err)
		}
	}

	id, err := enqueueErr(e, map[string]any{"runAfterSeconds": 3600.0})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(readJob(t, e, id)["run_after"].(time.Time)); d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("runAfterSeconds 3600 runs in %v", d)
	}
	for _, at := range []string{"2030-01-02T03:04:05Z", "2030-01-02 03:04:05", "2030-01-02T03:04:05", "2030-01-02"} {
		id, err := enqueueErr(e, map[string]any{"runAfter": at})
		if err != nil {
			t.Errorf("%s: %v", at, err)
			continue
		}
		if y := readJob(t, e, id)["run_after"].(time.Time).Year(); y != 2030 {
			t.Errorf("%s: run_after in %d", at, y)
		}
	}
	// an empty runAfter is no runAfter, as it always was
	id, err = enqueueErr(e, map[string]any{"runAfter": "", "runAfterSeconds": 0.0})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(readJob(t, e, id)["run_after"].(time.Time)); d > time.Minute {
		t.Errorf("runAfterSeconds 0 runs in %v", d)
	}
}

// The pools' claim filters: a named pool takes its queue, the default pool
// everything else, and a lone default pool everything.
func TestWorkerPoolFilter(t *testing.T) {
	pools := map[string]int{"default": 2, "bot": 1, "mail": 1}
	if f := poolFilter(pools, "bot"); f.only != "bot" || f.except != nil {
		t.Errorf("bot: %+v", f)
	}
	if f := poolFilter(pools, "default"); f.only != "" || !reflect.DeepEqual(f.except, []string{"bot", "mail"}) {
		t.Errorf("default: %+v", f)
	}
	if f := poolFilter(map[string]int{"default": 2}, "default"); f.only != "" || f.except != nil {
		t.Errorf("lone default: %+v", f)
	}
	if w, p := (queueFilter{}).where(1); w != "" || p != nil {
		t.Errorf("no filter: %q %v", w, p)
	}
}

func enqueueOn(t *testing.T, e *Engine, queue, method string, args map[string]any) int64 {
	t.Helper()
	return enqueueWithHooks(t, e, method, args, map[string]any{"queue": queue})
}

// #98's case: with a pool per queue, a bot queue whose worker is stuck — here
// on an outbound call, in the app on a lock — does not keep a default job from
// running, and its next job waits for the bot pool instead of being taken by
// the default one.
func TestWorkerPoolsKeepASlowQueueFromStarvingTheRest(t *testing.T) {
	e := setupWith(t, shutdownFiles)
	restore := shortenJobPolling()
	defer restore()
	srv, requests, release := blockingServer(t)

	stuck := enqueueOn(t, e, "bot", "demo.services.slow.get", map[string]any{"url": srv.URL})
	next := enqueueOn(t, e, "bot", "demo.services.hooks.ok", nil)
	w := e.StartWorkerPools(map[string]int{"default": 1, "bot": 1})
	defer w.Stop(0)
	waitForStatus(t, e, stuck, "running")
	waitForRequest(t, requests)

	other := enqueueOn(t, e, "default", "demo.services.hooks.ok", nil)
	waitForStatus(t, e, other, "done")
	if s := readJob(t, e, next)["status"]; s != "queued" {
		t.Errorf("the bot queue's next job is %v: the default pool must leave it to the bot pool", s)
	}
	release()
	waitForStatus(t, e, stuck, "done")
	waitForStatus(t, e, next, "done")
}

// A named pool never takes another queue's job. The default job is the older
// one, so a bot worker that could take it would have taken it first.
func TestWorkerPoolNamedTakesOnlyItsQueue(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	restore := shortenJobPolling()
	defer restore()

	def := enqueueOn(t, e, "default", "demo.services.hooks.ok", nil)
	bot := enqueueOn(t, e, "bot", "demo.services.hooks.ok", nil)
	w := e.StartWorkerPools(map[string]int{"default": 0, "bot": 1})
	defer w.Stop(0)
	waitForStatus(t, e, bot, "done")
	time.Sleep(100 * time.Millisecond)
	if s := readJob(t, e, def)["status"]; s != "queued" {
		t.Errorf("the default job is %v: a bot pool took it", s)
	}
}

// A queue no pool is named for — the scheduler's, an app's — is the default
// pool's; a process started for some pools only leaves it alone.
func TestWorkerPoolDefaultTakesUnnamedQueues(t *testing.T) {
	e := setupWith(t, jobHookFiles)
	restore := shortenJobPolling()
	defer restore()

	mail := enqueueOn(t, e, "mail", "demo.services.hooks.ok", nil)
	sched := enqueueOn(t, e, "scheduler", "demo.services.hooks.ok", nil)
	bot := enqueueOn(t, e, "bot", "demo.services.hooks.ok", nil)

	only := e.StartWorkerPools(map[string]int{"default": 1, "bot": 1}, "bot")
	waitForStatus(t, e, bot, "done")
	time.Sleep(100 * time.Millisecond)
	only.Stop(0)
	if s := readJob(t, e, mail)["status"]; s != "queued" {
		t.Errorf("a process started for the bot pool only ran a mail job (%v)", s)
	}

	w := e.StartWorkerPools(map[string]int{"default": 1, "bot": 0})
	defer w.Stop(0)
	waitForStatus(t, e, mail, "done")
	waitForStatus(t, e, sched, "done")
}
