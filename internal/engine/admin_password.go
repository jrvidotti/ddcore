package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/jrvidotti/ddcore/internal/db"
)

// adminPasswordLength is the generated password's length, raised to the site's
// minimum when that is longer.
const adminPasswordLength = 10

// The alphabet leaves out look-alikes (0/O, 1/l/I) and the characters a shell
// would expand, so the password can be read off a console and typed back
// into `ddcore user passwd` as it is.
const (
	pwLower   = "abcdefghijkmnopqrstuvwxyz"
	pwUpper   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwDigits  = "23456789"
	pwSymbols = "@%-_+="
)

// ensureAdminPassword gives Admin a password when it has none, which is what a
// first install leaves: a superuser nobody can sign in as. The deployment's
// DDCORE_ADMIN_PASSWORD is used when set — a container has no console to read
// a generated one from — and is never shown; otherwise one is generated and
// returned in clear, once, for the caller to show. Only the hash is kept, and
// a password Admin already has is never replaced.
//
// A test engine is left alone — its tests sign in however they like, and an
// Argon2 hash per setup is time they would pay for nothing.
func ensureAdminPassword(ctx context.Context, c *Ctx) (string, error) {
	if c.E.Cfg.Test {
		return "", nil
	}
	if _, err := c.St.DocType("User"); err != nil {
		return "", nil
	}
	rows, err := db.Select(ctx, c.Tx, `SELECT 1 FROM tab_user WHERE id = 'Admin' AND coalesce(password_hash, '') = ''`)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	pw, shown := c.E.Cfg.AdminPassword, ""
	if pw == "" {
		n := adminPasswordLength
		if m := c.E.Cfg.Auth.MinPasswordLength; m > n {
			n = m
		}
		if pw, err = generatePassword(n); err != nil {
			return "", err
		}
		shown = pw
	}
	hash, err := c.E.HashNewPassword("Admin", pw)
	if err != nil {
		if shown == "" {
			return "", fmt.Errorf("DDCORE_ADMIN_PASSWORD: %w", err)
		}
		return "", err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE tab_user SET password_hash = $1 WHERE id = 'Admin'`, hash); err != nil {
		return "", err
	}
	if shown == "" {
		c.E.Log.Info("Admin password set from DDCORE_ADMIN_PASSWORD")
	}
	return shown, nil
}

// generatePassword returns n characters with at least one of each class.
func generatePassword(n int) (string, error) {
	classes := []string{pwLower, pwUpper, pwDigits, pwSymbols}
	all := pwLower + pwUpper + pwDigits + pwSymbols
	out := make([]byte, n)
	for i := range out {
		set := all
		if i < len(classes) {
			set = classes[i]
		}
		ch, err := randIndex(len(set))
		if err != nil {
			return "", err
		}
		out[i] = set[ch]
	}
	// Shuffle, so the guaranteed classes are not always the first four.
	for i := n - 1; i > 0; i-- {
		j, err := randIndex(i + 1)
		if err != nil {
			return "", err
		}
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

func randIndex(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}
