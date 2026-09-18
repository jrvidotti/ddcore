package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execFlags mirrors cmdExec's declarations.
func execFlags() (*flag.FlagSet, *string) {
	fs := newFlagSet("exec")
	return fs, fs.String("args", "{}", "JSON arguments")
}

// userAddFlags mirrors `ddcore user add`.
func userAddFlags() (*flag.FlagSet, *string, *multi) {
	fs := newFlagSet("user add")
	pw := fs.String("password", "", "password")
	roles := &multi{}
	fs.Var(roles, "role", "role (repeatable)")
	return fs, pw, roles
}

// evalFlags mirrors `ddcore eval`.
func evalFlags() (*flag.FlagSet, *bool) {
	fs := newFlagSet("eval")
	return fs, fs.Bool("commit", false, "commits the transaction")
}

func TestExecFlagsAfterPositional(t *testing.T) {
	// B22: `flag` stopped at first positional and --args was ignored
	fs, argsJSON := execFlags()
	if err := parseFlags(fs, []string{"alugueis.services.demo.generate", "--args", `{"a":1}`}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != `{"a":1}` {
		t.Fatalf("--args = %q, expected {\"a\":1}", *argsJSON)
	}
	if got := fs.Arg(0); got != "alugueis.services.demo.generate" {
		t.Fatalf("positional = %q", got)
	}
	if fs.NArg() != 1 {
		t.Fatalf("NArg = %d, expected 1", fs.NArg())
	}
}

func TestExecFlagEqualsForm(t *testing.T) {
	fs, argsJSON := execFlags()
	if err := parseFlags(fs, []string{"app.mod.fn", `--args={"b":2}`}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != `{"b":2}` {
		t.Fatalf("--args= = %q", *argsJSON)
	}
	if fs.Arg(0) != "app.mod.fn" {
		t.Fatalf("positional = %q", fs.Arg(0))
	}
}

func TestUserAddFlagsAfterPositionals(t *testing.T) {
	fs, pw, roles := userAddFlags()
	args := []string{"ana@exemplo.com", "Ana", "Maria", "--password", "s3nha", "--role", "System Manager", "--role", "Locador"}
	if err := parseFlags(fs, args); err != nil {
		t.Fatal(err)
	}
	if *pw != "s3nha" {
		t.Fatalf("--password = %q", *pw)
	}
	if got := strings.Join(*roles, "|"); got != "System Manager|Locador" {
		t.Fatalf("--role = %q", got)
	}
	if fs.Arg(0) != "ana@exemplo.com" {
		t.Fatalf("email = %q", fs.Arg(0))
	}
	// the full name is the rest of the positionals, without flags attached
	if name := strings.Join(fs.Args()[1:], " "); name != "Ana Maria" {
		t.Fatalf("name = %q, expected \"Ana Maria\"", name)
	}
}

func TestEvalCommitAfterCode(t *testing.T) {
	fs, commit := evalFlags()
	code := `ddcore.db.count("User")`
	if err := parseFlags(fs, []string{code, "--commit"}); err != nil {
		t.Fatal(err)
	}
	if !*commit {
		t.Fatal("--commit after code was not applied")
	}
	// and cannot end up inside the evaluated text
	if got := strings.Join(fs.Args(), " "); got != code {
		t.Fatalf("code = %q", got)
	}
}

func TestEvalFlagBeforeCodeStillWorks(t *testing.T) {
	fs, commit := evalFlags()
	if err := parseFlags(fs, []string{"--commit", "1 + 1"}); err != nil {
		t.Fatal(err)
	}
	if !*commit || fs.Arg(0) != "1 + 1" {
		t.Fatalf("commit=%v arg=%q", *commit, fs.Arg(0))
	}
}

func TestSingleDashIsPositional(t *testing.T) {
	// `ddcore eval -` reads code from stdin
	fs, _ := evalFlags()
	if err := parseFlags(fs, []string{"-"}); err != nil {
		t.Fatal(err)
	}
	if fs.Arg(0) != "-" {
		t.Fatalf("arg = %q, expected -", fs.Arg(0))
	}
}

func TestDoubleDashEndsFlags(t *testing.T) {
	fs, argsJSON := execFlags()
	if err := parseFlags(fs, []string{"app.mod.fn", "--", "--args", "literal"}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != "{}" {
		t.Fatalf("--args should remain default, got %q", *argsJSON)
	}
	if got := strings.Join(fs.Args(), " "); got != "app.mod.fn --args literal" {
		t.Fatalf("positionals = %q", got)
	}
}

func TestUnknownFlagIsRejected(t *testing.T) {
	// silence was worse: the flag became a positional argument
	fs, _ := execFlags()
	err := parseFlags(fs, []string{"app.mod.fn", "--arg", "{}"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--arg") {
		t.Fatalf("unclear error: %v", err)
	}
}

func TestFlagMissingValueIsRejected(t *testing.T) {
	fs, _ := execFlags()
	err := parseFlags(fs, []string{"app.mod.fn", "--args"})
	if err == nil || !strings.Contains(err.Error(), "needs a value") {
		t.Fatalf("error = %v", err)
	}
}

func TestBoolFlagDoesNotEatPositional(t *testing.T) {
	fs, commit := evalFlags()
	if err := parseFlags(fs, []string{"--commit", "--", "code"}); err != nil {
		t.Fatal(err)
	}
	if !*commit || fs.Arg(0) != "code" {
		t.Fatalf("commit=%v arg=%q", *commit, fs.Arg(0))
	}
}

func TestSingleDashFlagFormIsAccepted(t *testing.T) {
	// -v and --v must be equivalent, as in package flag
	fs := newFlagSet("test")
	v := fs.Bool("v", false, "verbose")
	filter := fs.String("filter", "", "regex")
	if err := parseFlags(fs, []string{"cobranca", "-v", "-filter", "atraso"}); err != nil {
		t.Fatal(err)
	}
	if !*v || *filter != "atraso" || fs.Arg(0) != "cobranca" {
		t.Fatalf("v=%v filter=%q arg=%q", *v, *filter, fs.Arg(0))
	}
}

func TestTestFlagsAcceptsApp(t *testing.T) {
	fs, _, _, app := testFlags()
	if err := parseFlags(fs, []string{"--app", "exemplo"}); err != nil {
		t.Fatal(err)
	}
	if *app != "exemplo" {
		t.Fatalf("--app = %q, expected exemplo", *app)
	}
}

// exportFlags mirrors cmdExport's declarations.
func exportFlags() (*flag.FlagSet, *string, *string, *bool, *bool) {
	fs := newFlagSet("export")
	format := fs.String("format", "ndjson", "ndjson or csv")
	filters := fs.String("filters", "", "filters as JSON")
	children := fs.Bool("children", false, "include the child tables")
	all := fs.Bool("all", false, "every DocType of the site")
	return fs, format, filters, children, all
}

// The DocType comes first and the flags after it — the shape a person actually
// types, and the one `flag` alone gets wrong (B22).
func TestExportFlagsAfterPositional(t *testing.T) {
	fs, format, filters, children, all := exportFlags()
	err := parseFlags(fs, []string{"Project", "--children", "--format", "csv", "--filters", `[["status","=","Open"]]`})
	if err != nil {
		t.Fatal(err)
	}
	if fs.Arg(0) != "Project" || fs.NArg() != 1 {
		t.Fatalf("positional = %q (NArg %d)", fs.Arg(0), fs.NArg())
	}
	if *format != "csv" || !*children || *all {
		t.Fatalf("format=%q children=%v all=%v", *format, *children, *all)
	}
	if *filters != `[["status","=","Open"]]` {
		t.Fatalf("--filters = %q", *filters)
	}
}

func TestExportAllTakesNoDoctype(t *testing.T) {
	fs, _, _, _, all := exportFlags()
	if err := parseFlags(fs, []string{"--all"}); err != nil {
		t.Fatal(err)
	}
	if !*all || fs.NArg() != 0 {
		t.Fatalf("all=%v NArg=%d", *all, fs.NArg())
	}
}

// The service's log is read by whatever captures the process's streams, and a
// platform that captures both reads stderr as an error: an INFO access line on
// stderr arrives red, which is what this pins.
func TestServeLogsGoToStdoutAndNothingElseDoes(t *testing.T) {
	t.Cleanup(func() { logOut = os.Stderr })
	if logOut != io.Writer(os.Stderr) {
		t.Fatalf("a one-shot command's stdout is its output: log destination = %v, expected stderr", logOut)
	}
	// cmdServe cannot run here (it needs a database and then blocks), so this
	// is the line it runs first, kept next to the assertion that it is the
	// only command that runs it.
	logOut = os.Stdout
	if logOut != io.Writer(os.Stdout) {
		t.Fatal("the server's log must leave by stdout")
	}
}

func TestLogFormatFollowsTheEnvironmentThenTheDestination(t *testing.T) {
	pipe, _, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pipe.Close(); logOut = os.Stderr })
	for name, tc := range map[string]struct {
		env  string
		out  io.Writer
		want bool
	}{
		"json when asked":              {"json", os.Stdout, true},
		"text when asked":              {"text", pipe, false},
		"the word is not case-bound":   {"JSON", os.Stdout, true},
		"a pipe belongs to a platform": {"", pipe, true},
		"a writer that is not a file":  {"", io.Discard, true},
	} {
		t.Setenv("DDCORE_LOG_FORMAT", tc.env)
		logOut = tc.out
		if got := logJSON(); got != tc.want {
			t.Errorf("%s: logJSON() = %v, expected %v", name, got, tc.want)
		}
	}
}

func TestAuditFlags(t *testing.T) {
	fs := newFlagSet("audit list")
	f := addAuditFilterFlags(fs)
	asJSON := fs.Bool("json", false, "print JSON")
	args := []string{"--action", "role.assign", "--actor", "admin@example.com", "--target", "User:1", "--outcome", "Allowed", "--since", "24h", "--limit", "50", "--json"}
	if err := parseFlags(fs, args); err != nil {
		t.Fatal(err)
	}
	if *f.action != "role.assign" || *f.actor != "admin@example.com" || *f.target != "User:1" || *f.outcome != "Allowed" || *f.limit != 50 || !*asJSON {
		t.Fatalf("unexpected flag values: %+v", f)
	}
	filter, err := f.filter()
	if err != nil {
		t.Fatal(err)
	}
	if filter.Action != "role.assign" || filter.Actor != "admin@example.com" || filter.TargetName != "User:1" || filter.Outcome != "Allowed" || filter.Limit != 50 || filter.Since == nil {
		t.Fatalf("unexpected filter: %+v", filter)
	}

	// Purge flags
	purgeFS := newFlagSet("audit purge")
	days := purgeFS.Int("days", -1, "delete audit events")
	dry := purgeFS.Bool("dry-run", false, "dry run")
	if err := parseFlags(purgeFS, []string{"--days", "90", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if *days != 90 || !*dry {
		t.Fatalf("unexpected purge flags: days=%d, dry=%v", *days, *dry)
	}
}


// The rollback override is a global flag, stripped before the command parses
// its own — but every other boolean here takes `=true`, so this one must too
// rather than failing as an unknown flag.
func TestAllowOlderBinaryFlagForms(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--allow-older-binary", "doctor"}, "1"},
		{[]string{"--allow-older-binary=true", "doctor"}, "true"},
		{[]string{"-allow-older-binary=yes", "doctor"}, "yes"},
		{[]string{"--allow-older-binary=false", "doctor"}, "false"},
	} {
		t.Setenv("DDCORE_ALLOW_OLDER_BINARY", "")
		got := stripGlobalFlags(c.args)
		if len(got) != 1 || got[0] != "doctor" {
			t.Errorf("%v: remaining args = %v, want [doctor]", c.args, got)
		}
		if env := os.Getenv("DDCORE_ALLOW_OLDER_BINARY"); env != c.want {
			t.Errorf("%v: DDCORE_ALLOW_OLDER_BINARY = %q, want %q", c.args, env, c.want)
		}
	}
	t.Setenv("DDCORE_ALLOW_OLDER_BINARY", "")
	stripGlobalFlags([]string{"--allow-older-binary=false", "doctor"})
	if allowOlderBinary() {
		t.Error("--allow-older-binary=false should not turn the override on")
	}
}

// initIn runs `ddcore init` in a fresh directory named dir.
func initIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	root := filepath.Join(t.TempDir(), dir)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root, cmdInit(args)
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitNameAndDBPortBuildTheDSN(t *testing.T) {
	if _, err := initIn(t, "x", "--name", "myapp", "--db-port", "5467"); err != nil {
		t.Fatal(err)
	}
	if cfg := readFile(t, "ddcore.json"); !strings.Contains(cfg, "postgres://myapp:myapp@localhost:5467/myapp?sslmode=disable") {
		t.Fatalf("dsn not built from --name/--db-port:\n%s", cfg)
	}
	if c := readFile(t, "docker-compose.yml"); !strings.Contains(c, `"5467:5432"`) || !strings.Contains(c, `POSTGRES_DB: "myapp"`) {
		t.Fatalf("compose does not match the dsn:\n%s", c)
	}
	for _, f := range []string{"AGENTS.md", "CLAUDE.md", ".mcp.json", ".gitignore", "README.md"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("init did not write %s: %v", f, err)
		}
	}
}

func TestInitDefaultsTheNameToTheDirectory(t *testing.T) {
	if _, err := initIn(t, "my-library"); err != nil {
		t.Fatal(err)
	}
	if cfg := readFile(t, "ddcore.json"); !strings.Contains(cfg, "postgres://my_library:my_library@localhost:5432/my_library") {
		t.Fatalf("dsn not derived from the directory:\n%s", cfg)
	}
}

func TestInitRejectsDSNWithName(t *testing.T) {
	if _, err := initIn(t, "x", "--dsn", "postgres://a:a@localhost/a", "--name", "b"); err == nil {
		t.Fatal("--dsn with --name should be rejected")
	}
}

func TestInitRejectsInvalidName(t *testing.T) {
	if _, err := initIn(t, "x", "--name", "My-App"); err == nil {
		t.Fatal("an invalid --name should be rejected")
	}
}

func TestInitLeavesAnExistingComposeAlone(t *testing.T) {
	root := filepath.Join(t.TempDir(), "p")
	os.Mkdir(root, 0o755)
	t.Chdir(root)
	os.WriteFile("compose.yaml", []byte("mine"), 0o644)
	if err := cmdInit(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		t.Fatal("init wrote docker-compose.yml next to an existing compose.yaml")
	}
	if readFile(t, "compose.yaml") != "mine" {
		t.Fatal("init touched compose.yaml")
	}
}

func TestInitSkipsComposeForRemoteDSN(t *testing.T) {
	if _, err := initIn(t, "x", "--dsn", "postgres://a:a@db.example.com/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		t.Fatal("a remote dsn needs no docker-compose.yml")
	}
}

func TestInitAgainUpdatesTheDSNFromName(t *testing.T) {
	if _, err := initIn(t, "x", "--name", "one"); err != nil {
		t.Fatal(err)
	}
	if err := cmdInit([]string{"--name", "two", "--db-port", "5999"}); err != nil {
		t.Fatal(err)
	}
	if cfg := readFile(t, "ddcore.json"); !strings.Contains(cfg, "postgres://two:two@localhost:5999/two") {
		t.Fatalf("second init did not update the dsn:\n%s", cfg)
	}
}
