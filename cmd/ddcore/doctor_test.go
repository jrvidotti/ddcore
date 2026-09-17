package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/release"
)

func renderDoctor(t *testing.T, r *doctorReport) string {
	t.Helper()
	var b bytes.Buffer
	r.print(&b)
	return b.String()
}

// A doctor that dies when the database is down is silent in the one situation
// it exists for. The report has to come out, and it has to say so.
func TestPRD03_DoctorReportsAnUnreachableDatabase(t *testing.T) {
	r := &doctorReport{
		Database: db.Health{OK: false, Error: "dial tcp 127.0.0.1:5455: connect: connection refused"},
		DSN:      "postgres://ddcore:***@localhost:5455/ddcore?sslmode=disable",
		Engine:   "postgres: connection refused",
		Critical: []string{"database unreachable", "the apps could not be loaded"},
		Ops:      config.DefaultOps(),
	}
	out := renderDoctor(t, r)
	if !strings.Contains(out, "database:   unreachable") {
		t.Fatalf("the report does not name the failure:\n%s", out)
	}
	// The sections that need no database still have to print: they are what
	// tells the reader whether the DSN they think they configured is the one
	// in use.
	for _, want := range []string{"url:", "mail:", "sessions:", "critical:   database unreachable"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the report stops short of %q:\n%s", want, out)
		}
	}
	if code := r.exitCode(false); code != 1 {
		t.Fatalf("an unreachable database must exit 1, got %d", code)
	}
}

