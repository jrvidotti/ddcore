// ddcore — the CLI: dev server, migrations, tests, types, jobs, users, MCP.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

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

const usage = `ddcore — an application framework (DocTypes in TypeScript, core in Go, PostgreSQL)

Usage: ddcore <command> [options]

  init        create ddcore.json in the current directory
  new-app     create an app: ddcore new-app <name> [--dir apps/<name>]
  dev         development server with hot reload (port from ddcore.json)
  start       production server
  migrate     apply DDL, install apps, run patches (--dry-run, --prune)
  types       generate .ddcore/types.d.ts in every app
  i18n        i18n extract — rewrite translations/<lang>.csv from the code
  test        run the *.test.ts (--filter regex, --app name)
  exec        run a function: ddcore exec app.services.mod.fn --args '{"a":1}'
  eval        run loose TS: ddcore eval 'ddcore.db.count("User")' [--commit]
  demo        seed example data (<app>.services.demo.generate, idempotent)
  export      export a DocType (or --all) to NDJSON/CSV with a manifest
  jobs        inspect, retry, cancel and purge the queue (run: ddcore jobs)
  webhooks    list and replay outgoing webhook deliveries (run: ddcore webhooks)
  audit       inspect and purge administrative audit events (run: ddcore audit)
  user        user add|invite|passwd|reset|unlock|sessions (run: ddcore user)
  apikey      apikey <user> [--label x] [--days N]  → prints key:secret
  mcp         MCP server (stdio) for agents
  docs        print the framework documentation
  doctor      database readiness, meta, queue and errors (--json, --strict)
  maintenance maintenance on [--reason x] | off | status — pause writes and jobs
  backup      write a .tar of the database, files, config and versions (--to s3)
  restore     restore <archive> into this site's database and storage (--smoke)
  version     print the framework version

Variables: DDCORE_DSN overrides the dsn in ddcore.json.
Global flag: --allow-older-binary (DDCORE_ALLOW_OLDER_BINARY=1) opens a database
a newer release migrated — a rollback; see ` + "`ddcore docs backup`" + `.
`

