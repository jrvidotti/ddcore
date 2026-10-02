package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Tenancy keeps several tenants in one database (docs/agent/tenancy.md).
//
// Every row of a tenant-owned table carries the space it was written in: a
// tenant's id, or the empty string for the platform space, where a site's
// rows were before it had tenants. A Ctx works in exactly one space. Its
// transaction runs as a role that owns nothing, with the space in a setting
// the tables' row-level-security policy reads — so the statements of the
// engine, of an app's raw SQL and of a background job all meet the same wall
// without any of them naming the tenant.
//
// The few things that must cross spaces — a migration, finding a user at
// sign-in, claiming a job — say so, with RunSystem, System or elevated.

type tenantKey struct{}

// WithTenant names the tenant the work under ctx enters. Only a user of the
// platform space may enter one: for anyone else the space is the user's own,
// and naming a different one is refused.
func WithTenant(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantKey{}, id)
}

// TenantFrom is the tenant WithTenant put in ctx, if any.
func TenantFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(tenantKey{}).(string)
	return id, ok
}

func (e *Engine) tenantRole() string {
	if e.Cfg.TenantRole != "" {
		return e.Cfg.TenantRole
	}
	return db.DefaultTenantRole
}

func (e *Engine) openDB(ctx context.Context) (*db.DB, error) {
	role := ""
	if e.Cfg.Tenancy {
		role = e.tenantRole()
	}
	d, err := db.OpenConfined(ctx, e.Cfg.DSN, role)
	if err != nil {
		return nil, err
	}
	if !e.Cfg.Tenancy {
		applied, err := db.TenancyApplied(ctx, d.Pool)
		if err == nil && applied {
			err = errors.New(`this database has tenancy on, which cannot be undone: set "tenancy": true in ddcore.json`)
		}
		if err != nil {
			d.Close()
			return nil, err
		}
	}
	return d, nil
}

// tenancy reports whether transactions are to be confined: the site asks for
// tenancy and a migration has applied it. Between the two — a binary started
// with the switch on, not migrated yet — there is no role and no policy, and
// the answer is asked again on every transaction until it is yes.
func (e *Engine) tenancy(ctx context.Context) (bool, error) {
	if !e.Cfg.Tenancy || e.DB == nil {
		return false, nil
	}
	if e.tenancyReady.Load() {
		return true, nil
	}
	applied, err := db.TenancyApplied(ctx, e.DB.Pool)
	if err != nil {
		return false, err
	}
	if applied {
		e.tenancyReady.Store(true)
	}
	return applied, nil
}

// RunSystem is Run for the framework's own work that has to see every space.
// The transaction is not held by row-level security, so each statement on a
// tenant table says which tenant it means — an id alone names a row in every
// tenant that has one.
func (e *Engine) RunSystem(ctx context.Context, user string, fn func(c *Ctx) error) error {
	c := e.NewCtx(ctx, user)
	c.system = true
	return c.Run(fn)
}

// System runs fn on the pool that is not held by row-level security. On a
// site without tenancy that is the one pool there is. A statement run here
// on a tenant table addresses its row by something unique across the site —
// a user's id, a job's number, a random key — or names its tenant.
func (e *Engine) System(ctx context.Context, fn func(q db.Querier) error) error {
	return fn(e.DB.Sys)
}

