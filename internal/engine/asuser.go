package engine

import (
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// userCtx builds a ctx acting as user on this ctx's transaction and VM. It
// never inherits the privileged flags, roles or document cache, and it reads
// the user's roles, scopes, shares and user type from the transaction rather
// than the process cache: a row written a moment ago is not in the cache yet,
// and a row a test will roll back must never reach it.
func (c *Ctx) userCtx(user string) (*Ctx, error) {
	rt, err := c.RT()
	if err != nil {
		return nil, err
	}
	child := c.E.NewCtx(c.Ctx, user)
	child.St, child.Tx, child.rt = c.St, c.Tx, rt
	child.txOwner = c.owner()
	child.Lang = c.Lang
	child.savepoint, child.roSavepoint = c.savepoint, c.roSavepoint
	if err := c.enterUserSpace(child); err != nil {
		return nil, err
	}
	switch child.User {
	case "Admin", "Guest":
		child.roles, _ = c.RolesOf(child.User)
		return child, nil
	}
	if child.shares, err = child.loadShares(user); err != nil {
		return nil, err
	}
	child.sharesLoaded, child.sharesDirty = true, true
	// Authorization must observe role revocation even within this transaction,
	// before the ordinary role cache's after-commit invalidation.
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT role FROM tab_has_role WHERE parent=$1 AND parenttype='User'`, user)
	if err != nil {
		return nil, err
	}
	child.roles = []string{"All"}
	for _, row := range rows {
		child.roles = append(child.roles, db.Str(row["role"]))
	}
	if child.userPerms, err = child.loadUserPermissions(user); err != nil {
		return nil, err
	}
	if child.userType, err = child.loadUserType(user); err != nil {
		return nil, err
	}
	return child, nil
}

// withUser runs fn as user, sharing this ctx's transaction and VM. Rebinding
// avoids acquiring a second pool slot while every concurrent writer already
// holds one.
func (c *Ctx) withUser(user string, fn func(*Ctx) error) error {
	return c.withUserLang(user, c.Lang, fn)
}

func (c *Ctx) withUserLang(user, lang string, fn func(*Ctx) error) error {
	child, err := c.userCtx(user)
	if err != nil {
		return err
	}
	child.Lang = lang
	rt := child.rt
	old := rt.Ctx
	rt.Ctx = child
	rt.SetLang(child.Lang)
	defer func() { rt.Ctx = old; rt.SetLang(c.Lang) }()
	ferr := fn(child)
	if err := c.leaveUserSpace(child); err != nil && ferr == nil {
		return err
	}
	return ferr
}

// enterUser switches the VM to user until restoreUser. The JS side brackets its
// callback with the pair, in a try/finally.
func (c *Ctx) enterUser(user string) (*Ctx, error) {
	child, err := c.userCtx(user)
	if err != nil {
		return nil, err
	}
	child.asUserParent = c
	child.rt.Ctx = child
	return child, nil
}

// restoreUser returns the VM to the ctx that called enterUser, and returns
// that ctx.
func (c *Ctx) restoreUser() *Ctx {
	parent := c.asUserParent
	if parent == nil {
		return c
	}
	c.asUserParent = nil
	parent.rt.Ctx = parent
	// the child wrote through its own document cache
	parent.docCache = map[string]Doc{}
	if err := parent.leaveUserSpace(c); err != nil {
		parent.E.Log.Warn("could not return to the tenant of the calling user", "err", err)
	}
	parent.Messages = append(parent.Messages, c.Messages...)
	return parent
}

// checkActingUser refuses a user that code cannot act as: one that does not
// exist, or one that is disabled. It fails closed, because the alternative to
// the user's scopes is no scope at all.
func (c *Ctx) checkActingUser(user string) error {
	switch user {
	case "":
		return cerr.Validation("runAs needs a user")
	case "Admin", "Guest":
		return nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT enabled FROM tab_user WHERE id = $1`, user)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return cerr.Validation("runAs: user {0} does not exist", user)
	}
	if en, ok := rows[0]["enabled"].(bool); ok && !en {
		return cerr.Permission("runAs: user {0} is disabled", user)
	}
	return nil
}

// runAsEnter is ddcore.runAs: the code that follows reads and writes under
// user's roles and access scopes, whatever the calling ctx was allowed. The
// privileged flags are not inherited; what describes the unit of work rather
// than its privileges — the request, and the maintenance window it may be
// running inside — is.
func (c *Ctx) runAsEnter(user string) error {
	if err := c.checkActingUser(user); err != nil {
		return err
	}
	child, err := c.enterUser(user)
	if err != nil {
		return err
	}
	child.Request, child.ReqID = c.Request, c.ReqID
	if c.Flags[bypassMaintenanceFlag] == true {
		child.Flags[bypassMaintenanceFlag] = true
	}
	return nil
}

// testAsUser is ddcore.test.asUser: enterUser without the checks on the user,
// so a test can act as a user it never created. Test mode only.
func (c *Ctx) testAsUser(user string) error {
	if !c.E.Cfg.Test {
		return cerr.Permission("ddcore.test is only available inside ddcore test")
	}
	if user == "" {
		return cerr.Validation("ddcore.test.asUser needs a user")
	}
	_, err := c.enterUser(user)
	return err
}

// testRootCtx unwinds every asUser still open, and every tenant entered —
// the scratch one, a tenant.run, an inPlatform — so that a test that threw
// inside one can never leave the next test running as someone else, or
// somewhere else.
func (c *Ctx) testRootCtx() *Ctx {
	for {
		switch {
		case c.asUserParent != nil:
			c = c.restoreUser()
		case c.tenantParent != nil:
			c = c.testLeave()
		default:
			return c
		}
	}
}

// enterUserSpace puts a ctx that borrows this one's transaction in the space
// of its user. From the platform space that is how the operator's code acts
// as a tenant's user; from inside a tenant only that tenant's users, and
// Admin, can be acted as. The roles, scopes and shares userCtx reads next are
// then read where the user's rows are.
func (c *Ctx) enterUserSpace(child *Ctx) error {
	if !c.Tenancy() {
		return nil
	}
	child.Tenant, child.spaced = c.Tenant, true
	switch child.User {
	case "Admin", "Guest":
		return nil
	}
	own, found := "", false
	if err := c.elevated(func() error {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT tenant FROM tab_user WHERE id = $1`, child.User)
		if err == nil && len(rows) > 0 {
			own, found = db.Str(rows[0]["tenant"]), true
		}
		return err
	}); err != nil {
		return err
	}
	if !found || own == c.Tenant {
		return nil
	}
	if c.Tenant != "" {
		return cerr.Permission("A user of one tenant cannot work in another")
	}
	if err := c.E.checkTenant(c.Ctx, c.Tx, own); err != nil {
		return err
	}
	child.Tenant = own
	return child.applySpace()
}

// leaveUserSpace returns the transaction to this ctx's space after child.
func (c *Ctx) leaveUserSpace(child *Ctx) error {
	if child.Tenant == c.Tenant {
		return nil
	}
	return c.applySpace()
}
