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
	for _, want := range []string{"# gestao", dsn, "docker compose up -d", "http://localhost:8080", "prints the Admin password", "AGENTS.md"} {
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
	if err := App(dir, "base", "", ">=0.15.0 <0.16.0", false); err != nil {
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
	if err := App(dir, "base", "", "", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ddcore.app.ts"))
	if !strings.Contains(string(b), "  // ddcore: ") {
		t.Fatalf("expected a commented-out range:\n%s", b)
	}
}

// In a site whose ddcore.json declares the range, a new app leaves it to the
// site instead of starting a second copy that drifts.
func TestAppInSiteWithRangeDeclaresNone(t *testing.T) {
	dir := t.TempDir()
	if err := App(dir, "base", "", ">=0.15.0 <0.16.0", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ddcore.app.ts"))
	if strings.Contains(string(b), `  ddcore: "`) || !strings.Contains(string(b), "ddcore.json applies") {
		t.Fatalf("expected no range of its own and a pointer to ddcore.json:\n%s", b)
	}
}

// The Dockerfile pins the official image at the release it was written by,
// and a binary that is not a release says so instead of pinning nothing.
func TestDockerPinsTheImageAndLeavesFilesAlone(t *testing.T) {
	dir := t.TempDir()
	files, err := Docker(dir, "0.21")
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	for _, want := range []string{"FROM " + Image + ":0.21\n", "COPY ddcore.json ./", "COPY apps/ ./apps/"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("Dockerfile misses %q:\n%s", want, b)
		}
	}
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("mine"), 0o644)
	if files, _ := Docker(dir, "0.22"); len(files) != 0 {
		t.Fatalf("rewrote existing files: %v", files)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "Dockerfile")); string(b) != "mine" {
		t.Fatal("Dockerfile changed")
	}

	unpinned := t.TempDir()
	Docker(unpinned, "")
	if b, _ := os.ReadFile(filepath.Join(unpinned, "Dockerfile")); !strings.Contains(string(b), ":latest") || !strings.Contains(string(b), "Pin a minor series") {
		t.Fatalf("a non-release build should write latest with a note:\n%s", b)
	}
}

// The Railway project is written as Infrastructure as Code — railway.json
// stops being read on 2026-12-01 — with the site's repository as its source
// when the checkout knows it, the database wired in, and no secret in source.
func TestRailwayWritesTheProjectAsCode(t *testing.T) {
	dir := t.TempDir()
	files, err := Railway(dir, RailwaySite{Name: "gestao", Repo: "acme/gestao", Branch: "main"})
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "railway.json")); err == nil {
		t.Fatal("the legacy railway.json was written")
	}
	ts, _ := os.ReadFile(filepath.Join(dir, ".railway", "railway.ts"))
	for _, want := range []string{
		`from "railway/iac"`, `postgres("Postgres")`, `service("gestao"`,
		`source: github("acme/gestao", { branch: "main" })`,
		`healthcheck: "/api/ready"`, `DATABASE_URL: db.env.DATABASE_URL`,
		`DDCORE_SECRET_KEY: preserve()`, `project("gestao"`,
	} {
		if !strings.Contains(string(ts), want) {
			t.Errorf("railway.ts misses %s:\n%s", want, ts)
		}
	}
	var pkg struct{ DevDependencies map[string]string }
	b, _ := os.ReadFile(filepath.Join(dir, ".railway", "package.json"))
	if err := json.Unmarshal(b, &pkg); err != nil || pkg.DevDependencies["railway"] == "" {
		t.Fatalf("package.json should bring the SDK: %s (%v)", b, err)
	}

	// Without a GitHub origin, the source is left for the operator to write,
	// and the unused import goes with it.
	bare := t.TempDir()
	Railway(bare, RailwaySite{Name: "gestao"})
	ts, _ = os.ReadFile(filepath.Join(bare, ".railway", "railway.ts"))
	if strings.Contains(string(ts), "    source: github(") || strings.Contains(string(ts), "github, ") {
		t.Fatalf("a source was invented:\n%s", ts)
	}
}
