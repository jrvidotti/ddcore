package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Recovery and invitation tokens live in an internal table rather than a
// DocType, and that is a security decision rather than a modelling one.
//
// A DocType is readable through /api/resource, exportable, and kept out of an
// export today only by a hardcoded redaction map — so a token DocType would be
// one permission mistake away from handing out a live credential. It also has
// no use for version history, comments, a naming series or a form, and its
// rows are high-churn and swept. That is exactly what ddcore_session already
// is, and why that lives outside the doctype system too.

// Token kinds.
const (
	TokenReset  = "reset"
	TokenInvite = "invite"
)

// asTime reads a timestamp back out of a db.Select row.
//
// db.Normalize turns every time.Time into an RFC3339 string on the way out, so
// that rows serialise cleanly into JS — which means a `.(time.Time)` type
// assertion on a row value silently never matches. That is a quiet failure
// mode: an expiry check written that way compiles, runs, and never expires
// anything.
func asTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// hashToken is what the database stores. A dump, a backup or a System Manager
// with db.sql must not yield a working password reset.
//
// SHA-256 and not Argon2 on purpose: the token is 192 bits from crypto/rand,
// so there is nothing to brute-force, and the cost per verification has to
// stay near zero because it sits on an unauthenticated path — the same reason
// the login had to be throttled.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokenHandle is the opaque, non-reversible id of a token, safe to show.
func TokenHandle(token string) string { return hashToken(token)[:12] }

// AuthToken is a token as the server knows it — never including the token.
type AuthToken struct {
	Kind    string
	User    string
	Expires time.Time
}

// IssueToken mints a single-use token and returns it. The caller is the only
// one who will ever see this value: what is stored is its hash.
func (e *Engine) IssueToken(ctx context.Context, user, kind string, ttl time.Duration, createdBy, ip string) (string, time.Time, error) {
	token := RandomToken()
	expires := time.Now().Add(ttl)
	_, err := e.DB.Pool.Exec(ctx,
		`INSERT INTO ddcore_auth_token (token_hash, kind, "user", expires, created_by, ip)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		hashToken(token), kind, user, expires, createdBy, ip)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// PeekToken reports what a token is for without spending it, so the desk can
// say "set a password for ana@x.com" and tell an invitation from a recovery.
// It reveals nothing the holder of the token does not already have.
func (e *Engine) PeekToken(ctx context.Context, token string) (*AuthToken, error) {
	rows, err := db.Select(ctx, e.DB.Pool,
		`SELECT kind, "user", expires FROM ddcore_auth_token
		 WHERE token_hash = $1 AND used IS NULL AND expires > now()`, hashToken(token))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.NotFound("This link is no longer valid. Ask for a new one.")
	}
	t := &AuthToken{Kind: db.Str(rows[0]["kind"]), User: db.Str(rows[0]["user"])}
	if v, ok := asTime(rows[0]["expires"]); ok {
		t.Expires = v
	}
	return t, nil
}

// ConsumeToken spends a token, and is the reason this is one statement.
//
// Marking used and checking that it was unused have to happen together, or two
// submissions of the same link both win the race and both set a password. The
// UPDATE ... WHERE used IS NULL ... RETURNING does it in a single round trip
// that Postgres serialises for us.
//
// It runs on the caller's transaction so that spending the token and writing
// the password commit or roll back as one.
func (e *Engine) ConsumeToken(ctx context.Context, q db.Querier, token, kind string) (*AuthToken, error) {
	rows, err := db.Select(ctx, q,
		`UPDATE ddcore_auth_token SET used = now()
		 WHERE token_hash = $1 AND kind = $2 AND used IS NULL AND expires > now()
		 RETURNING kind, "user", expires`, hashToken(token), kind)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.Validation("This link is no longer valid. Ask for a new one.")
	}
	t := &AuthToken{Kind: db.Str(rows[0]["kind"]), User: db.Str(rows[0]["user"])}
	if v, ok := asTime(rows[0]["expires"]); ok {
		t.Expires = v
	}
	return t, nil
}

// RevokeTokens drops a user's outstanding tokens of a kind. A completed
// recovery invalidates the links that were not used — including any an
// attacker asked for while the account was in play.
func (e *Engine) RevokeTokens(ctx context.Context, q db.Querier, user, kind string) (int, error) {
	tag, err := q.Exec(ctx,
		`DELETE FROM ddcore_auth_token WHERE "user" = $1 AND kind = $2 AND used IS NULL`, user, kind)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// SweepCounts is what one sweep removed.
type SweepCounts struct{ Sessions, Tokens, Attempts int }

// SweepAuth deletes what has expired. It is hygiene and never correctness:
// the scheduler only runs when the site enables it, so every read path filters
// on `expires > now()` itself, exactly as UserFromSession already does. If
// this never ran, nothing would become valid again — the tables would only
// grow.
func (e *Engine) SweepAuth(ctx context.Context) (SweepCounts, error) {
	var n SweepCounts
	tag, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_session WHERE expires < now()`)
	if err != nil {
		return n, err
	}
	n.Sessions = int(tag.RowsAffected())

	// A used token is kept for a while after it is spent: it is the evidence
	// that a recovery happened, which is worth more than the row costs.
	tag, err = e.DB.Pool.Exec(ctx,
		`DELETE FROM ddcore_auth_token WHERE expires < now() - interval '7 days'`)
	if err != nil {
		return n, err
	}
	n.Tokens = int(tag.RowsAffected())

	// Attempts outlive the lockout window by a margin, because they are also
	// the audit trail of an attack and a window's worth is too short to read.
	tag, err = e.DB.Pool.Exec(ctx,
		`DELETE FROM ddcore_login_attempt WHERE created < now() - interval '30 days'`)
	if err != nil {
		return n, err
	}
	n.Attempts = int(tag.RowsAffected())

	// A sign-in state is worthless the moment it expires, and nothing reads
	// an old one.
	if _, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_oidc_state WHERE expires < now()`); err != nil {
		return n, err
	}
	return n, nil
}
