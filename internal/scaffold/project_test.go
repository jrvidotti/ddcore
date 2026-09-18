package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectWritesTheGuides(t *testing.T) {
	dir := t.TempDir()
	dsn := LocalDSN("gestao", 5467)
	files, err := Project(dir, ProjectInfo{Name: "gestao", DSN: dsn, Port: 8080, Compose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("wrote %v, want AGENTS.md, .mcp.json, .gitignore, README.md, CLAUDE.md", files)
	}
	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if read("CLAUDE.md") != read("AGENTS.md") && read("CLAUDE.md") != "@AGENTS.md\n" {
		t.Fatal("CLAUDE.md does not lead to AGENTS.md")
	}
	var mcp struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(".mcp.json")), &mcp); err != nil || mcp.MCPServers["ddcore"].Command != "ddcore" {
		t.Fatalf(".mcp.json: %v %+v", err, mcp)
	}
	if !strings.Contains(read(".gitignore"), "\n.env\n") {
		t.Fatal(".gitignore does not keep .env out")
	}
	r := read("README.md")
	for _, want := range []string{"# gestao", dsn, "docker compose up -d", "http://localhost:8080", "ddcore user passwd Admin", "AGENTS.md"} {
		if !strings.Contains(r, want) {
			t.Errorf("README misses %q", want)
		}
	}
}

func TestProjectWithoutComposeSaysNothingOfDocker(t *testing.T) {
	dir := t.TempDir()
	if _, err := Project(dir, ProjectInfo{Name: "x", DSN: "postgres://a:a@db.example.com/a", Port: 8080}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if strings.Contains(string(b), "docker compose") {
		t.Fatalf("README mentions compose without a compose file:\n%s", b)
	}
}

func TestProjectLeavesExistingFilesAlone(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("mine too"), 0o644)
	files, err := Project(dir, ProjectInfo{Name: "x", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "README.md" || f == "CLAUDE.md" {
			t.Fatalf("overwrote %s", f)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "mine" {
		t.Fatal("README.md changed")
	}
}

func TestAppWritesVersionAndRange(t *testing.T) {
	dir := t.TempDir()
	if err := App(dir, "base", "", ">=0.15.0 <0.16.0"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ddcore.app.ts"))
	for _, want := range []string{`version: "0.1.0"`, `  ddcore: ">=0.15.0 <0.16.0",`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("ddcore.app.ts misses %s:\n%s", want, b)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Fatal("the app guide belongs at the project root now")
	}
}

func TestAppWithoutRangeCommentsItOut(t *testing.T) {
	dir := t.TempDir()
	if err := App(dir, "base", "", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ddcore.app.ts"))
	if !strings.Contains(string(b), "  // ddcore: ") {
		t.Fatalf("expected a commented-out range:\n%s", b)
	}
}
