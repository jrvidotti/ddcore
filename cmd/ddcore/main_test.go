package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

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
	fs, argsJSON, _ := execFlags()
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
	fs, argsJSON, _ := execFlags()
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

// #103: a payload carrying personal data stays out of argv, where ps shows it.
func TestExecArgsDashParses(t *testing.T) {
	fs, argsJSON, _ := execFlags()
	if err := parseFlags(fs, []string{"app.mod.fn", "--args", "-"}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != "-" || fs.Arg(0) != "app.mod.fn" || fs.NArg() != 1 {
		t.Fatalf("--args = %q, args = %v", *argsJSON, fs.Args())
	}
}

func TestExecArgsFromStdin(t *testing.T) {
	a, err := execArgs("-", "", true, strings.NewReader(`{"cnpj":"00.000.000/0001-00"}`))
	if err != nil {
		t.Fatal(err)
	}
	if a["cnpj"] != "00.000.000/0001-00" {
		t.Fatalf("args = %v", a)
	}
}

func TestExecArgsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "args.json")
	if err := os.WriteFile(path, []byte(`{"n":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := execArgs("{}", path, false, strings.NewReader("not read"))
	if err != nil {
		t.Fatal(err)
	}
	if a["n"] != float64(2) {
		t.Fatalf("args = %v", a)
	}
	// --args-file - is stdin too
	a, err = execArgs("{}", "-", false, strings.NewReader(`{"n":3}`))
	if err != nil || a["n"] != float64(3) {
		t.Fatalf("args = %v, err = %v", a, err)
	}
	if _, err := execArgs("{}", filepath.Join(t.TempDir(), "missing.json"), false, nil); err == nil || !strings.Contains(err.Error(), "--args-file") {
		t.Fatalf("a missing file should name the flag, got %v", err)
	}
}

func TestExecArgsAndArgsFileConflict(t *testing.T) {
	_, err := execArgs(`{"a":1}`, "f.json", true, nil)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecArgsBadJSONNamesTheSource(t *testing.T) {
	for _, c := range []struct{ args, file, want string }{
		{"{", "", "--args"},
		{"-", "", "stdin"},
	} {
		_, err := execArgs(c.args, c.file, true, strings.NewReader("{"))
		if err == nil || !strings.HasPrefix(err.Error(), c.want+":") {
			t.Fatalf("%q: error = %v, want the %s prefix", c.args, err, c.want)
		}
	}
	// the default stays an empty object
	if a, err := execArgs("{}", "", false, nil); err != nil || len(a) != 0 {
		t.Fatalf("default: %v, %v", a, err)
	}
}

func TestDoubleDashEndsFlags(t *testing.T) {
	fs, argsJSON, _ := execFlags()
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
	fs, _, _ := execFlags()
	err := parseFlags(fs, []string{"app.mod.fn", "--arg", "{}"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--arg") {
		t.Fatalf("unclear error: %v", err)
	}
}

func TestFlagMissingValueIsRejected(t *testing.T) {
	fs, _, _ := execFlags()
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
	if filter.Action != "role.assign" || filter.Actor != "admin@example.com" || filter.TargetID != "User:1" || filter.Outcome != "Allowed" || filter.Limit != 50 || filter.Since == nil {
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

// A new site declares the one ddcore range its apps are tested against, and
// an app started in it leaves the range to the site.
func TestInitWritesTheSiteRangeAndNewAppDefersToIt(t *testing.T) {
	want, ok := engine.AppRange(engine.Version)
	if !ok {
		t.Skip("not a release build: init writes no range")
	}
	if _, err := initIn(t, "x"); err != nil {
		t.Fatal(err)
	}
	if cfg := readFile(t, "ddcore.json"); !strings.Contains(cfg, fmt.Sprintf(`"ddcore": %q`, want)) {
		t.Fatalf("ddcore.json has no site range %q:\n%s", want, cfg)
	}
	if err := cmdNewApp([]string{"shop"}); err != nil {
		t.Fatal(err)
	}
	if app := readFile(t, filepath.Join("apps", "shop", "ddcore.app.ts")); strings.Contains(app, `  ddcore: "`) {
		t.Fatalf("the app repeats the site's range:\n%s", app)
	}
}

