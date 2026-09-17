package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// A paused site refuses writes in a process that enforces maintenance, and
// only there: the CLI is the bypass, and a migration always runs.
func TestMaintenanceGuardsWrites(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	insert := func() error {
		return e.Run(ctx, "Administrator", func(c *Ctx) error {
			d, _ := c.NewDoc("Pessoa", Doc{"nome": "M " + time.Now().Format("150405.000000")})
			_, err := c.Insert(d, SaveOpts{})
			return err
		})
	}
	if _, err := e.SetMaintenance(ctx, true, "cutover", "tester"); err != nil {
		t.Fatal(err)
	}

	// not enforcing: this is the CLI
	if err := insert(); err != nil {
		t.Fatalf("a process that does not enforce maintenance must still write: %v", err)
	}

	e.Cfg.EnforceMaintenance = true
	t.Cleanup(func() { e.Cfg.EnforceMaintenance = false })
	err := insert()
	var ce *cerr.Error
	if !errors.As(err, &ce) || ce.Type != "MaintenanceError" || ce.Status != 503 {
		t.Fatalf("insert while paused: %v", err)
	}
	if m, _ := ce.Extra.(map[string]any); m["reason"] != "cutover" {
		t.Errorf("the refusal should carry the reason: %#v", ce.Extra)
	}
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Enqueue("demo.services.loop.ok", nil, nil)
		return err
	}); !errors.As(err, &ce) || ce.Type != "MaintenanceError" {
		t.Fatalf("enqueue while paused: %v", err)
	}
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.DBSet("Pessoa", "nobody", Doc{"limite": 1}, false)
		return err
	}); !errors.As(err, &ce) || ce.Type != "MaintenanceError" {
		t.Fatalf("dbSet while paused: %v", err)
	}
	if !e.Paused(ctx) {
		t.Error("Paused should report true")
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatalf("migrate must run inside the window: %v", err)
	}

	if _, err := e.SetMaintenance(ctx, false, "", "tester"); err != nil {
		t.Fatal(err)
	}
	if err := insert(); err != nil {
		t.Fatalf("insert after resuming: %v", err)
	}

	var on, off int
	e.DB.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action = 'ops.maintenance_on'), count(*) FILTER (WHERE action = 'ops.maintenance_off') FROM tab_audit_event`).Scan(&on, &off)
	if on != 1 || off != 1 {
		t.Errorf("audit events: on=%d off=%d", on, off)
	}
}

// Another process flipping the flag reaches this one through the cache window.
func TestMaintenanceSeenAcrossProcesses(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if e.Maintenance(ctx).Enabled {
		t.Fatal("a fresh site is not paused")
	}
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_maintenance (id, enabled, reason, since, actor) VALUES (1, true, 'elsewhere', now(), 'cli')`); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(maintenanceTTL + 2*time.Second)
	for !e.Maintenance(ctx).Enabled {
		if time.Now().After(deadline) {
			t.Fatal("the flag written by another process was never seen")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A paused worker leaves queued jobs alone.
func TestMaintenancePausesWorkers(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := plantJob(t, e, map[string]any{"method": "demo.services.loop.ok", "args": `{"x":1}`})
	e.Cfg.EnforceMaintenance = true
	if _, err := e.SetMaintenance(ctx, true, "", "tester"); err != nil {
		t.Fatal(err)
	}
	go e.Worker(ctx, 0)
	time.Sleep(3 * time.Second)
	if st := readJob(t, e, id)["status"]; st != "queued" {
		t.Fatalf("a paused worker ran the job: %v", st)
	}
	if _, err := e.SetMaintenance(ctx, false, "", "tester"); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, e, id, "done")
}

// A job caught mid-run by the pause is given back, not failed: the window is a
// decision, so it must not spend the job's last attempt or file a fault.
func TestMaintenanceRequeuesRunningJob(t *testing.T) {
	e := setupWith(t, map[string]string{
		"services/writer.ts": `export function write() { ddcore.newDoc("Pessoa", { nome: "Escrita", tipo: "PF" }).insert(); return { ok: true }; }`,
	})
	ctx := context.Background()
	id := plantJob(t, e, map[string]any{"method": "demo.services.writer.write", "attempts": 2, "max_attempts": 3})
	e.Cfg.EnforceMaintenance = true
	if _, err := e.SetMaintenance(ctx, true, "cutover", "tester"); err != nil {
		t.Fatal(err)
	}
	if ran, err := e.runOneJob(ctx); err != nil || !ran {
		t.Fatalf("runOneJob: ran=%v err=%v", ran, err)
	}

	j := readJob(t, e, id)
	if j["status"] != "queued" {
		t.Errorf("status = %v, want queued", j["status"])
	}
	if got := int(toFloat(j["attempts"])); got != 2 {
		t.Errorf("attempts = %d, want the attempt given back (2)", got)
	}
	var logged int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tab_error_log`).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 0 {
		t.Errorf("a paused write filed %d Error Log rows", logged)
	}
}

// The ledger records the versions that migrated, once per change, and an
// older binary is refused unless it is told the rollback is deliberate.
func TestSiteVersionLedger(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	sv, err := LastSiteVersion(ctx, e.DB.Pool)
	if err != nil || sv == nil {
		t.Fatalf("migrate should have recorded a version: %v %v", sv, err)
	}
	if sv.Core != Version {
		t.Errorf("core = %q, want %q", sv.Core, Version)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	var n int
	e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_site_version`).Scan(&n)
	if n != 1 {
		t.Errorf("an unchanged migrate added a ledger row: %d rows", n)
	}
	if err := e.CheckSiteVersion(ctx); err != nil {
		t.Fatalf("same version refused: %v", err)
	}

	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_site_version (core, apps) VALUES ('v99.0.0', '{"demo":"9.0.0"}')`); err != nil {
		t.Fatal(err)
	}
	err = e.CheckSiteVersion(ctx)
	if err == nil || !strings.Contains(err.Error(), "99.0.0") || !strings.Contains(err.Error(), "--allow-older-binary") {
		t.Fatalf("an older binary must be refused, naming the version: %v", err)
	}
	e.Cfg.AllowOlderBinary = true
	defer func() { e.Cfg.AllowOlderBinary = false }()
	if err := e.CheckSiteVersion(ctx); err != nil {
		t.Fatalf("--allow-older-binary: %v", err)
	}
}

// A core that cannot be compared must not erase the release the ledger already
// holds: recording `dev` as the newest row would leave every later binary,
// however old, passing the core check unnoticed.
func TestSiteVersionLedgerKeepsLastRelease(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_site_version (core, apps) VALUES ('v99.0.0', '{"demo":"9.0.0"}')`); err != nil {
		t.Fatal(err)
	}
	was := Version
	Version = "dev"
	defer func() { Version = was }()

	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	sv, err := LastSiteVersion(ctx, e.DB.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Core != "v99.0.0" {
		t.Errorf("core = %q, want the last release v99.0.0 carried forward", sv.Core)
	}

	Version = was
	if err := e.CheckSiteVersion(ctx); err == nil {
		t.Error("a migrate by a dev build must not disarm the rollback guard")
	}
}

