package engine

import "github.com/jrvidotti/ddcore/internal/cerr"

// testOnly is an op that only runs under ddcore test.
func testOnly(fn func(e *Engine, c *Ctx, a *hostArgs) error) hostOp {
	return func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
		if err := e.requireTestMode(); err != nil {
			return nil, err
		}
		return nil, fn(e, c, a)
	}
}

func init() {
	addHostOps(map[string]hostOp{
		// the savepoint first, so the rollback takes back what the test
		// wrote in the scratch tenant too
		"test.begin": testOnly(func(e *Engine, c *Ctx, a *hostArgs) error {
			if err := c.Begin(); err != nil {
				return err
			}
			return c.testEnter(a.App)
		}),
		"test.rollback": testOnly(func(e *Engine, c *Ctx, _ *hostArgs) error { return c.testRootCtx().RollbackTo() }),
		"test.enter":    testOnly(func(e *Engine, c *Ctx, a *hostArgs) error { return c.testEnter(a.App) }),
		"test.leave": testOnly(func(e *Engine, c *Ctx, _ *hostArgs) error {
			c.testRootCtx()
			return nil
		}),
		"test.inPlatform": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) { return nil, c.testInPlatform() },
		"test.asUser":     func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return nil, c.testAsUser(a.User) },
		"test.restoreUser": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			c.restoreUser()
			return nil, nil
		},
		"runAs.enter": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return nil, c.runAsEnter(a.User) },
		"runAs.exit": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			c.restoreUser()
			return nil, nil
		},
		"tenant.current": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) { return c.Tenant, nil },
		"tenant.list":    func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) { return c.TenantList() },
		"tenant.enter": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if err := c.requireTenancy(); err != nil {
				return nil, err
			}
			if c.Tenant != "" && c.Tenant != a.Key {
				return nil, cerr.Permission("A tenant cannot enter another tenant")
			}
			// entering the tenant the ctx is already in still nests, so that the
			// leave that follows has something to undo
			_, err := c.enterTenantCtx(a.Key)
			return nil, err
		},
		"tenant.leave": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			if _, err := c.leaveTenantCtx(); err != nil {
				e.Log.Warn("could not return from a tenant", "err", err)
			}
			return nil, nil
		},
		"tenant.created": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if c.Tenant != "" {
				return nil, cerr.Permission("Tenants are managed from the platform space")
			}
			return nil, c.TenantCreated(a.Key)
		},
	})
}
