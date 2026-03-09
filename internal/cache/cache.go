package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

// Cache is a thread-safe in-memory cache with TTL expiration.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
}

// New creates a cache with the given TTL for entries.
func New(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]entry),
		ttl:     ttl,
	}
}

// Get retrieves a value. Returns (value, true) on hit, (nil, false) on miss or expired.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(e.expiresAt) {
		if ok {
			c.Delete(key)
		}
		return nil, false
	}
	return e.value, true
}

// Set stores a value with the cache's default TTL.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	c.entries[key] = entry{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]entry)
	c.mu.Unlock()
}

// Key builds a cache key from components.
func Key(parts ...string) string {
	k := ""
	for i, p := range parts {
		if i > 0 {
			k += ":"
		}
		k += p
	}
	return k
}
