package engine

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Impersonation lets an administrator's session work as another user: the
// desk, the permissions and the scopes are the target's, and so are the
// owner and modified_by of what gets written. It is kept on the
// administrator's own session row, the way an entered tenant is, rather than
// in a new session of the target's: that would touch the target's last
// login, show up in their own list of sessions, and be theirs to revoke.
// Every audit event and Version written meanwhile names the administrator in
// impersonated_by.

type impersonatorKey struct{}

// WithImpersonator records that the work under ctx is done by real on behalf
// of the user it runs as. NewCtx reads it into Ctx.ImpersonatedBy.
func WithImpersonator(ctx context.Context, real string) context.Context {
	return context.WithValue(ctx, impersonatorKey{}, real)
}

// ImpersonatorFrom is the administrator WithImpersonator put in ctx; empty
// when the work is nobody's impersonation.
func ImpersonatorFrom(ctx context.Context) string {
	v, _ := ctx.Value(impersonatorKey{}).(string)
	return v
}

func impersonationKey(sid string) string { return "sidas:" + sid }

// SessionImpersonation is the user a session is impersonating; empty when it
// impersonates nobody.
func (e *Engine) SessionImpersonation(ctx context.Context, sid string) string {
	if sid == "" {
		return ""
	}
	key := impersonationKey(sid)
	if v, ok := e.Cache.Get(key); ok {
		return v.(string)
	}
	var target string
	if err := e.DB.Pool.QueryRow(ctx, `SELECT coalesce(data->>'impersonate', '') FROM ddcore_session WHERE sid = $1`, sid).Scan(&target); err != nil {
		return ""
	}
	e.Cache.Set(key, target, time.Minute)
	return target
}

// MayImpersonate reports whether user is still someone who may impersonate:
// Admin, or a System Manager. It is asked on every impersonated request, so
// that revoking the role ends an impersonation already under way.
func (e *Engine) MayImpersonate(ctx context.Context, user string) bool {
	switch user {
	case "Admin":
		return true
	case "", "Guest":
		return false
	}
	c := e.NewCtx(ctx, user)
	if c.IsWebsiteUser() {
		return false
	}
	roles, err := c.RolesOf(user)
	if err != nil {
		return false
	}
	for _, r := range roles {
		if r == "System Manager" {
			return true
		}
	}
	return false
}

// StartImpersonation makes this ctx's session work as target from the next
// request on.
func (c *Ctx) StartImpersonation(target string) error {
	if c.Sid == "" {
		return cerr.Validation("Impersonating needs a session; an API key cannot impersonate")
	}
	if c.ImpersonatedBy != "" {
		return cerr.Validation("Stop impersonating {0} first", c.User)
	}
	if !c.canAdministerUsers() {
		c.AuditDenied("account.impersonate", "User", target, nil)
		return cerr.Permission("Only a System Manager can impersonate a user")
	}
	switch target {
	case "":
		return cerr.Validation("Impersonating needs a user")
	case "Admin", "Guest":
		return cerr.Permission("{0} cannot be impersonated", target)
	case c.User:
		return cerr.Validation("You cannot impersonate yourself")
	}
	// across spaces: a tenant's user is not a row of the platform space
	var enabled bool
	if err := c.E.System(c.Ctx, func(q db.Querier) error {
		return q.QueryRow(c.Ctx, `SELECT enabled FROM tab_user WHERE id = $1`, target).Scan(&enabled)
	}); errors.Is(err, pgx.ErrNoRows) {
		return cerr.NotFound("User {0} does not exist", target)
	} else if err != nil {
		return err
	}
	// The spaces follow enterUserSpace: from inside a tenant only that
	// tenant's users, and a tenant's user from the platform space only by
	// someone who may enter tenants at all.
	tenant, err := c.E.TenantOfUser(c.Ctx, target)
	if err != nil {
		return err
	}
	if c.Tenancy() && tenant != c.Tenant {
		// from inside a tenant, another space's account is not there to find
		if c.Tenant != "" {
			return cerr.NotFound("User {0} does not exist", target)
		}
		if ok, err := c.E.CanEnterTenants(c.Ctx, c.User); err != nil {
			return err
		} else if !ok {
			c.AuditDenied("account.impersonate", "User", target, nil)
			return cerr.Permission("Only a System Manager of the platform space can impersonate a tenant's user")
		}
	}
	if !enabled {
		return cerr.Permission("User {0} is disabled", target)
	}
	if _, err := c.E.DB.Pool.Exec(c.Ctx, `UPDATE ddcore_session
		SET data = coalesce(data, '{}'::jsonb) || jsonb_build_object('impersonate', $2::text) WHERE sid = $1`, c.Sid, target); err != nil {
		return err
	}
	if err := c.E.forgetImpersonations(c.Ctx, c.E.DB.Pool, []string{c.Sid}); err != nil {
		return err
	}
	return c.E.RecordAudit(c.Ctx, c.User, "account.impersonate", "Allowed", "User", target, nil)
}

// StopImpersonation returns the session sid, impersonating target on behalf
// of real, to real.
func (e *Engine) StopImpersonation(ctx context.Context, sid, real, target string) error {
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_session SET data = data - 'impersonate' WHERE sid = $1`, sid); err != nil {
		return err
	}
	if err := e.forgetImpersonations(ctx, e.DB.Pool, []string{sid}); err != nil {
		return err
	}
	return e.RecordAudit(ctx, real, "account.impersonate_stop", "Allowed", "User", target, nil)
}

// endImpersonationsOf stops every impersonation of user: what DropSessions
// does to the user's own sessions, it does to the ones working as them.
func (e *Engine) endImpersonationsOf(ctx context.Context, q db.Querier, user string) error {
	rows, err := db.Select(ctx, q, `UPDATE ddcore_session SET data = data - 'impersonate'
		WHERE data->>'impersonate' = $1 RETURNING sid`, user)
	if err != nil || len(rows) == 0 {
		return err
	}
	sids := make([]string, len(rows))
	for i, r := range rows {
		sids[i] = db.Str(r["sid"])
	}
	return e.forgetImpersonations(ctx, q, sids)
}

// forgetImpersonations drops the cached impersonation of sids here and in the
// other processes.
func (e *Engine) forgetImpersonations(ctx context.Context, q db.Querier, sids []string) error {
	keys := make([]string, len(sids))
	for i, sid := range sids {
		keys[i] = impersonationKey(sid)
		e.Cache.Del(keys[i])
	}
	return broadcastInvalidation(ctx, q, keys, nil)
}

// refuseWhileImpersonating guards what would outlive an impersonation: a
// credential minted, a password changed or sessions revoked in the target's
// name are the target's to decide.
func (c *Ctx) refuseWhileImpersonating() error {
	if c.ImpersonatedBy != "" {
		return cerr.Permission("Not available while impersonating")
	}
	return nil
}