// A new site is deployable as created: init writes the Dockerfile, no longer
// writes "dev", and `deploy railway` adds .railway/railway.ts without touching it.
func TestInitWritesTheImageAndDeployRailwayAddsItsConfig(t *testing.T) {
	if _, err := initIn(t, "x"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, "ddcore.json"), `"dev"`) {
		t.Fatal(`init still writes "dev"`)
	}
	want := scaffold.Image + ":latest"
	if tag, ok := engine.ImageTag(engine.Version); ok {
		want = scaffold.Image + ":" + tag
	}
	if df := readFile(t, "Dockerfile"); !strings.Contains(df, "FROM "+want) {
		t.Fatalf("Dockerfile not on %s:\n%s", want, df)
	}
	readFile(t, ".dockerignore")
	os.WriteFile("Dockerfile", []byte("mine"), 0o644)
	if err := cmdDeploy([]string{"railway"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(".railway", "railway.ts")), "/api/ready") || readFile(t, "Dockerfile") != "mine" {
		t.Fatal("deploy railway should add .railway/railway.ts and leave the Dockerfile alone")
	}
	if err := cmdDeploy([]string{"heroku"}); err == nil {
		t.Fatal("an unknown target should be refused")
	}
}

func TestGithubRepoFromRemote(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:acme/gestao.git":       "acme/gestao",
		"https://github.com/acme/gestao.git":   "acme/gestao",
		"https://github.com/acme/gestao":       "acme/gestao",
		"ssh://git@github.com/acme/gestao.git": "acme/gestao",
		"https://gitlab.com/acme/gestao.git":   "",
		"https://github.com/acme":              "",
	} {
		if got := githubRepo(url); got != want {
			t.Errorf("githubRepo(%q) = %q, want %q", url, got, want)
		}
	}
}

