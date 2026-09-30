package engine

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestCacheSetAtDropsAValueReadBeforeARemoval(t *testing.T) {
	c := NewCache()
	gen := c.Gen()
	c.Del("user_perms:ana") // the invalidation lands while the read is in flight
	c.SetAt("user_perms:ana", "stale", 0, gen)
	if _, ok := c.Get("user_perms:ana"); ok {
		t.Fatal("a value read before a removal was stored over it")
	}
	gen = c.Gen()
	c.SetAt("user_perms:ana", "fresh", 0, gen)
	if v, _ := c.Get("user_perms:ana"); v != "fresh" {
		t.Fatalf("SetAt with the current generation stored %v", v)
	}
}

// Two engines on one database are two processes: a replica, `ddcore eval
// --commit`, a worker. What one of them changes, the other must stop
// answering from its cache (#45).
func TestCacheInvalidationReachesOtherProcess(t *testing.T) {
	server := setupPerm(t)
	ctx, cancel := context.WithCancel(context.Background())
	listening := make(chan struct{}, 1)
	server.cacheListening = func() { listening <- struct{}{} }
	done := make(chan struct{})
	go func() { server.WatchCache(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-listening:
	case <-time.After(10 * time.Second):
		t.Fatal("WatchCache never listened")
	}

	other, err := New(context.Background(), server.Cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.DB.Close() })

	const user = "ana@x.com"
	scopes := func() []UserPerm {
		t.Helper()
		var perms []UserPerm
		if err := server.Run(context.Background(), user, func(c *Ctx) error {
			var err error
			perms, err = c.UserPermissions()
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return perms
	}
	eventually := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("the server never saw %s", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	// barrier waits until everything the other process broadcast before it
	// has reached the server: notifications arrive in commit order.
	barrier := func() {
		t.Helper()
		server.Cache.Set("test:barrier", true, 0)
		if err := other.Run(context.Background(), "Admin", func(c *Ctx) error {
			return c.broadcastInvalidation([]string{"test:barrier"}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		eventually("the barrier", func() bool { _, ok := server.Cache.Get("test:barrier"); return !ok })
	}

	if got := scopes(); len(got) != 0 {
		t.Fatalf("ana starts unscoped, got %+v", got)
	}
	var name string
	if err := other.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.Insert(Doc{"doctype": "User Permission", "user": user, "allow": "Company", "for_value": "P1"}, SaveOpts{})
		name = doc.ID()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	eventually("the grant", func() bool { p := scopes(); return len(p) == 1 && p[0].ForValue == "P1" })

	// A write rolled back announces nothing: the cached scope stays.
	if err := other.Run(context.Background(), "Admin", func(c *Ctx) error {
		c.Flags["rollback"] = true
		return c.Delete("User Permission", name, false, false)
	}); err != nil {
		t.Fatal(err)
	}
	barrier()
	if _, ok := server.Cache.Get("user_perms:" + user); !ok {
		t.Fatal("a rolled back revocation dropped the server's cache")
	}

	// The revocation is the dangerous direction: the grant must not outlive it.
	if err := other.Run(context.Background(), "Admin", func(c *Ctx) error {
		return c.Delete("User Permission", name, false, false)
	}); err != nil {
		t.Fatal(err)
	}
	eventually("the revocation", func() bool { return len(scopes()) == 0 })

	// Roles have the same shape; the User controller drops them with
	// ddcore.cache.del, which reaches the other processes too.
	roles := func() []string {
		t.Helper()
		var r []string
		server.Run(context.Background(), "Admin", func(c *Ctx) error {
			var err error
			r, err = c.RolesOf(user)
			return err
		})
		return r
	}
	if !slices.Contains(roles(), "Gestor") {
		t.Fatalf("ana starts as Gestor, got %v", roles())
	}
	if err := other.Run(context.Background(), "Admin", func(c *Ctx) error {
		u, err := c.GetDoc("User", user)
		if err != nil {
			return err
		}
		u["roles"] = []any{}
		_, err = c.Save(u, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	eventually("the role revoked", func() bool { return !slices.Contains(roles(), "Gestor") })
}
