package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	in := `
# a comment
DDCORE_PORT=8090
export DDCORE_SITE=demo
QUOTED="um dois"
SINGLE='nao \n expande'
DOUBLE="linha\nquebrada"
DSN=postgres://u:p@h:5455/db?sslmode=disable
EMPTY=
SPACED  =  valor
COMMENTED=8090 # a porta
HASHINVALUE=abc#def
naoehumalinha
=semchave
`
	want := [][2]string{
		{"DDCORE_PORT", "8090"},
		{"DDCORE_SITE", "demo"},
		{"QUOTED", "um dois"},
		{"SINGLE", `nao \n expande`},
		{"DOUBLE", "linha\nquebrada"},
		{"DSN", "postgres://u:p@h:5455/db?sslmode=disable"},
		{"EMPTY", ""},
		{"SPACED", "valor"},
		{"COMMENTED", "8090"},
		{"HASHINVALUE", "abc#def"},
	}
	got, err := parseDotenv(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseDotenv: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("esperava %d pares, veio %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("par %d: esperava %v, veio %v", i, w, got[i])
		}
	}
}

// A variável já presente no ambiente real tem de sobreviver ao arquivo: ali é o
// deploy falando, e o arquivo é só o padrão de quem desenvolve.
func TestDotenvDoesNotOverrideTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, DotenvName), "DDCORE_TEST_A=do_arquivo\nDDCORE_TEST_B=do_arquivo\n")
	t.Setenv("DDCORE_TEST_A", "do_ambiente")
	os.Unsetenv("DDCORE_TEST_B")
	t.Cleanup(func() { os.Unsetenv("DDCORE_TEST_B") })

	if err := loadDotenv(filepath.Join(dir, DotenvName)); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DDCORE_TEST_A"); got != "do_ambiente" {
		t.Errorf("o ambiente real devia vencer o arquivo, veio %q", got)
	}
	if got := os.Getenv("DDCORE_TEST_B"); got != "do_arquivo" {
		t.Errorf("uma chave ausente do ambiente devia vir do arquivo, veio %q", got)
	}
}

func TestDotenvMissingFileIsNotAnError(t *testing.T) {
	if err := loadDotenv(filepath.Join(t.TempDir(), DotenvName)); err != nil {
		t.Fatalf("um .env ausente não é erro: %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
