// ddcore — the CLI: dev server, migrations, tests, types, jobs, users, MCP.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	ddcore "github.com/jrvidotti/ddcore"
	"github.com/jrvidotti/ddcore/desk"
	"github.com/jrvidotti/ddcore/internal/api"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/mcp"
	"github.com/jrvidotti/ddcore/internal/scaffold"
	"github.com/jrvidotti/ddcore/internal/typegen"
	"github.com/jrvidotti/ddcore/internal/watch"
)

func main() {
	status, msg := exitStatus(run(stripGlobalFlags(os.Args[1:])))
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(status)
}

// stripGlobalFlags removes the flags every command accepts, turning them into
// the environment variables load reads, so no command's FlagSet has to know
// about them.
func stripGlobalFlags(args []string) []string {
	out := args[:0:0]
	for i := 0; i < len(args); i++ {
		a := args[i]
		// `--tenant acme` before the command: the tenant it works in. Only
		// before it — after the command the word belongs to the command.
		if len(out) == 0 && (a == "--tenant" || a == "-tenant") && i+1 < len(args) {
			os.Setenv("DDCORE_TENANT", args[i+1])
			i++
			continue
		}
		if v, ok := strings.CutPrefix(a, "--tenant="); ok && len(out) == 0 {
			os.Setenv("DDCORE_TENANT", v)
			continue
		}
		if a == "--allow-older-binary" || a == "-allow-older-binary" {
			os.Setenv("DDCORE_ALLOW_OLDER_BINARY", "1")
			continue
		}
		// `--flag=value` is how every other boolean here is written, so it
		// cannot fall through to the command's own FlagSet as an unknown flag.
		if v, ok := strings.CutPrefix(a, "--allow-older-binary="); ok {
			os.Setenv("DDCORE_ALLOW_OLDER_BINARY", v)
			continue
		}
		if v, ok := strings.CutPrefix(a, "-allow-older-binary="); ok {
			os.Setenv("DDCORE_ALLOW_OLDER_BINARY", v)
			continue
		}
		out = append(out, a)
	}
	return out
}

// loadOpts says how load builds the engine. The zero value is a one-shot
// command: it bypasses maintenance mode, needs the database and logs to stderr.
type loadOpts struct {
	Test bool
	Dev  bool
	// LogOut is where the log goes; nil is stderr. A one-shot command keeps it
	// there, because its stdout is its output — a log line in the middle of an
	// exported NDJSON, a printed key, a doctor report or the MCP JSON-RPC
	// stream is corruption, not a log. The server flips it to stdout, because
	// there the log *is* the output and a platform that captures both streams
	// reads everything on stderr as an error: on stderr an INFO access line
	// arrives painted red and the level policy stops meaning anything.
	LogOut io.Writer
	// EnforceMaintenance is set by the commands that serve traffic or run jobs
	// (`dev`, `start`, `jobs work`). Every other command leaves it false,
	// which is what makes the CLI the maintenance bypass.
	EnforceMaintenance bool
	// DeferDB is set by `mcp`, the one command that is still useful without
	// the database: see engine.Config.DeferDB.
	DeferDB bool
	// NoDB builds the engine without a database, for a command that reads the
	// app sources and the registry they produce and never a row (`types`,
	// `i18n extract`): it needs no dsn and has no tenant to enter.
	NoDB bool
	// Quiet logs warnings and errors only.
	Quiet bool
}

// serveOpts is how `start` and `dev` load the engine.
func serveOpts(dev bool) loadOpts {
	return loadOpts{Dev: dev, LogOut: os.Stdout, EnforceMaintenance: true}
}

// mcpOpts is how `mcp` loads the engine.
func mcpOpts() loadOpts {
	return loadOpts{Dev: true, DeferDB: true}
}

