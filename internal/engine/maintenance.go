package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Maintenance mode (PRD-02) pauses a site for a controlled cutover: a backup
// that must be consistent, a restore, a migration that old code must not race.
//
// The flag lives in the database, not in memory, because every replica and
// every process has to see the same answer. A serving process reads it through
// a short cache; while it is on, the HTTP border refuses mutating requests, the
// engine refuses writes, workers stop claiming jobs and the scheduler stops
// enqueueing. The CLI is the bypass: a process built without
// Config.EnforceMaintenance — `ddcore backup`, `restore`, `migrate`, `exec` —
// writes regardless, which is what lets an operator work inside the window.

// MaintenanceState is the flag as last read.
type MaintenanceState struct {
	Enabled bool       `json:"enabled"`
	Reason  string     `json:"reason,omitempty"`
	Since   *time.Time `json:"since,omitempty"`
	Actor   string     `json:"actor,omitempty"`
}

// maintenanceTTL bounds how long a replica keeps writing after another process
// switched maintenance on. Short enough that a cutover waits seconds, long
// enough that a busy site does not read the flag on every request.
const maintenanceTTL = 2 * time.Second

type maintenanceCache struct {
	mu    sync.Mutex
	state MaintenanceState
	read  time.Time
}

// Maintenance returns the current flag, from the cache when it is fresh. A
// database without the table has never been paused, and a read that fails for
// another reason keeps the last known state rather than guessing "open".
func (e *Engine) Maintenance(ctx context.Context) MaintenanceState {
	e.maint.mu.Lock()
	defer e.maint.mu.Unlock()
	if time.Since(e.maint.read) < maintenanceTTL {
		return e.maint.state
	}
	st, err := e.readMaintenance(ctx)
	if err != nil {
		e.Log.Warn("could not read the maintenance flag", "err", db.RedactError(err))
		return e.maint.state
	}
	e.maint.state, e.maint.read = st, time.Now()
	return st
}

func (e *Engine) readMaintenance(ctx context.Context) (MaintenanceState, error) {
	var st MaintenanceState
	if e.DB == nil {
		return st, nil
	}
	err := e.DB.Pool.QueryRow(ctx, `SELECT enabled, reason, since, actor FROM ddcore_maintenance WHERE id = 1`).
		Scan(&st.Enabled, &st.Reason, &st.Since, &st.Actor)
	if errors.Is(err, pgx.ErrNoRows) || db.UndefinedTable(err) {
		return MaintenanceState{}, nil
	}
	return st, err
}

// SetMaintenance switches the flag and records who did it. The audit row is
// best effort: a database restored from a dump that predates Audit Event must
// still be able to leave maintenance.
func (e *Engine) SetMaintenance(ctx context.Context, on bool, reason, actor string) (MaintenanceState, error) {
	if e.DB == nil {
		return MaintenanceState{}, errors.New("maintenance: no database configured")
	}
	if actor == "" {
		actor = "cli"
	}
	if err := db.EnsureOps(ctx, e.DB.Pool); err != nil {
		return MaintenanceState{}, err
	}
	if !on {
		reason = ""
	}
	_, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_maintenance (id, enabled, reason, since, actor)
		VALUES (1, $1, $2, CASE WHEN $1 THEN now() END, $3)
		ON CONFLICT (id) DO UPDATE SET enabled = EXCLUDED.enabled, reason = EXCLUDED.reason,
		since = CASE WHEN EXCLUDED.enabled AND ddcore_maintenance.enabled THEN ddcore_maintenance.since ELSE EXCLUDED.since END,
		actor = EXCLUDED.actor`, on, reason, actor)
	if err != nil {
		return MaintenanceState{}, err
	}
	st, err := e.readMaintenance(ctx)
	if err != nil {
		return st, err
	}
	e.maint.mu.Lock()
	e.maint.state, e.maint.read = st, time.Now()
	e.maint.mu.Unlock()
	action := "ops.maintenance_off"
	if on {
		action = "ops.maintenance_on"
	}
	if err := e.RecordAudit(ctx, actor, action, "Allowed", "", "", map[string]any{"reason": reason}); err != nil {
		e.Log.Warn("could not audit the maintenance change", "action", action, "err", db.RedactError(err))
	}
	return st, nil
}

// WatchMaintenance publishes a `maintenance` event whenever the flag changes,
// whoever changed it. The event hub is per process and the usual writer of the
// flag is another process — the CLI — so a server has to look for itself.
func (e *Engine) WatchMaintenance(ctx context.Context) {
	last := e.Maintenance(ctx)
	t := time.NewTicker(maintenanceTTL)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur := e.Maintenance(ctx)
		if cur.Enabled != last.Enabled || cur.Reason != last.Reason {
			e.Log.Info("maintenance mode changed", "enabled", cur.Enabled, "reason", cur.Reason, "actor", cur.Actor)
			e.Events.Publish(Event{Name: "maintenance", Payload: cur})
		}
		last = cur
	}
}

// Paused reports whether this process must hold its writes and jobs. Only a
// process that enforces maintenance — a server — is ever paused.
func (e *Engine) Paused(ctx context.Context) bool {
	return e.Cfg.EnforceMaintenance && e.Maintenance(ctx).Enabled
}

// MaintenanceError is the refusal every guarded write returns.
func MaintenanceError(st MaintenanceState) *cerr.Error {
	err := cerr.Maintenance("The site is in maintenance mode; changes are paused. Try again shortly.")
	// retryAfter becomes the Retry-After header at the HTTP border; the reason
	// is what the desk's banner shows.
	extra := map[string]any{"retryAfter": 60}
	if st.Reason != "" {
		extra["reason"] = st.Reason
	}
	err.Extra = extra
	return err
}

// bypassMaintenanceFlag marks a unit of work that runs inside the window on
// purpose — a migration — even in a process that enforces it.
const bypassMaintenanceFlag = "bypassMaintenance"

// checkWritable guards the engine's write paths. Error Log is exempt: a
// failure during the window must still leave its record.
func (c *Ctx) checkWritable(doctype string) error {
	if doctype == "Error Log" || !c.E.Cfg.EnforceMaintenance || c.Flags[bypassMaintenanceFlag] == true {
		return nil
	}
	if st := c.E.Maintenance(c.Ctx); st.Enabled {
		return MaintenanceError(st)
	}
	return nil
}
