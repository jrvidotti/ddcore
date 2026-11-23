package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	d, err := db.OpenConfined(ctx, e.Cfg.DSN, db.Options{TenantRole: role, MaxConns: e.Cfg.PoolMaxConns})
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
	was := c.confinedNow
	c.confinedNow = true
	ferr := fn()
	c.confinedNow = was
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

// requireTenancy refuses the calls that only make sense when the site has tenants.
func (e *Engine) requireTenancy(ctx context.Context) error {
	if e != nil {
		if on, err := e.tenancy(ctx); on || err != nil {
			return err
		}
	}
	return cerr.Validation("This site has no tenants: tenancy is off")
}

func (c *Ctx) requireTenancy() error { return c.E.requireTenancy(c.Ctx) }

const (
	userTenantTTL  = 5 * time.Minute
	tenantStateTTL = 10 * time.Second
)

// spaceOf decides the space a user's work runs in when nothing above it
// did: the tenant WithTenant (or --tenant) named, which the user must work
// in; or, with none named, the space of the user's one membership. A person
// in several spaces has to be told which (#167). A tenant that is disabled,
// or does not exist, admits nobody.
func (e *Engine) spaceOf(ctx context.Context, user string) (string, error) {
	named, ok := e.namedTenant(ctx)
	if platformAccount(user) || user == "" {
		if ok {
			return named, e.checkNamed(ctx, named)
		}
		return "", nil
	}
	ms, err := e.Memberships(ctx, user)
	if err != nil {
		return "", err
	}
	if ok {
		if _, member := membershipIn(ms, named); !member && named != "" && !noTenantRows(ms) {
			return "", cerr.Permission("A user of one tenant cannot work in another")
		}
		return named, e.checkNamed(ctx, named)
	}
	space, err := soleSpace(ms, user)
	if err != nil {
		return "", err
	}
	return space, e.checkNamed(ctx, space)
}

func (e *Engine) checkNamed(ctx context.Context, tenant string) error {
	if tenant == "" {
		return nil
	}
	return e.checkTenant(ctx, nil, tenant)
}

// noTenantRows reports whether a person has no row inside any tenant: a
// user of the platform space, or nobody yet. Such a user may be sent into a
// tenant by name, as before memberships — the operator's case.
func noTenantRows(ms []Membership) bool {
	for _, m := range ms {
		if m.Tenant != "" {
			return false
		}
	}
	return true
}