// engineConfig maps the site's configuration to the engine's.
func engineConfig(cfg *config.File, o loadOpts) (engine.Config, error) {
	root, err := filepath.Abs(filepath.Dir(cfg.Path))
	if err != nil {
		return engine.Config{}, err
	}
	if cfg.DSN == "" && !o.NoDB {
		return engine.Config{}, fmt.Errorf("dsn not configured: run `ddcore init` or set DDCORE_DSN")
	}
	level := slog.LevelInfo
	if o.Quiet {
		level = slog.LevelWarn
	}
	if os.Getenv("DDCORE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	out := o.LogOut
	if out == nil {
		out = os.Stderr
	}
	// --tenant is for a command that does one thing and exits. A process that
	// serves requests or runs jobs works in every tenant, each from its own
	// user or its own job; pinning it to one would be a quiet way to break it.
	enter := os.Getenv("DDCORE_TENANT")
	if o.NoDB {
		enter = ""
	}
	if enter != "" && (o.EnforceMaintenance || o.DeferDB) {
		return engine.Config{}, fmt.Errorf("--tenant is for one-shot commands, not for a server, a worker or mcp")
	}
	if enter != "" && !cfg.Tenancy {
		return engine.Config{}, fmt.Errorf(`--tenant needs "tenancy": true in ddcore.json`)
	}
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: js.AppName(dir), Dir: dir})
	}
	ec := engine.Config{
		DSN: cfg.DSN, Apps: apps, DDCore: cfg.DDCore, Workers: cfg.Workers.Total(), Scheduler: cfg.Scheduler, Tenancy: cfg.Tenancy, TenantRole: cfg.TenantRole, PoolMaxConns: cfg.PoolMaxConns, EnterTenant: enter, Dev: o.Dev || config.DevFromEnv(), Test: o.Test,
		Port: cfg.Port, SiteTitle: cfg.Title, Lang: cfg.Lang, Currency: cfg.Currency, CurrencyPrecision: cfg.CurrencyPrecision, Rounding: cfg.RoundingMode(), Timezone: cfg.Timezone, DataDir: cfg.DataDir, Root: root, ExportMaxRows: cfg.ExportMaxRows, ImportMaxRows: cfg.ImportMaxRows, LogLevel: level,
		Auth: cfg.Auth, Ops: cfg.Ops, LogJSON: logJSON(out), LogOut: out, Mail: cfg.Mail, Webhooks: cfg.Webhooks, Storage: cfg.Storage, SiteURL: cfg.PublicURL(), TrustProxy: cfg.TrustProxy, CORS: cfg.CORS, Login: cfg.Login, Feedback: cfg.Feedback, AI: cfg.AI, OIDC: cfg.OIDC, Portal: cfg.Portal, Map: cfg.Map,
		EnforceMaintenance: o.EnforceMaintenance, AllowOlderBinary: config.AllowOlderBinaryFromEnv(), AdminPassword: cfg.AdminPassword, DeferDB: o.DeferDB,
	}
	if o.NoDB {
		ec.DSN = ""
	}
	return ec, nil
}

// loadSite creates the engine cfg describes and tells the log what is off
// about the configuration.
func loadSite(cfg *config.File, o loadOpts) (*engine.Engine, error) {
	ec, err := engineConfig(cfg, o)
	if err != nil {
		return nil, err
	}
	e, err := engine.New(context.Background(), ec)
	if err == nil {
		warnConfig(e, cfg)
	}
	return e, err
}

// load builds the engine from ddcore.json.
func load(o loadOpts) (*engine.Engine, *config.File, error) {
	cfg, _, err := config.Load(".")
	if err != nil {
		return nil, nil, err
	}
	e, err := loadSite(cfg, o)
	return e, cfg, err
}

// warnConfig logs what Load let pass in ddcore.json, once the engine has a logger.
func warnConfig(e *engine.Engine, cfg *config.File) {
	for _, w := range cfg.Warnings {
		e.Log.Warn(w)
	}
	if !cfg.HasPublicURL() {
		// Say it once, at boot, rather than letting someone discover it in a
		// recovery e-mail that points at a machine the reader does not have.
		e.Log.Warn("no public URL configured: recovery and invitation links will point at "+cfg.PublicURL(),
			"fix", "set DDCORE_URL in .env")
	}
}

// withSite opens the engine, runs fn with the site's configuration and closes the pool.
func withSite(o loadOpts, fn func(*engine.Engine, *config.File) error) error {
	e, cfg, err := load(o)
	if err != nil {
		return err
	}
	if e.DB != nil {
		defer e.DB.Close()
	}
	return fn(e, cfg)
}

