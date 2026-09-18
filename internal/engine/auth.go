package engine

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// HashPassword returns an argon2id hash in PHC-like format.
func HashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	h := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 2, 32)
	return "argon2id$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(h)
}

// decoyHash is what an unknown address is checked against, so that failing to
// find a user costs the same 64 MiB and the same fifty milliseconds as finding
// one. It hashes a value nobody can supply, and is computed once because
// computing it per request would itself be the cost being hidden.
var decoyHash = HashPassword(RandomToken())

func CheckPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// LoginFrom describes where a sign-in came from. It is recorded on the session
// so a person reading their own session list can recognise a device — and
// recognise one they do not.
type LoginFrom struct {
	IP        string
	UserAgent string
}

// Login checks credentials and creates a session, returning the sid.
//
// The throttle comes first, before the lookup and before any hashing. That is
// the order the cost demands as much as the policy: CheckPassword runs Argon2
// at 64 MiB per call, so an unthrottled login endpoint is a memory-exhaustion
// vector that needs no credentials at all.
func (e *Engine) Login(ctx context.Context, user, password string, from LoginFrom) (string, error) {
	identity := strings.TrimSpace(user)
	if err := e.guardLogin(ctx, identity, from.IP); err != nil {
		return "", err
	}
	sid, err := e.login(ctx, identity, password, from)
	// Recorded outside Ctx.Run on purpose — see the comment in throttle.go.
	// An attempt written inside the transaction would be rolled back by the
	// error it is counting.
	e.recordLogin(ctx, identity, from.IP, err == nil)
	return sid, err
}

func (e *Engine) login(ctx context.Context, user, password string, from LoginFrom) (string, error) {
	var sid string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		rows, err := db.Select(ctx, c.Tx, `SELECT name, password_hash, enabled FROM tab_user WHERE lower(name) = lower($1) OR lower(email) = lower($1) LIMIT 1`, strings.TrimSpace(user))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			// Hash anyway. Returning early for an unknown address would make
			// it answer in a millisecond while a real one takes fifty, and
			// that difference is a working account-enumeration oracle.
			CheckPassword(decoyHash, password)
			return cerr.Auth("Invalid username or password")
		}
		if !CheckPassword(db.Str(rows[0]["password_hash"]), password) {
			return cerr.Auth("Invalid username or password")
		}
		if en, ok := rows[0]["enabled"].(bool); ok && !en {
			// Checked after the password on purpose: saying "disabled" to
			// someone who has not proved they own the account would hand them
			// the fact that it exists.
			return cerr.Auth("User is disabled")
		}
		name := db.Str(rows[0]["name"])
		if !e.Cfg.Auth.AllowPasswordLogin() && name != "Admin" {
			// After the password, for the same reason as "disabled" above.
			return cerr.Auth("Password sign-in is disabled. Use single sign-on.")
		}
		sid, err = e.createSession(c, name, from)
		return err
	})
	return sid, err
}

// createSession opens a session for a user whose identity the caller has
// already established — by password or by a provider's signed token.
func (e *Engine) createSession(c *Ctx, user string, from LoginFrom) (string, error) {
	sid := RandomToken()
	if _, err := c.Tx.Exec(c.Ctx, `INSERT INTO ddcore_session (sid, "user", expires, ip, user_agent)
		VALUES ($1, $2, now() + $3::interval, $4, $5)`,
		sid, user, intervalOf(e.Cfg.Auth.SessionTTL()), from.IP, truncate(from.UserAgent, 400)); err != nil {
		return "", err
	}
	if _, err := c.Tx.Exec(c.Ctx, `UPDATE tab_user SET last_login = now() WHERE name = $1`, user); err != nil {
		return "", err
	}
	return sid, nil
}

func (e *Engine) Logout(ctx context.Context, sid string) {
	e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_session WHERE sid = $1`, sid)
}

// UserFromSession resolves a session id into a user name.
func (e *Engine) UserFromSession(ctx context.Context, sid string) (string, error) {
	if sid == "" {
		return "", nil
	}
	if v, ok := e.Cache.Get("sid:" + sid); ok {
		return v.(string), nil
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT "user" FROM ddcore_session WHERE sid = $1 AND expires > now()`, sid)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	u := db.Str(rows[0]["user"])
	e.Cache.Set("sid:"+sid, u, time.Minute)
	go e.DB.Pool.Exec(context.Background(), `UPDATE ddcore_session SET last_seen = now() WHERE sid = $1`, sid)
	return u, nil
}

