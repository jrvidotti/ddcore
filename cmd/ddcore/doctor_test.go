package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
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
