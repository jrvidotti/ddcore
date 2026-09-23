// Package engine ties meta, db and the JS runtime together: it owns the
// Document lifecycle, permissions, queries and the bridge the TS code calls.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/mail"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
	"github.com/jrvidotti/ddcore/internal/storage"
)

type Config struct {
	DSN       string
	Apps      []js.App // in load order; core is prepended automatically
	Workers   int      // job workers
	Scheduler bool
	Dev       bool
	Test      bool // include *.test.ts and mark runtime as test
	Port      int
	Lang      string
	Currency  string
	// CurrencyPrecision is how many decimal places a Currency field is rounded
	// to. Zero means "not set": New resolves it from Currency.
	CurrencyPrecision *int
	Rounding          num.Rounding
	Timezone          string
	SecretKey         string
	DataDir           string // uploads
	// Root is the directory holding ddcore.json: the checkout the apps and
	// their translation catalogues live in. Empty when the engine was built
	// by hand (tests, embedders), in which case nothing that rewrites a
	// checkout is available.
	Root          string
	ExportMaxRows int // cap for GET /api/export; 0 = DefaultExportMaxRows
	ImportMaxRows int // cap for POST /api/data-import; 0 = DefaultImportMaxRows
	LogLevel      slog.Level
	// Auth is the site's access policy. The zero value is not a policy —
	// New fills it from config.DefaultAuth so a Config built by hand (tests,
	// embedders) still locks out and still expires a session.
	Auth config.AuthPolicy
	// Ops is the site's operational policy, filled from config.DefaultOps for
	// the same reason: a zero backlog threshold would warn about every job.
	Ops config.OpsPolicy
	// LogJSON writes the log as JSON objects instead of text. A collector that
	// has to regex a text line cannot group by request id, which is most of
	// what the id is for.
	LogJSON bool
	// LogOut is where the log is written. Nil means os.Stdout, which is what a
	// platform reads as ordinary output; stderr, slog's own default, is what it
	// reads as an error. Only a process whose stdout already carries something
	// else — `ddcore mcp` speaks JSON-RPC over it — sets this, and it sets it
	// to os.Stderr.
	LogOut io.Writer
	// Mail says where a recovery or invitation link goes.
	Mail config.Mail
	// Webhooks switches outgoing webhooks off for a deployment that must have
	// no external effects. The zero value sends.
	Webhooks config.Webhooks
	// Storage says where uploaded bytes live; the zero value is DataDir/files.
	Storage config.Storage
	// SiteURL is the public base those links are built from, already
	// defaulted to localhost by config.PublicURL.
	SiteURL string
	// TrustProxy makes the API believe X-Forwarded-For.
	TrustProxy bool
	// Login is the sign-in screen's notice and demo account, served to
	// visitors by /api/boot.
	Login config.LoginPage
	// OIDC lists the single sign-on providers. Their callback addresses are
	// built from SiteURL.
	OIDC []config.OIDCProvider
	// EnforceMaintenance makes this process honour maintenance mode: refuse
	// writes, stop claiming jobs, skip scheduled runs. A server sets it; the
	// CLI does not, and that is the bypass an operator works through.
	EnforceMaintenance bool
	// AllowOlderBinary lets this binary open a database a newer core or app
	// version migrated — the deliberate rollback. See CheckSiteVersion.
	AllowOlderBinary bool
}

// AppMeta is what defineApp produced, minus functions.
type AppMeta struct {
	Name            string                      `json:"name"`
	Title           string                      `json:"title"`
	Description     string                      `json:"description"`
	Requires        []string                    `json:"requires"`
	Version         string                      `json:"version"`
	Ddcore          string                      `json:"ddcore"`
	Scheduler       map[string]any              `json:"scheduler"`
	Desk            map[string]any              `json:"desk"`
	Roles           []string                    `json:"roles"`
	Fixtures        map[string][]map[string]any `json:"fixtures"`
	HasAfterInstall bool                        `json:"hasAfterInstall"`
	HasAfterMigrate bool                        `json:"hasAfterMigrate"`
	Dir             string                      `json:"-"`
}

type Whitelisted struct {
	Path string         `json:"path"`
	Opts map[string]any `json:"opts"`
}

