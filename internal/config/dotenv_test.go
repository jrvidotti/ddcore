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
		t.Fatalf("expected %d pairs, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("pair %d: expected %v, got %v", i, w, got[i])
		}
	}
}

// Variables already present in the real environment must take precedence over the file:
// that represents the deployment, whereas the file is merely local developer defaults.
func TestDotenvDoesNotOverrideTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, DotenvName), "DDCORE_TEST_A=from_file\nDDCORE_TEST_B=from_file\n")
	t.Setenv("DDCORE_TEST_A", "from_env")
	os.Unsetenv("DDCORE_TEST_B")
	t.Cleanup(func() { os.Unsetenv("DDCORE_TEST_B") })

	if err := loadDotenv(filepath.Join(dir, DotenvName)); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DDCORE_TEST_A"); got != "from_env" {
		t.Errorf("real environment should override the file, got %q", got)
	}
	if got := os.Getenv("DDCORE_TEST_B"); got != "from_file" {
		t.Errorf("key missing from environment should come from file, got %q", got)
	}
}

func TestDotenvMissingFileIsNotAnError(t *testing.T) {
	if err := loadDotenv(filepath.Join(t.TempDir(), DotenvName)); err != nil {
		t.Fatalf("a missing .env is not an error: %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
