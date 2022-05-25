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
		Sub           string
		Tier          int
		QuotaExceeded bool
		ExpiresAt     time.Time
	}
)

// newUserTierCache creates a new userTierCache.
func newUserTierCache() *userTierCache {
	return &userTierCache{
		cache: make(map[string]userTierCacheEntry),
	}
}

// Get returns the user's tier, a quota exceeded flag, and an OK indicator
// which is true when the cache entry exists and hasn't expired, yet.
func (utc *userTierCache) Get(sub string) (userTierCacheEntry, bool) {
	utc.mu.Lock()
	ce, exists := utc.cache[sub]
	utc.mu.Unlock()
	if !exists || ce.ExpiresAt.Before(time.Now().UTC()) {
		anon := userTierCacheEntry{
			Tier: database.TierAnonymous,
		}
		return anon, false
	}
	return ce, true
}

// Set stores the user's tier in the cache under the given key.
func (utc *userTierCache) Set(key string, u *database.User) {
	utc.mu.Lock()
	utc.cache[key] = userTierCacheEntry{
		Sub:           u.Sub,
		Tier:          u.Tier,
		QuotaExceeded: u.QuotaExceeded,
		ExpiresAt:     time.Now().UTC().Add(userTierCacheTTL).Truncate(time.Millisecond),
	}
	utc.mu.Unlock()
}