type Patch struct {
	App  string `json:"app"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Phase is "beforeSchema" or "afterSchema" (the default). It is what makes
	// expand → backfill → contract possible: before the DDL a patch can make
	// the data fit what the schema change is about to do.
	Phase       string `json:"phase,omitempty"`
	Description string `json:"description,omitempty"`
}

// BeforeSchema reports whether this patch runs ahead of the DDL.
func (p Patch) BeforeSchema() bool { return p.Phase == "beforeSchema" }

// Snapshot is the JS registry as seen from Go.
type Snapshot struct {
	Doctypes   map[string]json.RawMessage `json:"doctypes"`
	Reports    map[string]map[string]any  `json:"reports"`
	Workspaces map[string]map[string]any  `json:"workspaces"`
	// MailTemplates arrives without its `subject` and `body` functions: Go
	// never renders a template, it only needs to know one exists and whether
	// its arguments may be stored.
	MailTemplates  map[string]MailTemplate    `json:"mailTemplates"`
	PrintTemplates map[string]PrintTemplate   `json:"printTemplates"`
	Notifications  map[string]js.Notification `json:"notifications"`
	Workflows      map[string]js.Workflow     `json:"workflows"`
	Apps           map[string]*AppMeta        `json:"apps"`
	Whitelisted    []Whitelisted              `json:"whitelisted"`
	Patches        []Patch                    `json:"patches"`
	Extensions     []*meta.Extension          `json:"extensions"`
}

// PrintTemplate is a declared document print format, as Go sees it.
type PrintTemplate struct {
	Name       string `json:"name"`
	Doctype    string `json:"doctype"`
	Label      string `json:"label"`
	App        string `json:"app"`
	SourceFile string `json:"sourceFile"`
}

// MailTemplate is a declared message, as Go sees it.
type MailTemplate struct {
	Name       string `json:"name"`
	App        string `json:"app"`
	SourceFile string `json:"sourceFile"`
	// Sensitive templates carry a credential in their arguments, so those
	// arguments are never written to the delivery record. See docs/agent/mail.md.
	Sensitive bool `json:"sensitive"`
}

// State is everything Load produces: meta, snapshot, apps, runtime pool, and
// translations catalog. It is immutable once published; a reload builds
// a new one and swaps the pointer atomically, so that no caller observes new
// metadata with an old pool (B08).
type State struct {
	Notifications     []js.Notification
	Workflows         map[string]*js.Workflow
	WorkflowByDocType map[string]*js.Workflow
	Meta              *meta.Registry
	Snap              *Snapshot
	Apps              []js.App
	Pool              *js.Pool
	I18n              *I18n
	Loaded            time.Time
	whitelisted       map[string]map[string]any
	// metaCache holds the translated copies of DocTypes, per language. It
	// needs no invalidation: a reload builds a new State and this dies with
	// the old one.
	metaCache metaCache
}

// Version is the core's version, reported by /api/boot, by the MCP server and
// by an export manifest — which is the one that matters later, because a
// reconciliation needs to know what produced the file.
// It can be overridden at build time via -ldflags "-X github.com/jrvidotti/ddcore/internal/engine.Version=..."
var Version = "0.18.2"

type Engine struct {
	// *State is embedded solely to keep `e.Meta`, `e.Snap`, `e.Apps`,
	// `e.Pool`, `e.I18n`, and `e.Loaded` compiling across internal/api,
	// internal/mcp, and cmd. The authoritative reader is Current().
	*State

	Cfg    Config
	DB     *db.DB
	Log    *slog.Logger
	Events *Hub
	Cache  *Cache

	cur   atomic.Pointer[State]
	sched atomic.Pointer[cron.Cron]
	// ready memoises the database probe; see Engine.Ready.
	ready    atomic.Pointer[readyCache]
	mu       sync.Mutex
	locOnce  sync.Once
	loc      *time.Location
	castOnce sync.Once
	casts    castOpts
	mailOnce sync.Once
	mailer   mail.Sender
	store    storage.Store
	// webhooks caches the enabled subscriptions; nil means "read them again".
	// See webhookSubs.
	webhookMu sync.Mutex
	webhooks  []webhookSub
	// oidc caches discovered providers; see oidcClientFor.
	oidcMu sync.Mutex
	oidc   map[string]*oidcClient
	// extDB keeps one pool per external database; see externalPool.
	extDBMu sync.Mutex
	extDB   map[string]*extPool
	maint   maintenanceCache
}

// Storage is where the bytes of File documents are kept.
func (e *Engine) Storage() storage.Store { return e.store }

// Current returns the state this moment sees. Each request captures it once
// and uses that same reference until the end.
func (e *Engine) Current() *State { return e.cur.Load() }

// New connects to the database and loads all apps.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.Lang == "" {
		cfg.Lang = "en"
	}
	if cfg.Currency == "" {
		cfg.Currency = "USD"
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}
	// A Config assembled by hand — every test does — would otherwise carry a
	// zero policy, which is not "no policy" but a session that expires
	// immediately and a lockout after zero attempts. Fill the gaps one field
	// at a time so an embedder can still override just one of them.
	def := config.DefaultAuth()
	if cfg.Auth.SessionDays <= 0 {
		cfg.Auth.SessionDays = def.SessionDays
	}
	if cfg.Auth.MinPasswordLength <= 0 {
		cfg.Auth.MinPasswordLength = def.MinPasswordLength
	}
	if cfg.Auth.MaxLoginAttempts <= 0 {
		cfg.Auth.MaxLoginAttempts = def.MaxLoginAttempts
	}
	if cfg.Auth.LockoutMinutes <= 0 {
		cfg.Auth.LockoutMinutes = def.LockoutMinutes
	}
	if cfg.Auth.ResetMinutes <= 0 {
		cfg.Auth.ResetMinutes = def.ResetMinutes
	}
	if cfg.Auth.InviteHours <= 0 {
		cfg.Auth.InviteHours = def.InviteHours
	}
	if cfg.Mail.Transport == "" {
		cfg.Mail.Transport = config.MailLog
	}
	cfg.Mail.Dev = cfg.Dev
	cfg.Ops = cfg.Ops.WithDefaults()
	e := &Engine{Cfg: cfg, Log: slog.New(logHandler(cfg)), Events: NewHub(), Cache: NewCache()}
	store, err := storage.New(ctx, cfg.Storage, cfg.DataDir)
	if err != nil {
		return nil, err
	}
	e.store = store
	if cfg.DSN != "" {
		d, err := db.Open(ctx, cfg.DSN)
		if err != nil {
			return nil, err
		}
		e.DB = d
	}
	if err := e.Load(); err != nil {
		return nil, err
	}
	if e.DB != nil {
		if err := e.CheckSiteVersion(ctx); err != nil {
			e.DB.Close()
			return nil, err
		}
	}
	return e, nil
}

// buildPool compiles the apps in order and returns a fresh pool plus the
// registry snapshot the runtime produced.
func (e *Engine) buildPool(apps []js.App) (*js.Pool, *Snapshot, error) {
	var bundles []*js.Bundle
	for _, a := range apps {
		b, err := js.BuildServer(a, e.Cfg.Test)
		if err != nil {
			return nil, nil, err
		}
		bundles = append(bundles, b)
	}
	pool, err := js.NewPool(e, bundles, e.Cfg.Workers+4, e.Cfg.Test)
	if err != nil {
		return nil, nil, err
	}
	rt, err := pool.Acquire()
	if err != nil {
		return nil, nil, err
	}
	raw, err := rt.Meta()
	pool.Release(rt)
	if err != nil {
		return nil, nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, nil, fmt.Errorf("meta: %w", err)
	}
	return pool, &snap, nil
}

// checkAppNames refuses a load where an app is registered under a namespace
// other than the one its manifest declares.
//
// js.AppName reads that name out of ddcore.app.ts before the bundle exists, so
// it is a guess; this is the same name after defineApp really ran. Letting the
// two drift apart would rename an app silently — its whitelisted methods, its
// scheduler targets and the module paths recorded in patch history all carry
// the namespace — so the mismatch is a configuration error, not a warning.
func checkAppNames(apps []js.App, snap *Snapshot) error {
	for _, a := range apps {
		am := snap.Apps[a.Name]
		if am == nil || am.Name == "" || am.Name == a.Name {
			continue
		}
		return fmt.Errorf("app in %s declares name %q but was loaded as %q", a.Dir, am.Name, a.Name)
	}
	return nil
}

// orderApps validates `requires` and returns the apps in dependency order.
// Core is always first. An app requiring a missing app, or a cycle, is a
// configuration error and must not lead to installation in inconsistent order.
func orderApps(apps []js.App, metas map[string]*AppMeta) ([]js.App, error) {
	byName := map[string]js.App{}
	for _, a := range apps {
		byName[a.Name] = a
	}
	var out []js.App
	state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("circular dependency between apps: %s", strings.Join(append(path, name), " → "))
		}
		state[name] = 1
		if m := metas[name]; m != nil {
			reqs := append([]string(nil), m.Requires...)
			sort.Strings(reqs)
			for _, r := range reqs {
				if _, ok := byName[r]; !ok {
					return fmt.Errorf("app %s requires %s, which is not installed", name, r)
				}
				if err := visit(r, append(path, name)); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		out = append(out, byName[name])
		return nil
	}
	for _, a := range apps {
		if err := visit(a.Name, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func sameAppOrder(a, b []js.App) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// Load (re)compiles the apps and rebuilds the runtime pool and meta.
func (e *Engine) Load() error {
	apps := append([]js.App{CoreApp()}, e.Cfg.Apps...)
	pool, snap, err := e.buildPool(apps)
	if err != nil {
		return err
	}
	if err := checkAppNames(apps, snap); err != nil {
		pool.Close()
		return err
	}
	if skipped, err := checkCoreCompat(snap, Version); err != nil {
		pool.Close()
		return err
	} else if skipped {
		e.Log.Warn("core version is not a release; ddcore ranges are not enforced", "version", Version)
	}
	for _, n := range predatesIDKey(snap, Version) {
		e.Log.Warn("app declares a ddcore range from before 0.17, when the document key became `id`; check its code and patches, then raise the range", "app", n, "ddcore", snap.Apps[n].Ddcore)
	}
	// `requires` is only known after reading metadata: validate and, if the
	// declared order violates dependencies, recompile in correct order.
	ordered, err := orderApps(apps, snap.Apps)
	if err != nil {
		pool.Close()
		return err
	}
	if !sameAppOrder(apps, ordered) {
		pool.Close()
		apps = ordered
		if pool, snap, err = e.buildPool(apps); err != nil {
			return err
		}
	}

	reg := meta.NewRegistry()
	for name, dj := range snap.Doctypes {
		var d meta.DocType
		if err := json.Unmarshal(dj, &d); err != nil {
			return fmt.Errorf("DocType %s: %w", name, err)
		}
		if d.Label == "" {
			d.Label = d.Name
		}
		if err := reg.Add(&d); err != nil {
			return err
		}
	}
	// before the extensions, so the parent Link and `is_group` a tree DocType
	// does not declare itself are ordinary fields by the time anything else
	// looks at them: an extension may relabel them, and the schema planner,
	// the typings and the extractor see them like any other.
	reg.ApplyTrees()
	// before Validate, so a field an extension adds is checked like any other:
	// reserved names, duplicates, a Link that points nowhere.
	graph := meta.Apps{Requires: map[string][]string{}}
	for _, a := range apps {
		graph.Order = append(graph.Order, a.Name)
		if am := snap.Apps[a.Name]; am != nil {
			graph.Requires[a.Name] = am.Requires
		}
	}
	if err := reg.ApplyExtensions(snap.Extensions, graph); err != nil {
		return err
	}
	// before Validate: User.language is a Select whose options are the site's
	// languages, and Validate refuses a Select with no options list.
	i18n, err := LoadI18n(apps, e.Cfg.Lang)
	if err != nil {
		return err
	}
	applyLanguageOptions(reg, i18n)
	// after the extensions, so a fieldtype ddcore dropped is forgiven wherever
	// it came from — an app written against an older version still loads
	for _, w := range reg.DropObsoleteFields() {
		e.Log.Warn("obsolete fieldtype in a DocType", "detail", w)
	}
	if err := reg.Validate(); err != nil {
		return err
	}
	notifications := make([]js.Notification, 0, len(snap.Notifications))
	for _, name := range sortedNotificationNames(snap.Notifications) {
		rule := snap.Notifications[name]
		if err := rule.ValidateTarget(reg, func(name string) bool { _, ok := snap.MailTemplates[name]; return ok }); err != nil {
			pool.Close()
			return err
		}
		notifications = append(notifications, rule)
	}
	for _, a := range apps {
		if am, ok := snap.Apps[a.Name]; ok {
			am.Dir = a.Dir
		} else {
			snap.Apps[a.Name] = &AppMeta{Name: a.Name, Title: a.Name, Dir: a.Dir}
		}
		// a form script belongs to the DocType's own app or to one extending
		// it; the desk loads every one of them, and their handlers accumulate
		for _, f := range js.ListFiles(a, ".form.ts") {
			for _, d := range reg.DocTypes {
				if d.App != a.Name && !contains(d.ExtendedBy, a.Name) {
					continue
				}
				if strings.HasSuffix(f, "/"+meta.Snake(d.Name)+".form.ts") || f == meta.Snake(d.Name)+".form.ts" {
					if !contains(d.FormApps, a.Name) {
						d.FormApps = append(d.FormApps, a.Name)
					}
				}
			}
		}
	}
	// the runtimes loaded their own app's meta; hand them the merged one, so
	// `ddcore.getMeta` in TS sees what the database and the desk see
	merged, err := mergedMeta(reg)
	if err != nil {
		return err
	}
	if err := pool.SetMeta(merged); err != nil {
		return err
	}
	wl := map[string]map[string]any{}
	for _, w := range snap.Whitelisted {
		wl[w.Path] = w.Opts
	}
	for _, name := range sortedWorkflowNames(snap.Workflows) {
		if err := snap.Workflows[name].ValidateTarget(reg); err != nil {
			pool.Close()
			return err
		}
	}
	workflows := make(map[string]*js.Workflow, len(snap.Workflows))
	workflowsByDocType := make(map[string]*js.Workflow, len(snap.Workflows))
	for k := range snap.Workflows {
		wf := snap.Workflows[k]
		// the state is shown to, and moved by, everyone the workflow serves
		if d, ok := reg.Get(wf.Doctype); ok {
			if f := d.Field(wf.StateField); f != nil && f.Permlevel > 0 {
				pool.Close()
				return fmt.Errorf("workflow %q: state field %q has permlevel %d; it must be permlevel 0", wf.Name, wf.StateField, f.Permlevel)
			}
		}
		workflows[k] = &wf
		if wf.Doctype != "" {
			workflowsByDocType[wf.Doctype] = &wf
		}
	}
	st := &State{
		Notifications:     notifications,
		Workflows:         workflows,
		WorkflowByDocType: workflowsByDocType,
		Meta:              reg,
		Snap:              snap,
		Apps:              apps,
		Pool:              pool,
		I18n:              i18n,
		Loaded:            time.Now(),
		whitelisted:       wl,
	}
	e.mu.Lock()
	old := e.cur.Swap(st)
	e.State = st
	e.mu.Unlock()
	if old != nil && old.Pool != nil {
		// runtimes still in use return to the old pool and are discarded there
		old.Pool.Close()
	}
	e.Log.Info("apps loaded", "apps", len(apps), "doctypes", len(reg.DocTypes))
	return nil
}

// WorkflowFor returns the workflow defined for the given doctype, or nil if none.
func (s *State) WorkflowFor(doctype string) *js.Workflow {
	if s == nil || s.WorkflowByDocType == nil {
		return nil
	}
	return s.WorkflowByDocType[doctype]
}

// mergedMeta is the JSON of every DocType an extension touched, keyed by name:
// what the runtimes have to replace to agree with Go about the meta.
func mergedMeta(reg *meta.Registry) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for name, d := range reg.DocTypes {
		if len(d.ExtendedBy) == 0 {
			continue
		}
		b, err := json.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("DocType %s: %w", name, err)
		}
		out[name] = b
	}
	return out, nil
}

// AppOrder returns app names in load order.
func (s *State) AppOrder() []string {
	var out []string
	for _, a := range s.Apps {
		out = append(out, a.Name)
	}
	return out
}

// SiteTitle is what the site is called wherever a person is told: the sidebar's
// heading, the browser tab, a recovery e-mail, the health report. It is the
// title of the first app that is not the core — the app whose screens the desk
// is showing — and the core's own title when there is no other app.
//
// It is resolved here, once, rather than by each reader: the name has to be the
// same word in the desk, in the mail and in /health, and two derivations that
// "should" agree is the bug nobody finds. The value is a catalogue key, like any
// label; the caller translates it into the language of whoever is reading.
func (s *State) SiteTitle() string {
	core := ""
	for _, a := range s.Apps {
		m := s.Snap.Apps[a.Name]
		if m == nil || m.Title == "" {
			continue
		}
		if a.Name == "core" {
			core = m.Title
			continue
		}
		return m.Title
	}
	return core
}

func (s *State) App(name string) js.App {
	for _, a := range s.Apps {
		if a.Name == name {
			return a
		}
	}
	return js.App{}
}

func (s *State) Whitelisted(path string) (map[string]any, bool) {
	o, ok := s.whitelisted[path]
	return o, ok
}

func (s *State) WhitelistedPaths() []string {
	var out []string
	for p := range s.whitelisted {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (s *State) DocType(name string) (*meta.DocType, error) {
	d, ok := s.Meta.Get(name)
	if !ok {
		return nil, cerr.NotFound("DocType {0} does not exist", name)
	}
	return d, nil
}

// ------------------------------------------------------------------ context

type Message struct {
	Message   string `json:"message"`
	Title     string `json:"title,omitempty"`
	Indicator string `json:"indicator,omitempty"`
	Alert     bool   `json:"alert,omitempty"`
}

// Ctx is one unit of work: a user, a transaction and (lazily) a JS runtime.
type Ctx struct {
	E *Engine
	// St is the engine state captured when the ctx was created: meta, pool, and
	// translations do not change in the middle of a request, even with reload (B08).
	St       *State
	Ctx      context.Context
	User     string
	Lang     string
	Tx       pgx.Tx
	Request  map[string]any
	Messages []Message
	Flags    map[string]any
	// Sid is the session this request arrived on, and is deliberately not in
	// Request: Request crosses into TS as ddcore.session.request, and a raw
	// sid is a bearer token. App code gets only TokenHandle(Sid), which is
	// enough to mark "this is the session you are using" and useless to steal.
	Sid string
	// ReqID ties this unit of work to the access log line, the Error Log row
	// and the X-Request-Id the caller saw. It is the opposite of Sid: safe to
	// show, and useless to anyone who cannot already read the logs. Empty when
	// no request is behind the work — a migration, a test, a CLI command.
	ReqID string

	roles        []string
	userPerms    []UserPerm
	shares       []DocShare
	sharesLoaded bool
	sharesDirty  bool
	rt           *js.Runtime
	savepoint    int
	roSavepoint  int
	docCache     map[string]Doc
	// scopeAncestors memoises a tree value's ancestors while a User Permission
	// scope is being checked row by row (DAT-07).
	scopeAncestors       map[string][]string
	afterCommit          []func()
	inWorkflowTransition bool
}

func (e *Engine) NewCtx(ctx context.Context, user string) *Ctx {
	if user == "" {
		user = "Guest"
	}
	// ReqID is read from the context rather than passed in: every caller that
	// has a request already carries it there, and deriving it here means no
	// entry point can forget to correlate.
	return &Ctx{E: e, St: e.Current(), Ctx: ctx, User: user, Lang: e.Cfg.Lang, ReqID: RequestIDFrom(ctx),
		Flags: map[string]any{}, docCache: map[string]Doc{}}
}

// Run executes fn inside a transaction with a fresh Ctx; commits on success.
func (e *Engine) Run(ctx context.Context, user string, fn func(c *Ctx) error) (err error) {
	c := e.NewCtx(ctx, user)
	return c.Run(fn)
}

func (c *Ctx) Run(fn func(c *Ctx) error) (err error) {
	tx, err := c.E.DB.Pool.Begin(c.Ctx)
	if err != nil {
		return err
	}
	c.Tx = tx
	done := false
	defer func() {
		// Also covers panics and runtime.Goexit (t.Fatal inside fn).
		if !done {
			tx.Rollback(c.Ctx)
			c.release()
		}
	}()
	err = fn(c)
	if err != nil {
		tx.Rollback(c.Ctx)
		c.release()
		done = true
		return err
	}
	committed := false
	if c.Flags["rollback"] == true {
		err = tx.Rollback(c.Ctx)
	} else {
		err = tx.Commit(c.Ctx)
		committed = err == nil
	}
	c.release()
	done = true
	// afterCommit callbacks only run when an actual commit occurred: an
	// explicit rollback must not publish events for changes that never happened.
	if committed {
		for _, f := range c.afterCommit {
			f()
		}
	}
	c.afterCommit = nil
	return err
}

func (c *Ctx) release() {
	if c.rt != nil {
		// returns to the pool that created the VM, not the current pool (B08)
		c.rt.Release()
		c.rt = nil
	}
}

// RT returns the JS runtime bound to this ctx.
func (c *Ctx) RT() (*js.Runtime, error) {
	if c.rt == nil {
		rt, err := c.St.Pool.Acquire()
		if err != nil {
			return nil, err
		}
		rt.Ctx = c
		rt.SetLang(c.Lang)
		c.rt = rt
	}
	return c.rt, nil
}

func (c *Ctx) AfterCommit(f func()) { c.afterCommit = append(c.afterCommit, f) }

func (c *Ctx) Q() db.Querier {
	if c.Tx != nil {
		return c.Tx
	}
	return c.E.DB.Pool
}

func (c *Ctx) IgnorePermissions() bool { return c.Flags["ignorePermissions"] == true }

// WithIgnorePermissions runs fn with permission checks disabled.
func (c *Ctx) WithIgnorePermissions(fn func() error) error {
	old := c.Flags["ignorePermissions"]
	c.Flags["ignorePermissions"] = true
	defer func() { c.Flags["ignorePermissions"] = old }()
	return fn()
}

// WorkflowFor returns the workflow defined for the given doctype, or nil if none.
func (c *Ctx) WorkflowFor(doctype string) *js.Workflow {
	if c == nil || c.St == nil {
		return nil
	}
	return c.St.WorkflowFor(doctype)
}

// InWorkflowTransition reports whether execution is currently inside a workflow transition.
func (c *Ctx) InWorkflowTransition() bool {
	return c != nil && c.inWorkflowTransition
}

// SetInWorkflowTransition sets the workflow transition guard bypass flag.
func (c *Ctx) SetInWorkflowTransition(v bool) {
	if c != nil {
		c.inWorkflowTransition = v
	}
}

// WithWorkflowTransition runs fn with inWorkflowTransition set to true.
func (c *Ctx) WithWorkflowTransition(fn func() error) error {
	old := c.inWorkflowTransition
	c.inWorkflowTransition = true
	defer func() { c.inWorkflowTransition = old }()
	return fn()
}

func (c *Ctx) Msgprint(m Message) { c.Messages = append(c.Messages, m) }

// T translates a string.
func (c *Ctx) T(s string, args ...any) string { return c.St.I18n.T(c.Lang, s, args...) }

// Now is the current instant in the site's timezone. The instant is the same
// everywhere; what the site's zone decides is which wall clock it is read on,
// and that is what an app means when it asks the framework what time it is.
func (c *Ctx) Now() time.Time { return time.Now().In(c.E.Location()) }

// Today is the current civil date in the *site's* timezone, not the process's.
// It is the same day the desk calls today, which is what makes a comparison
// like `due_date < today()` give one answer on both sides of the wire.
func (c *Ctx) Today() string { return time.Now().In(c.E.Location()).Format("2006-01-02") }

// CurrencyPrecision is how many decimal places a Currency value is rounded to
// on this site: the currency's own minor unit (2 for USD, 0 for JPY), unless
// ddcore.json overrides it.
func (e *Engine) CurrencyPrecision() int {
	if e.Cfg.CurrencyPrecision != nil {
		return *e.Cfg.CurrencyPrecision
	}
	return num.MinorUnits(e.Cfg.Currency)
}

// castOpts are everything castValue needs that is a property of the site
// rather than of the request. Resolved once: castAll runs per field per row per
// save, and re-deriving a timezone and an ISO minor unit inside that loop is a
// cost an import would feel.
func (e *Engine) castOpts() castOpts {
	e.castOnce.Do(func() {
		e.casts = castOpts{loc: e.Location(), currencyPrec: e.CurrencyPrecision(), rounding: e.Cfg.Rounding}
	})
	return e.casts
}

// Location is the site's timezone, resolved once. An unloadable zone name
// falls back to UTC rather than to the machine's local time: where a server
// happens to be running is not a business fact.
func (e *Engine) Location() *time.Location {
	e.locOnce.Do(func() {
		loc, err := time.LoadLocation(e.Cfg.Timezone)
		if err != nil {
			e.Log.Warn("unknown timezone, falling back to UTC", "timezone", e.Cfg.Timezone, "err", err)
			loc = time.UTC
		}
		e.loc = loc
	})
	return e.loc
}

// Savepoint helpers used by tests.
func (c *Ctx) Begin() error {
	c.savepoint++
	_, err := c.Tx.Exec(c.Ctx, fmt.Sprintf("SAVEPOINT sp%d", c.savepoint))
	return err
}

func (c *Ctx) RollbackTo() error {
	if c.savepoint == 0 {
		return nil
	}
	_, err := c.Tx.Exec(c.Ctx, fmt.Sprintf("ROLLBACK TO SAVEPOINT sp%d", c.savepoint))
	c.savepoint--
	c.docCache = map[string]Doc{}
	return err
}

// WithSavepoint runs fn inside a savepoint of the current transaction. When fn
// fails, its writes, cached documents and after-commit callbacks are undone
// and the transaction stays usable, so a best-effort side effect can fail
// without taking the caller's work down with it.
func (c *Ctx) WithSavepoint(fn func() error) error {
	if c.Tx == nil {
		return fn()
	}
	c.roSavepoint++
	sp := fmt.Sprintf("ddcore_sp%d", c.roSavepoint)
	defer func() { c.roSavepoint-- }()
	if _, err := c.Tx.Exec(c.Ctx, "SAVEPOINT "+sp); err != nil {
		return err
	}
	pending := len(c.afterCommit)
	if err := fn(); err != nil {
		if _, rbErr := c.Tx.Exec(c.Ctx, "ROLLBACK TO SAVEPOINT "+sp); rbErr != nil {
			c.E.Log.Warn("could not roll back to savepoint", "savepoint", sp, "err", rbErr)
		}
		if _, relErr := c.Tx.Exec(c.Ctx, "RELEASE SAVEPOINT "+sp); relErr != nil {
			c.E.Log.Warn("could not release savepoint", "savepoint", sp, "err", relErr)
		}
		c.afterCommit = c.afterCommit[:pending]
		c.docCache = map[string]Doc{}
		return err
	}
	_, err := c.Tx.Exec(c.Ctx, "RELEASE SAVEPOINT "+sp)
	return err
}

// ---------------------------------------------------------------- app files

// AppDir returns the directory of an app (for reading form scripts etc.).
func (e *Engine) AppDir(name string) string {
	return e.App(name).Dir
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// RunTests executes the app tests inside one rolled-back transaction.
func (e *Engine) RunTests(ctx context.Context, filter, app string) ([]js.TestResult, error) {
	if !e.Cfg.Test {
		return nil, fmt.Errorf("the engine was not loaded in test mode")
	}
	var out []js.TestResult
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		c.Flags["rollback"] = true
		// Tests run in the source language, whatever the site is set to. An
		// assertion is about a message's *key*; making it depend on
		// ddcore.json:lang means flipping the site language breaks the suite,
		// which is exactly what it did.
		c.Lang = "en"
		rt, err := c.RT()
		if err != nil {
			return err
		}
		out, err = rt.RunTests(filter, app)
		return err
	})
	return out, err
}

// Eval runs a TS snippet as Admin; returns its JSON result and logs.
func (e *Engine) Eval(ctx context.Context, code string, commit bool) (json.RawMessage, []string, error) {
	var out json.RawMessage
	var logs []string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		c.Flags["rollback"] = !commit
		c.Flags["captureLogs"] = true
		c.Flags["logs"] = []string{}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		out, err = rt.Eval(code)
		logs, _ = c.Flags["logs"].([]string)
		return err
	})
	return out, logs, err
}

func sortedWorkflowNames(workflows map[string]js.Workflow) []string {
	names := make([]string, 0, len(workflows))
	for name := range workflows {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedNotificationNames(rules map[string]js.Notification) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