// soleSpace is the space work for a person goes to when nothing names one:
// the platform row if there is one, else the one membership. A person in
// several tenants has none — the caller names it.
func soleSpace(ms []Membership, user string) (string, error) {
	if len(ms) == 0 {
		return "", nil
	}
	if _, ok := membershipIn(ms, ""); ok {
		return "", nil
	}
	if len(ms) == 1 {
		return ms[0].Tenant, nil
	}
	return "", cerr.Validation("{0} works in more than one tenant: name one with --tenant", user)
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

// CheckTenant refuses a tenant that does not exist or is disabled: what a
// command checks before it starts work that names one.
func (e *Engine) CheckTenant(ctx context.Context, id string) error {
	if err := e.requireTenancy(ctx); err != nil {
		return err
	}
	return e.checkTenant(ctx, nil, id)
}

// InTenant runs fn as the same user inside a tenant, on this ctx's
// transaction. Only the platform space may enter a tenant; a ctx already in
// that tenant just runs fn.
func (c *Ctx) InTenant(id string, fn func(*Ctx) error) error {
	if err := c.requireTenancy(); err != nil {
		return err
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
	return c.enterSpaceCtx(id)
}

// enterSpaceCtx is enterTenantCtx without the check on the tenant, so that it
// also takes "" — the platform space, which a tenant's ctx enters only in a
// test (ddcore.test.inPlatform) and for a server write of a DocType the
// platform keeps (inPlatform).
func (c *Ctx) enterSpaceCtx(id string) (*Ctx, error) {
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
//
// The other way round, a tenant-only DocType (`space: "tenant"`) is refused
// to the platform space altogether, reads included: a row written there
// would land where no tenant ever sees it (#105). A migration patch is the
// exception — it sees every space, and is the one place a backfill runs.
//
// A shared DocType with `tenantAccess: "server"` is refused inside a tenant
// to everything but server code — server, a call that ignores permissions —
// whatever the roles say (#108). Server code reads it like any shared
// DocType; of its writes, Insert and Save divert to the platform space
// (serverWriteInPlatform) before they get here, and every other one is
// refused below like any write of a shared DocType.
func (c *Ctx) spaceRefusal(d *meta.DocType, write, server bool) error {
	if c.Tenant == "" && d.TenantOnly && c.Flags["inPatch"] != true && c.Tenancy() {
		return cerr.Permission("{0} lives inside a tenant: enter one (on the command line, pass --tenant)", c.T(d.Label))
	}
	if c.Tenant == "" || d.TenantOwned || d.IsVirtual() {
		return nil
	}
	if d.ServerOnlyInTenant() && !server {
		return cerr.Permission("{0} is kept by the platform: inside a tenant only server code reaches it", c.T(d.Label))
	}
	if d.Name == meta.TenantDocType {
		return cerr.Permission("Tenants are managed from the platform space")
	}
	if write {
		return cerr.Permission("{0} is shared by every tenant and can only be changed from the platform space", c.T(d.Label))
	}
	return nil
}

// serverWriteInPlatform reports whether a write of d is a tenant's server
// code adding to a DocType the platform keeps (#108), which inPlatform then
// performs in the platform space.
func (c *Ctx) serverWriteInPlatform(d *meta.DocType, server bool) bool {
	return server && c.Tenant != "" && d.ServerOnlyInTenant() && c.Tenancy()
}

// inPlatform runs a tenant's write of a DocType the platform keeps in the
// platform space, on the same transaction and VM: the row is the platform's,
// and so is everything the write derives — its naming series, Version, audit
// events, vault secrets, webhooks, notifications and realtime events, and
// the hooks run there. The Version and the audit events carry the tenant as
// source_tenant; the document's owner and modified_by are the tenant's user,
// whose id is the site's.
func (c *Ctx) inPlatform(fn func(p *Ctx) (Doc, error)) (Doc, error) {
	p, err := c.enterSpaceCtx("")
	if err != nil {
		return nil, err
	}
	p.sourceTenant = c.Tenant
	out, ferr := fn(p)
	if _, err := p.leaveTenantCtx(); err != nil && ferr == nil {
		return nil, err
	}
	return out, ferr
}

// SpaceRefuses reports whether this ctx's space may not even read d: a
// tenant-only DocType in the platform space, `Site Tenant` inside a tenant.
// Boot leaves such a DocType out, so the desk neither lists nor opens it.
func (c *Ctx) SpaceRefuses(d *meta.DocType) bool { return c.SpaceRefusal(d) != nil }

// SpaceRefusal is the error SpaceRefuses stands for, for an endpoint that
// answers with it: `/api/meta` says why the platform cannot open a tenant's
// DocType instead of "No permission".
func (c *Ctx) SpaceRefusal(d *meta.DocType) error { return c.spaceRefusal(d, false, false) }

// SpaceRefusesName is SpaceRefuses by name; an unknown DocType is not refused
// here (whatever looks it up next says so).
func (c *Ctx) SpaceRefusesName(doctype string) bool {
	d, ok := c.St.Meta.Get(doctype)
	return ok && c.SpaceRefuses(d)
}

// appSpaces is each app's default `space` for its DocTypes. "tenant" is the
// only one an app may declare: "any" is what a DocType says to opt out of it.
func appSpaces(apps map[string]*AppMeta) (map[string]string, error) {
	out := map[string]string{}
	names := mapNames(apps)
	sort.Strings(names)
	for _, name := range names {
		a := apps[name]
		if a == nil {
			continue
		}
		if ts := a.Tests.Space; ts != "" && ts != SpaceTenant && ts != SpacePlatform {
			return nil, fmt.Errorf("app %q: tests.space %q is neither %q nor %q", name, ts, SpaceTenant, SpacePlatform)
		}
		if a.Space == "" {
			continue
		}
		if a.Space != meta.SpaceTenant {
			return nil, fmt.Errorf("app %q: space %q is not %q", name, a.Space, meta.SpaceTenant)
		}
		out[name] = a.Space
	}
	return out, nil
}

// tenantOnlyFixtures refuses a fixture of a tenant-only DocType. Fixtures fill
// the platform space, which cannot hold such a document; a new tenant's
// starting records come from onTenantCreate.
func tenantOnlyFixtures(reg *meta.Registry, apps map[string]*AppMeta) error {
	names := mapNames(apps)
	sort.Strings(names)
	for _, name := range names {
		a := apps[name]
		if a == nil {
			continue
		}
		doctypes := mapNames(a.Fixtures)
		sort.Strings(doctypes)
		for _, dt := range doctypes {
			if d, ok := reg.Get(dt); ok && d.TenantOnly {
				return fmt.Errorf("app %q: fixtures of %q, which lives inside a tenant: fixtures fill the platform space; give each tenant its records in onTenantCreate", name, dt)
			}
		}
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

// RunAdminFor is Run as Admin in a space of user: the framework acting on
// somebody's account without that somebody being signed in — a recovery
// link, an API key issued from the command line. The documents it writes
// land in the user's space, where the user will look for them. That is the
// tenant ctx (or --tenant) names, which the user must work in; else the one
// space the user works in. A person in several tenants needs it named.
func (e *Engine) RunAdminFor(ctx context.Context, user string, fn func(c *Ctx) error) error {
	tenant, err := e.adminSpaceFor(ctx, user)
	if err != nil {
		return err
	}
	return e.Run(WithTenant(ctx, tenant), "Admin", fn)
}

func (e *Engine) adminSpaceFor(ctx context.Context, user string) (string, error) {
	on, err := e.tenancy(ctx)
	if err != nil || !on {
		return "", err
	}
	ms, err := e.Memberships(ctx, user)
	if err != nil {
		return "", err
	}
	if named, ok := e.namedTenant(ctx); ok {
		if _, member := membershipIn(ms, named); !member && len(ms) > 0 {
			return "", cerr.NotFound("User {0} does not exist", user)
		}
		return named, nil
	}
	return soleSpace(ms, user)
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
		// a DocType the platform keeps stays with the platform's sessions
		if d, err := c.St.DocType(doctype); err == nil && !d.TenantOwned && !d.ServerOnlyInTenant() {
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

// SpaceReq is what a request says about the space it wants.
type SpaceReq struct {
	// Header is the X-Tenant header, which only an operator may use.
	Header string
	// Session is the space the session row records, when SessionSet.
	Session    string
	SessionSet bool
	// Fixed is the space the credential itself is bound to — an API key's
	// row, a session a tenant's credential provider opened, an
	// impersonation. Nil when the credential goes wherever its user may.
	Fixed *string
}

// RequestSpace is how a request finds where it works. An operator works in
// the platform space or the tenant named. Anyone else works in one of their
// own enabled memberships: the one the session records, or else where they
// land (LandingSpace). A credential bound to a space stays there; only an
// operator's platform key moves, with X-Tenant. Everything the request does
// afterwards, in a transaction or outside one, reads its space from the
// context returned. A disabled tenant is refused here.
func (e *Engine) RequestSpace(ctx context.Context, user string, req SpaceReq) (context.Context, error) {
	on, err := e.tenancy(ctx)
	if err != nil || !on {
		return ctx, err
	}
	if user == "" || user == "Guest" {
		return WithTenant(ctx, ""), nil
	}
	operator, err := e.CanEnterTenants(ctx, user)
	if err != nil {
		return ctx, err
	}
	var space string
	switch {
	case req.Fixed != nil && (*req.Fixed != "" || !operator):
		space = *req.Fixed
		if user != "Admin" {
			ms, err := e.Memberships(ctx, user)
			if err != nil {
				return ctx, err
			}
			// an operator may hold such a credential where they have no row
			m, ok := membershipIn(ms, space)
			if (ok && !m.Enabled) || (!ok && !operator) {
				return ctx, cerr.Auth("User is disabled")
			}
		}
	case operator && req.Header != "":
		space = req.Header
	case operator && req.Fixed != nil:
		space = ""
	case operator:
		// a name that may not be honoured is ignored, not refused: a stale
		// session value must not lock an account out of its own space
		space = req.Session
	default:
		ms, err := e.Memberships(ctx, user)
		if err != nil {
			return ctx, err
		}
		if m, ok := membershipIn(ms, req.Session); req.SessionSet && ok && m.Enabled && e.checkNamed(ctx, req.Session) == nil {
			space = req.Session
		} else if space, err = e.LandingSpace(ctx, user); err != nil {
			return ctx, err
		}
	}
	if err := e.checkNamed(ctx, space); err != nil {
		return ctx, err
	}
	return WithTenant(ctx, space), nil
}

// SessionSpace is what a session row says about its space.
type SessionSpace struct {
	// Tenant is the space the session works in, when Set.
	Tenant string
	Set    bool
	// Bound is whether the session may not leave it: one a tenant's
	// credential provider opened.
	Bound bool
}

// SessionTenant is the space a session works in, as its row records it.
func (e *Engine) SessionTenant(ctx context.Context, sid string) SessionSpace {
	if on, err := e.tenancy(ctx); sid == "" || err != nil || !on {
		return SessionSpace{}
	}
	key := "sidtenant:" + sid
	if v, ok := e.Cache.Get(key); ok {
		return v.(SessionSpace)
	}
	var tenant *string
	var bound bool
	if err := e.DB.Pool.QueryRow(ctx, `SELECT data->>'tenant', coalesce(data->>'bound', '') <> '' FROM ddcore_session WHERE sid = $1`, sid).Scan(&tenant, &bound); err != nil {
		return SessionSpace{}
	}
	out := SessionSpace{Bound: bound}
	if tenant != nil {
		out.Tenant, out.Set = *tenant, true
	}
	e.Cache.Set(key, out, time.Minute)
	return out
}

// EnterTenant makes the session work in another space from the next request
// on. An operator enters any tenant, and "" returns them to the platform
// space. Anyone else moves between their own enabled memberships (#167): a
// tenant they do not work in is refused exactly as one that does not exist,
// so the refusal tells them nothing about it.
func (c *Ctx) EnterTenant(tenant string) error {
	if err := c.requireTenancy(); err != nil {
		return err
	}
	if err := c.refuseWhileImpersonating(); err != nil {
		return err
	}
	if c.Sid == "" {
		return cerr.Validation("Entering a tenant needs a session; with an API key, send the X-Tenant header")
	}
	if c.E.SessionTenant(c.Ctx, c.Sid).Bound {
		return cerr.Permission("This session signed in through its tenant and stays in it")
	}
	operator, err := c.E.CanEnterTenants(c.Ctx, c.User)
	if err != nil {
		return err
	}
	if !operator {
		ms, err := c.E.Memberships(c.Ctx, c.User)
		if err != nil {
			return err
		}
		if m, ok := membershipIn(ms, tenant); !ok || !m.Enabled || c.E.checkNamed(c.Ctx, tenant) != nil {
			c.AuditDenied("tenant.enter", meta.TenantDocType, tenant, nil)
			return cerr.Permission("You do not work in tenant {0}", tenant)
		}
	} else if tenant != "" {
		if err := c.E.checkTenant(c.Ctx, nil, tenant); err != nil {
			return err
		}
	}
	if err := c.E.moveSession(c.Ctx, c.Sid, c.User, tenant); err != nil {
		return err
	}
	return c.E.RecordAudit(WithTenant(c.Ctx, tenant), c.User, "tenant.enter", "Allowed", meta.TenantDocType, tenant, nil)
}

// moveSession records the space a session works in, and remembers it as the
// person's: the next sign-in lands there.
func (e *Engine) moveSession(ctx context.Context, sid, user, tenant string) error {
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_session
		SET data = coalesce(data, '{}'::jsonb) || jsonb_build_object('tenant', $2::text) WHERE sid = $1`, sid, tenant); err != nil {
		return err
	}
	key := "sidtenant:" + sid
	e.Cache.Del(key)
	if err := broadcastInvalidation(ctx, e.DB.Pool, []string{key}, nil); err != nil {
		return err
	}
	return e.System(ctx, func(q db.Querier) error { return rememberSpace(ctx, q, user, tenant) })
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
	if c.boundCredential() || (c.Sid != "" && c.E.SessionTenant(c.Ctx, c.Sid).Bound) {
		// an impersonation, or a session bound to its tenant, goes nowhere
		// else: and an administrator watching as someone must not learn
		// where else that someone works
		out["platform"] = false
		return out, nil
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
		return out, nil
	}
	// a person's own tenants: what the switcher lists for them (#167)
	spaces, err := c.E.openSpaces(c.Ctx, c.User)
	if err != nil {
		return nil, err
	}
	if len(spaces) > 1 {
		tenants := make([]map[string]any, 0, len(spaces))
		for _, sp := range spaces {
			tenants = append(tenants, map[string]any{"id": sp.id, "title": sp.title, "enabled": true})
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
	// a person with a platform row; without tenancy, the one row there is
	if on, err := e.tenancy(ctx); err != nil {
		return false, err
	} else if on {
		ms, err := e.Memberships(ctx, user)
		if err != nil {
			return false, err
		}
		if m, ok := membershipIn(ms, ""); !ok || !m.Enabled {
			return false, nil
		}
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

// platformAccount reports whether a user id is one of adoptKeep's: an
// account that stays in the platform space, whatever moves or is loaded
// into a tenant.
func platformAccount(id string) bool { return id == "Admin" || id == "Guest" }

// adoptStep is one table an adopt moves rows of: every row of the platform
// space but the ones except keeps there. except is SQL on the table's own
// columns; $1 is the tenant and args are $2 onwards.
type adoptStep struct {
	table  string
	except string
	args   []any
}

func (s adoptStep) where() string {
	w := "tenant = ''"
	if s.except != "" {
		w += " AND NOT (" + s.except + ")"
	}
	return w
}

// adoptPlan is the rule of an adopt, table by table: what the move and its
// dry run both follow, so the preview counts exactly what the move moves.
func (c *Ctx) adoptPlan() ([]adoptStep, error) {
	var steps []adoptStep
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
		steps = append(steps, adoptStep{table: d.TableName(), except: except})
	}
	for _, table := range []string{"ddcore_series", "ddcore_notification", "ddcore_notification_due", "ddcore_import_record"} {
		steps = append(steps, adoptStep{table: table})
	}
	// a shared DocType's documents stay, and so do the secrets of their
	// Vault fields
	shared, err := c.sharedVaultKeys()
	if err != nil {
		return nil, err
	}
	steps = append(steps, adoptStep{table: "ddcore_vault", except: "name = ANY($2)", args: []any{shared}})
	// jobs still to run belong with the rows they will touch
	steps = append(steps, adoptStep{table: "ddcore_job", except: `status NOT IN ('queued', 'running') OR "user" IN ` + adoptKeep})
	return steps, nil
}

// AdoptPreview is what an adopt would do, table by table: the rows it would
// move, and those whose key the tenant already uses.
type AdoptPreview struct {
	Tenant string       `json:"tenant"`
	Tables []AdoptTable `json:"tables"`
}

// AdoptTable is one table of an AdoptPreview. Only tables with something to
// move are listed.
type AdoptTable struct {
	Table      string           `json:"table"`
	Rows       int64            `json:"rows"`
	Collisions []AdoptCollision `json:"collisions,omitempty"`
}

// AdoptCollision is one unique key of a table on which rows of the platform
// space meet rows the tenant already has: how many, and a few of the values.
type AdoptCollision struct {
	Index   string   `json:"index"`
	Columns []string `json:"columns"`
	Count   int64    `json:"count"`
	Samples []string `json:"samples"`
}

// adoptSamples is how many colliding keys a preview names per unique key.
const adoptSamples = 10

// Collides reports whether the adopt would stop on a key the tenant uses.
func (p *AdoptPreview) Collides() bool {
	for _, t := range p.Tables {
		if len(t.Collisions) > 0 {
			return true
		}
	}
	return false
}

// collisionSummary names each colliding key and a few of its values, for a
// message: "tab_pessoa (id): Comum, Outro, and 3 more".
func (p *AdoptPreview) collisionSummary() string {
	var parts []string
	for _, t := range p.Tables {
		for _, col := range t.Collisions {
			s := fmt.Sprintf("%s (%s): %s", t.Table, strings.Join(col.Columns, ", "), strings.Join(col.Samples, "; "))
			if more := col.Count - int64(len(col.Samples)); more > 0 {
				s += fmt.Sprintf(" and %d more", more)
			}
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// AdoptPreview is the dry run of AdoptPlatformRows: it counts what would move
// into tenant and finds every row the tenant already has under the same key
// — the primary key and each unique index that is not an expression — and
// writes nothing.
func (e *Engine) AdoptPreview(ctx context.Context, tenant string) (*AdoptPreview, error) {
	if err := e.requireTenancy(ctx); err != nil {
		return nil, err
	}
	var p *AdoptPreview
	err := e.RunSystem(ctx, "Admin", func(c *Ctx) error {
		if err := e.checkTenant(ctx, c.Tx, tenant); err != nil {
			return err
		}
		var err error
		p, err = c.adoptPreview(tenant)
		return err
	})
	return p, err
}

// adoptPreview runs on a system ctx, which sees every space.
func (c *Ctx) adoptPreview(tenant string) (*AdoptPreview, error) {
	steps, err := c.adoptPlan()
	if err != nil {
		return nil, err
	}
	p := &AdoptPreview{Tenant: tenant, Tables: []AdoptTable{}}
	for _, s := range steps {
		args := append([]any{tenant}, s.args...)
		var n int64
		// $1 is not used by the count, but the arguments are numbered from it
		if err := c.Tx.QueryRow(c.Ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE $1::text IS NOT NULL AND %s`,
			db.Ident(s.table), s.where()), args...).Scan(&n); err != nil {
			return nil, fmt.Errorf("%s: %w", s.table, err)
		}
		if n == 0 {
			continue
		}
		t := AdoptTable{Table: s.table, Rows: n}
		keys, err := tenantUniqueKeys(c.Ctx, c.Tx, s.table)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			col, err := adoptCollisions(c.Ctx, c.Tx, s, k, args)
			if err != nil {
				return nil, err
			}
			if col != nil {
				t.Collisions = append(t.Collisions, *col)
			}
		}
		p.Tables = append(p.Tables, t)
	}
	return p, nil
}

// uniqueKey is a unique index of a table that leads with its tenant: the
// columns after it, and the index's predicate when it is partial.
type uniqueKey struct {
	index     string
	cols      []string
	predicate string
}

// tenantUniqueKeys reads the unique indexes of a table that include its
// tenant column — the only ones moving a row to another tenant can break.
// One on an expression is left out: there is no column to compare.
func tenantUniqueKeys(ctx context.Context, q db.Querier, table string) ([]uniqueKey, error) {
	rows, err := db.Select(ctx, q, `SELECT ic.relname AS name, i.indisprimary AS pk,
		  array_to_string(array(SELECT a.attname FROM unnest(i.indkey::int2[]) WITH ORDINALITY k(attnum, ord)
		    JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		    WHERE k.ord <= i.indnkeyatts ORDER BY k.ord), ',') AS cols,
		  coalesce(pg_get_expr(i.indpred, i.indrelid), '') AS predicate
		FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE i.indrelid = to_regclass($1) AND i.indisunique AND i.indexprs IS NULL
		ORDER BY i.indisprimary DESC, ic.relname`, table)
	if err != nil {
		return nil, err
	}
	var out []uniqueKey
	for _, r := range rows {
		var cols []string
		hasTenant := false
		for _, col := range strings.Split(db.Str(r["cols"]), ",") {
			if col == meta.TenantColumn {
				hasTenant = true
				continue
			}
			if col != "" {
				cols = append(cols, col)
			}
		}
		if !hasTenant || len(cols) == 0 {
			continue
		}
		out = append(out, uniqueKey{index: db.Str(r["name"]), cols: cols, predicate: db.Str(r["predicate"])})
	}
	return out, nil
}

// adoptCollisions finds the rows a step would move whose key, on one unique
// index, the tenant already holds. The two sides are subqueries so that the
// step's exceptions and a partial index's predicate, written on the table's
// own columns, apply to each as they are.
func adoptCollisions(ctx context.Context, q db.Querier, s adoptStep, k uniqueKey, args []any) (*AdoptCollision, error) {
	pred := ""
	if k.predicate != "" {
		pred = " AND (" + k.predicate + ")"
	}
	t := db.Ident(s.table)
	var on, key []string
	for _, col := range k.cols {
		c := db.Ident(col)
		on = append(on, "src."+c+" = dst."+c)
		key = append(key, "src."+c+"::text")
	}
	sql := fmt.Sprintf(`WITH src AS (SELECT * FROM %[1]s WHERE %[2]s%[3]s),
		  dst AS (SELECT * FROM %[1]s WHERE tenant = $1%[3]s)
		SELECT count(*) OVER () AS n, concat_ws(', ', %[4]s) AS k
		FROM src JOIN dst ON %[5]s ORDER BY 2 LIMIT %[6]d`,
		t, s.where(), pred, strings.Join(key, ", "), strings.Join(on, " AND "), adoptSamples)
	rows, err := db.Select(ctx, q, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.table, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	col := &AdoptCollision{Index: k.index, Columns: k.cols, Count: int64(toFloat(rows[0]["n"]))}
	for _, r := range rows {
		col.Samples = append(col.Samples, db.Str(r["k"]))
	}
	return col, nil
}

// AdoptPlatformRows moves every row of the platform space into a tenant: the
// way a site that had one customer before it had tenancy makes that customer
// its first tenant. Admin and Guest stay. It is one transaction, and it runs
// the preview first: a row whose key the tenant already uses stops it with
// nothing moved, and the refusal names every such key.
func (e *Engine) AdoptPlatformRows(ctx context.Context, tenant string) (map[string]int64, error) {
	moved := map[string]int64{}
	if err := e.requireTenancy(ctx); err != nil {
		return nil, err
	}
	err := e.RunSystem(ctx, "Admin", func(c *Ctx) error {
		if err := e.checkTenant(ctx, c.Tx, tenant); err != nil {
			return err
		}
		p, err := c.adoptPreview(tenant)
		if err != nil {
			return err
		}
		if p.Collides() {
			return cerr.Duplicate("Adopting into {0} would collide with rows the tenant already has, so nothing was moved: {1}. Run ddcore tenant adopt {0} --dry-run for the full list",
				tenant, p.collisionSummary())
		}
		steps, err := c.adoptPlan()
		if err != nil {
			return err
		}
		for _, s := range steps {
			sql := fmt.Sprintf(`UPDATE %s SET tenant = $1 WHERE %s`, db.Ident(s.table), s.where())
			tag, err := c.Tx.Exec(ctx, sql, append([]any{tenant}, s.args...)...)
			if err != nil {
				return fmt.Errorf("%s: %w", s.table, err)
			}
			if n := tag.RowsAffected(); n > 0 {
				moved[s.table] = n
			}
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

// awayFromHome reports whether the ctx's user has no row in the space it
// works in: an operator inside a tenant. What the process caches per user —
// scopes, shares — describes a user's own rows, and is neither read nor
// written from anywhere else.
func (c *Ctx) awayFromHome() (bool, error) {
	if c.Tenant == "" || !c.Tenancy() || platformAccount(c.User) || c.User == "" {
		return c.Tenant != "" && c.Tenancy() && c.User == "Admin", nil
	}
	ms, err := c.memberships(c.User)
	if err != nil {
		return false, err
	}
	_, ok := membershipIn(ms, c.Tenant)
	return !ok, nil
}

// operatorLink reports whether, inside a tenant, a Link to User may name id
// although the tenant has no such User: id is an operator — Admin, or a
// System Manager of the platform space — who entered the tenant and was
// recorded doing something there (#110). Any operator, not only the one
// acting: a tenant's user saving the document later must not be refused it.
func (c *Ctx) operatorLink(doctype, id string) (bool, error) {
	if doctype != "User" || c.Tenant == "" || !c.Tenancy() {
		return false, nil
	}
	return c.E.CanEnterTenants(c.Ctx, id)
}

// operatorTitles are the names of the operators among names, read in the
// platform space, where their accounts are. It is how a Link to User inside a
// tenant shows the operator it recorded by name rather than by id.
func (c *Ctx) operatorTitles(names []string) map[string]string {
	var ops []string
	for _, n := range names {
		if ok, err := c.operatorLink("User", n); err == nil && ok {
			ops = append(ops, n)
		}
	}
	out := map[string]string{}
	if len(ops) == 0 {
		return out
	}
	rows, err := spaceStatements{e: c.E, tenant: ""}.Select(c.Ctx, `SELECT id, full_name FROM tab_user WHERE id = ANY($1)`, ops)
	if err != nil {
		return out
	}
	for _, r := range rows {
		if title := db.Str(r["full_name"]); title != "" {
			out[db.Str(r["id"])] = title
		}
	}
	return out
}
