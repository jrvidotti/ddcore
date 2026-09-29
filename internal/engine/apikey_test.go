package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// A key's secret is hashed once per cached row, not once per request; a wrong
// secret is still hashed and counted, and dropping the cache entry — what the
// API Key and User controllers do — forgets the verification.
func TestAPIKeyVerifiedSecretSkipsArgon2(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	token, err := e.CreateAPIKey(ctx, "Admin", "bench")
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := strings.Cut(token, ":")

	auth := func(tok string) (user string, hashes int64) {
		t.Helper()
		before := argon2Calls.Load()
		u, err := e.UserFromAPIKey(ctx, tok)
		if err != nil {
			t.Fatal(err)
		}
		return u, argon2Calls.Load() - before
	}

	if u, n := auth(token); u != "Admin" || n != 1 {
		t.Fatalf("first call: user=%q hashes=%d, want Admin and 1", u, n)
	}
	for range 5 {
		if u, n := auth(token); u != "Admin" || n != 0 {
			t.Fatalf("repeat call: user=%q hashes=%d, want Admin and 0", u, n)
		}
	}

	if u, n := auth(key + ":wrong"); u != "" || n != 1 {
		t.Fatalf("wrong secret: user=%q hashes=%d, want none and 1", u, n)
	}
	if v, ok := e.Cache.Get("apikeyfail:" + key); !ok || v.(int) != 1 {
		t.Fatalf("wrong secret not counted: %v %v", v, ok)
	}
	if u, n := auth(token); u != "Admin" || n != 0 {
		t.Fatalf("after a wrong secret: user=%q hashes=%d, want Admin and 0", u, n)
	}

	e.Cache.Del("apikey:" + key)
	if u, n := auth(token); u != "Admin" || n != 1 {
		t.Fatalf("after invalidation: user=%q hashes=%d, want Admin and 1", u, n)
	}

	// A disabled key stops working as soon as its entry is dropped, even for
	// the secret that verified a moment ago.
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SetValue("API Key", key, Doc{"enabled": false})
	}); err != nil {
		t.Fatal(err)
	}
	e.Cache.Del("apikey:" + key)
	if u, _ := auth(token); u != "" {
		t.Fatalf("disabled key authenticated as %q", u)
	}
}

// The owner of a key keeps working while someone trips its failure brake.
func TestAPIKeyBrakeSparesVerifiedSecret(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	token, err := e.CreateAPIKey(ctx, "Admin", "bench")
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := strings.Cut(token, ":")
	if u, _ := e.UserFromAPIKey(ctx, token); u != "Admin" {
		t.Fatalf("got %q", u)
	}
	e.Cache.Set("apikeyfail:"+key, 20, 0)
	if u, _ := e.UserFromAPIKey(ctx, token); u != "Admin" {
		t.Fatalf("braked key refused its verified secret: %q", u)
	}
	before := argon2Calls.Load()
	if u, _ := e.UserFromAPIKey(ctx, key+":junk"); u != "" {
		t.Fatalf("junk authenticated as %q", u)
	}
	if n := argon2Calls.Load() - before; n != 0 {
		t.Fatalf("braked junk secret was hashed %d times", n)
	}
}

// Requests that arrive together on a cold entry share one hash instead of
// allocating 64 MiB each.
func TestAPIKeyConcurrentColdChecksShareAHash(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	token, err := e.CreateAPIKey(ctx, "Admin", "bench")
	if err != nil {
		t.Fatal(err)
	}
	before := argon2Calls.Load()
	start := make(chan struct{})
	var wg sync.WaitGroup
	users := make([]string, 20)
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			users[i], _ = e.UserFromAPIKey(ctx, token)
		}()
	}
	close(start)
	wg.Wait()
	for _, u := range users {
		if u != "Admin" {
			t.Fatalf("a concurrent check failed: %q", u)
		}
	}
	if n := argon2Calls.Load() - before; n > 3 {
		t.Fatalf("20 concurrent checks of one secret hashed %d times", n)
	}
}
