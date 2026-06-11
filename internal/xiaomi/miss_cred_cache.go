package xiaomi

import (
	"sync"
	"time"
)

// Keep MISS credentials short-lived: they are only meant to bridge the
// near-simultaneous startup of dual-channel streams, not later reconnects.
const missCredTTL = 30 * time.Second

var missCreds = newMissCredCache(missCredTTL, time.Now)

type missCredKey struct {
	userID string
	region string
	did    string
}

type missCredEntry struct {
	cred      missCred
	expiresAt time.Time
}

type missCredCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	now   func() time.Time
	items map[missCredKey]missCredEntry
}

func newMissCredCache(ttl time.Duration, now func() time.Time) *missCredCache {
	if now == nil {
		now = time.Now
	}
	return &missCredCache{
		ttl:   ttl,
		now:   now,
		items: make(map[missCredKey]missCredEntry),
	}
}

func (c *missCredCache) Get(key missCredKey) (missCred, bool) {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[key]
	if !ok {
		return missCred{}, false
	}
	if !now.Before(entry.expiresAt) {
		delete(c.items, key)
		return missCred{}, false
	}
	return entry.cred, true
}

func (c *missCredCache) Set(key missCredKey, cred missCred) {
	c.mu.Lock()
	c.items[key] = missCredEntry{
		cred:      cred,
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}
