// Package cache holds Redfish responses for a bounded time, so that walking
// back up a resource tree is instant and a BMC is not asked for the same
// resource on every keypress.
package cache

import (
	"sync"
	"time"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// entry is one cached response together with when it was stored.
type entry struct {
	response *redfish.Response
	storedAt time.Time
}

// Cache is an in-memory store of responses that expire after a fixed time. It
// is safe for concurrent use. A zero or negative time to live disables it
// entirely.
//
// Every response is cached, error statuses included: a 404 or a 501 is a
// finding worth showing instantly, and re-fetching it changes nothing.
// Transport failures never reach the cache, because they produce no response;
// the user can retry them by re-navigating or reloading.
type Cache struct {
	mu sync.Mutex

	ttl     time.Duration
	now     func() time.Time
	entries map[string]entry
}

// New returns a cache whose entries expire after ttl. A ttl of zero or less
// disables caching: Set does nothing and Get always misses.
func New(ttl time.Duration) *Cache {
	return &Cache{
		ttl:     ttl,
		now:     time.Now,
		entries: map[string]entry{},
	}
}

// Get returns the response stored under key, if it is still fresh. An expired
// entry is reported as a miss and dropped.
//
// Keys are absolute URLs, so that they stay correct if rfx ever talks to more
// than one endpoint.
func (c *Cache) Get(key string) (*redfish.Response, bool) {
	if !c.enabled() {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	stored, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if c.now().Sub(stored.storedAt) >= c.ttl {
		delete(c.entries, key)

		return nil, false
	}

	return stored.response, true
}

// Set stores a response under key, replacing any earlier one. A nil response is
// ignored.
func (c *Cache) Set(key string, response *redfish.Response) {
	if !c.enabled() || response == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{response: response, storedAt: c.now()}
}

// Invalidate drops the entry stored under key, if there is one.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Len reports how many entries are held, expired ones included.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// enabled reports whether the cache stores anything at all.
func (c *Cache) enabled() bool {
	return c.ttl > 0
}
