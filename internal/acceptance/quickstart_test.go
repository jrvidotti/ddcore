package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// guideFiles are the files docs/guide/first-app.md tells a reader to write, as
// "Create `path`:" or "Replace `path` with:" followed by a typescript block.
var guideFiles = regexp.MustCompile("(?s)(?:Create|Replace) `([^`]+)`(?: with)?:\\s*```typescript\\n(.*?)```")

// TestQuickstartGuide follows the first-app tutorial the way a new developer
// does: a built binary, an empty directory, no framework checkout. The
// TypeScript comes from the guide itself, so a guide the binary no longer
// agrees with fails here instead of on a reader's machine.
func TestQuickstartGuide(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	ctx := context.Background()
	dsn, adminDSN, dbName := dsnFor("_quickstart")
	admin, err := engine.New(ctx, engine.Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	admin.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := admin.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	admin.DB.Close()

	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "guide", "first-app.md"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, m := range guideFiles.FindAllStringSubmatch(string(guide), -1) {
		files[m[1]] = m[2]
	}
	const (
		doctype    = "apps/library/doctypes/book/book.doctype.ts"
		controller = "apps/library/doctypes/book/book.controller.ts"
		tests      = "apps/library/doctypes/book/book.test.ts"
		workspace  = "apps/library/workspaces/library.workspace.ts"
		app        = "apps/library/ddcore.app.ts"
	)
	for _, p := range []string{doctype, workspace, app, controller, tests} {
		if files[p] == "" {
			t.Fatalf("the guide no longer creates %s (found %v)", p, keys(files))
		}
	}

	bin := filepath.Join(t.TempDir(), "ddcore")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/ddcore")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	dir := t.TempDir()
	port := freePort(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env = dir, cleanEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ddcore %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(files[rel]), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Create the project and the app
	run("init", "--dsn", dsn, "--port", strconv.Itoa(port))
	run("new-app", "library")
	var cfg struct {
		Apps []string `json:"apps"`
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ddcore.json"))
	if err := json.Unmarshal(b, &cfg); err != nil || len(cfg.Apps) != 1 || cfg.Apps[0] != "apps/library" {
		t.Fatalf("new-app did not register apps/library: %v\n%s", err, b)
	}

	// 4–5. Define a DocType and migrate
	write(doctype)
	if out := run("migrate"); !strings.Contains(out, "apps installed: [core library]") {
		t.Fatalf("migrate did not install the app:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "apps/library/.ddcore/types.d.ts")); err != nil {
		t.Fatalf("migrate did not generate the typings: %v", err)
	}

	// 7–9. The sidebar, the rule and its test
	write(workspace)
	write(app)
	write(controller)
	write(tests)
	if out := run("test"); !strings.Contains(out, "2 tests, 0 failures") {
		t.Fatalf("the guide's tests did not pass:\n%s", out)
	}

	// 6–8. Sign in, find Book in the sidebar, see the rule refuse a write
	run("user", "passwd", "Administrator", "admin1234")
	srv := exec.Command(bin, "start")
	srv.Dir, srv.Env = dir, cleanEnv()
	var log bytes.Buffer
	srv.Stdout, srv.Stderr = &log, &log
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })
	base := "http://localhost:" + strconv.Itoa(port)
	waitReady(t, base+"/readyz", &log)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	post := func(path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-DDCore-CSRF", "1")
		req.Header.Set("X-Lang", "en")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(res.Body)
		return res.StatusCode, buf.String()
	}
	if code, body := post("/api/login", `{"usr":"Administrator","pwd":"admin1234"}`); code != 200 {
		t.Fatalf("login: %d %s", code, body)
	}
	boot := getWithClient(t, client, base+"/api/boot")
	if !strings.Contains(boot, `"sidebar":[{"doctype":"Book"`) {
		t.Fatalf("the guide's workspace does not put Book in the sidebar:\n%s", boot)
	}
	if code, body := post("/api/resource/Book", `{"isbn":"1","title":"T","author":"A","published_year":2099}`); code != 417 ||
		!strings.Contains(body, "Published year cannot be in the future") {
		t.Fatalf("a future year should be refused with 417: %d %s", code, body)
	}
	if code, body := post("/api/resource/Book", `{"isbn":"2","title":"T","author":"A","published_year":1999}`); code != 200 {
		t.Fatalf("a valid book should save: %d %s", code, body)
	}
}

func getWithClient(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("GET %s: %d %s", url, res.StatusCode, buf.String())
	}
	return buf.String()
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// cleanEnv is the test process's environment without the variables that
// would point the binary at another database or port than ddcore.json names.
func cleanEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "DDCORE_") || k == "DATABASE_URL" || k == "PORT" {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitReady(t *testing.T, url string, log *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if res, err := http.Get(url); err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server not ready at %s:\n%s", url, log.String())
}
