package engine

import (
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// These exist because ddcore.db.sql is read-only and a self-service write
// has nowhere else to go: no TS can touch ddcore_session or
// ddcore_auth_token. They take the user explicitly and the callers in
// core/services check that it is the session's own.
func init() {
	addHostOps(map[string]hostOp{
		"auth.sessions":       hostAuthSessions,
		"auth.revokeSessions": hostRevokeSessions,
		"auth.currentSid":     func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) { return c.Sid, nil },
		"auth.checkPassword": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			rows, err := db.Select(c.Ctx, c.Q(), `SELECT password_hash FROM tab_user WHERE id = $1`, a.User)
			if err != nil || len(rows) == 0 {
				return false, err
			}
			return CheckPassword(db.Str(rows[0]["password_hash"]), a.Password), nil
		},
		"auth.setPassword": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, e.SetPasswordExcept(c.Ctx, a.User, a.Password, a.ExceptSid)
		},
		// Routed through the policy so that the User form, `ddcore user add`
		// and any app that writes new_password all meet the same minimum. The
		// returned *cerr.Error surfaces in TS as a throw, like ddcore.throw.
		"hashPassword": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.HashNewPassword(a.User, a.Text)
		},
		"dropSessions": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			_, err := e.DropSessions(c.Ctx, c.Q(), a.User, "")
			return nil, err
		},
		"users.invite": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.InviteUser(c, Invitation{Email: a.Email, FullName: a.FullName, Roles: a.Roles, UserType: a.UserType})
		},
		"users.resendInvite": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return e.ResendInvite(c, a.User) },
		"users.createApiKey": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.CreateUserAPIKey(c, a.User, a.Label, int(a.Days), a.Prefix)
		},
		"idp.queueSync": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, e.QueueIdPSync(c, a.User, a.Before, a.Roles)
		},
		"idp.sync": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return nil, e.IdPSync(c, a.User) },
		"idp.setDisabled": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.SetIdPDisabled(c, a.User, a.Email, a.Disabled)
		},
		"idp.status": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) { return e.IdPStatus(), nil },
		"auth.startRecovery": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			rec, err := e.StartRecovery(c, a.User, orDefault(a.Kind, TokenReset), db.Str(c.Request["ip"]))
			if err != nil {
				return nil, err
			}
			return map[string]any{"link": rec.Link, "expires": rec.Expires, "delivered": rec.Delivered}, nil
		},
		"auth.throttle": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			limit := int(a.Limit)
			if limit <= 0 {
				limit = e.Cfg.Auth.MaxLoginAttempts
			}
			window := time.Duration(a.Minutes) * time.Minute
			if window <= 0 {
				window = e.Cfg.Auth.LockoutWindow()
			}
			if err := e.CheckThrottle(c.Ctx, a.Key, limit, window); err != nil {
				return nil, err
			}
			// Written on the pool, outside this request's transaction: an attempt
			// recorded on c.Tx would be rolled back by the error it counts.
			e.RecordAttempt(c.Ctx, a.Key, db.Str(c.Request["ip"]), false)
			return nil, nil
		},
		"auth.clearAttempts": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return e.ClearAttempts(c.Ctx, a.Key) },
		"auth.clearCredentialAttempts": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.ClearCredentialAttempts(c, a.User)
		},
		"auth.createAPIKey": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return e.CreateAPIKeyFor(c, a.User, a.Label, int(a.Days), "")
		},
		// Not db.getList: that checks the doctype's permissions, and API Key
		// is System Manager only — deliberately, since read there would be
		// read on everyone's keys. Scoped to the one user instead.
		"auth.apiKeys": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return db.Select(c.Ctx, c.Q(),
				`SELECT id, label, enabled, creation, last_used, expires FROM tab_api_key
			 WHERE "user" = $1 ORDER BY creation DESC LIMIT 100`, a.User)
		},
		"auth.revokeAPIKey": hostRevokeAPIKey,
	})
}

func hostAuthSessions(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
	rows, err := db.Select(c.Ctx, e.DB.Pool,
		`SELECT sid, ip, user_agent, created, last_seen, expires FROM ddcore_session
		 WHERE "user" = $1 AND expires > now() ORDER BY last_seen DESC`, a.User)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		sid := db.Str(r["sid"])
		// The raw sid never leaves: it is a bearer token, and one XSS
		// reading this list would otherwise own every device the person
		// is signed in on. The handle is enough to revoke by and useless
		// to replay.
		out = append(out, map[string]any{
			"id": TokenHandle(sid), "current": sid == c.Sid,
			"ip": r["ip"], "userAgent": r["user_agent"],
			"created": r["created"], "lastSeen": r["last_seen"], "expires": r["expires"],
		})
	}
	return out, nil
}

func hostRevokeSessions(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
	if handle := a.id(); handle != "" {
		// Revoking one, addressed by handle. Scoped to the user, so a
		// handle guessed from someone else's list reaches nothing.
		rows, err := db.Select(c.Ctx, e.DB.Pool,
			`SELECT sid FROM ddcore_session WHERE "user" = $1`, a.User)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			sid := db.Str(r["sid"])
			if TokenHandle(sid) != handle {
				continue
			}
			n, err := e.endSessions(c.Ctx, e.DB.Pool, sid)
			return orErr(map[string]any{"revoked": n}, err)
		}
		return map[string]any{"revoked": 0}, nil
	}
	n, err := e.DropSessions(c.Ctx, e.DB.Pool, a.User, a.ExceptSid)
	return orErr(map[string]any{"revoked": n}, err)
}

func hostRevokeAPIKey(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
	// The owner is part of the WHERE, so a name from someone else's list
	// deletes nothing rather than deleting theirs.
	tag, err := c.Q().Exec(c.Ctx, `DELETE FROM tab_api_key WHERE id = $1 AND "user" = $2`, a.id(), a.User)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, cerr.NotFound("That key is not yours")
	}
	if err := c.invalidateNow([]string{"apikey:" + a.id()}, nil); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
