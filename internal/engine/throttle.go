package engine

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Throttling is an append-only log of attempts rather than a counter row.
//
// A counter would need an upsert on every failed login — contention on exactly
// the row an attacker is hammering — and it could only say "locked", not "for
// how much longer". Rows carry their own timestamps, so the window slides on
// its own, Retry-After is a subtraction, and what is left behind is the audit
// material SEC-04 asks for. The cost is that the table grows; SweepAuth is
// what pays it.
//
// Every function here writes on e.DB.Pool and never on a Ctx transaction. That
// is not a style choice: Engine.Login runs inside Ctx.Run, which rolls back on
// any returned error, so a failed attempt recorded on the transaction would be
// erased by the very error it is counting.

// throttleKey namespaces a counter. The identity is the string as typed, not
// the user it resolved to: if an unknown address were counted differently from
// a known one, the lockout itself would answer "does this account exist".
func throttleKey(prefix, identity string) string {
	return prefix + ":" + strings.ToLower(strings.TrimSpace(identity))
}

// CheckThrottle refuses the attempt when too many have already failed inside
// the window, and says how long the caller has to wait.
func (e *Engine) CheckThrottle(ctx context.Context, key string, limit int, window time.Duration) error {
	if e.DB == nil || limit <= 0 || window <= 0 {
		return nil
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT count(*) AS n, min(created) AS oldest
		FROM ddcore_login_attempt WHERE identity = $1 AND NOT ok AND created > now() - $2::interval`,
		key, intervalOf(window))
	if err != nil || len(rows) == 0 {
		// A throttle that cannot read its own table must not become a way to
		// lock everybody out. Fail open here; the credential check still
		// stands between the caller and the account.
		return nil
	}
	n := 0
	switch v := rows[0]["n"].(type) {
	case int64:
		n = int(v)
	case int32:
		n = int(v)
	case int:
		n = v
	}
	if n < limit {
		return nil
	}
	wait := window
	if oldest, ok := rows[0]["oldest"].(time.Time); ok {
		wait = time.Until(oldest.Add(window))
	}
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return cerr.TooMany("Too many attempts. Try again in {0} second(s)", secs).WithRetryAfter(secs)
}

// RecordAttempt logs one attempt. It never returns an error: the caller is in
// the middle of answering a login, and failing to write the audit row is not a
// reason to refuse a correct password.
func (e *Engine) RecordAttempt(ctx context.Context, key, ip string, ok bool) {
	if e.DB == nil {
		return
	}
	if _, err := e.DB.Pool.Exec(ctx,
		`INSERT INTO ddcore_login_attempt (identity, ip, ok) VALUES ($1, $2, $3)`, key, ip, ok); err != nil {
		e.Log.Warn("could not record a login attempt", "err", err)
	}
}

// ClearAttempts forgets the failures under a key, and reports how many it
// forgot. A correct password clears the account's own counter; an operator
// running `ddcore user unlock` clears it on someone's behalf.
func (e *Engine) ClearAttempts(ctx context.Context, key string) (int, error) {
	if e.DB == nil {
		return 0, nil
	}
	tag, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_login_attempt WHERE identity = $1 AND NOT ok`, key)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// guardLogin is the pair of checks a sign-in passes before any hashing
// happens: this account, and this address.
//
// The address limit is deliberately looser. It exists to catch one client
// spraying many accounts, and it has to stay far enough above ordinary use
// that an office behind one NAT does not lock itself out at lunchtime.
func (e *Engine) guardLogin(ctx context.Context, identity, ip string) error {
	p := e.Cfg.Auth
	if err := e.CheckThrottle(ctx, throttleKey("login", identity), p.MaxLoginAttempts, p.LockoutWindow()); err != nil {
		return err
	}
	if ip == "" {
		return nil
	}
	return e.CheckThrottle(ctx, throttleKey("loginip", ip), p.MaxLoginAttempts*5, p.LockoutWindow())
}

// recordLogin logs the attempt against both keys, and on success clears the
// account's failures so a person who finally remembers their password is not
// still serving out a lockout.
func (e *Engine) recordLogin(ctx context.Context, identity, ip string, ok bool) {
	e.RecordAttempt(ctx, throttleKey("login", identity), ip, ok)
	if ip != "" {
		e.RecordAttempt(ctx, throttleKey("loginip", ip), ip, ok)
	}
	if ok {
		e.ClearAttempts(ctx, throttleKey("login", identity))
		if ip != "" {
			e.ClearAttempts(ctx, throttleKey("loginip", ip))
		}
	}
}
