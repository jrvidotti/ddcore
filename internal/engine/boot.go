package engine

import (
	"context"
)

// Boot runs each app's onBoot hook, once per process: after the server
// listens, or after a worker-only process starts its workers (#126).
//
// It is not afterMigrate. That one runs inside the migration's transaction,
// before the server answers, on every `ddcore migrate` and every reload of
// `ddcore dev`, and not at all when the boot migration is off. A hook that has
// a provider call the site back (a webhook registration) needs the server up,
// and an app's reaction to its deployment needs to run whether or not the
// schema changed. A reload of the definitions does not run it again.
//
// Each app's hook runs in a transaction of its own, as Admin in the platform
// space, so one that fails is logged and rolled back without keeping the next
// app's from running or the process from serving.
func (e *Engine) Boot(ctx context.Context) {
	e.bootOnce.Do(func() {
		st := e.Current()
		if st == nil {
			return
		}
		for _, name := range st.AppOrder() {
			if a := st.Snap.Apps[name]; a == nil || !a.HasOnBoot {
				continue
			}
			if e.Paused(ctx) {
				// the writes it would make are refused; say so instead of
				// logging a maintenance error per app
				e.Log.Warn("onBoot skipped: the site is in maintenance mode", "app", name)
				continue
			}
			err := e.Run(ctx, "Admin", func(c *Ctx) error {
				rt, err := c.RT()
				if err != nil {
					return err
				}
				return rt.AppHook(name, "onBoot")
			})
			if err != nil {
				e.Log.Error("onBoot failed", "app", name, "err", err)
			}
		}
	})
}