func main() {
	argv := stripGlobalFlags(os.Args[1:])
	if len(argv) < 1 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := argv[0], argv[1:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "new-app":
		err = cmdNewApp(args)
	case "dev", "start":
		err = cmdServe(args, cmd == "dev")
	case "migrate":
		err = cmdMigrate(args)
	case "types":
		err = cmdTypes(args)
	case "i18n":
		err = cmdI18n(args)
	case "test":
		err = cmdTest(args)
	case "exec":
		err = cmdExec(args)
	case "eval":
		err = cmdEval(args)
	case "jobs":
		err = cmdJobs(args)
	case "webhooks":
		err = cmdWebhooks(args)
	case "audit":
		err = cmdAudit(args)
	case "user":
		err = cmdUser(args)
	case "apikey":
		err = cmdAPIKey(args)
	case "mcp":
		err = cmdMCP(args)
	case "export":
		err = cmdExport(args)
	case "demo":
		err = cmdDemo(args)
	case "docs":
		// `ddcore docs [name]` — the argument was documented but never read.
		name := "index"
		if len(args) > 0 {
			name = args[0]
		}
		fmt.Print(mcp.Docs(name))
	case "doctor":
		err = cmdDoctor(args)
	case "maintenance":
		err = cmdMaintenance(args)
	case "backup":
		err = cmdBackup(args)
	case "restore":
		err = cmdRestore(args)
	case "version", "-v", "--version":
		fmt.Printf("ddcore %s (%s/%s)\n", engine.Version, runtime.GOOS, runtime.GOARCH)
		return
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// enforceMaintenance is set by the commands that serve traffic or run jobs
// (`dev`, `start`, `jobs work`) before they load the engine. Every other
// command leaves it false, which is what makes the CLI the maintenance bypass.
var enforceMaintenance bool

// stripGlobalFlags removes the flags every command accepts, turning them into
// the environment variables load reads, so no command's FlagSet has to know
// about them.
func stripGlobalFlags(args []string) []string {
	out := args[:0:0]
	for _, a := range args {
		if a == "--allow-older-binary" || a == "-allow-older-binary" {
			os.Setenv("DDCORE_ALLOW_OLDER_BINARY", "1")
			continue
		}
		out = append(out, a)
	}
	return out
}

func allowOlderBinary() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DDCORE_ALLOW_OLDER_BINARY"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// load builds the engine from ddcore.json.
func load(test bool, dev bool) (*engine.Engine, *config.File, error) {
	cfg, cfgPath, err := config.Load(".")
	if err != nil {
		return nil, nil, err
	}
	root, err := filepath.Abs(filepath.Dir(cfgPath))
	if err != nil {
		return nil, nil, err
	}
	if cfg.DSN == "" {
		return nil, nil, fmt.Errorf("dsn not configured: run `ddcore init` or set DDCORE_DSN")
	}
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: js.AppName(dir), Dir: dir})
	}
	level := slog.LevelInfo
	if os.Getenv("DDCORE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	isDev := dev || cfg.Dev || os.Getenv("DDCORE_DEV") == "1" || os.Getenv("DDCORE_DEV") == "true"
	cfg.Mail.Dev = isDev
	e, err := engine.New(context.Background(), engine.Config{
		DSN: cfg.DSN, Apps: apps, Workers: cfg.Workers, Scheduler: cfg.Scheduler, Dev: isDev, Test: test,
		Port: cfg.Port, Lang: cfg.Lang, Currency: cfg.Currency, CurrencyPrecision: cfg.CurrencyPrecision, Rounding: cfg.RoundingMode(), Timezone: cfg.Timezone, DataDir: cfg.DataDir, Root: root, ExportMaxRows: cfg.ExportMaxRows, LogLevel: level,
		Auth: cfg.Auth, Ops: cfg.Ops, LogJSON: logJSON(), LogOut: logOut, Mail: cfg.Mail, Webhooks: cfg.Webhooks, Storage: cfg.Storage, SiteURL: cfg.PublicURL(), TrustProxy: cfg.TrustProxy, Login: cfg.Login, OIDC: cfg.OIDC,
		EnforceMaintenance: enforceMaintenance, AllowOlderBinary: allowOlderBinary(),
	})
	if err == nil && !cfg.HasPublicURL() {
		// Say it once, at boot, rather than letting someone discover it in a
		// recovery e-mail that points at a machine the reader does not have.
		e.Log.Warn("no public URL configured: recovery and invitation links will point at "+cfg.PublicURL(),
			"fix", "set DDCORE_URL in .env")
	}
	return e, cfg, err
}

func cmdInit(args []string) error {
	fs := newFlagSet("init")
	dsn := fs.String("dsn", "postgres://ddcore:ddcore@localhost:5432/ddcore?sslmode=disable", "Postgres connection")
	port := fs.Int("port", 8080, "HTTP port")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	// Idempotent: in a checkout that already contains ddcore.json, the bootstrap
	// script's `init && migrate` must work — update what was requested and notify.
	if _, err := os.Stat(config.Name); err == nil {
		changed := false
		path, err := config.Edit(".", func(cur *config.File, _ string) bool {
			fs.Visit(func(f *flag.Flag) {
				switch f.Name {
				case "dsn":
					cur.DSN, changed = *dsn, true
				case "port":
					cur.Port, changed = *port, true
				}
			})
			return changed
		})
		if err != nil {
			return err
		}
		if !changed {
			fmt.Printf("%s already exists — nothing to do (use --dsn/--port to update)\n", config.Name)
			return nil
		}
		fmt.Println("updated", path)
		return nil
	}
	f := config.Default()
	f.DSN, f.Port, f.Dev = *dsn, *port, true
	if err := f.Save(config.Name); err != nil {
		return err
	}
	if err := writeEnvExample(); err != nil {
		return err
	}
	fmt.Println("created", config.Name, "— now: ddcore new-app <name> && ddcore migrate && ddcore dev")
	return nil
}

