package engine

import (
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Linked identities, managed by hand.
//
// A sign-in links a provider's subject to a User on its own, by the verified
// address. Some providers do not vouch for the address — Keycloak federating a
// directory that lets its admins edit it — and tell applications to key people
// by subject instead. These functions are how an administrator says "this
// User is that subject" ahead of the first sign-in (#165): the site decides
// the link, so nothing about the address has to be trusted.

// identityUser resolves what an administrator typed — a User's id or its
// address — to the User, in the space of c.
func identityUser(c *Ctx, typed string) (string, error) {
	typed = strings.TrimSpace(typed)
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id FROM tab_user
		WHERE lower(id) = lower($1) OR lower(email) = lower($1) ORDER BY (lower(id) = lower($1)) DESC LIMIT 1`, typed)
	if err != nil {
		return "", err
	}
	if typed == "" || len(rows) == 0 {
		return "", cerr.NotFound("User {0} does not exist", typed)
	}
	return db.Str(rows[0]["id"]), nil
}

// LinkIdentity links a provider's subject to a User, so that the next sign-in
// carrying it lands on that account whatever the token says about the address.
// Linking the same pair again changes nothing.
func (e *Engine) LinkIdentity(c *Ctx, user, provider, subject string) (map[string]any, error) {
	if !c.canAdministerUsers() {
		return nil, cerr.Permission("Only a System Manager can link an identity to an account")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	subject = strings.TrimSpace(subject)
	if _, ok := e.OIDCProvider(provider); !ok {
		return nil, cerr.Validation("Sign-in provider {0} is not configured", provider)
	}
	if subject == "" {
		return nil, cerr.Validation("The subject of the identity is required")
	}
	name, err := identityUser(c, user)
	if err != nil {
		return nil, err
	}
	if name == "Guest" {
		return nil, cerr.Validation("Guest cannot sign in through a provider")
	}
	out := map[string]any{"user": name, "provider": provider, "subject": subject, "linked": true}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT "user" FROM ddcore_user_identity WHERE provider = $1 AND subject = $2`, provider, subject)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		other := db.Str(rows[0]["user"])
		if other == name {
			out["linked"] = false
			return out, nil
		}
		// The identities are the site's, and the other account may belong to
		// another tenant: it is named only when this space can see it. A row
		// left pointing at nobody is replaced.
		var exists bool
		if err := e.System(c.Ctx, func(q db.Querier) error {
			return q.QueryRow(c.Ctx, `SELECT EXISTS (SELECT 1 FROM tab_user WHERE id = $1)`, other).Scan(&exists)
		}); err != nil {
			return nil, err
		}
		if exists {
			seen, err := db.Select(c.Ctx, c.Q(), `SELECT 1 FROM tab_user WHERE id = $1`, other)
			if err != nil {
				return nil, err
			}
			if len(seen) == 0 {
				return nil, cerr.Duplicate("This identity is already linked to another account")
			}
			return nil, cerr.Duplicate("This identity is already linked to {0}", other)
		}
		if _, err := c.Q().Exec(c.Ctx, `DELETE FROM ddcore_user_identity WHERE provider = $1 AND subject = $2`, provider, subject); err != nil {
			return nil, err
		}
	}
	if _, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_user_identity (provider, subject, "user") VALUES ($1, $2, $3)`,
		provider, subject, name); err != nil {
		return nil, err
	}
	if err := c.Audit("account.identity_link", "User", name, map[string]any{"provider": provider, "subject": subject}); err != nil {
		return nil, err
	}
	return out, nil
}

// UnlinkIdentity removes a User's link to a provider: the one subject named,
// or every one it has there when subject is empty. The provider need not be
// configured any more — a link may outlive the provider it was made for.
func (e *Engine) UnlinkIdentity(c *Ctx, user, provider, subject string) (map[string]any, error) {
	if !c.canAdministerUsers() {
		return nil, cerr.Permission("Only a System Manager can unlink an identity from an account")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	subject = strings.TrimSpace(subject)
	if provider == "" {
		return nil, cerr.Validation("The sign-in provider is required")
	}
	name, err := identityUser(c, user)
	if err != nil {
		return nil, err
	}
	rows, err := db.Select(c.Ctx, c.Q(), `DELETE FROM ddcore_user_identity
		WHERE provider = $1 AND "user" = $2 AND ($3 = '' OR subject = $3) RETURNING subject`, provider, name, subject)
	if err != nil {
		return nil, err
	}
	subjects := make([]string, 0, len(rows))
	for _, r := range rows {
		subjects = append(subjects, db.Str(r["subject"]))
	}
	if len(subjects) > 0 {
		if err := c.Audit("account.identity_unlink", "User", name, map[string]any{"provider": provider, "subjects": subjects}); err != nil {
			return nil, err
		}
	}
	return map[string]any{"user": name, "provider": provider, "removed": len(subjects)}, nil
}

// UserIdentities lists the identities linked to a User.
func (e *Engine) UserIdentities(c *Ctx, user string) ([]map[string]any, error) {
	if !c.canAdministerUsers() {
		return nil, cerr.Permission("Only a System Manager can list the identities of an account")
	}
	name, err := identityUser(c, user)
	if err != nil {
		return nil, err
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT provider, subject, email, created, last_login FROM ddcore_user_identity
		WHERE "user" = $1 ORDER BY provider, created`, name)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}