// withEngine is withSite for a command that needs no configuration, only a context.
func withEngine(fn func(*engine.Engine, context.Context) error) error {
	return withSite(loadOpts{}, func(e *engine.Engine, _ *config.File) error {
		return fn(e, context.Background())
	})
}

func cmdInit(args []string) error {
	fs := newFlagSet("init")
	dsn := fs.String("dsn", "", "Postgres connection (an existing or remote database)")
	name := fs.String("name", "", "database name, user and password (defaults to the directory's name)")
	dbPort := fs.Int("db-port", 5432, "port of the local Postgres")
	port := fs.Int("port", 8080, "HTTP port")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["dsn"] && (set["name"] || set["db-port"]) {
		return fmt.Errorf("use --dsn or --name/--db-port, not both")
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if *name == "" {
		*name = scaffold.DBName(filepath.Base(wd))
	}
	if *dsn == "" {
		if !scaffold.DBNameRe.MatchString(*name) {
			return fmt.Errorf("--name %q: use lowercase letters, digits and _, not starting with a digit", *name)
		}
		*dsn = scaffold.LocalDSN(*name, *dbPort)
	}
	// Idempotent: in a checkout that already contains ddcore.json, the bootstrap
	// script's `init && migrate` must work — update what was requested and notify.
	if _, err := os.Stat(config.Name); err == nil {
		changed := false
		path, err := config.Edit(".", func(cur *config.File, _ string) bool {
			if set["dsn"] || set["name"] || set["db-port"] {
				cur.DSN, changed = *dsn, true
			}
			if set["port"] {
				cur.Port, changed = *port, true
			}
			return changed
		})
		if err != nil {
			return err
		}
		if !changed {
			fmt.Printf("%s already exists — nothing to do (use --dsn, --name/--db-port or --port to update)\n", config.Name)
			return nil
		}
		fmt.Println("updated", path)
		return nil
	}
	f := config.Default()
	f.DSN, f.Port = *dsn, *port
	// the site's one range for all its apps; a build that is not a release
	// cannot say which one it is, and leaves it out
	f.DDCore, _ = engine.AppRange(engine.Version)
	if err := f.Save(config.Name); err != nil {
		return err
	}
	if err := writeEnvExample(); err != nil {
		return err
	}
	compose, err := writeCompose(*dsn)
	if err != nil {
		return err
	}
	files, err := scaffold.Project(".", scaffold.ProjectInfo{Name: *name, DSN: *dsn, Port: *port, Compose: compose})
	if err == nil {
		tag, _ := engine.ImageTag(engine.Version)
		var docker []string
		docker, err = scaffold.Docker(".", tag)
		files = append(files, docker...)
	}
	for _, f := range files {
		fmt.Println("created", f)
	}
	if err != nil {
		return err
	}
	fmt.Println("created", config.Name)
	fmt.Println("\nnext:")
	if compose {
		fmt.Println("  docker compose up -d")
	}
	fmt.Println("  ddcore new-app <name>")
	fmt.Println("  ddcore migrate                                → prints the Admin password")
	fmt.Printf("  ddcore dev                                    → http://localhost:%d\n", *port)
	return nil
}

// composeFiles are the names `docker compose` looks for; init leaves a
// project that already has one of them alone.
var composeFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// writeCompose drops a docker-compose.yml running the Postgres the dsn points
// at, so a new project needs Docker rather than a database set up by hand. It
// reports whether it wrote one.
func writeCompose(dsn string) (bool, error) {
	for _, n := range composeFiles {
		if _, err := os.Stat(n); err == nil {
			fmt.Println(n, "already exists — left as is")
			return false, nil
		}
	}
	content, ok := scaffold.Compose(dsn)
	if !ok {
		fmt.Println("no docker-compose.yml: the dsn does not point at a Postgres on this machine")
		return false, nil
	}
	if err := os.WriteFile("docker-compose.yml", []byte(content), 0o644); err != nil {
		return false, err
	}
	fmt.Println("created docker-compose.yml — `docker compose up -d` starts the database")
	return true, nil
}

// writeEnvExample drops the committed record of which environment variables
// exist. .env itself is gitignored, so without this nobody deploying the site
// can tell what it expects to be set.
func writeEnvExample() error {
	const name = ".env.example"
	if _, err := os.Stat(name); err == nil {
		return nil
	}
	if err := os.WriteFile(name, []byte(ddcore.EnvExample), 0o644); err != nil {
		return err
	}
	fmt.Println("created", name, "— copy to .env for this machine's database, URL and mail")
	return nil
}

func cmdNewApp(args []string) error {
	fs := newFlagSet("new-app")
	dir := fs.String("dir", "", "directory (defaults to apps/<name>)")
	title := fs.String("title", "", "title")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ddcore new-app <name>")
	}
	name := fs.Arg(0)
	if *dir == "" {
		*dir = filepath.Join("apps", name)
	}
	rng, _ := engine.AppRange(engine.Version)
	// A site that declares its range in ddcore.json keeps it there for every app.
	siteRange := false
	if cfg, _, err := config.Load("."); err == nil {
		siteRange = cfg.DDCore != ""
	}
	if err := scaffold.App(*dir, name, *title, rng, siteRange); err != nil {
		return err
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	if _, err := config.Edit(".", func(cfg *config.File, path string) bool {
		// relative to ddcore.json, so the file means the same on every machine
		entry := abs
		if r, err := filepath.Rel(filepath.Dir(path), abs); err == nil && !strings.HasPrefix(r, "..") {
			entry = r
		}
		if slices.Contains(cfg.Apps, entry) {
			return false
		}
		cfg.Apps = append(cfg.Apps, entry)
		return true
	}); err != nil {
		// the app exists on disk but no site will load it: say so rather than
		// leave `migrate` to report an app it never heard of
		return fmt.Errorf("app %s created at %s, but not registered: %w", name, *dir, err)
	}
	fmt.Printf("app %s created at %s\n", name, *dir)
	return nil
}

func cmdServe(args []string, dev bool) error {
	name := "start"
	if dev {
		name = "dev"
	}
	fs := newFlagSet(name)
	autoMigrate := fs.Bool("auto-migrate", config.AutoMigrateFromEnv(), "apply pending DDL and patches on (re)load (DDCORE_AUTO_MIGRATE=0 turns the default off)")
	port := fs.Int("port", 0, "port")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	e, cfg, err := load(serveOpts(dev))
	if err != nil {
		return err
	}
	if *port != 0 {
		cfg.Port = *port
	}
	ctx, cancel := signalContext()
	defer cancel()
	if *autoMigrate {
		res, err := e.Migrate(ctx, false)
		if err != nil {
			return err
		}
		showAdminPassword(res)
	} else if plan, err := e.Plan(ctx, false); err != nil {
		// A refusal surfaces here too: swallowing it would report "0 pending"
		// for a migration the planner will not run.
		e.Log.Warn("migrate cannot run as declared", "err", err)
	} else if len(plan) > 0 {
		e.Log.Warn("there is pending DDL — run `ddcore migrate`", "statements", len(plan))
	}
	srv := api.New(e, desk.FS())
	if dev {
		// MCP over HTTP only with Admin/System Manager API key (B01)
		srv.MCPHandler = srv.RequireAdminAPIKey(mcp.HTTPHandler(e))
		srv.Router.Handle("/mcp", srv.MCPHandler)
		srv.Router.Handle("/mcp/*", srv.MCPHandler)
	}
	go e.WatchMaintenance(ctx)
	go e.WatchCache(ctx)
	if st := e.Maintenance(ctx); st.Enabled {
		e.Log.Warn("site is in maintenance mode: writes and jobs are paused — `ddcore maintenance off` to resume", "reason", st.Reason, "since", st.Since)
	}
	workers := e.StartWorkerPools(cfg.Workers.Pools())
	if cfg.Scheduler {
		e.StartScheduler(ctx)
	} else {
		e.Log.Warn("scheduler disabled (scheduler: false in ddcore.json)")
	}
	if dev {
		go watch.Apps(ctx, e, func() {
			if err := e.Load(); err != nil {
				e.Log.Error("reload failed", "err", err)
				e.Events.Publish(engine.Event{Name: "reload_error", SiteWide: true, Payload: map[string]any{"error": err.Error()}})
				return
			}
			if *autoMigrate {
				if res, err := e.Migrate(ctx, false); err != nil {
					e.Log.Error("migrate failed", "err", err)
				} else {
					if len(res.DDL) > 0 {
						e.Log.Info("migrate", "ddl", len(res.DDL))
					}
					showAdminPassword(res)
				}
			}
			e.Cache.Clear()
			e.Events.Publish(engine.Event{Name: "reload", SiteWide: true, Payload: map[string]any{"loaded": e.Current().Loaded.UnixMilli()}})
		})
	}
	h := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: srv.Router, ReadHeaderTimeout: 10 * time.Second}
	// The process returns only once this has run: ListenAndServe returns the
	// moment Shutdown is called, and a process that exits then takes its
	// running jobs down with it, mid-call, before they could be given back.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown(e, cancel, workers, cfg.ShutdownGrace(), func() {
			sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()
			h.Shutdown(sctx)
		})
	}()
	// listen is where this process answers; url is the public address links are
	// built from, which behind a proxy is a different host altogether.
	// Listening apart from serving is what lets onBoot run once the socket
	// accepts: a provider it registers a webhook with may call the site back
	// before the registration returns.
	ln, err := net.Listen("tcp", h.Addr)
	if err == nil {
		e.Log.Info("ddcore running", "version", engine.Version, "listen", fmt.Sprintf("http://localhost:%d", cfg.Port), "url", cfg.PublicURL(), "dev", e.Cfg.Dev, "apps", e.Current().AppOrder())
		go e.Boot(ctx)
		err = h.Serve(ln)
	}
	cancel() // a server that could not listen stops the rest as well
	<-stopped
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// shutdown is how a process that runs jobs stops, once it has been told to:
// the scheduler first, so nothing new is enqueued; then the job workers, which
// claim nothing more and give the jobs still running the grace to finish,
// alongside whatever else the process serves (also, which may be nil). It
// returns when all of it has stopped. stopSignals is called first so that a
// second Ctrl-C ends the process at once instead of waiting out the grace.
func shutdown(e *engine.Engine, stopSignals func(), workers *engine.Workers, grace time.Duration, also func()) {
	stopSignals()
	e.Log.Info("stopping: running jobs have the grace to finish", "grace", grace.String())
	e.StopScheduler()
	done := make(chan struct{})
	go func() { defer close(done); workers.Stop(grace) }()
	if also != nil {
		also()
	}
	<-done
}

