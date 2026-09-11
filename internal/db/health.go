package db

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health is what one connection attempt learned.
//
// It is deliberately usable without a *DB. The failure `ddcore doctor` exists
// to report is precisely the one where no pool and no engine were ever built —
// engine.New calls db.Open, which pings, and returns the error instead of an
// engine. A probe that is a method on the engine is unreachable exactly when it
// matters.
type Health struct {
	OK        bool    `json:"ok"`
	LatencyMS float64 `json:"latencyMs"`
	// Error is already redacted; see redactErr.
	Error         string  `json:"error,omitempty"`
	Conns         int32   `json:"conns,omitempty"`
	Idle          int32   `json:"idle,omitempty"`
	MaxConns      int32   `json:"maxConns,omitempty"`
	AcquireWaitMS float64 `json:"acquireWaitMs,omitempty"`
}

// Check pings the pool this process already has. The timeout is the point: a
// probe that waits forever is a probe that lies, because the caller reads "no
// answer yet" as "still thinking".
func (d *DB) Check(ctx context.Context, timeout time.Duration) Health {
	if d == nil || d.Pool == nil {
		return Health{Error: "no database configured"}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	err := d.Pool.Ping(ctx)
	h := Health{OK: err == nil, LatencyMS: msSince(start)}
	if err != nil {
		h.Error = redactErr(err)
	}
	if st := d.Pool.Stat(); st != nil {
		h.Conns, h.Idle, h.MaxConns = st.TotalConns(), st.IdleConns(), st.MaxConns()
		h.AcquireWaitMS = float64(st.AcquireDuration().Microseconds()) / 1000
	}
	return h
}

// Probe opens a pool of its own, asks, and closes it. It is for the caller that
// has a DSN and nothing else — `doctor` reporting on a site whose engine could
// not be built.
func Probe(ctx context.Context, dsn string, timeout time.Duration) Health {
	if strings.TrimSpace(dsn) == "" {
		return Health{Error: "no dsn configured"}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return Health{Error: redactErr(err), LatencyMS: msSince(start)}
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return Health{Error: redactErr(err), LatencyMS: msSince(start)}
	}
	return Health{OK: true, LatencyMS: msSince(start)}
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000
}

var (
	dsnURL = regexp.MustCompile(`(?i)postgre(?:s|sql)://[^\s"']*`)
	// The quoted alternative comes first on purpose: libpq's keyword/value form
	// allows `password='p@ss word'`, and matching \S+ would stop at the space
	// and leave the rest of the secret in the message.
	dsnKV    = regexp.MustCompile(`(?i)\b(password|pass)\s*=\s*('(?:[^']|'')*'|\S+)`)
	userAtKV = regexp.MustCompile(`(?i)\buser\s*=\s*('(?:[^']|'')*'|\S+)`)
)

// RedactDSN keeps enough of a connection string to recognise which database was
// meant, and none of what would let anyone else connect to it.
func RedactDSN(dsn string) string {
	u := dsnURL.FindString(dsn)
	if u == "" {
		return dsnKV.ReplaceAllString(dsn, "password=***")
	}
	// scheme://user:password@host/db → scheme://user:***@host/db
	at := strings.LastIndex(u, "@")
	scheme := strings.Index(u, "://")
	if at < 0 || scheme < 0 {
		return u
	}
	creds := u[scheme+3 : at]
	if i := strings.Index(creds, ":"); i >= 0 {
		creds = creds[:i] + ":***"
	}
	return u[:scheme+3] + creds + u[at:]
}

// redactErr is why every error in this file goes through one funnel.
//
// pgx phrases a connection failure as "failed to connect to `user=… database=…`",
// and a misconfigured site puts the whole postgres:// URL in the message. A
// doctor report is pasted into issues and chat windows — the same reason the
// secrets section prints names and never values.
func RedactError(err error) string { return redactErr(err) }

func redactErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		msg = "timed out waiting for the database: " + msg
	}
	msg = dsnURL.ReplaceAllStringFunc(msg, RedactDSN)
	msg = dsnKV.ReplaceAllString(msg, "password=***")
	msg = userAtKV.ReplaceAllString(msg, "user=***")
	return msg
}
