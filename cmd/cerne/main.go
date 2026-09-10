// cerne — the CLI: dev server, migrations, tests, types, jobs, users, MCP.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jrvidotti/cerne/desk"
	"github.com/jrvidotti/cerne/internal/api"
	"github.com/jrvidotti/cerne/internal/config"
	"github.com/jrvidotti/cerne/internal/engine"
	"github.com/jrvidotti/cerne/internal/js"
	"github.com/jrvidotti/cerne/internal/mcp"
	"github.com/jrvidotti/cerne/internal/scaffold"
	"github.com/jrvidotti/cerne/internal/typegen"
	"github.com/jrvidotti/cerne/internal/watch"
)

const usage = `cerne — framework de aplicações (DocTypes em TypeScript, core em Go, PostgreSQL)

Uso: cerne <comando> [opções]

  init        cria cerne.json no diretório atual
  new-app     cria um app: cerne new-app <nome> [--dir apps/<nome>]
  dev         servidor de desenvolvimento com hot-reload (porta do cerne.json)
  start       servidor de produção
  migrate     aplica DDL, instala apps, roda patches (--dry-run, --prune)
  types       gera .cerne/types.d.ts em cada app
  test        roda os *.test.ts (--filter regex, --app nome)
  exec        executa uma função: cerne exec app.services.mod.fn --args '{"a":1}'
  eval        executa TS avulso: cerne eval 'cerne.db.count("User")' [--commit]
  demo        popula dados de exemplo (<app>.services.demo.gerar, idempotente)
  jobs        jobs list | jobs run <fn> | jobs work
  user        user add <email> <nome> [--password x] [--role R]... | user passwd <email>
  apikey      apikey <usuario> [--label x]  → imprime key:secret
  mcp         servidor MCP (stdio) para agentes
  docs        imprime a documentação do framework
  doctor      verifica banco, meta e scheduler

Variáveis: CERNE_DSN sobrescreve o dsn do cerne.json.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
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
	case "test":
		err = cmdTest(args)
	case "exec":
		err = cmdExec(args)
	case "eval":
		err = cmdEval(args)
	case "jobs":
		err = cmdJobs(args)
	case "user":
		err = cmdUser(args)
	case "apikey":
		err = cmdAPIKey(args)
	case "mcp":
		err = cmdMCP(args)
	case "demo":
		err = cmdDemo(args)
	case "docs":
		fmt.Print(mcp.Docs("index"))
	case "doctor":
		err = cmdDoctor(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

// load builds the engine from cerne.json.
func load(test bool, dev bool) (*engine.Engine, *config.File, error) {
	cfg, _, err := config.Load(".")
	if err != nil {
		return nil, nil, err
	}
	if cfg.DSN == "" {
		return nil, nil, fmt.Errorf("dsn não configurado: rode `cerne init` ou defina CERNE_DSN")
	}
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: filepath.Base(dir), Dir: dir})
	}
	level := slog.LevelInfo
	if os.Getenv("CERNE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	e, err := engine.New(context.Background(), engine.Config{
		DSN: cfg.DSN, Apps: apps, Workers: cfg.Workers, Scheduler: cfg.Scheduler, Dev: dev || cfg.Dev, Test: test,
		Port: cfg.Port, SiteName: cfg.Site, Lang: cfg.Lang, Currency: cfg.Currency, DataDir: cfg.DataDir, LogLevel: level,
	})
	return e, cfg, err
}

func cmdInit(args []string) error {
	fs := newFlagSet("init")
	dsn := fs.String("dsn", "postgres://cerne:cerne@localhost:5432/cerne?sslmode=disable", "conexão Postgres")
	port := fs.Int("port", 8080, "porta HTTP")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	// idempotente: num checkout que já traz cerne.json, `init && migrate` do
	// roteiro de bootstrap precisa funcionar — atualiza o que foi pedido e avisa.
	if _, err := os.Stat(config.Name); err == nil {
		cur, path, err := config.Load(".")
		if err != nil {
			return fmt.Errorf("%s existe mas não pôde ser lido: %w", config.Name, err)
		}
		changed := false
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "dsn":
				cur.DSN, changed = *dsn, true
			case "port":
				cur.Port, changed = *port, true
			}
		})
		if !changed {
			fmt.Printf("%s já existe — nada a fazer (use --dsn/--port para atualizar)\n", config.Name)
			return nil
		}
		if err := cur.Save(path); err != nil {
			return err
		}
		fmt.Println("atualizado", path)
		return nil
	}
	f := &config.File{DSN: *dsn, Port: *port, Workers: 2, Scheduler: false, Site: "cerne", Lang: "pt-BR", Currency: "BRL", Apps: []string{}, Dev: true}
	if err := f.Save(config.Name); err != nil {
		return err
	}
	fmt.Println("criado", config.Name, "— agora: cerne new-app <nome> && cerne migrate && cerne dev")
	return nil
}

func cmdNewApp(args []string) error {
	fs := newFlagSet("new-app")
	dir := fs.String("dir", "", "diretório (padrão apps/<nome>)")
	title := fs.String("title", "", "título")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("uso: cerne new-app <nome>")
	}
	name := fs.Arg(0)
	if *dir == "" {
		*dir = filepath.Join("apps", name)
	}
	if err := scaffold.App(*dir, name, *title); err != nil {
		return err
	}
	cfg, path, err := config.Load(".")
	if err == nil {
		abs, _ := filepath.Abs(*dir)
		rel, _ := filepath.Rel(filepath.Dir(path), abs)
		cfg.Apps = append(cfg.Apps, rel)
		// keep paths relative in the file
		for i, a := range cfg.Apps {
			if r, err := filepath.Rel(filepath.Dir(path), a); err == nil && !strings.HasPrefix(r, "..") {
				cfg.Apps[i] = r
			}
		}
		if err := cfg.Save(path); err != nil {
			return err
		}
	}
	fmt.Printf("app %s criado em %s\n", name, *dir)
	return nil
}

func cmdServe(args []string, dev bool) error {
	fs := newFlagSet("serve")
	autoMigrate := fs.Bool("auto-migrate", dev, "aplica DDL pendente ao (re)carregar")
	port := fs.Int("port", 0, "porta")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
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
	} else if plan, _ := e.Plan(ctx, false); len(plan) > 0 {
		e.Log.Warn("há DDL pendente — rode `cerne migrate`", "statements", len(plan))
	}
	srv := api.New(e, desk.FS())
	if dev {
		// MCP por HTTP só com chave de API de Administrator/System Manager (B01)
		srv.MCPHandler = srv.RequireAdminAPIKey(mcp.HTTPHandler(e))
		srv.Router.Handle("/mcp", srv.MCPHandler)
		srv.Router.Handle("/mcp/*", srv.MCPHandler)
	}
	for i := 0; i < cfg.Workers; i++ {
		go e.Worker(ctx, i)
	}
	if cfg.Scheduler {
		cr := e.StartScheduler(ctx)
		defer cr.Stop()
	} else {
		e.Log.Warn("scheduler desabilitado (scheduler: false no cerne.json)")
	}
	if dev {
		go watch.Apps(ctx, e, func() {
			if err := e.Load(); err != nil {
				e.Log.Error("reload falhou", "err", err)
				e.Events.Publish(engine.Event{Name: "reload_error", Payload: map[string]any{"error": err.Error()}})
				return
			}
			if *autoMigrate {
				if res, err := e.Migrate(ctx, false); err != nil {
					e.Log.Error("migrate falhou", "err", err)
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
	e.Log.Info("cerne no ar", "url", fmt.Sprintf("http://localhost:%d", cfg.Port), "dev", dev, "apps", e.AppOrder())
	if err := h.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func cmdMigrate(args []string) error {
	fs := newFlagSet("migrate")
	dry := fs.Bool("dry-run", false, "só imprime o DDL")
	prune := fs.Bool("prune", false, "dropa colunas/tabelas que não estão mais na meta")
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
		if len(plan) == 0 {
			fmt.Println("-- nada a fazer")
		}
		for _, s := range plan {
			fmt.Println(s)
		}
		return nil
	}
	res, err := e.Migrate(ctx, *prune)
	if err != nil {
		return err
	}
	for _, s := range res.DDL {
		fmt.Println(s)
	}
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
		fmt.Println("types:", filepath.Join(dir, ".cerne/types.d.ts"))
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
			return fmt.Errorf("app não carregado: %s", *app)
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
	fmt.Printf("%d testes, %d falhas\n", len(results), failed)
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
		return fmt.Errorf("uso: cerne exec app.mod.fn --args '{}'")
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
	commit := fs.Bool("commit", false, "grava a transação (padrão: rollback)")
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

// cmdDemo runs `<app>.services.demo.gerar` for every app that whitelists it.
func cmdDemo(args []string) error {
	fs := newFlagSet("demo")
	app := fs.String("app", "", "só este app (padrão: todos que tiverem demo)")
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
		// `gerar` não precisa ser whitelisted (roda como Administrator), então
		// a existência do app se verifica pelo arquivo
		if dir := e.AppDir(name); dir == "" {
			continue
		} else if _, err := os.Stat(filepath.Join(dir, "services", "demo.ts")); err != nil {
			continue
		}
		method := name + ".services.demo.gerar"
		res, err := e.RunJob(context.Background(), "Administrator", method, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		fmt.Printf("%s: %s\n", method, strings.TrimSpace(string(res)))
		ran++
	}
	if ran == 0 {
		return fmt.Errorf("nenhum app tem `services/demo.ts`")
	}
	return nil
}

func cmdJobs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: cerne jobs list|run <fn>|work")
	}
	e, cfg, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx := context.Background()
	switch args[0] {
	case "list":
		for _, s := range e.ScheduledMethods() {
			fmt.Println(s)
		}
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("uso: cerne jobs run <fn>")
		}
		res, err := e.RunJob(ctx, "Administrator", args[1], nil)
		if err != nil {
			return err
		}
		fmt.Println(string(res))
	case "work":
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		for i := 0; i < cfg.Workers; i++ {
			go e.Worker(ctx, i)
		}
		if cfg.Scheduler {
			e.StartScheduler(ctx)
		}
		<-ctx.Done()
	default:
		return fmt.Errorf("subcomando desconhecido: %s", args[0])
	}
	return nil
}

func cmdUser(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: cerne user add <email> <nome> [--password x] [--role R] | user passwd <email> <senha>")
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
		fs.Var(&roles, "role", "papel (repetível)")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 2 {
			return fmt.Errorf("uso: cerne user add <email> <nome>")
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
				fmt.Println("usuário criado:", fs.Arg(0))
			}
			return err
		})
	case "passwd":
		if len(args) < 3 {
			return fmt.Errorf("uso: cerne user passwd <email> <senha>")
		}
		if err := e.SetPassword(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("senha alterada")
	default:
		return fmt.Errorf("subcomando desconhecido: %s", args[0])
	}
	return nil
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

func cmdAPIKey(args []string) error {
	fs := newFlagSet("apikey")
	label := fs.String("label", "cli", "descrição")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("uso: cerne apikey <usuario>")
	}
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	tok, err := e.CreateAPIKey(context.Background(), fs.Arg(0), *label)
	if err != nil {
		return err
	}
	fmt.Println(tok)
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
			e.Log.Error("reload falhou", "err", err)
		}
	})
	return mcp.ServeStdio(ctx, e)
}

func cmdDoctor(args []string) error {
	e, cfg, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx := context.Background()
	fmt.Println("banco:      ok")
	fmt.Printf("apps:       %v\n", e.AppOrder())
	fmt.Printf("doctypes:   %d\n", len(e.Meta.DocTypes))
	plan, err := e.Plan(ctx, false)
	if err != nil {
		return err
	}
	fmt.Printf("migrate:    %d statements pendentes\n", len(plan))
	fmt.Printf("scheduler:  %v\n", cfg.Scheduler)
	fmt.Printf("workers:    %d\n", cfg.Workers)
	return nil
}