func cmdMigrate(args []string) error {
	fs := newFlagSet("migrate")
	dry := fs.Bool("dry-run", false, "only print the DDL")
	prune := fs.Bool("prune", false, "drop columns and tables the meta no longer has")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withSite(loadOpts{}, func(e *engine.Engine, cfg *config.File) error {
		ctx := context.Background()
		if *dry {
			plan, err := e.Plan(ctx, *prune)
			if err != nil {
				return err
			}
			fmt.Print(db.Report(plan))
			if pending := e.PendingPatches(); len(pending) > 0 {
				fmt.Println("patches")
				for _, p := range pending {
					fmt.Printf("  %-12s %s\n", p.Phase, p.Path)
				}
			}
			return nil
		}
		res, err := e.Migrate(ctx, *prune)
		if err != nil {
			return err
		}
		fmt.Print(db.Report(res.DDL))
		fmt.Println("ok:", res.String())
		showAdminPassword(res)
		return writeTypes(e, cfg)
	})
}

// cmdTypes writes the typings; see loadOpts.NoDB.
func cmdTypes(args []string) error {
	return withSite(loadOpts{NoDB: true}, writeTypes)
}

// writeTypes writes each app's .ddcore/types.d.ts from the registry e holds.
func writeTypes(e *engine.Engine, cfg *config.File) error {
	written, err := typegen.WriteAll(cfg.Apps, e.Current().Meta)
	for _, path := range written {
		fmt.Println("types:", path)
	}
	return err
}

