package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// A person is a user id as the whole site knows it (#167). On a site with
// tenancy the same id may have a User row — a membership — in several
// spaces: the platform's and any number of tenants'. Each row carries its
// own roles, enabled flag, language and type, and is administered by its
// tenant. What belongs to the person, not to a row, is in ddcore_person: the
// password, and the tenant they last worked in.
//
// Without tenancy there is one space, so a person is one row.

// Membership is one User row of a person: the space it is in, and whether
// it lets the person in.
type Membership struct {
	Tenant   string
	Enabled  bool
	UserType string
}

const personSpacesTTL = 5 * time.Minute

// Memberships are the person's User rows, in every space, platform first and
// then by tenant id. Empty when the id has no row. It reads committed data
// and caches it; a ctx in the middle of a transaction asks c.memberships.
func (e *Engine) Memberships(ctx context.Context, user string) ([]Membership, error) {
	if user == "" {
		return nil, nil
	}
	on, err := e.tenancy(ctx)
	if err != nil {
		return nil, err
	}
	key := "spaces:" + user
	if v, ok := e.Cache.Get(key); ok {
		return v.([]Membership), nil
	}
	gen := e.Cache.Gen()
	var ms []Membership
	err = e.System(ctx, func(q db.Querier) error {
		ms, err = readMemberships(ctx, q, user, on)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(ms) > 0 {
		// not cached when empty: the user may be created a moment from now
		e.Cache.SetAt(key, ms, personSpacesTTL, gen)
	}
	return ms, nil
}

func readMemberships(ctx context.Context, q db.Querier, user string, tenancy bool) ([]Membership, error) {
	tenant := "''"
	if tenancy {
		tenant = meta.TenantColumn
	}
	rows, err := db.Select(ctx, q, `SELECT `+tenant+` AS tenant, enabled, user_type FROM tab_user WHERE id = $1`, user)
	if err != nil {
		return nil, err
	}
	ms := make([]Membership, 0, len(rows))
	for _, r := range rows {
		en, ok := r["enabled"].(bool)
		ms = append(ms, Membership{Tenant: db.Str(r["tenant"]), Enabled: !ok || en, UserType: db.Str(r["user_type"])})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Tenant < ms[j].Tenant })
	return ms, nil
}

// memberships is Memberships as this ctx sees it: a row this transaction
// wrote is on no other connection yet (#110), so inside a transaction the
// answer comes from it, uncached.
func (c *Ctx) memberships(user string) ([]Membership, error) {
	if c.Tx == nil || user == "" {
		return c.E.Memberships(c.Ctx, user)
	}
	on := c.Tenancy()
	var ms []Membership
	err := c.elevated(func() (err error) {
		ms, err = readMemberships(c.Ctx, c.Q(), user, on)
		return err
	})
	return ms, err
}

// membershipIn finds the person's row in one space.
func membershipIn(ms []Membership, tenant string) (Membership, bool) {
	for _, m := range ms {
		if m.Tenant == tenant {
			return m, true
		}
	}
	return Membership{}, false
}

// elsewhere reports whether the person has a row in a space other than
// tenant.
func elsewhere(ms []Membership, tenant string) bool {
	for _, m := range ms {
		if m.Tenant != tenant {
			return true
		}
	}
	return false
}

// dropPersonCache forgets what the process remembers about where a person
// works, here and in the other processes.
//
// Now and after the commit: a lookup in between reads the committed rows,
// which the commit then makes stale.
func (c *Ctx) dropPersonCache(user string) error {
	keys := append([]string{"spaces:" + user, "landing:" + user}, userKeys(c.Tenant, user)...)
	if err := c.invalidateNow(keys, nil); err != nil {
		return err
	}
	return c.invalidate(keys, nil)
}

// ---- credentials bound to a space -------------------------------------------

type boundKey struct{}

// WithBoundCredential records that the request signed in with a credential
// bound to its space: an API key, a session a tenant's credential provider
// opened. A tenant can hold such a credential for a person — it mints the
// keys of its rows, and decides whom its provider signs in — so it is never
// the person speaking for their whole sign-in (#167).
func WithBoundCredential(ctx context.Context) context.Context {
	return context.WithValue(ctx, boundKey{}, true)
}

// boundCredential reports whether the ctx acts on such a credential, or for
// an impersonation, which is as much someone else's say.
func (c *Ctx) boundCredential() bool {
	v, _ := c.Ctx.Value(boundKey{}).(bool)
	return v || c.ImpersonatedBy != ""
}

// ---- the credential --------------------------------------------------------

// personHash is the person's password hash; empty when they have none.
func (c *Ctx) personHash(user string) (string, error) {
	var hash string
	err := c.siteQ(func(q db.Querier) error {
		rows, err := db.Select(c.Ctx, q, `SELECT password_hash FROM ddcore_person WHERE id = $1`, user)
		if err == nil && len(rows) > 0 {
			hash = db.Str(rows[0]["password_hash"])
		}
		return err
	})
	return hash, err
}

// setPersonHash stores the person's password hash. setIn is the tenant
// whose administrator chose it, nil when the person or the operator did.
func (c *Ctx) setPersonHash(user, hash string, setIn *string) error {
	return c.siteQ(func(q db.Querier) error {
		_, err := q.Exec(c.Ctx, `INSERT INTO ddcore_person (id, password_hash, password_changed, modified, set_in)
			VALUES ($1, $2, now(), now(), $3)
			ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash, password_changed = now(),
			  modified = now(), set_in = EXCLUDED.set_in`,
			user, hash, setIn)
		return err
	})
}

// chosenBy is what setPersonHash records about who chose a password the ctx
// sets for person: nil for the person themself and for the operator, else
// the tenant whose administrator it is.
func (c *Ctx) chosenBy(person string) *string {
	if !c.Tenancy() || (person == c.User && !c.boundCredential()) {
		return nil
	}
	if op, err := c.isOperator(); err == nil && op {
		return nil
	}
	t := c.Tenant
	return &t
}

// forgetChosenPassword drops a password some tenant chose for person once
// they come to work in another space: that tenant knows it, and would
// otherwise sign in as them there. Their sessions and links go with it; they
// choose their own with "Forgot password".
func (c *Ctx) forgetChosenPassword(q db.Querier, person string) error {
	rows, err := db.Select(c.Ctx, q, `UPDATE ddcore_person SET password_hash = '', set_in = NULL, modified = now()
		WHERE id = $1 AND set_in IS NOT NULL AND set_in <> $2 RETURNING id`, person, c.Tenant)
	if err != nil || len(rows) == 0 {
		return err
	}
	if _, err := c.E.DropSessions(c.Ctx, q, person, ""); err != nil {
		return err
	}
	_, err = q.Exec(c.Ctx, `DELETE FROM ddcore_auth_token WHERE "user" = $1 AND used IS NULL`, person)
	return err
}

// siteQ runs fn on a querier that reaches the site-wide tables the confined
// role may not: the ctx's transaction, elevated for the duration, or the
// system pool when there is none.
func (c *Ctx) siteQ(fn func(q db.Querier) error) error {
	if c.Tx == nil {
		return c.E.System(c.Ctx, fn)
	}
	if c.system && c.confinedNow {
		// a system ctx confined for the moment (fixtures, hooks)
		if err := db.Elevate(c.Ctx, c.Tx); err != nil {
			return err
		}
		defer c.applySpaceAfter()
		return fn(c.Tx)
	}
	return c.elevated(func() error { return fn(c.Tx) })
}

// applySpaceAfter puts a system ctx's confinement back after siteQ: whatever
// confined() set is what applySpace would not restore, so it asks again.
func (c *Ctx) applySpaceAfter() {
	if c.confinedNow {
		_ = db.Confine(c.Ctx, c.Tx, c.E.tenantRole())
	}
}

// mayManageCredential is the guard on everything that sets or hands out a
// way to sign in as person — a password, a reset or invitation link, an
// identity, the address sign-in finds them by (#167). A tenant's System
// Manager administers that tenant's row and nothing more: a person who also
// works elsewhere on the site is theirs only in part, and taking over the
// sign-in would take over the other tenants with it. The person themself,
// and the operator, always may. The refusal names no other space.
func (c *Ctx) mayManageCredential(person string) error {
	if !c.Tenancy() || person == "" {
		return nil
	}
	if op, err := c.isOperator(); err != nil || op {
		return err
	}
	ms, err := c.memberships(person)
	if err != nil {
		return err
	}
	// The person themself may, from wherever they work — but only from a
	// space that is theirs alone to speak for: a person with a platform row
	// (an operator) changes their sign-in from the platform space, never
	// through a row some tenant gave their address and could hold a key to.
	if person == c.User && !c.boundCredential() {
		if _, platform := membershipIn(ms, ""); !platform || c.Tenant == "" {
			return nil
		}
	}
	if elsewhere(ms, c.Tenant) {
		return cerr.Permission("This person also works elsewhere on this site, so only they or the operator can change how they sign in")
	}
	return nil
}

// isOperator reports whether the ctx acts for an operator, acting in person
// and as the operator: in the platform space, or in a tenant they entered
// without a row of their own there. An operator who works in a tenant as one
// of its members — a row the tenant gave their address — is that member
// there, and holds no more over sign-ins than the tenant does (#167): a
// credential the tenant minted for that row must not carry the site's power.
func (c *Ctx) isOperator() (bool, error) {
	if c.ImpersonatedBy != "" {
		return false, nil
	}
	// an operator's power comes from the platform space, not from a key or a
	// session some tenant could hold
	if c.Tenant != "" {
		if v, _ := c.Ctx.Value(boundKey{}).(bool); v {
			return false, nil
		}
	}
	op, err := c.E.CanEnterTenants(c.Ctx, c.User)
	if err != nil || !op || c.Tenant == "" || c.User == "Admin" || !c.Tenancy() {
		return op, err
	}
	ms, err := c.memberships(c.User)
	if err != nil {
		return false, err
	}
	_, member := membershipIn(ms, c.Tenant)
	return !member, nil
}

// sharedPerson reports whether person has a User row in a space other than
// the ctx's — what a tenant's administrator may not change on their own.
func (c *Ctx) sharedPerson(person string) (bool, error) {
	if !c.Tenancy() {
		return false, nil
	}
	ms, err := c.memberships(person)
	if err != nil {
		return false, err
	}
	return elsewhere(ms, c.Tenant), nil
}

// takeNewPassword takes a User's new_password out of the document before it
// is written, and hashes it under the site's policy. The column is never
// written: the password is the person's, and lands in ddcore_person once the
// row is (storeNewPassword). Who may set it is checked here, in the engine,
// so no controller or extension can skip it.
func (c *Ctx) takeNewPassword(doc Doc) (string, error) {
	pw := doc.Str("new_password")
	if _, ok := doc["new_password"]; ok {
		doc["new_password"] = nil
	}
	if pw == "" {
		return "", nil
	}
	id := doc.ID()
	if err := c.mayManageCredential(id); err != nil {
		return "", err
	}
	identity := doc.Str("email")
	if identity == "" {
		identity = id
	}
	return c.E.HashNewPassword(identity, pw)
}

// storeNewPassword is the second half of takeNewPassword. A changed password
// ends the person's sessions: if it was changed because it leaked, leaving
// them open would change nothing.
func (c *Ctx) storeNewPassword(id, hash string, changed bool) error {
	if err := c.setPersonHash(id, hash, c.chosenBy(id)); err != nil {
		return err
	}
	if changed {
		if _, err := c.E.DropSessions(c.Ctx, c.Q(), id, ""); err != nil {
			return err
		}
	}
	c.AfterCommit(func() { c.E.ClearAttempts(context.Background(), throttleKey("login", id)) })
	return nil
}

// ---- where a person works --------------------------------------------------

type openSpace struct{ id, title string }

// openSpaces are the spaces a person may work in now: each enabled
// membership in an enabled tenant, the platform's first, then by the
// tenant's title.
func (e *Engine) openSpaces(ctx context.Context, user string) ([]openSpace, error) {
	ms, err := e.Memberships(ctx, user)
	if err != nil {
		return nil, err
	}
	var ids []string
	var out []openSpace
	for _, m := range ms {
		if !m.Enabled {
			continue
		}
		if m.Tenant == "" {
			out = append(out, openSpace{})
			continue
		}
		ids = append(ids, m.Tenant)
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id, title FROM `+meta.TenantTable+`
		WHERE id = ANY($1) AND enabled ORDER BY title, id`, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, openSpace{id: db.Str(r["id"]), title: db.Str(r["title"])})
	}
	return out, nil
}

// LandingSpace is where a person's new session works (#167): the space they
// last worked in, while they still may; else the platform, for a person
// with a row there; else the first of their tenants by title. A person
// with nowhere to go is disabled.
func (e *Engine) LandingSpace(ctx context.Context, user string) (string, error) {
	if on, err := e.tenancy(ctx); err != nil || !on {
		return "", err
	}
	key := "landing:" + user
	if v, ok := e.Cache.Get(key); ok {
		return v.(string), nil
	}
	gen := e.Cache.Gen()
	var last *string
	if err := e.System(ctx, func(q db.Querier) error {
		rows, err := db.Select(ctx, q, `SELECT last_tenant FROM ddcore_person WHERE id = $1`, user)
		if err == nil && len(rows) > 0 && rows[0]["last_tenant"] != nil {
			l := db.Str(rows[0]["last_tenant"])
			last = &l
		}
		return err
	}); err != nil {
		return "", err
	}
	space, err := e.landing(ctx, user, last)
	if err != nil {
		return "", err
	}
	e.Cache.SetAt(key, space, time.Minute, gen)
	return space, nil
}

func (e *Engine) landing(ctx context.Context, user string, last *string) (string, error) {
	if user == "Admin" {
		if last != nil && (*last == "" || e.checkTenant(ctx, nil, *last) == nil) {
			return *last, nil
		}
		return "", nil
	}
	operator, err := e.CanEnterTenants(ctx, user)
	if err != nil {
		return "", err
	}
	if operator && last != nil && (*last == "" || e.checkTenant(ctx, nil, *last) == nil) {
		return *last, nil
	}
	spaces, err := e.openSpaces(ctx, user)
	if err != nil {
		return "", err
	}
	if last != nil {
		for _, sp := range spaces {
			if sp.id == *last {
				return sp.id, nil
			}
		}
	}
	if len(spaces) == 0 {
		if operator {
			return "", nil
		}
		return "", cerr.Auth("User is disabled")
	}
	return spaces[0].id, nil
}

// rememberSpace records the space a person last worked in.
func rememberSpace(ctx context.Context, q db.Querier, user, tenant string) error {
	_, err := q.Exec(ctx, `INSERT INTO ddcore_person (id, last_tenant) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET last_tenant = EXCLUDED.last_tenant, modified = now()`, user, tenant)
	return err
}

// personHere reports whether this ctx may ask about person's credential at
// all: the person is the ctx's own user, has a row in the ctx's space, or the
// ctx acts for an operator. A tenant's code must not learn anything about a
// sign-in it has no row for.
func (c *Ctx) personHere(person string) (bool, error) {
	if person == "" {
		return false, nil
	}
	if person == c.User || !c.Tenancy() {
		return true, nil
	}
	if op, err := c.isOperator(); err != nil || op {
		return op, err
	}
	ms, err := c.memberships(person)
	if err != nil {
		return false, err
	}
	_, ok := membershipIn(ms, c.Tenant)
	return ok, nil
}

// UserKey is the cache key of something the process remembers about a user
// in one space: roles, type, language, scopes, shares. A person's rows
// differ from one tenant to the next (#167), so each space has its own key.
// The platform's is the plain prefix+user, the same as on a site without
// tenancy, and a prefix invalidation ("roles:") still reaches every space.
func UserKey(prefix, tenant, user string) string {
	if tenant == "" {
		return prefix + user
	}
	return prefix + user + "\x00" + tenant
}

// userKeyPrefixes are the prefixes UserKey is used with: a key of one of
// them dropped by name (the User controller's ddcore.cache.del) is dropped in
// the ctx's space too.
var userKeyPrefixes = []string{"roles:", "utype:", "lang:", "user_perms:", "shares:"}

// userKeys are the keys of user in the space tenant, for every prefix.
func userKeys(tenant, user string) []string {
	out := make([]string, 0, len(userKeyPrefixes))
	for _, p := range userKeyPrefixes {
		out = append(out, UserKey(p, tenant, user))
	}
	return out
}

// joinPerson keeps a person one id in every space (#167). A User row written
// for an address that already signs in elsewhere on the site joins that
// person, under their id; a changed address may not be another person's.
// It runs under a lock on the address, so two tenants adding the same
// person at once agree on who it is. Nothing here tells the caller about
// the person's other rows: a join is silent.
func (c *Ctx) joinPerson(doc, before Doc) error {
	if !c.Tenancy() {
		return nil
	}
	id, email := doc.ID(), strings.TrimSpace(doc.Str("email"))
	if c.Tenant != "" && (platformAccount(id) || platformAccount(email)) {
		// the framework's own accounts are the platform's, never a tenant's
		return cerr.Validation("User {0} already exists", id)
	}
	if before != nil && strings.EqualFold(strings.TrimSpace(before.Str("email")), email) {
		return nil
	}
	if before != nil {
		// the address sign-in finds the person by is part of how they sign in
		if err := c.mayManageCredential(id); err != nil {
			return err
		}
	}
	return c.siteQ(func(q db.Querier) error {
		for _, k := range []string{id, email} {
			if k == "" {
				continue
			}
			if _, err := q.Exec(c.Ctx, `SELECT pg_advisory_xact_lock(hashtext('ddcore.person:' || lower($1)))`, k); err != nil {
				return err
			}
		}
		rows, err := db.Select(c.Ctx, q, `SELECT DISTINCT id FROM tab_user
			WHERE (lower(id) = lower($1) OR ($2 <> '' AND (lower(email) = lower($2) OR lower(id) = lower($2))))
			  AND NOT (tenant = $3 AND id = $1)`, id, email, c.Tenant)
		if err != nil {
			return err
		}
		switch {
		case len(rows) == 0:
			return nil
		case len(rows) > 1:
			return cerr.Validation("{0} is already used by another account", email)
		}
		person := db.Str(rows[0]["id"])
		if before != nil {
			if person != id {
				return cerr.Validation("{0} is already used by another account", email)
			}
			return nil
		}
		doc["id"] = person
		return c.forgetChosenPassword(q, person)
	})
}

// personRowDeleted is what a User row's delete does to the person. The
// person's sign-in — identities, password, outstanding links — goes with
// their last row only: deleting them from one tenant must not lock them out
// of the others. While it lasts, a sign-in identity left behind would sign a
// stranger into a User later created with the same name.
func (c *Ctx) personRowDeleted(id string) error {
	if err := c.dropPersonCache(id); err != nil {
		return err
	}
	ms, err := c.memberships(id)
	if err != nil {
		return err
	}
	if len(ms) > 0 {
		return nil
	}
	return c.siteQ(func(q db.Querier) error {
		for _, st := range []string{
			`DELETE FROM ddcore_user_identity WHERE "user" = $1`,
			`DELETE FROM ddcore_person WHERE id = $1`,
			`DELETE FROM ddcore_auth_token WHERE "user" = $1 AND used IS NULL`,
		} {
			if _, err := q.Exec(c.Ctx, st, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// personRenamed carries what belongs to the person over to the new id. A
// person with rows in other spaces is renamed by nobody: the id is what they
// are in every one of them, and one tenant renaming it would split them.
func (c *Ctx) personRenamed(oldID, newID string) error {
	ms, err := c.memberships(oldID)
	if err != nil {
		return err
	}
	if elsewhere(ms, c.Tenant) {
		return cerr.Permission("This person also works elsewhere on this site, so they cannot be renamed")
	}
	if others, err := c.memberships(newID); err != nil {
		return err
	} else if elsewhere(others, c.Tenant) {
		return cerr.Validation("{0} is already used by another account", newID)
	}
	if err := c.siteQ(func(q db.Querier) error {
		for _, st := range []string{
			`UPDATE ddcore_user_identity SET "user" = $1 WHERE "user" = $2`,
			`UPDATE ddcore_person SET id = $1 WHERE id = $2`,
		} {
			if _, err := q.Exec(c.Ctx, st, newID, oldID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := c.dropPersonCache(oldID); err != nil {
		return err
	}
	return c.dropPersonCache(newID)
}

// anyEnabled reports whether one of the person's rows lets them in.
func anyEnabled(ms []Membership) bool {
	for _, m := range ms {
		if m.Enabled {
			return true
		}
	}
	return false
}
