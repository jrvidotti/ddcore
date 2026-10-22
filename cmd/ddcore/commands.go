package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/mcp"
)

// command is one entry of the table that `ddcore` dispatches on and lists in
// its usage text: adding a command is adding a line here.
type command struct {
	name string
	// summary is the command's line in the usage text. Empty keeps the command
	// out of it, which is what an alias is.
	summary string
	// usage is the usage text of a command whose first argument is a
	// subcommand: `ddcore <name> -h` prints it. A command that takes flags
	// directly prints its flags from parseFlags instead, and leaves this empty.
	usage string
	run   func(args []string) error
}

// commands is the table, in the order the usage text lists it. It is a
// function because the help command prints the table it is part of.
func commands() []command {
	return []command{
		{"init", "create ddcore.json in the current directory", "", cmdInit},
		{"new-app", "create an app: ddcore new-app <name> [--dir apps/<name>]", "", cmdNewApp},
		{"dev", "development server with hot reload (port from ddcore.json)", "", func(args []string) error { return cmdServe(args, true) }},
		{"start", "production server (migrates first; DDCORE_AUTO_MIGRATE=0 turns that off)", "", func(args []string) error { return cmdServe(args, false) }},
		{"deploy", "write the files a deployment builds from: deploy docker | deploy railway", deployUsage, cmdDeploy},
		{"migrate", "apply DDL, install apps, run patches (--dry-run, --prune)", "", cmdMigrate},
		{"types", "generate .ddcore/types.d.ts in every app", "", cmdTypes},
		{"i18n", "i18n extract — rewrite translations/<lang>.csv from the code", i18nUsage, cmdI18n},
		{"test", "run the *.test.ts (--filter regex, --app name)", "", cmdTest},
		{"exec", "run a function: ddcore exec app.services.mod.fn --args '{\"a\":1}'\n" +
			"              (--args - reads the JSON from stdin, --args-file f.json from a file)", "", cmdExec},
		{"eval", "run loose TS: ddcore eval 'ddcore.db.count(\"User\")' [--commit]", "", cmdEval},
		{"demo", "seed example data (<app>.services.demo.generate, idempotent)", "", cmdDemo},
		{"export", "export a DocType (or --all) to NDJSON/CSV with a manifest", "", cmdExport},
		{"import", "load an export directory into this site (run: ddcore import)", importUsage, cmdImport},
		{"jobs", "inspect, retry, cancel and purge the queue (run: ddcore jobs)", jobsUsage, cmdJobs},
		{"webhooks", "list and replay outgoing webhook deliveries (run: ddcore webhooks)", webhooksUsage, cmdWebhooks},
		{"push", "push keys — print a new VAPID key pair for Web Push (run: ddcore push)", pushUsage, cmdPush},
		{"audit", "inspect and purge administrative audit events (run: ddcore audit)", auditUsage, cmdAudit},
		{"user", "user add|invite|passwd|reset|unlock|sessions (run: ddcore user)", userUsage, cmdUser},
		{"apikey", "apikey <user> [--label x] [--days N] [--prefix p]  → prints key:secret", "", cmdAPIKey},
		{"vault", "vault rekey — re-encrypt the vault under DDCORE_SECRET_KEY (run: ddcore vault)", vaultUsage, cmdVault},
		{"tenant", "tenant list|create|enable|disable|adopt — the tenants of a site with tenancy (run: ddcore tenant)", tenantUsage, cmdTenant},
		{"mcp", "MCP server (stdio) for agents", "", cmdMCP},
		{"docs", "print the framework documentation", "", cmdDocs},
		{"doctor", "database readiness, meta, queue and errors (--json, --strict)", "", cmdDoctor},
		{"maintenance", "maintenance on [--reason x] | off | status — pause writes and jobs", maintenanceUsage, cmdMaintenance},
		{"backup", "write a .tar of the database, files, config and versions (--to s3)", "", cmdBackup},
		{"restore", "restore <archive> into this site's database and storage (--smoke)", "", cmdRestore},
		{"version", "print the framework version", "", cmdVersion},
		{"-v", "", "", cmdVersion},
		{"--version", "", "", cmdVersion},
		{"help", "", "", cmdHelp},
		{"-h", "", "", cmdHelp},
		{"--help", "", "", cmdHelp},
	}
}

func findCommand(name string) (command, bool) {
	for _, c := range commands() {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// usageText is what `ddcore` with no command, or with help, prints: the table's
// summaries between the fixed head and tail.
func usageText() string {
	var b strings.Builder
	b.WriteString(usageHead)
	for _, c := range commands() {
		if c.summary != "" {
			fmt.Fprintf(&b, "  %-11s %s\n", c.name, c.summary)
		}
	}
	b.WriteString(usageTail)
	return b.String()
}

const usageHead = `ddcore — an application framework (DocTypes in TypeScript, core in Go, PostgreSQL)

Usage: ddcore <command> [options]

`

const usageTail = `
Variables: DDCORE_DSN overrides the dsn in ddcore.json.
Global flag: --tenant <slug>, before the command, runs a one-shot command inside
a tenant of a site with tenancy (ddcore --tenant acme user add …).
Global flag: --allow-older-binary (DDCORE_ALLOW_OLDER_BINARY=1) opens a database
a newer release migrated — a rollback; see ` + "`ddcore docs backup`" + `.
`

func cmdHelp([]string) error {
	fmt.Print(usageText())
	return nil
}

func cmdVersion([]string) error {
	fmt.Printf("ddcore %s (%s/%s)\n", engine.Version, runtime.GOOS, runtime.GOARCH)
	return nil
}

// cmdDocs prints `ddcore docs [name]`.
func cmdDocs(args []string) error {
	name := "index"
	if len(args) > 0 {
		name = args[0]
	}
	fmt.Print(mcp.Docs(name))
	return nil
}

// exitCode is the error a command returns to end the process with that status
// without main printing "error: …" over what the command already reported: the
// failures of `test` and `doctor`, whose output is the report.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

// run executes the command argv names. The error it returns is main's to turn
// into a status: errHelpShown is a success, an exitCode carries its own status,
// anything else is printed.
func run(argv []string) error {
	if len(argv) < 1 {
		fmt.Print(usageText())
		return exitCode(2)
	}
	name, args := argv[0], argv[1:]
	c, ok := findCommand(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", name, usageText())
		return exitCode(2)
	}
	// a command that dispatches on a subcommand prints its usage for -h
	if c.usage != "" && len(args) > 0 && isHelp(args[0]) {
		fmt.Print(strings.TrimRight(c.usage, "\n") + "\n")
		return nil
	}
	return c.run(args)
}

// exitStatus is the process status for what run returned, and the message to
// print on stderr before exiting, if any.
func exitStatus(err error) (status int, message string) {
	var code exitCode
	switch {
	case err == nil, errors.Is(err, errHelpShown):
		return 0, ""
	case errors.As(err, &code):
		return int(code), ""
	}
	return 1, "error: " + err.Error()
}

// subcommand is one verb of a command that dispatches on its first argument.
type subcommand struct {
	name string
	run  func(args []string) error
}

// dispatch runs the subcommand args names with the arguments after it. With no
// subcommand it fails with the usage, and with an unknown one it says so above
// the usage.
func dispatch(usage string, subs []subcommand, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", usage)
	}
	for _, s := range subs {
		if s.name == args[0] {
			return s.run(args[1:])
		}
	}
	return fmt.Errorf("unknown subcommand: %s\n\n%s", args[0], usage)
}
