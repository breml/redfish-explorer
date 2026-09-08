package cache

import "time"

// SetClock replaces the cache's clock, so that expiry can be tested without
// sleeping. It exists for tests only.
func (c *Cache) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = now
}