func testFlags() (*flag.FlagSet, *string, *bool, *string) {
	fs := newFlagSet("test")
	filter := fs.String("filter", "", "regex over the test's name and file")
	verbose := fs.Bool("v", false, "list every test")
	app := fs.String("app", "", "run the tests of one loaded app")
	return fs, filter, verbose, app
}

func cmdTest(args []string) error {
	fs, filter, verbose, app := testFlags()
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 && *filter == "" {
		*filter = fs.Arg(0)
	}
	return withSite(loadOpts{Test: true}, func(e *engine.Engine, _ *config.File) error {
		ctx := context.Background()
		if *app != "" {
			found := false
			for _, loaded := range e.Current().Apps {
				if loaded.Name == *app {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("app not loaded: %s", *app)
			}
		}
		if _, err := e.Migrate(ctx, false); err != nil {
			return err
		}
		results, err := e.RunTests(ctx, *filter, *app)
		if err != nil {
			return err
		}
		failed := 0
		for _, r := range results {
			if r.OK {
				if *verbose {
					fmt.Printf("  ok   %s (%dms)\n", r.Name, r.Ms)
				}
				continue
			}
			failed++
			fmt.Printf("  FAIL %s\n       %s\n", r.Name, strings.ReplaceAll(r.Error, "\n", "\n       "))
			if r.File != "" {
				fmt.Printf("       in %s\n", r.File)
			}
		}
		fmt.Printf("%d tests, %d failures\n", len(results), failed)
		if failed > 0 {
			return exitCode(1)
		}
		return nil
	})
}

func execFlags() (*flag.FlagSet, *string, *string) {
	fs := newFlagSet("exec")
	argsJSON := fs.String("args", "{}", "JSON arguments; - reads them from stdin")
	argsFile := fs.String("args-file", "", "read the JSON arguments from a file (- for stdin)")
	return fs, argsJSON, argsFile
}

// execArgs resolves the arguments of `ddcore exec`. Whatever is on the command
// line is readable by every user of the host through ps and /proc/<pid>/cmdline
// and is capped by ARG_MAX, so a payload carrying personal data, or a large
// one, comes from stdin or a file instead (#103).
func execArgs(argsJSON, argsFile string, argsSet bool, stdin io.Reader) (map[string]any, error) {
	if argsSet && argsFile != "" {
		return nil, fmt.Errorf("use --args or --args-file, not both")
	}
	src, raw := "--args", []byte(argsJSON)
	switch {
	case argsJSON == "-" || argsFile == "-":
		src = "stdin"
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src, err)
		}
		raw = b
	case argsFile != "":
		src = "--args-file " + argsFile
		b, err := os.ReadFile(argsFile)
		if err != nil {
			return nil, fmt.Errorf("--args-file: %w", err)
		}
		raw = b
	}
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	return a, nil
}