// InSpace runs fn on a querier confined to one space, for the framework's
// work that has no Ctx: a worker's status update, an audit row. Without
// tenancy it is the pool.
func (e *Engine) InSpace(ctx context.Context, tenant string, fn func(q db.Querier) error) error {
	on, err := e.tenancy(ctx)
	if err != nil {
		return err
	}
	if !on {
		return fn(e.DB.Pool)
	}
	tx, err := e.DB.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := db.Confine(ctx, tx, e.tenantRole()); err != nil {
		return err
	}
	if err := db.SetTenant(ctx, tx, tenant); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// enterSpace is the first thing a transaction does: decide the space the ctx
// works in and confine the transaction to it.
func (c *Ctx) enterSpace() error {
	on, err := c.E.tenancy(c.Ctx)
	if err != nil || !on {
		return err
	}
	if !c.spaced && !c.system {
		tenant, err := c.E.spaceOf(c.Ctx, c.User)
		if err != nil {
			return err
		}
		c.Tenant = tenant
	}
	return c.applySpace()
}

// applySpace makes the transaction agree with the ctx: its role and its
// tenant setting. It is issued again whenever something may have changed
// them behind the ctx's back — a rollback to a savepoint takes back every
// SET LOCAL made after it.
func (c *Ctx) applySpace() error {
	if c.Tx == nil {
		return nil
	}
	on, err := c.E.tenancy(c.Ctx)
	if err != nil || !on {
		return err
	}
	if c.system {
		if err := db.Elevate(c.Ctx, c.Tx); err != nil {
			return err
		}
	} else if err := db.Confine(c.Ctx, c.Tx, c.E.tenantRole()); err != nil {
		return err
	}
	return db.SetTenant(c.Ctx, c.Tx, c.Tenant)
}

// elevated runs fn with the transaction out of row-level security, then puts
// the ctx's own confinement back. For a statement that has to look across
// spaces in the middle of ordinary work; it names its tenant or reads a
// table whose ids are site-wide.
func (c *Ctx) elevated(fn func() error) error {
	on, err := c.E.tenancy(c.Ctx)
	if err != nil {
		return err
	}
	if !on || c.system || c.Tx == nil {
		return fn()
	}
	if err := db.Elevate(c.Ctx, c.Tx); err != nil {
		return err
	}
	ferr := fn()
	if ferr != nil {
		// the transaction may be aborted, in which case nothing more runs on
		// it anyway; otherwise the confinement must be back before anything does
		_ = db.Confine(c.Ctx, c.Tx, c.E.tenantRole())
		return ferr
	}
	return db.Confine(c.Ctx, c.Tx, c.E.tenantRole())
}

// confined is the reverse, for a system ctx about to run document code: a
// fixture or an app hook inside a migration works in the platform space like
// any other code, and must not find a tenant's row because it shares an id.
func (c *Ctx) confined(fn func() error) error {
	on, err := c.E.tenancy(c.Ctx)
	if err != nil {
		return err
	}
	if !on || !c.system || c.Tx == nil {
		return fn()
	}
	if err := db.Confine(c.Ctx, c.Tx, c.E.tenantRole()); err != nil {
		return err
	}
	ferr := fn()
	if err := db.Elevate(c.Ctx, c.Tx); err != nil && ferr == nil {
		return err
	}
	return ferr
}

// Tenancy reports whether this ctx is on a site with tenancy applied.
func (c *Ctx) Tenancy() bool {
	if c.E == nil {
		return false
	}
	on, _ := c.E.tenancy(c.Ctx)
	return on
}

const (
	userTenantTTL  = 5 * time.Minute
	tenantStateTTL = 10 * time.Second
)

// spaceOf decides the space a user's work runs in: the user's own tenant, or
// for a user of the platform space the tenant WithTenant named. A tenant
// that is disabled, or does not exist, admits nobody.
func (e *Engine) spaceOf(ctx context.Context, user string) (string, error) {
	own, err := e.TenantOfUser(ctx, user)
	if err != nil {
		return "", err
	}
	space := own
	if named, ok := e.namedTenant(ctx); ok && named != own {
		if own != "" {
			return "", cerr.Permission("A user of one tenant cannot work in another")
		}
		space = named
	}
	if space == "" {
		return "", nil
	}
	if err := e.checkTenant(ctx, nil, space); err != nil {
		return "", err
	}
	return space, nil
}

// TenantOfUser is the tenant a user belongs to; empty for Admin, Guest, a
// user of the platform space, and a user that does not exist.
func (e *Engine) TenantOfUser(ctx context.Context, user string) (string, error) {
	switch user {
	case "", "Admin", "Guest":
		return "", nil
	}
	if on, err := e.tenancy(ctx); err != nil || !on {
		return "", err
	}
	key := "tenant:" + user
	if v, ok := e.Cache.Get(key); ok {
		return v.(string), nil
	}
	gen := e.Cache.Gen()
	var tenant string
	err := e.System(ctx, func(q db.Querier) error {
		err := q.QueryRow(ctx, `SELECT tenant FROM tab_user WHERE id = $1`, user).Scan(&tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoSuchUser
		}
		return err
	})
	if errors.Is(err, errNoSuchUser) {
		// not cached: the user may be created a moment from now
		return "", nil
	}
	if err != nil {
		return "", err
	}
	e.Cache.SetAt(key, tenant, userTenantTTL, gen)
	return tenant, nil
}

var errNoSuchUser = errors.New("no such user")

// checkTenant refuses a tenant that does not exist or is disabled. Tenant is
// a shared DocType, so the lookup needs no elevation. q is the transaction
// to look in when the tenant may have been created by it; nil asks the pool,
// and only that answer is remembered.
func (e *Engine) checkTenant(ctx context.Context, q db.Querier, id string) error {
	if !db.ValidTenantID(id) {
		return cerr.Validation("Invalid tenant id: {0}", id)
	}
	key := "tenant_state:" + id
	state, ok := e.Cache.Get(key)
	if !ok || q != nil {
		gen := e.Cache.Gen()
		cache := q == nil
		if q == nil {
			q = e.DB.Pool
		}
		var enabled bool
		err := q.QueryRow(ctx, fmt.Sprintf(`SELECT enabled IS TRUE FROM %s WHERE id = $1`,
			meta.TenantTable), id).Scan(&enabled)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			state = "missing"
		case err != nil:
			return err
		case enabled:
			state = "enabled"
		default:
			state = "disabled"
		}
		if cache {
			e.Cache.SetAt(key, state, tenantStateTTL, gen)
		}
	}
	switch state {
	case "enabled":
		return nil
	case "disabled":
		return cerr.Permission("The tenant {0} is disabled", id)
	}
	return cerr.NotFound("Tenant {0} not found", id)
}

