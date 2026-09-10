package engine

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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

// Login checks credentials and creates a session, returning the sid.
func (e *Engine) Login(ctx context.Context, user, password string) (string, error) {
	var sid string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		rows, err := db.Select(ctx, c.Tx, `SELECT name, password_hash, enabled FROM tab_user WHERE lower(name) = lower($1) OR lower(email) = lower($1) LIMIT 1`, strings.TrimSpace(user))
		if err != nil {
			return err
		}
		if len(rows) == 0 || !CheckPassword(db.Str(rows[0]["password_hash"]), password) {
			return cerr.Auth("Usuário ou senha inválidos")
		}
		if en, ok := rows[0]["enabled"].(bool); ok && !en {
			return cerr.Auth("Usuário desativado")
		}
		name := db.Str(rows[0]["name"])
		sid = RandomToken()
		if _, err := c.Tx.Exec(ctx, `INSERT INTO ddcore_session (sid, "user", expires) VALUES ($1, $2, now() + interval '30 days')`, sid, name); err != nil {
			return err
		}
		_, err = c.Tx.Exec(ctx, `UPDATE tab_user SET last_login = now() WHERE name = $1`, name)
		return err
	})
	return sid, err
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
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT k."user", k.secret_hash, k.enabled, u.enabled AS user_enabled
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
	// uma chave de um usuário desativado não vale nada
	if en, ok := row["user_enabled"].(bool); ok && !en {
		return "", nil
	}
	if !CheckPassword(db.Str(row["secret_hash"]), secret) {
		return "", nil
	}
	go e.DB.Pool.Exec(context.Background(), `UPDATE tab_api_key SET last_used = now() WHERE name = $1`, key)
	return db.Str(row["user"]), nil
}

// CreateAPIKey issues a key for a user and returns "key:secret".
func (e *Engine) CreateAPIKey(ctx context.Context, user, label string) (string, error) {
	secret := RandomToken()
	var token string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.NewDoc("API Key", Doc{"user": user, "label": label, "secret_hash": HashPassword(secret), "enabled": true})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		token = saved.Name() + ":" + secret
		return nil
	})
	return token, err
}

// SetPassword sets a user's password directly.
func (e *Engine) SetPassword(ctx context.Context, user, password string) error {
	return e.Run(ctx, "Administrator", func(c *Ctx) error {
		tag, err := c.Tx.Exec(ctx, `UPDATE tab_user SET password_hash = $2 WHERE name = $1`, user, HashPassword(password))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return cerr.NotFound("Usuário {0} não existe", user)
		}
		return nil
	})
}

// RandomToken returns a 48-char hex token (session ids, API secrets, file
// names).
func RandomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var _ = fmt.Sprintf
