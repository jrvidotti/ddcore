package db

import (
	"strings"
	"testing"
	"time"
)

type errString string

func (e errString) Error() string { return string(e) }

// A doctor report is pasted into issues and chat windows, and a pgx connection
// failure quotes the connection string back at you. Every shape it can take has
// to lose its password and keep enough to recognise which database was meant.
func TestPRD03_RedactionKeepsTheHostAndLosesTheSecret(t *testing.T) {
	for name, tc := range map[string]struct{ in, keep string }{
		"url":          {"postgres://ddcore:s3cr3t@localhost:5455/db?sslmode=disable", "localhost:5455"},
		"postgresql":   {"postgresql://u:p@ss:w0rd@h:5432/d", "h:5432"},
		"pgx backtick": {"failed to connect to `user=ddcore password=s3cr3t database=prod`", "database=prod"},
		"key value":    {"host=db.internal port=5432 user=ddcore password=s3cr3t dbname=d", "db.internal"},
		// libpq's keyword/value form allows a quoted password with a space in
		// it; matching up to the first space would leave half the secret.
		"quoted password":  {"host=db.internal password='s3cr3t with space' dbname=d", "db.internal"},
		"url mid sentence": {"dial failed for postgres://ddcore:s3cr3t@h/d and then gave up", "gave up"},
	} {
		got := redactErr(errString(tc.in))
		if strings.Contains(got, "s3cr3t") || strings.Contains(got, "p@ss:w0rd") || strings.Contains(got, "with space") {
			t.Fatalf("%s: the password survived: %s", name, got)
		}
		// Redaction that takes the host too leaves nothing to act on.
		if !strings.Contains(got, tc.keep) {
			t.Fatalf("%s: lost %q, which is what identifies the target: %s", name, tc.keep, got)
		}
	}
}

// A DSN with no password at all must not be mangled into looking like one.
func TestPRD03_RedactDSNLeavesAPasswordlessDSNAlone(t *testing.T) {
	const dsn = "postgres://ddcore@h/d"
	if got := RedactDSN(dsn); got != dsn {
		t.Fatalf("RedactDSN invented a password: %s", got)
	}
}

// An empty DSN and a missing pool are both ordinary states — `ddcore init` has
// not been run yet — and neither may panic a probe.
func TestPRD03_ProbeRefusesQuietlyWithoutADSN(t *testing.T) {
	if h := Probe(t.Context(), "   ", time.Second); h.OK || h.Error == "" {
		t.Fatalf("an empty dsn must report why, got %+v", h)
	}
	var d *DB
	if h := d.Check(t.Context(), time.Second); h.OK || h.Error == "" {
		t.Fatalf("a nil DB must report why, got %+v", h)
	}
}
