package engine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jrvidotti/ddcore/internal/db"
)

// cacheChannel carries cache invalidations between the processes that share
// a database: a server, its replicas, `ddcore jobs work`, the MCP server, and
// the one-shot commands (`eval --commit`, `exec`, `import`) that write through
// the same engine code. The cache answers who may see what — scopes, roles,
// shares, sessions — so a process that kept serving what another one revoked
// would leak data between tenants until it restarted.
const cacheChannel = "ddcore_cache"

// cacheNotifyMax keeps a payload under Postgres's 8000-byte NOTIFY limit; a
// larger one is sent as "drop everything", which is always correct.
const cacheNotifyMax = 7000

type cacheInvalidation struct {
	Keys     []string `json:"k,omitempty"`
	Prefixes []string `json:"p,omitempty"`
	All      bool     `json:"all,omitempty"`
}

// broadcastInvalidation tells every other process listening on the database
// to drop keys and every key under prefixes. It does not touch this
// process's cache: each caller already drops its own entries when it always
// did, now or after commit.
//
// Inside a transaction Postgres delivers the notification only if and when
// it commits — a rollback, or a savepoint rolled back, sends nothing — so
// the other processes never drop a value for a change that did not happen,
// nor before the change is visible to the read that reloads it. An error
// aborts the transaction, which is what the caller returns: a revocation the
// other processes could not be told about must not commit.
func (c *Ctx) broadcastInvalidation(keys, prefixes []string) error {
	if c.Tx == nil && c.E.DB == nil {
		return nil
	}
	return broadcastInvalidation(c.Ctx, c.Q(), keys, prefixes)
}

// broadcastInvalidation is Ctx.broadcastInvalidation for the paths that hold
// a querier and no Ctx: q is a transaction or the pool.
func broadcastInvalidation(ctx context.Context, q db.Querier, keys, prefixes []string) error {
	if len(keys) == 0 && len(prefixes) == 0 {
		return nil
	}
	payload, err := json.Marshal(cacheInvalidation{Keys: keys, Prefixes: prefixes})
	if err != nil {
		return err
	}
	if len(payload) > cacheNotifyMax {
		return broadcastClear(ctx, q)
	}
	_, err = q.Exec(ctx, `SELECT pg_notify($1, $2)`, cacheChannel, string(payload))
	return err
}

// broadcastClear tells the other processes to drop their whole cache.
func broadcastClear(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT pg_notify($1, $2)`, cacheChannel, `{"all":true}`)
	return err
}

// apply drops what a notification names from this process's cache.
func (inv cacheInvalidation) apply(cache *Cache) {
	if inv.All {
		cache.Clear()
		return
	}
	for _, k := range inv.Keys {
		cache.Del(k)
	}
	for _, p := range inv.Prefixes {
		cache.DelPrefix(p)
	}
}

// WatchCache listens for the invalidations other processes broadcast and
// applies them to this process's cache until ctx is done. Every long-running
// process calls it: the server, `ddcore jobs work`, the MCP server.
//
// It holds a connection of its own rather than one of the pool's, so closing
// the pool never waits on it. Each time the LISTEN is (re)established the
// whole cache is dropped: whatever was broadcast while nobody listened is
// gone, and a cache cleared is only a cache cold.
func (e *Engine) WatchCache(ctx context.Context) {
	if e.Cfg.DSN == "" {
		return
	}
	backoff := time.Second
	for {
		listened, err := e.listenCache(ctx)
		if ctx.Err() != nil {
			return
		}
		if listened {
			backoff = time.Second
		}
		e.Log.Warn("cache invalidation listener lost; retrying", "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// listenCache reports whether it got as far as listening, which is what
// resets WatchCache's backoff.
func (e *Engine) listenCache(ctx context.Context) (bool, error) {
	// Parsed as the pool parses it: pgx.Connect would send pool_max_conns
	// and the other pool_* settings a site's DSN may carry to Postgres as
	// runtime parameters, which it refuses (#67).
	cfg, err := pgxpool.ParseConfig(e.Cfg.DSN)
	if err != nil {
		return false, err
	}
	conn, err := pgx.ConnectConfig(ctx, cfg.ConnConfig)
	if err != nil {
		return false, err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN "+cacheChannel); err != nil {
		return false, err
	}
	e.Cache.Clear()
	if e.cacheListening != nil {
		e.cacheListening()
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, err
		}
		var inv cacheInvalidation
		if err := json.Unmarshal([]byte(n.Payload), &inv); err != nil {
			// not ours to read, so not ours to trust: drop everything
			inv = cacheInvalidation{All: true}
		}
		inv.apply(e.Cache)
	}
}