// InTenant runs fn as the same user inside a tenant, on this ctx's
// transaction. Only the platform space may enter a tenant; a ctx already in
// that tenant just runs fn.
func (c *Ctx) InTenant(id string, fn func(*Ctx) error) error {
	if !c.Tenancy() {
		return cerr.Validation("This site has no tenants: tenancy is off")
	}
	if c.Tenant == id {
		return fn(c)
	}
	if c.Tenant != "" {
		return cerr.Permission("A tenant cannot enter another tenant")
	}
	if c.Tx == nil {
		return c.E.NewCtx(WithTenant(c.Ctx, id), c.User).Run(fn)
	}
	child, err := c.enterTenantCtx(id)
	if err != nil {
		return err
	}
	ferr := fn(child)
	if _, err := child.leaveTenantCtx(); err != nil && ferr == nil {
		return err
	}
	return ferr
}

// enterTenantCtx builds the ctx that works in tenant id on this ctx's
// transaction and VM, and makes it the VM's current one; leaveTenantCtx
// undoes it. InTenant brackets a Go callback with the pair, and
// ddcore.tenant.run a TS one.
func (c *Ctx) enterTenantCtx(id string) (*Ctx, error) {
	if err := c.E.checkTenant(c.Ctx, c.Tx, id); err != nil {
		return nil, err
	}
	rt, err := c.RT()
	if err != nil {
		return nil, err
	}
	// the context names the tenant too: whatever takes it — an audit row, an
	// Error Log, a nested Run — then works where the ctx does
	child := c.E.NewCtx(WithTenant(c.Ctx, id), c.User)
	child.St, child.Tx, child.rt = c.St, c.Tx, rt
	child.txOwner = c.owner()
	child.Lang, child.ReqID, child.Sid = c.Lang, c.ReqID, c.Sid
	child.savepoint, child.roSavepoint = c.savepoint, c.roSavepoint
	child.Tenant, child.spaced = id, true
	child.tenantParent = c
	// the privileged flags travel: a job or a migration that enters a tenant
	// is still a job or a migration. Roles and scopes are read again there.
	for _, k := range []string{"ignorePermissions", bypassMaintenanceFlag, "inPatch"} {
		if v, ok := c.Flags[k]; ok {
			child.Flags[k] = v
		}
	}
	if err := child.applySpace(); err != nil {
		return nil, err
	}
	rt.Ctx = child
	return child, nil
}

// leaveTenantCtx returns the VM and the transaction to the ctx that entered
// the tenant, and returns that ctx.
func (c *Ctx) leaveTenantCtx() (*Ctx, error) {
	parent := c.tenantParent
	if parent == nil {
		return c, nil
	}
	c.tenantParent = nil
	parent.rt.Ctx = parent
	parent.Messages = append(parent.Messages, c.Messages...)
	parent.savepoint, parent.roSavepoint = c.savepoint, c.roSavepoint
	parent.docCache = map[string]Doc{}
	return parent, parent.applySpace()
}

