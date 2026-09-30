package engine

import (
	"strings"
	"sync"
	"time"
)

// Cache is a process-local TTL cache (no Redis). What one process drops,
// every other process sharing the database drops too when it goes through
// Ctx.broadcastInvalidation and those processes run WatchCache.
type Cache struct {
	mu    sync.Mutex
	items map[string]cacheItem
	// gen counts removals. A reader takes it before loading from the database
	// and hands it back to SetAt, so a value read before an invalidation that
	// landed while the read was in flight is not stored over it.
	gen uint64
}

type cacheItem struct {
	v   any
	exp time.Time
}

func NewCache() *Cache { return &Cache{items: map[string]cacheItem{}} }

func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || (!it.exp.IsZero() && time.Now().After(it.exp)) {
		delete(c.items, key)
		return nil, false
	}
	return it.v, true
}

func (c *Cache) Set(key string, v any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set(key, v, ttl)
}

// Gen returns the removal count that SetAt compares against.
func (c *Cache) Gen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// SetAt is Set for a value loaded after Gen returned gen: it is dropped when
// anything was removed since, because the removal may have been the one that
// made the value stale. The next read loads it again.
func (c *Cache) SetAt(key string, v any, ttl time.Duration, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return
	}
	c.set(key, v, ttl)
}

func (c *Cache) set(key string, v any, ttl time.Duration) {
	it := cacheItem{v: v}
	if ttl > 0 {
		it.exp = time.Now().Add(ttl)
	}
	c.items[key] = it
}

func (c *Cache) Del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	delete(c.items, key)
}

// DelPrefix removes every cache entry whose key begins with prefix.
func (c *Cache) DelPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.items = map[string]cacheItem{}
}
