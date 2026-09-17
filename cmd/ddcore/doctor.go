package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/release"
)

const doctorUsage = `ddcore doctor — report what this site's database, metadata and queue look like

Usage: ddcore doctor [--json] [--strict] [--window N] [--no-update-check]

  --json       print the report as JSON instead of text
  --strict     exit non-zero for warnings too, not only for critical findings
  --window N   minutes of history for the failure counts (default: ops.windowMinutes)
  --no-update-check
               skip the lookup of the newest published release
               (DDCORE_UPDATE_CHECK=off does the same for every run)

Exit code 1 means a critical finding: the database is unreachable, the apps
could not be loaded, a migration is refused, or an undeclared structure still
holds data. Warnings — pending DDL, a queue backlog, recent failures — exit 0
unless --strict, because this command is mostly run by a person who wants to
read it, not by a script that wants to fail.
`

// appInfo is an app's own version and the ddcore range it declares: the two
// numbers an upgrade is checked against.
type appInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Ddcore  string `json:"ddcore,omitempty"`
}

// doctorReport is the whole report as data, so that the text and the JSON are
// two renderings of one thing rather than two things that drift.
type doctorReport struct {
	DDCore    string              `json:"ddcore"`
	Database  db.Health           `json:"database"`
	DSN       string              `json:"dsn"`
	Engine    string              `json:"engineError,omitempty"`
	Apps      []string            `json:"apps,omitempty"`
	AppInfo   []appInfo           `json:"appInfo,omitempty"`
	DocTypes  int                 `json:"doctypes"`
	Migrate   *migrateSection     `json:"migrate,omitempty"`
	Orphans   *orphanSection      `json:"orphans,omitempty"`
	Patches   []patchRef          `json:"patches,omitempty"`
	Renames   []renameRef         `json:"renames,omitempty"`
	Queue     *engine.QueueHealth `json:"queue,omitempty"`
	Errors    *engine.ErrorHealth `json:"errors,omitempty"`
	Scheduler engine.SchedHealth  `json:"scheduler"`
	Workers   int                 `json:"workers"`
	Mail      string              `json:"mail"`
	Storage   string              `json:"storage"`
	URL       string              `json:"url"`
	URLSet    bool                `json:"urlConfigured"`
	Sessions  sessionSection      `json:"sessions"`
	SSO       ssoSection          `json:"sso"`
	Ops       config.OpsPolicy    `json:"ops"`
	// Secrets are names. A doctor report is pasted into issues and chat
	// windows, and a secret that reaches one of those has to be rotated.
	Secrets  []string              `json:"secrets"`
	Vault    *vaultSection         `json:"vault,omitempty"`
	Webhooks *engine.WebhookStatus `json:"webhooks,omitempty"`
	// Maintenance, MigratedBy and Backup are the recovery picture (PRD-01/02).
	Maintenance *engine.MaintenanceState `json:"maintenance,omitempty"`
	MigratedBy  *engine.SiteVersion      `json:"migratedBy,omitempty"`
	Backup      *engine.BackupStatus     `json:"backup,omitempty"`
	// Update is absent when the check was switched off, when this binary is
	// not a release, or when the lookup failed — an offline site is a normal
	// site, not a finding.
	Update   *release.Update `json:"update,omitempty"`
	Critical []string        `json:"critical,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
}

// ssoSection names the providers and whether each answered discovery — never
// a client id or secret.
type ssoSection struct {
	PasswordLogin bool          `json:"passwordLogin"`
	Providers     []ssoProvider `json:"providers"`
}

type ssoProvider struct {
	ID       string `json:"id"`
	Issuer   string `json:"issuer"`
	Callback string `json:"callback"`
	Error    string `json:"error,omitempty"`
}

type vaultSection struct {
	Configured bool     `json:"configured"`
	Count      int      `json:"count"`
	Secrets    []string `json:"secrets,omitempty"`
}

type migrateSection struct {
	Pending int    `json:"pending"`
	Refused string `json:"refused,omitempty"`
}

type orphanSection struct {
	Drops []string `json:"drops,omitempty"`
	// Refused is set when the plan could not be made because undeclared
	// structures still hold data.
	Refused string `json:"refused,omitempty"`
}

type patchRef struct {
	Phase string `json:"phase"`
	Path  string `json:"path"`
}

type renameRef struct {
	Where     string `json:"where"`
	OldName   string `json:"oldName"`
	Retirable bool   `json:"retirable"`
}

type sessionSection struct {
	Days             int `json:"days"`
	MaxLoginAttempts int `json:"maxLoginAttempts"`
	LockoutMinutes   int `json:"lockoutMinutes"`
}

func cmdDoctor(args []string) error {
	fs := newFlagSet("doctor")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	strict := fs.Bool("strict", false, "exit non-zero for warnings too")
	window := fs.Int("window", 0, "minutes of history for the failure counts")
	noUpdate := fs.Bool("no-update-check", false, "skip the lookup of the newest published release")
	fs.Usage = func() { fmt.Print(doctorUsage) }
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	// Config first, and the database probe before the engine. The failure this
	// command exists to report is the one where no engine can be built at all,
	// and a doctor that dies then is a doctor silent in the single situation
	// that needed it.
	cfg, _, err := config.Load(".")
	if err != nil {
		return err
	}
	ctx := context.Background()
	rep := gatherDoctor(ctx, cfg, *window, !*noUpdate && !cfg.UpdateCheck.Off)
	if *asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	} else {
		rep.print(os.Stdout)
	}
	// os.Exit rather than returning an error: main prints "error: …", which
	// would land on top of the report the reader is here for.
	if code := rep.exitCode(*strict); code != 0 {
		os.Exit(code)
	}
	return nil
}

// gatherDoctor builds the report. Every error that reaches it goes through
// db.RedactError first: this output is pasted into issues and chat windows, and
// a failure from the database layer quotes the connection string back at you.
func gatherDoctor(ctx context.Context, cfg *config.File, windowMin int, updateCheck bool) *doctorReport {
	rep := &doctorReport{
		DDCore: engine.Version, DSN: db.RedactDSN(cfg.DSN),
		Workers: cfg.Workers, Mail: mailSummary(cfg), Storage: storageSummary(cfg),
		URL: cfg.PublicURL(), URLSet: cfg.HasPublicURL(), Ops: cfg.Ops,
		Sessions: sessionSection{cfg.Auth.SessionDays, cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutMinutes},
		Secrets:  []string{},
		SSO:      ssoSection{PasswordLogin: cfg.Auth.AllowPasswordLogin(), Providers: []ssoProvider{}},
	}
	rep.Database = db.Probe(ctx, cfg.DSN, cfg.Ops.ReadyTimeout())
	if !rep.Database.OK {
		rep.Critical = append(rep.Critical, "database unreachable")
	}
	// Before the engine is loaded, because an engine that will not load is
	// often an engine too old for the apps in front of it, and that is exactly
	// when knowing a newer release exists helps. A lookup that fails says
	// nothing: a site with no outbound internet access is a normal site.
	if updateCheck {
		uctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if u, err := release.Check(uctx, engine.Version); err == nil && u != nil {
			rep.Update = u
			if u.Available {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("a newer ddcore release is available: %s (this binary is %s)", u.Latest, u.Current))
			}
		}
		cancel()
	}
	window := cfg.Ops.Window()
	if windowMin > 0 {
		window = time.Duration(windowMin) * time.Minute
	}

	e, _, err := load(false, false)
	if err != nil {
		rep.Engine = db.RedactError(err)
		rep.Critical = append(rep.Critical, engineCritical(err))
		return rep
	}
	defer e.DB.Close()

	rep.Apps = e.AppOrder()
	for _, n := range rep.Apps {
		if am := e.Snap.Apps[n]; am != nil {
			rep.AppInfo = append(rep.AppInfo, appInfo{Name: n, Version: am.Version, Ddcore: am.Ddcore})
		}
	}
	rep.DocTypes = len(e.Meta.DocTypes)
	rep.Scheduler = e.SchedulerHealth()
	if names := e.SecretNames(); len(names) > 0 {
		sort.Strings(names)
		rep.Secrets = names
	}
	if configured, count, names, err := e.VaultStatus(ctx); err == nil {
		sort.Strings(names)
		rep.Vault = &vaultSection{
			Configured: configured,
			Count:      count,
			Secrets:    names,
		}
		if !configured && count > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("vault: DDCORE_SECRET_KEY is not set but %d encrypted secret(s) exist", count))
		}
	}

	rep.Migrate = &migrateSection{}
	if plan, err := e.Plan(ctx, false); err != nil {
		rep.Migrate.Refused = db.RedactError(err)
		rep.Critical = append(rep.Critical, "migrate is refused")
	} else if rep.Migrate.Pending = len(plan); rep.Migrate.Pending > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d pending DDL statement(s)", rep.Migrate.Pending))
	}

	// The same plan with prune on names what the meta no longer declares. It is
	// reported separately because it is the data at risk, not work to do.
	rep.Orphans = &orphanSection{}
	if plan, err := e.Plan(ctx, true); err == nil {
		if _, drop := db.Destructive(plan); len(drop) > 0 {
			for _, st := range drop {
				rep.Orphans.Drops = append(rep.Orphans.Drops, strings.TrimSuffix(st.SQL, ";"))
			}
		}
	} else {
		rep.Orphans.Refused = db.RedactError(err)
		rep.Critical = append(rep.Critical, "undeclared structures still hold data")
	}

	for _, p := range e.PendingPatches() {
		rep.Patches = append(rep.Patches, patchRef{Phase: p.Phase, Path: p.Path})
	}
	if len(rep.Patches) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d pending patch(es)", len(rep.Patches)))
	}

	if renames, err := e.AppliedRenames(ctx); err == nil {
		for _, r := range renames {
			where := r.Doctype
			if r.Kind == "field" {
				where += "." + r.NewName
			}
			rep.Renames = append(rep.Renames, renameRef{Where: where, OldName: r.OldName, Retirable: r.Retirable})
		}
	} else {
		rep.Warnings = append(rep.Warnings, "renames could not be read: "+db.RedactError(err))
	}

	for _, p := range cfg.OIDC {
		sp := ssoProvider{ID: p.ID, Issuer: p.Issuer, Callback: e.OIDCCallbackURL(p.ID)}
		// A provider that does not answer is a warning, not a failure: the
		// site still serves everyone who signs in some other way.
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := e.OIDCDiscover(dctx, p.ID); err != nil {
			sp.Error = err.Error()
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("sso: provider %s did not answer discovery: %s", p.ID, err))
		}
		cancel()
		rep.SSO.Providers = append(rep.SSO.Providers, sp)
	}

	if ws, err := e.WebhookStatus(ctx); err == nil {
		rep.Webhooks = &ws
		if ws.Off && ws.Enabled > 0 {
			// Deliberate during a rehearsal, and exactly the setting nobody
			// remembers to undo afterwards.
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("webhooks: DDCORE_WEBHOOKS=off, %d enabled webhook(s) receive nothing", ws.Enabled))
		}
		if ws.FailedLast > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("webhooks: %d delivery(ies) failed in the last 24h (ddcore webhooks list --status Failed)", ws.FailedLast))
		}
	}

	if st := e.Maintenance(ctx); st.Enabled {
		rep.Maintenance = &st
		rep.Warnings = append(rep.Warnings, "maintenance mode is on: writes and jobs are paused (ddcore maintenance off)")
	}
	if sv, err := engine.LastSiteVersion(ctx, e.DB.Pool); err == nil {
		rep.MigratedBy = sv
	}
	if b, err := e.LastBackup(ctx); err == nil {
		rep.Backup = b
		if b != nil && b.Failures > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d backup(s) failed since the last successful one", b.Failures))
		}
	}

	h := e.Health(ctx, engine.HealthOpts{Queue: true, Errors: true})
	rep.Queue, rep.Errors = h.Queue, h.Errors
	rep.Warnings = append(rep.Warnings, h.Warnings...)
	if windowMin > 0 {
		// The health thresholds used the configured window; a caller that asked
		// for another one gets the counts recomputed over it.
		if q, err := e.QueueHealth(ctx, window); err == nil {
			rep.Queue = q
		}
		if er, err := e.ErrorHealth(ctx, window, 5); err == nil {
			rep.Errors = er
		}
	}
	return rep
}

// exitCode is 1 for a critical finding. A warning is something to read, not
// something to fail a shell over — turning today's "3 pending statements" into
// a non-zero exit would be a regression for everyone running this by hand.
func (r *doctorReport) exitCode(strict bool) int {
	if len(r.Critical) > 0 {
		return 1
	}
	if strict && len(r.Warnings) > 0 {
		return 1
	}
	return 0
}

func (r *doctorReport) print(w io.Writer) {
	p := func(label, format string, a ...any) {
		fmt.Fprintf(w, "%-12s%s\n", label+":", fmt.Sprintf(format, a...))
	}
	cont := func(format string, a ...any) {
		fmt.Fprintf(w, "              %s\n", fmt.Sprintf(format, a...))
	}

	p("version", "%s", r.DDCore)
	if r.Update != nil && r.Update.Available {
		cont("%s is available", r.Update.Latest)
	}
	if r.Database.OK {
		p("database", "ok (%.1f ms) %s", r.Database.LatencyMS, r.DSN)
	} else {
		p("database", "unreachable — %s", orDash(r.Database.Error))
		cont("%s", r.DSN)
	}
	if r.Engine != "" {
		p("apps", "could not be loaded")
		cont("%s", r.Engine)
		r.printTail(w, p)
		return
	}
	p("apps", "%v", r.Apps)
	for _, a := range r.AppInfo {
		if a.Version != "" || a.Ddcore != "" {
			cont("%s %s, ddcore %s", a.Name, orDash(a.Version), orDash(a.Ddcore))
		}
	}
	p("doctypes", "%d", r.DocTypes)
	if r.Migrate != nil {
		if r.Migrate.Refused != "" {
			p("migrate", "refused")
			cont("%s", r.Migrate.Refused)
		} else {
			p("migrate", "%d pending statement(s)", r.Migrate.Pending)
		}
	}
	if r.Orphans != nil {
		if r.Orphans.Refused != "" {
			p("orphans", "undeclared structures still hold data")
			cont("%s", r.Orphans.Refused)
		} else if len(r.Orphans.Drops) > 0 {
			p("orphans", "%d undeclared and empty", len(r.Orphans.Drops))
			for _, sql := range r.Orphans.Drops {
				cont("%s", sql)
			}
		}
	}
	if len(r.Patches) > 0 {
		p("patches", "%d pending", len(r.Patches))
		for _, x := range r.Patches {
			cont("%-12s %s", x.Phase, x.Path)
		}
	}
	if len(r.Renames) > 0 {
		retirable := 0
		for _, x := range r.Renames {
			if x.Retirable {
				retirable++
			}
		}
		p("renames", "%d applied, %d retirable in this database", len(r.Renames), retirable)
		for _, x := range r.Renames {
			note := ""
			if x.Retirable {
				note = "  → renamedFrom retirable here; delete it only when every site says the same"
			}
			cont("%s renamedFrom %q%s", x.Where, x.OldName, note)
		}
	}
	if q := r.Queue; q != nil {
		p("queue", "%d queued (%d runnable, oldest %s), %d running, %d stalled, %d failed in %dm",
			q.Queued, q.Runnable, short(q.OldestQueuedSeconds), q.Running, q.Stalled, q.FailedInWindow, q.WindowMinutes)
	}
	if er := r.Errors; er != nil {
		p("errors", "%d Error Log entr%s in %dm", er.InWindow, plural(er.InWindow), er.WindowMinutes)
		for _, x := range er.Latest {
			cont("%s  %s  id %s", x.Creation, x.Method, orDash(x.RequestID))
		}
	}
	p("scheduler", "%v, %d entr%s", r.Scheduler.Enabled, r.Scheduler.Entries, plural(int64(r.Scheduler.Entries)))
	if m := r.Maintenance; m != nil {
		p("paused", "maintenance ON since %s by %s — %s", m.Since.Format(time.RFC3339), orDash(m.Actor), orDash(m.Reason))
	} else {
		p("paused", "no (maintenance off)")
	}
	if sv := r.MigratedBy; sv != nil {
		apps := make([]string, 0, len(sv.Apps))
		for n, v := range sv.Apps {
			apps = append(apps, n+" "+v)
		}
		sort.Strings(apps)
		p("migrated", "by ddcore %s at %s; %s", sv.Core, sv.Migrated.Format(time.RFC3339), orDash(strings.Join(apps, ", ")))
	}
	if b := r.Backup; b != nil {
		p("backup", "%s ago, %s (%s)", time.Since(b.Finished).Round(time.Minute), b.Location, humanBytes(b.Bytes))
	} else {
		p("backup", "none recorded by ddcore backup")
	}
	r.printTail(w, p)
}

func (r *doctorReport) printTail(w io.Writer, p func(string, string, ...any)) {
	p("workers", "%d", r.Workers)
	p("mail", "%s", r.Mail)
	p("storage", "%s", r.Storage)
	url := r.URL
	if !r.URLSet {
		url += "  → not configured; set DDCORE_URL in .env before mailing a recovery link"
	}
	p("url", "%s", url)
	p("sessions", "%d day(s), lockout after %d failed attempts for %d minute(s)",
		r.Sessions.Days, r.Sessions.MaxLoginAttempts, r.Sessions.LockoutMinutes)
	if len(r.SSO.Providers) == 0 {
		p("sso", "no providers")
	} else {
		ids := make([]string, 0, len(r.SSO.Providers))
		for _, x := range r.SSO.Providers {
			ids = append(ids, x.ID)
		}
		pw := "password sign-in on"
		if !r.SSO.PasswordLogin {
			pw = "password sign-in off (Administrator only)"
		}
		p("sso", "%s; %s", strings.Join(ids, ", "), pw)
	}
	p("ops", "backlog %d, age %ds, %d failure(s)/%dm, %d error(s)/%dm",
		r.Ops.QueueBacklog, r.Ops.QueueAgeSeconds, r.Ops.JobFailures, r.Ops.WindowMinutes,
		r.Ops.ErrorLogEntries, r.Ops.WindowMinutes)
	if len(r.Secrets) > 0 {
		p("secrets", "%d configured: %s", len(r.Secrets), strings.Join(r.Secrets, ", "))
	} else {
		p("secrets", "none configured")
	}
	if r.Vault != nil {
		if r.Vault.Configured {
			if r.Vault.Count > 0 {
				p("vault", "%d secret(s) encrypted: %s", r.Vault.Count, strings.Join(r.Vault.Secrets, ", "))
			} else {
				p("vault", "key configured (empty)")
			}
		} else {
			p("vault", "key not configured (DDCORE_SECRET_KEY is missing)")
		}
	}
	if r.Webhooks != nil {
		state := "on"
		if r.Webhooks.Off {
			state = "off (DDCORE_WEBHOOKS)"
		}
		p("webhooks", "%s, %d enabled, %d retrying, %d failed/24h", state, r.Webhooks.Enabled, r.Webhooks.Retrying, r.Webhooks.FailedLast)
	}
	for _, x := range r.Warnings {
		fmt.Fprintf(w, "warning:    %s\n", x)
	}
	for _, x := range r.Critical {
		fmt.Fprintf(w, "critical:   %s\n", x)
	}
}

func plural(n int64) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func short(seconds float64) string {
	if seconds <= 0 {
		return "-"
	}
	return time.Duration(seconds * float64(time.Second)).Round(time.Second).String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func mailSummary(cfg *config.File) string {
	switch cfg.Mail.Transport {
	case config.MailSMTP:
		auth := "no auth"
		if cfg.Mail.Username != "" {
			auth = "as " + cfg.Mail.Username
		}
		summary := fmt.Sprintf("smtp %s:%d (%s, %s)", cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.TLS, auth)
		if cfg.Mail.Debug != "" {
			summary += fmt.Sprintf(" [debug redirect: %s]", cfg.Mail.Debug)
		}
		return summary
	case config.MailMethod:
		return "method " + cfg.Mail.Method
	default:
		return "log — links are written to the log, not delivered"
	}
}

// storageSummary names where file bytes live, never the credentials.
func storageSummary(cfg *config.File) string {
	if cfg.Storage.Backend != config.StorageS3 {
		return "local " + filepath.Join(cfg.DataDir, "files")
	}
	s3 := cfg.Storage.S3
	where := s3.Endpoint + "/" + s3.Bucket
	if s3.Prefix != "" {
		where += "/" + s3.Prefix
	}
	return fmt.Sprintf("s3 %s (presigned links valid %s)", where, s3.PresignTTL)
}

// engineCritical names the refusal in one line, for the critical list. A
// rollback the ledger refused is not a broken app, and saying so sends the
// reader to the binary's version rather than to app code.
func engineCritical(err error) string {
	if errors.Is(err, engine.ErrOlderBinary) {
		return "this binary is older than the release that migrated the database"
	}
	return "the apps could not be loaded"
}
