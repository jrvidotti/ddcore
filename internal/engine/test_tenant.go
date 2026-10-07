package engine

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// The scratch tenant of a `ddcore test` run (#107). On a site with tenancy,
// the tests of an app whose `tests.space` is "tenant" — by default an app
// whose DocTypes live inside a tenant — run inside a tenant the run creates
// in its own transaction: every app's onTenantCreate applied, no real
// tenant's documents in sight, and nothing left behind, since the run rolls
// back. Every other app's tests run in the platform space, as they always did.

// createTestTenant creates the scratch tenant when a selected app needs one,
// and records it on the run's root ctx. app is the run's app filter.
func (c *Ctx) createTestTenant(app string) error {
	if !c.Tenancy() {
		return nil
	}
	apps := map[string]bool{}
	for _, name := range c.St.AppOrder() {
		a := c.St.Snap.Apps[name]
		if a != nil && (app == "" || app == name) && a.TestSpace() == SpaceTenant {
			apps[name] = true
		}
	}
	if len(apps) == 0 {
		return nil
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	// random, so two runs at once never wait on each other's uncommitted row
	// and no tenant of the site is ever the one a test writes into
	id := "ddcore-test-" + hex.EncodeToString(b)
	doc, err := c.NewDoc(meta.TenantDocType, Doc{"slug": id, "title": "Test tenant"})
	if err != nil {
		return err
	}
	// the controller runs every app's onTenantCreate in it, as a real one gets
	if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
		return err
	}
	c.testTenant, c.testTenantApps = id, apps
	return nil
}

// testEnter puts the VM where app's tests run: inside the run's scratch
// tenant for an app that runs there, and nowhere else otherwise.
func (c *Ctx) testEnter(app string) error {
	o := c.owner()
	if o.testTenant == "" || !o.testTenantApps[app] || c.Tenant == o.testTenant {
		return nil
	}
	_, err := c.enterTenantCtx(o.testTenant)
	return err
}

// testLeave returns the VM from every tenant a test entered — the scratch
// one, a tenant.run, an inPlatform — to the ctx under them.
func (c *Ctx) testLeave() *Ctx {
	for c.tenantParent != nil {
		next, err := c.leaveTenantCtx()
		if err != nil {
			c.E.Log.Warn("could not return from a tenant", "err", err)
		}
		c = next
	}
	return c
}

// testInPlatform is ddcore.test.inPlatform: from inside a tenant, the
// platform space until tenant.leave. Test mode only — outside a test a
// tenant never leaves its space.
func (c *Ctx) testInPlatform() error {
	if !c.E.Cfg.Test {
		return cerr.Permission("ddcore.test is only available inside ddcore test")
	}
	if !c.Tenancy() {
		return nil // everything is the platform space; tenant.leave then has nothing to undo
	}
	_, err := c.enterSpaceCtx("")
	return err
}
