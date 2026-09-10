package main

import (
	"flag"
	"strings"
	"testing"
)

// execFlags mirrors cmdExec's declarations.
func execFlags() (*flag.FlagSet, *string) {
	fs := newFlagSet("exec")
	return fs, fs.String("args", "{}", "argumentos JSON")
}

// userAddFlags mirrors `cerne user add`.
func userAddFlags() (*flag.FlagSet, *string, *multi) {
	fs := newFlagSet("user add")
	pw := fs.String("password", "", "senha")
	roles := &multi{}
	fs.Var(roles, "role", "papel (repetível)")
	return fs, pw, roles
}

// evalFlags mirrors `cerne eval`.
func evalFlags() (*flag.FlagSet, *bool) {
	fs := newFlagSet("eval")
	return fs, fs.Bool("commit", false, "grava a transação")
}

func TestExecFlagsAfterPositional(t *testing.T) {
	// B22: `flag` parava no primeiro posicional e --args era ignorado
	fs, argsJSON := execFlags()
	if err := parseFlags(fs, []string{"alugueis.services.demo.gerar", "--args", `{"a":1}`}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != `{"a":1}` {
		t.Fatalf("--args = %q, esperado {\"a\":1}", *argsJSON)
	}
	if got := fs.Arg(0); got != "alugueis.services.demo.gerar" {
		t.Fatalf("posicional = %q", got)
	}
	if fs.NArg() != 1 {
		t.Fatalf("NArg = %d, esperado 1", fs.NArg())
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
		t.Fatalf("posicional = %q", fs.Arg(0))
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
	// o nome completo é o resto dos posicionais, sem as opções coladas
	if name := strings.Join(fs.Args()[1:], " "); name != "Ana Maria" {
		t.Fatalf("nome = %q, esperado \"Ana Maria\"", name)
	}
}

func TestEvalCommitAfterCode(t *testing.T) {
	fs, commit := evalFlags()
	code := `cerne.db.count("User")`
	if err := parseFlags(fs, []string{code, "--commit"}); err != nil {
		t.Fatal(err)
	}
	if !*commit {
		t.Fatal("--commit depois do código não foi aplicado")
	}
	// e não pode acabar dentro do texto avaliado
	if got := strings.Join(fs.Args(), " "); got != code {
		t.Fatalf("código = %q", got)
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
	// `cerne eval -` lê o código do stdin
	fs, _ := evalFlags()
	if err := parseFlags(fs, []string{"-"}); err != nil {
		t.Fatal(err)
	}
	if fs.Arg(0) != "-" {
		t.Fatalf("arg = %q, esperado -", fs.Arg(0))
	}
}

func TestDoubleDashEndsFlags(t *testing.T) {
	fs, argsJSON := execFlags()
	if err := parseFlags(fs, []string{"app.mod.fn", "--", "--args", "literal"}); err != nil {
		t.Fatal(err)
	}
	if *argsJSON != "{}" {
		t.Fatalf("--args devia continuar no padrão, veio %q", *argsJSON)
	}
	if got := strings.Join(fs.Args(), " "); got != "app.mod.fn --args literal" {
		t.Fatalf("posicionais = %q", got)
	}
}

func TestUnknownFlagIsRejected(t *testing.T) {
	// silêncio era pior: a flag virava argumento posicional
	fs, _ := execFlags()
	err := parseFlags(fs, []string{"app.mod.fn", "--arg", "{}"})
	if err == nil {
		t.Fatal("esperava erro para flag desconhecida")
	}
	if !strings.Contains(err.Error(), "flag desconhecida") || !strings.Contains(err.Error(), "--arg") {
		t.Fatalf("erro pouco claro: %v", err)
	}
}

func TestFlagMissingValueIsRejected(t *testing.T) {
	fs, _ := execFlags()
	err := parseFlags(fs, []string{"app.mod.fn", "--args"})
	if err == nil || !strings.Contains(err.Error(), "exige um valor") {
		t.Fatalf("erro = %v", err)
	}
}

func TestBoolFlagDoesNotEatPositional(t *testing.T) {
	fs, commit := evalFlags()
	if err := parseFlags(fs, []string{"--commit", "--", "código"}); err != nil {
		t.Fatal(err)
	}
	if !*commit || fs.Arg(0) != "código" {
		t.Fatalf("commit=%v arg=%q", *commit, fs.Arg(0))
	}
}

func TestSingleDashFlagFormIsAccepted(t *testing.T) {
	// -v e --v devem ser equivalentes, como no pacote flag
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
		t.Fatalf("--app = %q, esperado exemplo", *app)
	}
}
