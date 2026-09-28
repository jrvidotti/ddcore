package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Replicas that each migrate on boot take turns: while one holds the
// migration lock, another's Migrate waits for it instead of racing it.
func TestMigrationWaitsForAnotherInProgress(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	other, err := e.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	if _, err := other.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", migrateLockKey); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := e.Migrate(ctx, false)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("migrate ran while another migration held the lock (err=%v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := other.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("migrate did not proceed once the lock was released")
	}
}

// Every replica's scheduler fires; only the first to claim an entry's minute
// enqueues it.
func TestSchedulerTickIsClaimedOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if !e.claimTick(ctx, "0 0 * * * demo.services.tasks.daily") {
		t.Fatal("the first scheduler should claim the tick")
	}
	if e.claimTick(ctx, "0 0 * * * demo.services.tasks.daily") {
		t.Fatal("a second scheduler claimed the same minute")
	}
	if !e.claimTick(ctx, "0 * * * * demo.services.tasks.hourly") {
		t.Fatal("another entry's tick is its own")
	}
}

// A scheduler block added while `ddcore dev` runs reaches the running
// scheduler: Load rebuilds it (#23).
func TestLoadRebuildsRunningScheduler(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	boot := e.StartScheduler(ctx)
	defer e.StopScheduler()
	before := len(boot.Entries())
	app := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"],
  scheduler: { all: ["demo.services.tasks.tick"], cron: { "15 3 * * *": ["demo.services.tasks.nightly"] } } });`
	if err := os.WriteFile(filepath.Join(e.Cfg.Apps[0].Dir, "ddcore.app.ts"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	cur := e.sched.Load().cr
	if cur == boot {
		t.Fatal("Load kept the scheduler built from the old apps")
	}
	if n := len(cur.Entries()); n != before+2 {
		t.Fatalf("entries after reload = %d, want %d", n, before+2)
	}
}