// TenantList is the tenants of the site, for the platform space's own code:
// a scheduled method that has work in each of them.
func (c *Ctx) TenantList() ([]map[string]any, error) {
	if !c.Tenancy() {
		return []map[string]any{}, nil
	}
	if c.Tenant != "" {
		return nil, cerr.Permission("Tenants are managed from the platform space")
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, title, enabled IS TRUE AS enabled FROM `+meta.TenantTable+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

// TenantCreated runs every app's onTenantCreate inside a tenant that was just
// created. Fixtures and afterInstall fill the platform space only, so this is
// where an app gives a new tenant the records it cannot start without. The
// Tenant controller calls it, so it happens however the tenant was made.
func (c *Ctx) TenantCreated(id string) error {
	if !c.Tenancy() {
		return nil
	}
	return c.InTenant(id, func(t *Ctx) error {
		rt, err := t.RT()
		if err != nil {
			return err
		}
		return t.WithIgnorePermissions(func() error {
			for _, name := range t.St.AppOrder() {
				if t.St.Snap.Apps[name].HasOnTenantCreate {
					if err := rt.AppHook(name, "onTenantCreate"); err != nil {
						return fmt.Errorf("%s.onTenantCreate: %w", name, err)
					}
				}
			}
			return nil
		})
	})
}

// spaceRefusal is what tenancy forbids a tenant's ctx on a DocType that is
// not its own. Tenant documents are the platform's alone. A shared DocType
// is read by everyone and written from the platform space only: a tenant
// changing a row every other tenant reads would be the leak in reverse.
// Like the wall itself, it does not yield to ignorePermissions — a job runs
// with permissions ignored and is still inside its tenant.
func (c *Ctx) spaceRefusal(d *meta.DocType, write bool) error {
	if c.Tenant == "" || d.TenantOwned || d.IsVirtual() {
		return nil
	}
	if d.Name == meta.TenantDocType {
		return cerr.Permission("Tenants are managed from the platform space")
	}
	if write {
		return cerr.Permission("{0} is shared by every tenant and can only be changed from the platform space", c.T(d.Label))
	}
	return nil
}

// acrossSpaces runs fn for a document of d seeing the rows that can refer to
// it. A tenant's document is referred to from its own space, and fn runs as
// it is. A shared one is referred to from every tenant, so the check that
// nothing links to it before a delete, and the rewrite of what does on a
// rename, have to look at them all.
func (c *Ctx) acrossSpaces(d *meta.DocType, fn func() error) error {
	if d.TenantOwned {
		return fn()
	}
	return c.elevated(fn)
}

// RunAdminFor is Run as Admin in the space of user: the framework acting on
// somebody's account without that somebody being signed in — a recovery
// link, an API key issued from the command line. The documents it writes
// land in the user's tenant, where the user will look for them.
func (e *Engine) RunAdminFor(ctx context.Context, user string, fn func(c *Ctx) error) error {
	tenant, err := e.TenantOfUser(ctx, user)
	if err != nil {
		return err
	}
	if tenant != "" {
		ctx = WithTenant(ctx, tenant)
	}
	return e.Run(ctx, "Admin", fn)
}

// publish sends a document event to the subscribers of the space it happened
// in — or to everyone, when the DocType is one every tenant shares. It is
// called from an after-commit callback, so the space is read from the ctx
// and not from the transaction, which is gone by then.
func (c *Ctx) publish(ev Event) {
	ev.Tenant = c.Tenant
	doctype := ev.Doctype
	if doctype == "" {
		if p, ok := ev.Payload.(map[string]any); ok {
			doctype, _ = p["doctype"].(string)
		}
	}
	if c.St != nil && c.Tenancy() {
		if d, err := c.St.DocType(doctype); err == nil && !d.TenantOwned {
			ev.SiteWide = true
		}
	}
	c.E.Events.Publish(ev)
}

// appCacheKey is the key ddcore.cache stores an app's value under: the app's
// own key, and inside a tenant the tenant in front of it. An app caches what
// it computed from the rows it could see, so a value one tenant stored is not
// an answer for another.
func (c *Ctx) appCacheKey(key string) string {
	if c.Tenant == "" {
		return key
	}
	return "tenant:" + c.Tenant + "\x00" + key
}

// spaceQ is InSpace for the tenant ctx names: how a function that takes a
// context rather than a Ctx reads and writes where its caller works. The API
// puts every request's space in its context.
func (e *Engine) spaceQ(ctx context.Context, fn func(q db.Querier) error) error {
	tenant, _ := e.namedTenant(ctx)
	return e.InSpace(ctx, tenant, fn)
}

// spaceStatements runs single statements in one space, each in a transaction
// of its own: what the framework's pool-direct code uses in place of the
// pool, so that a job administrator sees the jobs of the tenant they work in
// and a delivery's status lands on the delivery of the tenant it belongs to.
// Without tenancy each call is the same statement on the pool.
type spaceStatements struct {
	e      *Engine
	tenant string
}

// statements is the statements of the space ctx names (see spaceQ).
func (e *Engine) statements(ctx context.Context) spaceStatements {
	tenant, _ := e.namedTenant(ctx)
	return spaceStatements{e: e, tenant: tenant}
}

// space is the statements of this ctx's space, outside its transaction: for
// what has to survive the transaction's rollback.
func (c *Ctx) space() spaceStatements { return spaceStatements{e: c.E, tenant: c.Tenant} }

func (s spaceStatements) Exec(ctx context.Context, sql string, args ...any) (tag pgconn.CommandTag, err error) {
	err = s.e.InSpace(ctx, s.tenant, func(q db.Querier) (err error) {
		tag, err = q.Exec(ctx, sql, args...)
		return err
	})
	return tag, err
}

func (s spaceStatements) Select(ctx context.Context, sql string, args ...any) (rows []map[string]any, err error) {
	err = s.e.InSpace(ctx, s.tenant, func(q db.Querier) (err error) {
		rows, err = db.Select(ctx, q, sql, args...)
		return err
	})
	return rows, err
}

func (s spaceStatements) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return spaceRow{s: s, ctx: ctx, sql: sql, args: args}
}

// Query keeps its transaction open until the rows are closed, which is what
// makes spaceStatements a db.Querier.
func (s spaceStatements) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	on, err := s.e.tenancy(ctx)
	if err != nil {
		return nil, err
	}
	if !on {
		return s.e.DB.Pool.Query(ctx, sql, args...)
	}
	tx, err := s.e.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := db.Confine(ctx, tx, s.e.tenantRole()); err == nil {
		err = db.SetTenant(ctx, tx, s.tenant)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return spaceRows{Rows: rows, ctx: ctx, tx: tx}, nil
}

type spaceRows struct {
	pgx.Rows
	ctx context.Context
	tx  pgx.Tx
}

func (r spaceRows) Close() {
	r.Rows.Close()
	if r.Rows.Err() != nil {
		r.tx.Rollback(r.ctx)
		return
	}
	r.tx.Commit(r.ctx)
}

type spaceRow struct {
	s    spaceStatements
	ctx  context.Context
	sql  string
	args []any
}

func (r spaceRow) Scan(dest ...any) error {
	return r.s.e.InSpace(r.ctx, r.s.tenant, func(q db.Querier) error {
		return q.QueryRow(r.ctx, r.sql, r.args...).Scan(dest...)
	})
}

// Spaces lists the spaces work that concerns every tenant has to visit: the
// platform space first, then each enabled tenant. Without tenancy that is the
// platform space alone.
func (e *Engine) Spaces(ctx context.Context) ([]string, error) {
	spaces := []string{""}
	on, err := e.tenancy(ctx)
	if err != nil || !on {
		return spaces, err
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id FROM `+meta.TenantTable+` WHERE enabled ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		spaces = append(spaces, db.Str(r["id"]))
	}
	return spaces, nil
}

// RequestSpace is how a request finds where it works: the context it returns
// names the space of user — the user's own tenant, or for a user of the
// platform space the tenant named, which is the one the operator entered.
// Everything the request does afterwards, in a transaction or outside one,
// reads its space from that context. A disabled tenant is refused here.
func (e *Engine) RequestSpace(ctx context.Context, user, named string) (context.Context, error) {
	on, err := e.tenancy(ctx)
	if err != nil || !on {
		return ctx, err
	}
	own, err := e.TenantOfUser(ctx, user)
	if err != nil {
		return ctx, err
	}
	space := own
	if own == "" && named != "" {
		// a name that may not be honoured is ignored, not refused: a stale
		// session value must not lock an account out of its own space
		if ok, err := e.CanEnterTenants(ctx, user); err != nil {
			return ctx, err
		} else if ok {
			space = named
		}
	}
	if space != "" {
		if err := e.checkTenant(ctx, nil, space); err != nil {
			return ctx, err
		}
	}
	return WithTenant(ctx, space), nil
}

// SessionTenant is the tenant a platform user's session has entered; empty
// when it has entered none.
func (e *Engine) SessionTenant(ctx context.Context, sid string) string {
	if on, err := e.tenancy(ctx); sid == "" || err != nil || !on {
		return ""
	}
	key := "sidtenant:" + sid
	if v, ok := e.Cache.Get(key); ok {
		return v.(string)
	}
	var tenant string
	if err := e.DB.Pool.QueryRow(ctx, `SELECT coalesce(data->>'tenant', '') FROM ddcore_session WHERE sid = $1`, sid).Scan(&tenant); err != nil {
		return ""
	}
	e.Cache.Set(key, tenant, time.Minute)
	return tenant
}

// EnterTenant makes a platform user's session work in a tenant from the next
// request on; an empty tenant returns it to the platform space.
func (c *Ctx) EnterTenant(tenant string) error {
	if !c.Tenancy() {
		return cerr.Validation("This site has no tenants: tenancy is off")
	}
	if ok, err := c.E.CanEnterTenants(c.Ctx, c.User); err != nil {
		return err
	} else if !ok {
		c.AuditDenied("tenant.enter", meta.TenantDocType, tenant, nil)
		return cerr.Permission("Only a System Manager of the platform space can enter a tenant")
	}
	if c.Sid == "" {
		return cerr.Validation("Entering a tenant needs a session; with an API key, send the X-Tenant header")
	}
	if tenant != "" {
		if err := c.E.checkTenant(c.Ctx, nil, tenant); err != nil {
			return err
		}
	}
	if _, err := c.E.DB.Pool.Exec(c.Ctx, `UPDATE ddcore_session
		SET data = coalesce(data, '{}'::jsonb) || jsonb_build_object('tenant', $2::text) WHERE sid = $1`, c.Sid, tenant); err != nil {
		return err
	}
	key := "sidtenant:" + c.Sid
	c.E.Cache.Del(key)
	if err := broadcastInvalidation(c.Ctx, c.E.DB.Pool, []string{key}, nil); err != nil {
		return err
	}
	return c.E.RecordAudit(WithTenant(c.Ctx, tenant), c.User, "tenant.enter", "Allowed", meta.TenantDocType, tenant, nil)
}

// TenantBoot is what the desk is told about tenancy: the space the request
// works in, and for a user of the platform space the tenants there are to
// enter. Nil on a site without tenancy. A tenant's own users learn the id and
// title of their tenant and nothing about any other.
func (c *Ctx) TenantBoot() (map[string]any, error) {
	if !c.Tenancy() {
		return nil, nil
	}
	out := map[string]any{"id": c.Tenant, "title": ""}
	platform, err := c.E.CanEnterTenants(c.Ctx, c.User)
	if err != nil {
		return nil, err
	}
	out["platform"] = platform
	if c.Tenant != "" {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT title FROM `+meta.TenantTable+` WHERE id = $1`, c.Tenant)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			out["title"] = db.Str(rows[0]["title"])
		}
	}
	if platform {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, title, enabled FROM `+meta.TenantTable+` ORDER BY title, id`)
		if err != nil {
			return nil, err
		}
		tenants := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			tenants = append(tenants, map[string]any{"id": r["id"], "title": r["title"], "enabled": r["enabled"] == true})
		}
		out["tenants"] = tenants
	}
	return out, nil
}

// CanEnterTenants reports whether user is an operator: Admin, or a System
// Manager whose account is in the platform space. Being in the platform space
// is not enough — on a site that had users before it had tenants, everyone
// who was there still is.
func (e *Engine) CanEnterTenants(ctx context.Context, user string) (bool, error) {
	if user == "Admin" {
		return true, nil
	}
	if user == "" || user == "Guest" {
		return false, nil
	}
	own, err := e.TenantOfUser(ctx, user)
	if err != nil || own != "" {
		return false, err
	}
	// the platform space's roles, whatever tenant ctx names
	roles, err := e.NewCtx(WithTenant(ctx, ""), user).RolesOf(user)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r == "System Manager" {
			return true, nil
		}
	}
	return false, nil
}

// namedTenant is the tenant work under ctx is to enter: the one ctx names,
// or failing that the one the command line named for the whole process.
func (e *Engine) namedTenant(ctx context.Context) (string, bool) {
	if id, ok := TenantFrom(ctx); ok {
		return id, true
	}
	if e.Cfg.EnterTenant != "" {
		return e.Cfg.EnterTenant, true
	}
	return "", false
}

// adoptKeep are the rows that stay in the platform space when a tenant
// adopts everything else: the two accounts the framework itself is.
const adoptKeep = `('Admin', 'Guest')`

// AdoptPlatformRows moves every row of the platform space into a tenant: the
// way a site that had one customer before it had tenancy makes that customer
// its first tenant. Admin and Guest stay. It is one transaction; a row whose
// id the tenant already uses stops it with nothing moved.
func (e *Engine) AdoptPlatformRows(ctx context.Context, tenant string) (map[string]int64, error) {
	moved := map[string]int64{}
	if on, err := e.tenancy(ctx); err != nil {
		return nil, err
	} else if !on {
		return nil, cerr.Validation("This site has no tenants: tenancy is off")
	}
	err := e.RunSystem(ctx, "Admin", func(c *Ctx) error {
		if err := e.checkTenant(ctx, c.Tx, tenant); err != nil {
			return err
		}
		move := func(table, except string, args ...any) error {
			sql := fmt.Sprintf(`UPDATE %s SET tenant = $1 WHERE tenant = ''`, db.Ident(table))
			if except != "" {
				sql += " AND NOT (" + except + ")"
			}
			tag, err := c.Tx.Exec(ctx, sql, append([]any{tenant}, args...)...)
			if err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
			if n := tag.RowsAffected(); n > 0 {
				moved[table] = n
			}
			return nil
		}
		for _, name := range c.St.Meta.Names() {
			d := c.St.Meta.DocTypes[name]
			if !d.TenantOwned {
				continue
			}
			except := ""
			switch d.Name {
			case "User":
				except = `id IN ` + adoptKeep
			case "Has Role":
				except = `parenttype = 'User' AND parent IN ` + adoptKeep
			case "API Key":
				except = `"user" IN ` + adoptKeep
			}
			if err := move(d.TableName(), except); err != nil {
				return err
			}
		}
		for _, table := range []string{"ddcore_series", "ddcore_notification", "ddcore_notification_due"} {
			if err := move(table, ""); err != nil {
				return err
			}
		}
		// a shared DocType's documents stay, and so do the secrets of their
		// Vault fields
		shared, err := c.sharedVaultKeys()
		if err != nil {
			return err
		}
		if err := move("ddcore_vault", "name = ANY($2)", shared); err != nil {
			return err
		}
		// jobs still to run belong with the rows they will touch
		if err := move("ddcore_job", `status NOT IN ('queued', 'running') OR "user" IN `+adoptKeep); err != nil {
			return err
		}
		if err := e.RecordAuditOn(ctx, c.Tx, "Admin", "tenant.adopt", "Allowed", meta.TenantDocType, tenant, "", "", nil); err != nil {
			return err
		}
		return broadcastClear(ctx, c.Tx)
	})
	if err != nil {
		return nil, err
	}
	e.Cache.Clear()
	return moved, nil
}

// awayFromHome reports whether the ctx works in a space that is not its
// user's own: an operator inside a tenant. What the process caches per user
// — scopes, shares — describes the user at home, and is neither read nor
// written from anywhere else.
func (c *Ctx) awayFromHome() (bool, error) {
	if c.Tenant == "" || !c.Tenancy() {
		return false, nil
	}
	own, err := c.E.TenantOfUser(c.Ctx, c.User)
	return own != c.Tenant, err
}
