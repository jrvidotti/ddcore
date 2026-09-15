package engine

import (
	"sync"
	"time"
)

// Cache is a process-local TTL cache (no Redis: one process per instance).
type Cache struct {
	mu    sync.Mutex
	items map[string]cacheItem
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
	it := cacheItem{v: v}
	if ttl > 0 {
		it.exp = time.Now().Add(ttl)
	}
	c.items[key] = it
}

func (c *Cache) Del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// DelPrefix removes every cache entry whose key begins with prefix.
func (c *Cache) DelPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.items, key)
		}
	}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = map[string]cacheItem{}
}
