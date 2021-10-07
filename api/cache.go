package api

import (
	"sync"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
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
		Tier       int
		LastUpdate time.Time
	}
)

// Get returns the user's tier and an OK indicator which is true when the cache
// entry exists and hasn't expired, yet.
func (utc *userTierCache) Get(sub string) (int, bool) {
	utc.mu.Lock()
	ce, exists := utc.cache[sub]
	utc.mu.Unlock()
	if !exists || time.Now().UTC().Sub(ce.LastUpdate) > userTierCacheTTL {
		return database.TierAnonymous, false
	}
	return ce.Tier, true
}

// Set stores the user's tier in the cache.
func (utc *userTierCache) Set(u *database.User) {
	var ce userTierCacheEntry
	now := time.Now().UTC()
	if u.SubscribedUntil.Before(now) {
		// The user is unsubscribed. Cache them as TierAnonymous and we'll purge
		// the cache in case they subscribe.
		ce = userTierCacheEntry{
			Tier:       database.TierAnonymous,
			LastUpdate: now,
		}
	} else if u.QuotaExceeded {
		// If their month rollover time is in less than an hour, adjust the
		// LastUpdate time, so the cache expires right before that.
		lastUpdated := now
		su := u.SubscribedUntil
		t := time.Date(now.Year(), now.Month(), su.Day(), su.Hour(), su.Minute(), su.Second(), su.Nanosecond(), time.UTC)
		if t.Sub(now) < time.Hour {
			lastUpdated = t.Add(-time.Hour)
		}
		ce = userTierCacheEntry{
			Tier:       database.TierAnonymous,
			LastUpdate: lastUpdated,
		}
	} else {
		ce = userTierCacheEntry{
			Tier:       u.Tier,
			LastUpdate: now,
		}
	}
	utc.mu.Lock()
	utc.cache[u.Sub] = ce
	utc.mu.Unlock()
}