func cmdExec(args []string) error {
	fs, argsJSON, argsFile := execFlags()
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ddcore exec app.mod.fn [--args '{}' | --args - | --args-file f.json]")
	}
	argsSet := false
	fs.Visit(func(f *flag.Flag) { argsSet = argsSet || f.Name == "args" })
	// before load: a malformed payload should not cost a database connection
	a, err := execArgs(*argsJSON, *argsFile, argsSet, os.Stdin)
	if err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		res, err := e.RunJob(ctx, "Admin", fs.Arg(0), a)
		if err != nil {
			return err
		}
		fmt.Println(string(res))
		return nil
	})
}

func cmdEval(args []string) error {
	fs := newFlagSet("eval")
	commit := fs.Bool("commit", false, "commit the transaction (default: rollback)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	code := strings.Join(fs.Args(), " ")
	if code == "" || code == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		code = string(b)
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		res, logs, err := e.Eval(ctx, code, *commit)
		for _, l := range logs {
			fmt.Fprintln(os.Stderr, l)
		}
		if err != nil {
			return err
		}
		fmt.Println(string(res))
		return nil
	})
}

// cmdDemo runs `<app>.services.demo.generate` for every app that whitelists it.
func cmdDemo(args []string) error {
	fs := newFlagSet("demo")
	app := fs.String("app", "", "only this app (default: every app that has a demo)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		ran := 0
		for _, name := range e.Current().AppOrder() {
			if *app != "" && name != *app {
				continue
			}
			// `generate` does not need to be whitelisted (runs as Admin), so
			// app existence is checked by file
			if dir := e.AppDir(name); dir == "" {
				continue
			} else if _, err := os.Stat(filepath.Join(dir, "services", "demo.ts")); err != nil {
				continue
			}
			method := name + ".services.demo.generate"
			res, err := e.RunJob(ctx, "Admin", method, nil)
			if err != nil {
				return fmt.Errorf("%s: %w", method, err)
			}
			fmt.Printf("%s: %s\n", method, strings.TrimSpace(string(res)))
			ran++
		}
		if ran == 0 {
			return fmt.Errorf("no app has `services/demo.ts`")
		}
		return nil
	})
}

