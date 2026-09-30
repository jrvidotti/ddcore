package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func enqueueKeyed(t *testing.T, e *Engine, key string) int64 {
	t.Helper()
	var id int64
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, map[string]any{"uniqueKey": key})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countKeyed(t *testing.T, e *Engine, key string) int {
	t.Helper()
	var n int
	if err := e.DB.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ddcore_job WHERE unique_key = $1`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Without a dedup key, N webhooks for one session queued N jobs that raced on
// it. While one is still queued, the same key queues nothing and names it.
func TestEnqueueUniqueKeyDedupsWhileQueued(t *testing.T) {
	e := setup(t)
	first := enqueueKeyed(t, e, "session:1")
	second := enqueueKeyed(t, e, "session:1")
	if second != first {
		t.Errorf("second enqueue returned %d, want the queued job %d", second, first)
	}
	if n := countKeyed(t, e, "session:1"); n != 1 {
		t.Errorf("%d jobs with the key, want 1", n)
	}
	if other := enqueueKeyed(t, e, "session:2"); other == first {
		t.Error("a different key was deduplicated")
	}
}

// A claimed job frees its key: a message arriving while the session is being
// drained needs a job of its own, or it would wait for the next message.
func TestEnqueueUniqueKeyFreedOnceClaimed(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	first := enqueueKeyed(t, e, "session:1")
	for _, status := range []string{"running", "done"} {
		if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = $2 WHERE id = $1`, first, status); err != nil {
			t.Fatal(err)
		}
		next := enqueueKeyed(t, e, "session:1")
		if next == first {
			t.Fatalf("enqueue while %s returned the old job", status)
		}
		if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'done' WHERE id = $1`, next); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnqueueWithoutUniqueKeyNeverDedups(t *testing.T) {
	e := setup(t)
	var a, b int64
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		if a, err = c.Enqueue("demo.services.loop.ok", nil, nil); err != nil {
			return err
		}
		b, err = c.Enqueue("demo.services.loop.ok", nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two unkeyed enqueues shared a job")
	}
}

// A rolled-back enqueue leaves no job behind, so the key is free again.
func TestEnqueueUniqueKeyRolledBackWithTheCaller(t *testing.T) {
	e := setup(t)
	boom := errors.New("boom")
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		if _, err := c.Enqueue("demo.services.loop.ok", nil, map[string]any{"uniqueKey": "session:1"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the callback's error", err)
	}
	if n := countKeyed(t, e, "session:1"); n != 0 {
		t.Fatalf("%d jobs survived the rollback", n)
	}
	enqueueKeyed(t, e, "session:1")
}

func TestEnqueueUniqueKeyConcurrent(t *testing.T) {
	e := setup(t)
	const n = 8
	ids := make([]int64, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i] = enqueueKeyed(t, e, "session:1")
		}(i)
	}
	wg.Wait()
	if got := countKeyed(t, e, "session:1"); got != 1 {
		t.Errorf("%d jobs with the key, want 1", got)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("ids = %v, want all the same", ids)
		}
	}
}

func TestEnqueueRejectsABadUniqueKey(t *testing.T) {
	e := setup(t)
	for _, v := range []any{42, ""} {
		err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
			_, err := c.Enqueue("demo.services.loop.ok", nil, map[string]any{"uniqueKey": v})
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "uniqueKey") {
			t.Errorf("uniqueKey %#v: err = %v, want a validation error naming it", v, err)
		}
	}
}