// UserFromAPIKey validates "key:secret". Only the key row is cached (60s,
// cleared when the API Key doc changes); the secret is verified every call.
func (e *Engine) UserFromAPIKey(ctx context.Context, token string) (string, error) {
	key, secret, ok := strings.Cut(token, ":")
	if !ok {
		return "", nil
	}
	var row map[string]any
	if v, ok := e.Cache.Get("apikey:" + key); ok {
		row = v.(map[string]any)
	} else {
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT k."user", k.secret_hash, k.enabled, k.expires, u.enabled AS user_enabled
			FROM tab_api_key k JOIN tab_user u ON u.name = k."user" WHERE k.name = $1`, key)
		if err != nil || len(rows) == 0 {
			return "", err
		}
		row = rows[0]
		e.Cache.Set("apikey:"+key, row, time.Minute)
	}
	if en, ok := row["enabled"].(bool); ok && !en {
		return "", nil
	}
	// a key belonging to a disabled user is invalid
	if en, ok := row["user_enabled"].(bool); ok && !en {
		return "", nil
	}
	// An expiry inside the 60s cache window is honoured up to a minute late —
	// the same property `enabled` already has, and the reason the User
	// controller drops apikey: cache entries by hand when it disables someone.
	// A key that expires on a schedule does not need that urgency.
	if exp, ok := asTime(row["expires"]); ok && !exp.IsZero() && time.Now().After(exp) {
		return "", nil
	}
	// An API key is Argon2-verified on every request by design, which makes a
	// loop with a real key id and a junk secret the same 64 MiB amplifier the
	// login endpoint was. Brake it before hashing.
	//
	// The counter lives in the process-local cache rather than in Postgres,
	// and that is the right trade here: this is a cost brake, not a security
	// boundary — the boundary is the secret itself — and a row written per bad
	// request would be worse than the problem it answers.
	fails := "apikeyfail:" + key
	if n, ok := e.Cache.Get(fails); ok && n.(int) >= 20 {
		return "", nil
	}
	if !CheckPassword(db.Str(row["secret_hash"]), secret) {
		n, _ := e.Cache.Get(fails)
		count, _ := n.(int)
		e.Cache.Set(fails, count+1, time.Minute)
		return "", nil
	}
	e.Cache.Del(fails)
	go e.DB.Pool.Exec(context.Background(), `UPDATE tab_api_key SET last_used = now() WHERE name = $1`, key)
	return db.Str(row["user"]), nil
}

// CreateAPIKey issues a key for a user and returns "key:secret".
func (e *Engine) CreateAPIKey(ctx context.Context, user, label string) (string, error) {
	var token string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		out, err := e.CreateAPIKeyFor(c, user, label, 0)
		if err != nil {
			return err
		}
		token = out["token"].(string)
		return nil
	})
	return token, err
}

// CreateAPIKeyFor mints a key on an existing transaction, and is the shape the
// self-service service needs. days of 0 falls back to the site's apiKeyDays,
// which is itself 0 for "never expires".
//
// The secret is returned once and never again: only its hash is stored, which
// is the same reason a lost key has to be replaced rather than looked up.
func (e *Engine) CreateAPIKeyFor(c *Ctx, user, label string, days int) (map[string]any, error) {
	secret := RandomToken()
	doc := Doc{"user": user, "label": label, "secret_hash": HashPassword(secret), "enabled": true}
	if days <= 0 {
		days = e.Cfg.Auth.APIKeyDays
	}
	var expires any
	if days > 0 {
		expires = time.Now().Add(time.Duration(days) * 24 * time.Hour)
		doc["expires"] = expires
	}
	d, err := c.NewDoc("API Key", doc)
	if err != nil {
		return nil, err
	}
	saved, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name": saved.Name(), "token": saved.Name() + ":" + secret, "expires": expires,
	}, nil
}

// SetPassword sets a user's password directly, and ends their other sessions.
//
// This is raw SQL by design — it must work for a user nobody may write, and
// during recovery there is no session to act as — so User.onUpdate does not
// run and the revocation has to happen here. It is the path `ddcore user
// passwd`, a recovery token and an invitation all take, and every one of them
// is a moment where the old sessions should stop being trusted.
//
// exceptSid spares one session: the person changing their own password keeps
// the tab they typed it in. Pass "" to end them all, which is what a recovery
// does, because the account may be the thing that was compromised.
func (e *Engine) SetPassword(ctx context.Context, user, password string) error {
	return e.SetPasswordExcept(ctx, user, password, "")
}

func (e *Engine) SetPasswordExcept(ctx context.Context, user, password, exceptSid string) error {
	hash, err := e.HashNewPassword(user, password)
	if err != nil {
		return err
	}
	return e.Run(ctx, "Admin", func(c *Ctx) error {
		tag, err := c.Tx.Exec(ctx, `UPDATE tab_user SET password_hash = $2 WHERE name = $1`, user, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return cerr.NotFound("User {0} does not exist", user)
		}
		if _, err := e.DropSessions(ctx, c.Tx, user, exceptSid); err != nil {
			return err
		}
		// A changed password also clears the lockout: the person who just
		// proved they can set it should not then be told to wait.
		e.ClearAttempts(ctx, throttleKey("login", user))
		return nil
	})
}

// intervalOf renders a duration for Postgres. Seconds keep one unit for every
// TTL the policy can express, so nothing converts days twice.
func intervalOf(d time.Duration) string {
	return strconv.FormatInt(int64(d/time.Second), 10) + " seconds"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// DropSessions ends a user's sessions and returns how many it ended. A
// non-empty exceptSid is spared, which is what lets someone change their own
// password without being thrown out of the tab they typed it in.
//
// The cached sid has to go with the row: UserFromSession answers from a
// one-minute cache, so deleting the row alone would leave a revoked session
// working for up to a minute.
func (e *Engine) DropSessions(ctx context.Context, q db.Querier, user, exceptSid string) (int, error) {
	rows, err := db.Select(ctx, q, `SELECT sid FROM ddcore_session WHERE "user" = $1 AND sid <> $2`, user, exceptSid)
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		e.Cache.Del("sid:" + db.Str(r["sid"]))
	}
	tag, err := q.Exec(ctx, `DELETE FROM ddcore_session WHERE "user" = $1 AND sid <> $2`, user, exceptSid)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// RandomToken returns a 48-char hex token (session ids, API secrets, file
// names).
func RandomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var _ = fmt.Sprintf