// This report is pasted into issues and chat windows — the same reason the
// secrets section prints names and never values.
func TestPRD03_DoctorNeverPrintsTheDSN(t *testing.T) {
	const dsn = "postgres://ddcore:s3cr3t@db.internal:5432/prod?sslmode=disable"
	r := &doctorReport{
		DSN:      db.RedactDSN(dsn),
		Database: db.Health{Error: db.RedactError(errString("failed to connect to `user=ddcore password=s3cr3t database=prod`"))},
		Engine:   db.RedactError(errString("postgres: " + dsn)),
		Ops:      config.DefaultOps(),
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{renderDoctor(t, r), string(b)} {
		if strings.Contains(out, "s3cr3t") {
			t.Fatalf("the password reached the report:\n%s", out)
		}
		if !strings.Contains(out, "db.internal") {
			t.Fatalf("redaction took the host too, leaving nothing to recognise:\n%s", out)
		}
	}
}

// A person runs doctor to read it. Turning "3 pending statements" into a shell
// failure would be a regression for everyone; --strict is for the script.
func TestPRD03_DoctorWarningsDoNotFailUnlessStrict(t *testing.T) {
	r := &doctorReport{
		Database: db.Health{OK: true},
		Queue:    &engine.QueueHealth{Runnable: 214, WindowMinutes: 15},
		Warnings: []string{"queue backlog: 214 runnable jobs (limit 100)"},
		Ops:      config.DefaultOps(),
	}
	if code := r.exitCode(false); code != 0 {
		t.Fatalf("a warning must not fail the shell, got %d", code)
	}
	if code := r.exitCode(true); code != 1 {
		t.Fatalf("--strict must fail on a warning, got %d", code)
	}
	if out := renderDoctor(t, r); !strings.Contains(out, "warning:    queue backlog") {
		t.Fatalf("the warning is not in the report:\n%s", out)
	}
}

func TestPRD03_DoctorFlagsAreParsed(t *testing.T) {
	fs := newFlagSet("doctor")
	asJSON := fs.Bool("json", false, "")
	strict := fs.Bool("strict", false, "")
	window := fs.Int("window", 0, "")
	if err := parseFlags(fs, []string{"--json", "--window=30"}); err != nil {
		t.Fatal(err)
	}
	if !*asJSON || *strict || *window != 30 {
		t.Fatalf("parsed json=%v strict=%v window=%d", *asJSON, *strict, *window)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestDoctorVaultReporting(t *testing.T) {
	r := &doctorReport{
		Database: db.Health{OK: true},
		Ops:      config.DefaultOps(),
		Vault: &vaultSection{
			Configured: true,
			Count:      2,
			Secrets:    []string{"asaas:token:1", "asaas:token:2"},
		},
	}
	out := renderDoctor(t, r)
	if !strings.Contains(out, "vault:      2 secret(s) encrypted: asaas:token:1, asaas:token:2") {
		t.Fatalf("unexpected vault report:\n%s", out)
	}

	rUnconf := &doctorReport{
		Database: db.Health{OK: true},
		Ops:      config.DefaultOps(),
		Vault: &vaultSection{
			Configured: false,
			Count:      0,
		},
	}
	outUnconf := renderDoctor(t, rUnconf)
	if !strings.Contains(outUnconf, "vault:      key not configured (DDCORE_SECRET_KEY is missing)") {
		t.Fatalf("unexpected unconfigured vault report:\n%s", outUnconf)
	}
}

// The storage line is read by someone checking where bytes go; a bucket with
// no prefix must not look like a truncated path.
func TestDoctorStorageSummaryWithoutPrefix(t *testing.T) {
	cfg := &config.File{DataDir: "/srv/site/data"}
	if got := storageSummary(cfg); got != "local /srv/site/data/files" {
		t.Errorf("local: %q", got)
	}
	cfg.Storage = config.Storage{Backend: config.StorageS3, S3: config.S3{
		Endpoint: "minio:9000", Bucket: "ddcore", PresignTTL: 5 * time.Minute,
	}}
	if got := storageSummary(cfg); got != "s3 minio:9000/ddcore (presigned links valid 5m0s)" {
		t.Errorf("no prefix: %q", got)
	}
	cfg.Storage.S3.Prefix = "site-a"
	if got := storageSummary(cfg); got != "s3 minio:9000/ddcore/site-a (presigned links valid 5m0s)" {
		t.Errorf("with prefix: %q", got)
	}
}

// A rollback refusal is not a broken app: sending the reader to look at app
// code for it costs an outage's worth of time.
func TestDoctorNamesARollbackRefusal(t *testing.T) {
	r := &doctorReport{
		Engine:   "this binary is older than the database: core 0.14.0 (database migrated by 0.15.0). Roll forward, or pass --allow-older-binary",
		Critical: []string{engineCritical(fmt.Errorf("%w: core 0.14.0 (database migrated by 0.15.0)", engine.ErrOlderBinary))},
		Ops:      config.DefaultOps(),
	}
	out := renderDoctor(t, r)
	if strings.Contains(out, "the apps could not be loaded") {
		t.Errorf("a rollback refusal is reported as an app failure:\n%s", out)
	}
	if !strings.Contains(out, "older than the database") {
		t.Errorf("the critical does not say what happened:\n%s", out)
	}
}

// The update check is the one finding a site can act on without reading
// anything else, so it has to reach the report and the warnings alike.
func TestDoctorReportsANewerRelease(t *testing.T) {
	r := &doctorReport{
		DDCore:   "v0.14.0",
		Database: db.Health{OK: true},
		Update:   &release.Update{Current: "v0.14.0", Latest: "v0.15.0", Available: true},
		Warnings: []string{"a newer ddcore release is available: v0.15.0 (this binary is v0.14.0)"},
		Ops:      config.DefaultOps(),
	}
	out := renderDoctor(t, r)
	if !strings.Contains(out, "version:    v0.14.0") {
		t.Fatalf("the running version is missing:\n%s", out)
	}
	if !strings.Contains(out, "v0.15.0 is available") {
		t.Fatalf("the available release is not next to the version:\n%s", out)
	}
	if !strings.Contains(out, "warning:    a newer ddcore release is available") {
		t.Fatalf("the warning is not in the report:\n%s", out)
	}
	if code := r.exitCode(true); code != 1 {
		t.Fatalf("--strict must fail on a stale binary, got %d", code)
	}
	if code := r.exitCode(false); code != 0 {
		t.Fatalf("a stale binary is not a critical finding, got %d", code)
	}
}

// Up to date, and the report says nothing about it: an operator reading this
// every morning must not learn to skim past a line that is always there.
func TestDoctorIsSilentWhenUpToDate(t *testing.T) {
	r := &doctorReport{
		DDCore:   "v0.15.0",
		Database: db.Health{OK: true},
		Update:   &release.Update{Current: "v0.15.0", Latest: "v0.15.0"},
		Ops:      config.DefaultOps(),
	}
	if out := renderDoctor(t, r); strings.Contains(out, "is available") {
		t.Fatalf("nothing should be announced:\n%s", out)
	}
}

// doctorConfig is the least a report needs: a database that refuses at once, so
// the gather reaches its early return without waiting on a dial.
func doctorConfig() *config.File {
	return &config.File{
		DSN: "postgres://nobody@127.0.0.1:1/nothing?sslmode=disable",
		Ops: config.DefaultOps(),
	}
}

// The lookup runs before the engine is loaded on purpose: a binary too old for
// the apps in front of it is exactly the case where knowing a newer release
// exists is the answer, and by then the gather has already given up.
func TestGatherDoctorChecksForUpdatesBeforeTheEngineLoads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0"}`))
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	rep := gatherDoctor(context.Background(), doctorConfig(), 0, true)
	if rep.Engine == "" {
		t.Fatal("this test needs the engine to fail to load")
	}
	if rep.Update == nil || !rep.Update.Available {
		t.Fatalf("the update was not reported: %+v", rep.Update)
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), "a newer ddcore release is available: v99.0.0") {
		t.Fatalf("the warning is missing: %v", rep.Warnings)
	}
}

// Off means no request at all, not a request whose answer is dropped.
func TestGatherDoctorSkipsTheCheckWhenOff(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0"}`))
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	rep := gatherDoctor(context.Background(), doctorConfig(), 0, false)
	if rep.Update != nil {
		t.Errorf("want no update section, got %+v", rep.Update)
	}
	if calls != 0 {
		t.Errorf("want no request, got %d", calls)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "release is available") {
			t.Errorf("unexpected warning: %q", w)
		}
	}
}

// A lookup that cannot be made is not a finding: an air-gapped site is a
// normal site, and a doctor that warned about its own reachability would be
// noise on every run.
func TestGatherDoctorIsQuietWhenTheLookupFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	rep := gatherDoctor(context.Background(), doctorConfig(), 0, true)
	if rep.Update != nil {
		t.Errorf("want no update section, got %+v", rep.Update)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "release is available") {
			t.Errorf("unexpected warning: %q", w)
		}
	}
}

func TestDoctorUpdateFlagIsParsed(t *testing.T) {
	fs := newFlagSet("doctor")
	noUpdate := fs.Bool("no-update-check", false, "")
	if err := parseFlags(fs, []string{"--no-update-check"}); err != nil {
		t.Fatal(err)
	}
	if !*noUpdate {
		t.Fatal("--no-update-check did not parse")
	}
}
