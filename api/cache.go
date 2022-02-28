package api

import (
	"sync"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
)

const (
	// userTierCacheTTL is the TTL of the entries in the userTierCache.
	userTierCacheTTL = time.Hour
)

type (
	// userTierCache is an in-mem cache that maps from a user's sub to their
	// tier.
	userTierCache struct {
		cache map[string]userTierCacheEntry
		mu    sync.Mutex
	}
	// userTierCacheEntry allows us to cache some basic information about the
	// user, so we don't need to hit the DB to fetch data that rarely changes.
	userTierCacheEntry struct {
		Tier      int
		ExpiresAt time.Time
	}
)

// newUserTierCache creates a new userTierCache.
func newUserTierCache() *userTierCache {
	return &userTierCache{
		cache: make(map[string]userTierCacheEntry),
	}
}

// Get returns the user's tier and an OK indicator which is true when the cache
// entry exists and hasn't expired, yet.
func (utc *userTierCache) Get(sub string) (int, bool) {
	utc.mu.Lock()
	ce, exists := utc.cache[sub]
	utc.mu.Unlock()
	if !exists || ce.ExpiresAt.Before(time.Now().UTC()) {
		return database.TierAnonymous, false
	}
	return ce.Tier, true
}

// Set stores the user's tier in the cache.
func (utc *userTierCache) Set(u *database.User) {
	var ce userTierCacheEntry
	now := time.Now().UTC()
	if u.QuotaExceeded {
		ce = userTierCacheEntry{
			Tier:      database.TierAnonymous,
			ExpiresAt: now.Add(userTierCacheTTL),
		}
	} else {
		ce = userTierCacheEntry{
			Tier:      u.Tier,
			ExpiresAt: now.Add(userTierCacheTTL),
		}
	}
	utc.mu.Lock()
	utc.cache[u.Sub] = ce
	utc.mu.Unlock()
}
