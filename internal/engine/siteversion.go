package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jrvidotti/ddcore/internal/db"
)

// The site version ledger (PRD-01/PRD-02). Every migration writes down which
// core and which app versions brought the database to its shape. That record
// is what makes a rollback a decision rather than an accident: a binary older
// than the one that last migrated may not understand the schema — a column it
// never heard of is harmless, a contraction it predates is not — so it refuses
// to open the database unless the operator says the change was expand-only.

// SiteVersion is one migration's entry in the ledger.
type SiteVersion struct {
	ID       int64             `json:"id"`
	Migrated time.Time         `json:"migrated"`
	Core     string            `json:"core"`
	Apps     map[string]string `json:"apps"`
}

// LastSiteVersion reads the newest ledger row; nil when no migration by a
// binary with the ledger has run yet.
func LastSiteVersion(ctx context.Context, q db.Querier) (*SiteVersion, error) {
	var sv SiteVersion
	var apps []byte
	err := q.QueryRow(ctx, `SELECT id, migrated, core, apps FROM ddcore_site_version ORDER BY id DESC LIMIT 1`).
		Scan(&sv.ID, &sv.Migrated, &sv.Core, &apps)
	if errors.Is(err, pgx.ErrNoRows) || db.UndefinedTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(apps) > 0 {
		if err := json.Unmarshal(apps, &sv.Apps); err != nil {
			return nil, err
		}
	}
	return &sv, nil
}

// appVersions is what this binary has loaded, app → declared version.
func (s *State) appVersions() map[string]string {
	out := map[string]string{}
	for name, am := range s.Snap.Apps {
		if am != nil && am.Version != "" {
			out[name] = am.Version
		}
	}
	return out
}

// recordSiteVersion appends this binary to the ledger, inside the migration's
// transaction: a migration that rolls back did not happen.
func (c *Ctx) recordSiteVersion() error {
	if err := db.EnsureOps(c.Ctx, c.Tx); err != nil {
		return err
	}
	versions := c.St.appVersions()
	last, err := LastSiteVersion(c.Ctx, c.Tx)
	if err != nil {
		return err
	}
	// one row per change of versions, not per run: `migrate` on every deploy
	// would otherwise bury the one row a rollback needs under identical ones
	if last != nil && last.Core == Version && maps.Equal(last.Apps, versions) {
		return nil
	}
	apps, err := json.Marshal(versions)
	if err != nil {
		return err
	}
	// A core that does not parse — a `dev` build, a bare hash — cannot be
	// compared, so recording it as the newest row would leave every later
	// binary passing the check against a version nobody can read. Carry the
	// last release forward instead: the apps still move, the guard still holds.
	core := Version
	if _, ok := parseCoreVersion(core); !ok && last != nil {
		core = last.Core
	}
	_, err = c.Tx.Exec(c.Ctx, `INSERT INTO ddcore_site_version (core, apps) VALUES ($1, $2)`, core, string(apps))
	return err
}

// OlderThanSite lists what in this binary is older than what last migrated
// the database. Versions that do not parse — a `dev` build, an app with no
// version — cannot be compared and are not reported.
func OlderThanSite(sv *SiteVersion, core string, apps map[string]string) []string {
	if sv == nil {
		return nil
	}
	var out []string
	if have, ok := parseCoreVersion(core); ok {
		if was, ok := parseCoreVersion(sv.Core); ok && have.cmp(was) < 0 {
			out = append(out, fmt.Sprintf("core %s (database migrated by %s)", have, was))
		}
	}
	names := make([]string, 0, len(sv.Apps))
	for n := range sv.Apps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		was, _, ok := parseVersion(sv.Apps[n])
		if !ok {
			continue
		}
		have, _, ok := parseVersion(apps[n])
		if !ok {
			continue
		}
		if have.cmp(was) < 0 {
			out = append(out, fmt.Sprintf("app %s %s (database migrated by %s)", n, have, was))
		}
	}
	return out
}

// ErrOlderBinary is the rollback refusal, so a caller can tell it apart from
// an app that would not load: the two send an operator to different places.
var ErrOlderBinary = errors.New("this binary is older than the database")

// CheckSiteVersion refuses a binary older than the one that last migrated the
// database, unless Config.AllowOlderBinary. A database that cannot be read is
// not this check's business: readiness and the first query report that.
func (e *Engine) CheckSiteVersion(ctx context.Context) error {
	sv, err := LastSiteVersion(ctx, e.DB.Pool)
	if err != nil {
		e.Log.Warn("could not read the site version ledger", "err", db.RedactError(err))
		return nil
	}
	older := OlderThanSite(sv, Version, e.Current().appVersions())
	if len(older) == 0 {
		return nil
	}
	if e.Cfg.AllowOlderBinary {
		e.Log.Warn("running a binary older than the database's last migration", "older", strings.Join(older, "; "))
		return nil
	}
	return fmt.Errorf("%w: %s. Roll forward, or pass --allow-older-binary (DDCORE_ALLOW_OLDER_BINARY=1) if every change since was expand-only; see docs/agent/backup.md",
		ErrOlderBinary, strings.Join(older, "; "))
}
