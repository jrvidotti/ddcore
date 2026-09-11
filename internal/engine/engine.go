// Package engine ties meta, db and the JS runtime together: it owns the
// Document lifecycle, permissions, queries and the bridge the TS code calls.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
)

type Config struct {
	DSN       string
	Apps      []js.App // in load order; core is prepended automatically
	Workers   int      // job workers
	Scheduler bool
	Dev       bool
	Test      bool // include *.test.ts and mark runtime as test
	Port      int
	SiteName  string
	Lang      string
	Currency  string
	// CurrencyPrecision is how many decimal places a Currency field is rounded
	// to. Zero means "not set": New resolves it from Currency.
	CurrencyPrecision *int
	Rounding          num.Rounding
	Timezone          string
	SecretKey         string
	DataDir           string // uploads
	LogLevel          slog.Level
}

// AppMeta is what defineApp produced, minus functions.
type AppMeta struct {
	Name            string                      `json:"name"`
	Title           string                      `json:"title"`
	Description     string                      `json:"description"`
	Requires        []string                    `json:"requires"`
	Version         string                      `json:"version"`
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
	Doctypes    map[string]json.RawMessage `json:"doctypes"`
	Reports     map[string]map[string]any  `json:"reports"`
	Workspaces  map[string]map[string]any  `json:"workspaces"`
	Apps        map[string]*AppMeta        `json:"apps"`
	Whitelisted []Whitelisted              `json:"whitelisted"`
	Patches     []Patch                    `json:"patches"`
	Extensions  []*meta.Extension          `json:"extensions"`
}

// State is everything Load produces: meta, snapshot, apps, runtime pool e
// catálogo de traduções. É imutável depois de publicada; um reload constrói
// outra e troca o ponteiro de uma vez só, de modo que ninguém enxerga meta
// nova com pool velho (B08).
type State struct {
	Meta        *meta.Registry
	Snap        *Snapshot
	Apps        []js.App
	Pool        *js.Pool
	I18n        *I18n
	Loaded      time.Time
	whitelisted map[string]map[string]any
	// metaCache holds the translated copies of DocTypes, per language. It
	// needs no invalidation: a reload builds a new State and this dies with
	// the old one.
	metaCache metaCache
}

type Engine struct {
	// *State é embutido só para manter `e.Meta`, `e.Snap`, `e.Apps`,
	// `e.Pool`, `e.I18n` e `e.Loaded` compilando em internal/api,
	// internal/mcp e cmd. O leitor autoritativo é Current().
	*State

	Cfg    Config
	DB     *db.DB
	Log    *slog.Logger
	Events *Hub
	Cache  *Cache

	cur      atomic.Pointer[State]
	sched    atomic.Pointer[cron.Cron]
	mu       sync.Mutex
	locOnce  sync.Once
	loc      *time.Location
	castOnce sync.Once
	casts    castOpts
}

// Current returns the state this moment sees. Cada requisição captura uma vez
// e usa a mesma até o fim.
func (e *Engine) Current() *State { return e.cur.Load() }

// New connects to the database and loads all apps.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.Lang == "" {
		cfg.Lang = "pt-BR"
	}
	if cfg.Currency == "" {
		cfg.Currency = "BRL"
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}
	e := &Engine{Cfg: cfg, Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})), Events: NewHub(), Cache: NewCache()}
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

// orderApps validates `requires` and returns the apps in dependency order.
// Core sempre primeiro. Um app que exige outro ausente, ou um ciclo, é erro
// de configuração e não pode virar instalação em ordem incoerente.
func orderApps(apps []js.App, metas map[string]*AppMeta) ([]js.App, error) {
	byName := map[string]js.App{}
	for _, a := range apps {
		byName[a.Name] = a
	}
	var out []js.App
	state := map[string]int{} // 0 novo, 1 visitando, 2 pronto
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
	// `requires` só é conhecido depois de ler a meta: valida e, se a ordem
	// declarada contraria as dependências, recompila na ordem correta.
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
	if err := reg.Validate(); err != nil {
		return err
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
	st := &State{Meta: reg, Snap: snap, Apps: apps, Pool: pool, I18n: i18n, Loaded: time.Now(), whitelisted: wl}
	e.mu.Lock()
	old := e.cur.Swap(st)
	e.State = st
	e.mu.Unlock()
	if old != nil && old.Pool != nil {
		// runtimes ainda em uso voltam ao pool antigo e são descartados lá
		old.Pool.Close()
	}
	e.Log.Info("apps loaded", "apps", len(apps), "doctypes", len(reg.DocTypes))
	return nil
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
	// St is the engine state captured when the ctx was created: meta, pool e
	// traduções não mudam no meio de uma requisição, mesmo com reload (B08).
	St       *State
	Ctx      context.Context
	User     string
	Lang     string
	Tx       pgx.Tx
	Request  map[string]any
	Messages []Message
	Flags    map[string]any

	roles       []string
	rt          *js.Runtime
	savepoint   int
	roSavepoint int
	docCache    map[string]Doc
	afterCommit []func()
}

func (e *Engine) NewCtx(ctx context.Context, user string) *Ctx {
	if user == "" {
		user = "Guest"
	}
	return &Ctx{E: e, St: e.Current(), Ctx: ctx, User: user, Lang: e.Cfg.Lang, Flags: map[string]any{}, docCache: map[string]Doc{}}
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
	// callbacks de afterCommit só rodam quando houve commit de verdade: um
	// rollback explícito não pode publicar eventos do que não aconteceu.
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
		// devolve ao pool que criou a VM, não ao pool corrente (B08)
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
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
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

// Eval runs a TS snippet as Administrator; returns its JSON result and logs.
func (e *Engine) Eval(ctx context.Context, code string, commit bool) (json.RawMessage, []string, error) {
	var out json.RawMessage
	var logs []string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
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