const userUsage = "usage: ddcore user add <email> <name> [--password x] [--role R]\n" +
	"       ddcore user passwd <email> <password>\n" +
	"       ddcore user invite <email> <name> [--role R]\n" +
	"       ddcore user reset <email>\n" +
	"       ddcore user unlock <email>\n" +
	"       ddcore user sessions <email> [--revoke]"

func cmdUser(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", userUsage)
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		sub, rest := args[0], args[1:]
		switch sub {
		case "add":
			fs := newFlagSet("user add")
			pw := fs.String("password", "", "password")
			var roles multi
			fs.Var(&roles, "role", "role (repeatable)")
			if err := parseFlags(fs, rest); err != nil {
				return err
			}
			if fs.NArg() < 2 {
				return fmt.Errorf("usage: ddcore user add <email> <name>")
			}
			return e.Run(ctx, "Admin", func(c *engine.Ctx) error {
				doc, err := c.NewDoc("User", engine.Doc{"email": fs.Arg(0), "full_name": strings.Join(fs.Args()[1:], " "), "new_password": *pw, "enabled": true})
				if err != nil {
					return err
				}
				var rs []any
				for _, r := range roles {
					rs = append(rs, map[string]any{"role": r})
				}
				doc["roles"] = rs
				_, err = c.Insert(doc, engine.SaveOpts{IgnorePermissions: true})
				if err == nil {
					fmt.Println("user created:", fs.Arg(0))
				}
				return err
			})
		case "passwd":
			if len(rest) < 2 {
				return fmt.Errorf("usage: ddcore user passwd <email> <password>")
			}
			if err := e.SetPassword(ctx, rest[0], rest[1]); err != nil {
				return err
			}
			fmt.Println("password changed; other sessions signed out")
			return nil
		case "invite":
			// Creates the account with no password and sends the link. No password
			// needs no flag for "pending": CheckPassword already refuses an empty
			// hash, so an invited person simply cannot sign in until they set one.
			fs := newFlagSet("user invite")
			var roles multi
			fs.Var(&roles, "role", "role (repeatable)")
			if err := parseFlags(fs, rest); err != nil {
				return err
			}
			if fs.NArg() < 2 {
				return fmt.Errorf("usage: ddcore user invite <email> <name> [--role R]")
			}
			email, name := fs.Arg(0), strings.Join(fs.Args()[1:], " ")
			// the same path as the desk's Save and invite: on a site that
			// provisions PocketID, the account is made there too
			return e.Run(ctx, "Admin", func(c *engine.Ctx) error {
				res, err := e.InviteUser(c, engine.Invitation{Email: email, FullName: name, Roles: roles})
				if err != nil {
					return err
				}
				rec := &engine.Recovery{}
				rec.Link, _ = res["link"].(string)
				rec.Expires, _ = res["expires"].(time.Time)
				reportRecovery(email, rec)
				return nil
			})
		case "reset":
			if len(rest) < 1 {
				return fmt.Errorf("usage: ddcore user reset <email>")
			}
			// in the account's own tenant: the link is mailed from there
			return e.RunAdminFor(ctx, rest[0], func(c *engine.Ctx) error {
				rec, err := e.StartRecovery(c, rest[0], engine.TokenReset, "")
				if err != nil {
					return err
				}
				reportRecovery(rest[0], rec)
				return nil
			})
		case "unlock":
			if len(rest) < 1 {
				return fmt.Errorf("usage: ddcore user unlock <email>")
			}
			n, err := e.UnlockAccount(ctx, rest[0], cliActor())
			if err != nil {
				return err
			}
			fmt.Printf("cleared %d failed attempt(s) for %s\n", n, rest[0])
			return nil
		case "sessions":
			fs := newFlagSet("user sessions")
			revoke := fs.Bool("revoke", false, "end every session of this user")
			if err := parseFlags(fs, rest); err != nil {
				return err
			}
			if fs.NArg() < 1 {
				return fmt.Errorf("usage: ddcore user sessions <email> [--revoke]")
			}
			user := fs.Arg(0)
			if *revoke {
				n, err := e.DropSessions(ctx, e.DB.Pool, user, "")
				if err != nil {
					return err
				}
				fmt.Printf("ended %d session(s) for %s\n", n, user)
				return nil
			}
			rows, err := db.Select(ctx, e.DB.Pool,
				`SELECT ip, user_agent, created, last_seen FROM ddcore_session
				 WHERE "user" = $1 AND expires > now() ORDER BY last_seen DESC`, user)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Println("no active sessions")
				return nil
			}
			for _, r := range rows {
				fmt.Printf("  %-20s %-28s last seen %s\n",
					db.Str(r["ip"]), truncateStr(db.Str(r["user_agent"]), 28), db.Str(r["last_seen"]))
			}
			return nil
		default:
			return fmt.Errorf("unknown subcommand: %s", sub)
		}
	})
}