func TestOlderThanSite(t *testing.T) {
	sv := &SiteVersion{Core: "v0.15.2", Apps: map[string]string{"a": "1.2.0", "b": "2.0.0", "c": "x"}}
	got := OlderThanSite(sv, "v0.15.0", map[string]string{"a": "1.3.0", "b": "1.9.9"})
	if len(got) != 2 || !strings.HasPrefix(got[0], "core 0.15.0") || !strings.HasPrefix(got[1], "app b 1.9.9") {
		t.Errorf("OlderThanSite = %v", got)
	}
	if got := OlderThanSite(sv, "dev", nil); len(got) != 0 {
		t.Errorf("a dev build cannot be compared: %v", got)
	}
	if got := OlderThanSite(nil, "v0.1.0", nil); got != nil {
		t.Errorf("no ledger: %v", got)
	}
}

func TestMaintenanceWithoutTable(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `DROP TABLE ddcore_maintenance`); err != nil {
		t.Fatal(err)
	}
	e.maint.read = time.Time{}
	if e.Maintenance(ctx).Enabled {
		t.Error("a database without the table is not paused")
	}
	if _, err := e.SetMaintenance(ctx, true, "", "t"); err != nil {
		t.Fatalf("SetMaintenance must create the table: %v", err)
	}
	if err := db.EnsureOps(ctx, e.DB.Pool); err != nil {
		t.Fatal(err)
	}
}

// A stale cache is refreshed by one caller at a time, and nobody queues behind
// that read while a state is known: a caller inside a write transaction already
// holds a pool connection, and waiting on a read that needs another one can
// stall every request once the pool is full.
func TestMaintenanceRefreshDoesNotQueue(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.SetMaintenance(ctx, true, "Upgrade", "t"); err != nil {
		t.Fatal(err)
	}
	inflight := make(chan struct{})
	e.maint.mu.Lock()
	e.maint.read, e.maint.refreshing = time.Time{}, inflight
	e.maint.mu.Unlock()

	done := make(chan MaintenanceState, 1)
	go func() { done <- e.Maintenance(ctx) }()
	select {
	case st := <-done:
		if !st.Enabled || st.Reason != "Upgrade" {
			t.Errorf("while another read is out, the last known state: %+v", st)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Maintenance waited on a refresh already in flight")
	}

	// with nothing known yet a caller waits, but no longer than its context
	e.maint.mu.Lock()
	e.maint.known = false
	e.maint.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	go func() { done <- e.Maintenance(cctx) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Maintenance outlived its context waiting on a refresh")
	}

	e.maint.mu.Lock()
	e.maint.refreshing = nil
	e.maint.mu.Unlock()
	close(inflight)
	if !e.Maintenance(ctx).Enabled {
		t.Error("a fresh read after the refresh cleared")
	}
	if _, err := e.SetMaintenance(ctx, false, "", "t"); err != nil {
		t.Fatal(err)
	}
}
