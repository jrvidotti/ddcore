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

// testAsUser switches the VM to user until testRestoreUser; the JS side of
// ddcore.test.asUser brackets its callback with the pair. Test mode only.
func (c *Ctx) testAsUser(user string) error {
	if !c.E.Cfg.Test {
		return cerr.Permission("ddcore.test is only available inside ddcore test")
	}
	if user == "" {
		return cerr.Validation("ddcore.test.asUser needs a user")
	}
	child, err := c.userCtx(user)
	if err != nil {
		return err
	}
	child.asUserParent = c
	child.rt.Ctx = child
	return nil
}

// testRestoreUser returns the VM to the ctx that called testAsUser, and
// returns that ctx.
func (c *Ctx) testRestoreUser() *Ctx {
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
	return parent
}

// testRootCtx unwinds every asUser still open, so that a test that threw
// inside one can never leave the next test running as someone else.
func (c *Ctx) testRootCtx() *Ctx {
	for c.asUserParent != nil {
		c = c.testRestoreUser()
	}
	return c
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