// reportRecovery prints the link when the site is not really delivering mail,
// so that development and a first-user bootstrap do not require reading a log.
// When mail is being sent, printing it would put a live credential into shell
// history for no reason.
func reportRecovery(user string, rec *engine.Recovery) {
	if rec.Link != "" {
		fmt.Printf("%s: no mail transport configured — send this link yourself:\n  %s\n", user, rec.Link)
		return
	}
	fmt.Printf("%s: link sent, valid until %s\n", user, rec.Expires.Format("2006-01-02 15:04"))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

func cmdAPIKey(args []string) error {
	fs := newFlagSet("apikey")
	label := fs.String("label", "cli", "description")
	days := fs.Int("days", 0, "expire after N days (0 = never)")
	prefix := fs.String("prefix", "", "start the key's id with <prefix>. (a tenant's slug, say)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ddcore apikey <user> [--label x] [--days N] [--prefix p]")
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		// the key is a document of its user's tenant, and audited like one app
		// code issues
		out, err := e.IssueAPIKey(ctx, fs.Arg(0), *label, *days, *prefix)
		if err != nil {
			return err
		}
		fmt.Printf("%s:%s\n", out["key"], out["secret"])
		if out["expires"] != nil {
			fmt.Fprintf(os.Stderr, "expires: %v\n", out["expires"])
		}
		return nil
	})
}

func cmdMCP(args []string) error {
	e, _, err := load(mcpOpts())
	if err != nil {
		return err
	}
	defer func() {
		if e.DB != nil {
			e.DB.Close()
		}
	}()
	ctx, cancel := signalContext()
	defer cancel()
	go watch.Apps(ctx, e, func() {
		if err := e.Load(); err != nil {
			e.Log.Error("reload failed", "err", err)
		}
	})
	go e.WatchCache(ctx)
	return mcp.ServeStdio(ctx, e)
}

// logJSON reads the log shape from the environment and not from ddcore.json:
// it is a property of where the process runs — a terminal wants text, a
// platform that ships the log to a collector wants objects it can index.
// DDCORE_LOG_FORMAT names it outright (`json` or `text`); with nothing set the
// process asks the log's own destination, since a character device is somebody
// watching and anything else — a pipe, a file, a container's log stream — is a
// collector that reads the level out of the object instead of guessing it from
// the stream.
func logJSON(out io.Writer) bool {
	switch {
	case strings.EqualFold(os.Getenv("DDCORE_LOG_FORMAT"), "json"):
		return true
	case strings.EqualFold(os.Getenv("DDCORE_LOG_FORMAT"), "text"):
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return true
	}
	// The file mode already answers "is this a terminal", so golang.org/x/term
	// would be a dependency taken on for one bit that is already here.
	fi, err := f.Stat()
	return err != nil || fi.Mode()&os.ModeCharDevice == 0
}

// showAdminPassword prints the password a migration generated for Admin. It
// goes to stderr, so a command whose stdout is JSON stays parseable, and it is
// the only time anyone sees it: the database keeps the hash.
func showAdminPassword(res *engine.MigrateResult) {
	if res == nil || res.AdminPassword == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n  Admin password: %s\n\n"+
		"  Generated because Admin had none. It is shown only this once;\n"+
		"  change it with `ddcore user passwd Admin <password>`.\n\n", res.AdminPassword)
}