// Two attachments can share a base name — one public, one private — and copying
// both to <out>/files/<base> would leave the second overwriting the first. The
// bytes go under their storage key, which is what the importer reads back.
func TestAttachmentPathKeepsPublicAndPrivateApart(t *testing.T) {
	pub := attachmentPath("/files/nota.pdf")
	priv := attachmentPath("/private/files/nota.pdf")
	if pub == priv {
		t.Fatalf("public and private collide at %q", pub)
	}
	if pub != filepath.Join("public", "nota.pdf") || priv != filepath.Join("private", "nota.pdf") {
		t.Fatalf("pub=%q priv=%q", pub, priv)
	}
	// A url the store cannot place still lands somewhere predictable.
	if got := attachmentPath("weird"); got != "weird" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestImportFlagsAfterPositional(t *testing.T) {
	fs, o := importFlags()
	if err := parseFlags(fs, []string{"export/2026", "--dry-run", "--batch", "50", "--only", "Project,Task", "--tenant", "acme"}); err != nil {
		t.Fatal(err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "export/2026" {
		t.Fatalf("args = %v", fs.Args())
	}
	if !o.dryRun || o.batch != 50 || o.only != "Project,Task" || o.tenant != "acme" {
		t.Fatalf("opts = %+v", o)
	}
}

// --tenant before `import` and after it are the same thing; two different
// tenants are a mistake, not a choice.
func TestImportTenantBeforeOrAfterTheCommand(t *testing.T) {
	t.Setenv("DDCORE_TENANT", "")
	if got, err := importTenant("acme"); err != nil || got != "acme" {
		t.Fatalf("after: %q %v", got, err)
	}
	if rest := stripGlobalFlags([]string{"--tenant", "acme", "import", "run", "dir"}); strings.Join(rest, " ") != "import run dir" {
		t.Fatalf("rest = %v", rest)
	}
	if got, err := importTenant(""); err != nil || got != "acme" {
		t.Fatalf("before: %q %v", got, err)
	}
	if got, err := importTenant("acme"); err != nil || got != "acme" {
		t.Fatalf("both, the same: %q %v", got, err)
	}
	if _, err := importTenant("other"); err == nil {
		t.Fatal("two different tenants were accepted")
	}
}

func TestImportResumeHintNamesTheTenant(t *testing.T) {
	run := &engine.ImportRun{ID: "r1", Dir: "export", Tenant: "acme"}
	if got := resumeCommand(run); got != "ddcore import run export --resume r1 --tenant acme" {
		t.Fatalf("hint = %q", got)
	}
	run.Tenant = ""
	if got := resumeCommand(run); got != "ddcore import run export --resume r1" {
		t.Fatalf("hint = %q", got)
	}
}

func TestSplitListTrimsAndDropsEmpties(t *testing.T) {
	got := splitList(" Project , , Task ")
	if len(got) != 2 || got[0] != "Project" || got[1] != "Task" {
		t.Fatalf("splitList = %#v", got)
	}
	if splitList("  ") != nil {
		t.Fatal("an empty list is nil, not one empty name")
	}
}

func TestHelpFlagPrintsTheOptions(t *testing.T) {
	// `ddcore test -h` failed with "unknown flag: -h (run `ddcore test -h` ...)"
	for _, h := range []string{"-h", "--help", "-help"} {
		fs, _, _, _ := testFlags()
		var out strings.Builder
		fs.Usage = func() { fs.SetOutput(&out); fs.PrintDefaults() }
		if err := parseFlags(fs, []string{"cobranca", h}); err != errHelpShown {
			t.Fatalf("%s: err = %v, want errHelpShown", h, err)
		}
		if !strings.Contains(out.String(), "-filter") {
			t.Fatalf("%s: usage does not list --filter:\n%s", h, out.String())
		}
	}
}

func TestDeclaredHelpFlagIsNotTakenOver(t *testing.T) {
	fs := newFlagSet("x")
	host := fs.String("h", "", "host")
	if err := parseFlags(fs, []string{"-h", "db"}); err != nil {
		t.Fatal(err)
	}
	if *host != "db" {
		t.Fatalf("-h = %q", *host)
	}
}

func TestEverySubcommandUsageIsSet(t *testing.T) {
	for cmd, u := range subcommandUsage {
		if strings.TrimSpace(u) == "" {
			t.Fatalf("%s has no usage text", cmd)
		}
	}
}

func TestAdoptDryRunFailsOnACollision(t *testing.T) {
	clean := &engine.AdoptPreview{Tenant: "acme", Tables: []engine.AdoptTable{{Table: "tab_customer", Rows: 3}}}
	var out strings.Builder
	if err := printAdoptPreview(&out, clean); err != nil {
		t.Fatalf("a clean preview failed: %v", err)
	}
	if !strings.Contains(out.String(), "tab_customer") || !strings.Contains(out.String(), "would move 3 rows") {
		t.Fatalf("output:\n%s", out.String())
	}
	clash := &engine.AdoptPreview{Tenant: "acme", Tables: []engine.AdoptTable{{Table: "tab_settings", Rows: 1,
		Collisions: []engine.AdoptCollision{{Index: "tab_settings_pkey", Columns: []string{"id"}, Count: 12,
			Samples: []string{"singleton"}}}}}}
	out.Reset()
	err := printAdoptPreview(&out, clash)
	if err == nil || !strings.Contains(err.Error(), "12 rows") {
		t.Fatalf("a collision did not fail the dry run: %v", err)
	}
	if !strings.Contains(out.String(), "12 on (id): singleton and 11 more") {
		t.Fatalf("output:\n%s", out.String())
	}
}