// writeEnvExample drops the committed record of which environment variables
// exist. .env itself is gitignored, so without this nobody deploying the site
// can tell what it expects to be set.
func writeEnvExample() error {
	const name = ".env.example"
	if _, err := os.Stat(name); err == nil {
		return nil
	}
	if err := os.WriteFile(name, []byte(config.EnvExample), 0o644); err != nil {
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
	if err := scaffold.App(*dir, name, *title); err != nil {
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
	// The server's log is its output: stdout, where nothing else is written and
	// where a platform reads it as a log instead of as a stream of errors.
	logOut = os.Stdout
	fs := newFlagSet("serve")
	autoMigrate := fs.Bool("auto-migrate", dev, "aplica DDL pendente ao (re)carregar")
	port := fs.Int("port", 0, "porta")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	enforceMaintenance = true
	e, cfg, err := load(false, dev)
	if err != nil {
		return err
	}
	if *port != 0 {
		cfg.Port = *port
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *autoMigrate {
		if _, err := e.Migrate(ctx, false); err != nil {
			return err
		}
	} else if plan, err := e.Plan(ctx, false); err != nil {
		// A refusal surfaces here too: swallowing it would report "0 pending"
		// for a migration the planner will not run.
		e.Log.Warn("migrate cannot run as declared", "err", err)
	} else if len(plan) > 0 {
		e.Log.Warn("there is pending DDL — run `ddcore migrate`", "statements", len(plan))
	}
	srv := api.New(e, desk.FS())
	if dev {
		// MCP over HTTP only with Administrator/System Manager API key (B01)
		srv.MCPHandler = srv.RequireAdminAPIKey(mcp.HTTPHandler(e))
		srv.Router.Handle("/mcp", srv.MCPHandler)
		srv.Router.Handle("/mcp/*", srv.MCPHandler)
	}
	go e.WatchMaintenance(ctx)
	if st := e.Maintenance(ctx); st.Enabled {
		e.Log.Warn("site is in maintenance mode: writes and jobs are paused — `ddcore maintenance off` to resume", "reason", st.Reason, "since", st.Since)
	}
	for i := 0; i < cfg.Workers; i++ {
		go e.Worker(ctx, i)
	}
	if cfg.Scheduler {
		cr := e.StartScheduler(ctx)
		defer cr.Stop()
	} else {
		e.Log.Warn("scheduler disabled (scheduler: false in ddcore.json)")
	}
	if dev {
		go watch.Apps(ctx, e, func() {
			if err := e.Load(); err != nil {
				e.Log.Error("reload failed", "err", err)
				e.Events.Publish(engine.Event{Name: "reload_error", Payload: map[string]any{"error": err.Error()}})
				return
			}
			if *autoMigrate {
				if res, err := e.Migrate(ctx, false); err != nil {
					e.Log.Error("migrate failed", "err", err)
				} else if len(res.DDL) > 0 {
					e.Log.Info("migrate", "ddl", len(res.DDL))
				}
			}
			e.Cache.Clear()
			e.Events.Publish(engine.Event{Name: "reload", Payload: map[string]any{"loaded": e.Loaded.UnixMilli()}})
		})
	}
	h := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: srv.Router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		h.Shutdown(sctx)
	}()
	e.Log.Info("ddcore running", "url", fmt.Sprintf("http://localhost:%d", cfg.Port), "dev", dev, "apps", e.AppOrder())
	if err := h.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func cmdMigrate(args []string) error {
	fs := newFlagSet("migrate")
	dry := fs.Bool("dry-run", false, "only print the DDL")
	prune := fs.Bool("prune", false, "drop columns and tables the meta no longer has")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
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
	return cmdTypes(nil)
}

func cmdTypes(args []string) error {
	e, cfg, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	for _, dir := range cfg.Apps {
		if err := typegen.Write(dir, e.Meta); err != nil {
			return err
		}
		fmt.Println("types:", filepath.Join(dir, ".ddcore/types.d.ts"))
	}
	return nil
}

func testFlags() (*flag.FlagSet, *string, *bool, *string) {
	fs := newFlagSet("test")
	filter := fs.String("filter", "", "regex sobre nome/arquivo do teste")
	verbose := fs.Bool("v", false, "lista todos os testes")
	app := fs.String("app", "", "roda testes de um app carregado")
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
	e, _, err := load(true, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	if *app != "" {
		found := false
		for _, loaded := range e.Apps {
			if loaded.Name == *app {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("app not loaded: %s", *app)
		}
	}
	ctx := context.Background()
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
			fmt.Printf("       em %s\n", r.File)
		}
	}
	fmt.Printf("%d tests, %d failures\n", len(results), failed)
	if failed > 0 {
		os.Exit(1)
	}
	return nil
}

func cmdExec(args []string) error {
	fs := newFlagSet("exec")
	argsJSON := fs.String("args", "{}", "argumentos JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("uso: ddcore exec app.mod.fn --args '{}'")
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	var a map[string]any
	if err := json.Unmarshal([]byte(*argsJSON), &a); err != nil {
		return fmt.Errorf("--args: %w", err)
	}
	res, err := e.RunJob(context.Background(), "Administrator", fs.Arg(0), a)
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func cmdEval(args []string) error {
	fs := newFlagSet("eval")
	commit := fs.Bool("commit", false, "commit the transaction (default: rollback)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	code := strings.Join(fs.Args(), " ")
	if code == "" || code == "-" {
		b, _ := readAll(os.Stdin)
		code = b
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	res, logs, err := e.Eval(context.Background(), code, *commit)
	for _, l := range logs {
		fmt.Fprintln(os.Stderr, l)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func readAll(f *os.File) (string, error) {
	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteString("\n")
	}
	return b.String(), sc.Err()
}

// cmdDemo runs `<app>.services.demo.generate` for every app that whitelists it.
func cmdDemo(args []string) error {
	fs := newFlagSet("demo")
	app := fs.String("app", "", "only this app (default: every app that has a demo)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ran := 0
	for _, name := range e.AppOrder() {
		if *app != "" && name != *app {
			continue
		}
		// `generate` does not need to be whitelisted (runs as Administrator), so
		// app existence is checked by file
		if dir := e.AppDir(name); dir == "" {
			continue
		} else if _, err := os.Stat(filepath.Join(dir, "services", "demo.ts")); err != nil {
			continue
		}
		method := name + ".services.demo.generate"
		res, err := e.RunJob(context.Background(), "Administrator", method, nil)
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
}

func cmdUser(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ddcore user add <email> <name> [--password x] [--role R]\n" +
			"       ddcore user passwd <email> <password>\n" +
			"       ddcore user invite <email> <name> [--role R]\n" +
			"       ddcore user reset <email>\n" +
			"       ddcore user unlock <email>\n" +
			"       ddcore user sessions <email> [--revoke]")
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx := context.Background()
	switch args[0] {
	case "add":
		fs := newFlagSet("user add")
		pw := fs.String("password", "", "senha")
		var roles multi
		fs.Var(&roles, "role", "role (repeatable)")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 2 {
			return fmt.Errorf("uso: ddcore user add <email> <nome>")
		}
		return e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
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
		if len(args) < 3 {
			return fmt.Errorf("usage: ddcore user passwd <email> <password>")
		}
		if err := e.SetPassword(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("password changed; other sessions signed out")

	case "invite":
		// Creates the account with no password and sends the link. No password
		// needs no flag for "pending": CheckPassword already refuses an empty
		// hash, so an invited person simply cannot sign in until they set one.
		fs := newFlagSet("user invite")
		var roles multi
		fs.Var(&roles, "role", "role (repeatable)")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 2 {
			return fmt.Errorf("usage: ddcore user invite <email> <name> [--role R]")
		}
		email, name := fs.Arg(0), strings.Join(fs.Args()[1:], " ")
		return e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
			doc, err := c.NewDoc("User", engine.Doc{"email": email, "full_name": name, "enabled": true})
			if err != nil {
				return err
			}
			var rs []any
			for _, r := range roles {
				rs = append(rs, map[string]any{"role": r})
			}
			doc["roles"] = rs
			if _, err := c.Insert(doc, engine.SaveOpts{IgnorePermissions: true}); err != nil {
				return err
			}
			rec, err := e.StartRecovery(c, email, engine.TokenInvite, "")
			if err != nil {
				return err
			}
			reportRecovery(email, rec)
			return nil
		})

	case "reset":
		if len(args) < 2 {
			return fmt.Errorf("usage: ddcore user reset <email>")
		}
		return e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
			rec, err := e.StartRecovery(c, args[1], engine.TokenReset, "")
			if err != nil {
				return err
			}
			reportRecovery(args[1], rec)
			return nil
		})

	case "unlock":
		if len(args) < 2 {
			return fmt.Errorf("usage: ddcore user unlock <email>")
		}
		// The counter is keyed on what was typed, not on the account it
		// resolved to — that is what stops the lockout from answering "does
		// this address exist" — so clear both spellings anyone would have used.
		n, err := e.ClearAttempts(ctx, "login:"+strings.ToLower(args[1]))
		if err != nil {
			return err
		}
		if email, _ := e.FindUserForRecovery(ctx, args[1]); email != "" && !strings.EqualFold(email, args[1]) {
			m, _ := e.ClearAttempts(ctx, "login:"+strings.ToLower(email))
			n += m
		}
		fmt.Printf("cleared %d failed attempt(s) for %s\n", n, args[1])

	case "sessions":
		fs := newFlagSet("user sessions")
		revoke := fs.Bool("revoke", false, "end every session of this user")
		if err := parseFlags(fs, args[1:]); err != nil {
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

	default:
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
	return nil
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
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ddcore apikey <user> [--label x] [--days N]")
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx := context.Background()
	var out map[string]any
	if err := e.Run(ctx, "Administrator", func(c *engine.Ctx) error {
		var err error
		out, err = e.CreateAPIKeyFor(c, fs.Arg(0), *label, *days)
		return err
	}); err != nil {
		return err
	}
	fmt.Println(out["token"])
	if out["expires"] != nil {
		fmt.Fprintf(os.Stderr, "expires: %v\n", out["expires"])
	}
	return nil
}

func cmdMCP(args []string) error {
	e, _, err := load(false, true)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go watch.Apps(ctx, e, func() {
		if err := e.Load(); err != nil {
			e.Log.Error("reload failed", "err", err)
		}
	})
	return mcp.ServeStdio(ctx, e)
}

// logOut is where the log goes. A one-shot command keeps it on stderr, because
// its stdout is its output — a log line in the middle of an exported NDJSON, a
// printed key, a doctor report or the MCP JSON-RPC stream is corruption, not a
// log. The server flips it to stdout, because there the log *is* the output and
// a platform that captures both streams reads everything on stderr as an
// error: on stderr an INFO access line arrives painted red and the level
// policy stops meaning anything.
var logOut io.Writer = os.Stderr

// logJSON reads the log shape from the environment and not from ddcore.json:
// it is a property of where the process runs — a terminal wants text, a
// platform that ships the log to a collector wants objects it can index.
// DDCORE_LOG_FORMAT names it outright (`json` or `text`); with nothing set the
// process asks the log's own destination, since a character device is somebody
// watching and anything else — a pipe, a file, a container's log stream — is a
// collector that reads the level out of the object instead of guessing it from
// the stream.
func logJSON() bool {
	switch {
	case strings.EqualFold(os.Getenv("DDCORE_LOG_FORMAT"), "json"):
		return true
	case strings.EqualFold(os.Getenv("DDCORE_LOG_FORMAT"), "text"):
		return false
	}
	f, ok := logOut.(*os.File)
	if !ok {
		return true
	}
	// The file mode already answers "is this a terminal", so golang.org/x/term
	// would be a dependency taken on for one bit that is already here.
	fi, err := f.Stat()
	return err != nil || fi.Mode()&os.ModeCharDevice == 0
}
