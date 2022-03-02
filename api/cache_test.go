package api

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestUserTierCache tests that working with userTierCache works as expected.
func TestUserTierCache(t *testing.T) {
	cache := newUserTierCache()
	u := &database.User{
		Sub:             t.Name(),
		Tier:            database.TierPremium5,
		SubscribedUntil: time.Now().UTC().Add(100 * time.Hour),
		QuotaExceeded:   false,
	}
	// Get the user from the empty cache.
	tier, ok := cache.Get(u.Sub)
	if ok || tier != database.TierAnonymous {
		t.Fatalf("Expected to get tier %d and %t, got %d and %t.", database.TierAnonymous, false, tier, ok)
	}
	// Set the use in the cache.
	cache.Set(u.Sub, u)
	// Check again.
	tier, ok = cache.Get(u.Sub)
	if !ok || tier != u.Tier {
		t.Fatalf("Expected to get tier %d and %t, got %d and %t.", u.Tier, true, tier, ok)
	}
	ce, exists := cache.cache[u.Sub]
	if !exists {
		t.Fatal("Expected the entry to exist.")
	}
	// Expect the cache entry's ExpiresAt to be after an hour.
	timeInAnHour := time.Now().UTC().Add(time.Hour)
	if ce.ExpiresAt.After(timeInAnHour) && ce.ExpiresAt.Before(timeInAnHour.Add(time.Second)) {
		t.Fatalf("Expected ExpiresAt to be within 1 second of %s, but it was %s (off by %d ns)", timeInAnHour.String(), ce.ExpiresAt.String(), (time.Hour - timeInAnHour.Sub(ce.ExpiresAt)).Nanoseconds())
	}
	// Set the user's end-of-month to be within 1 hour.
	timeToMonthRollover := 30 * time.Minute
	u.SubscribedUntil = time.Now().UTC().Add(timeToMonthRollover)
	// Update the cache.
	cache.Set(u.Sub, u)
	// Expect the cache entry's ExpiresAt to be after 30 minutes.
	timeIn30 := time.Now().UTC().Add(time.Hour - timeToMonthRollover)
	if ce.ExpiresAt.After(timeIn30) && ce.ExpiresAt.Before(timeIn30.Add(time.Second)) {
		t.Fatalf("Expected ExpiresAt to be within 1 second of %s, but it was %s (off by %d ns)", timeIn30.String(), ce.ExpiresAt.String(), (time.Hour - timeIn30.Sub(ce.ExpiresAt)).Nanoseconds())
	}

	// Create a new API key.
	ak := database.APIKey(base64.URLEncoding.EncodeToString(fastrand.Bytes(database.PubKeySize)))
	if !ak.IsValid() {
		t.Fatal("Invalid API key.")
	}
	// Try to get a value from the cache. Expect this to fail.
	_, ok = cache.Get(string(ak))
	if ok {
		t.Fatal("Did not expect to get a cache entry!")
	}
	// Update the cache with a custom key.
	cache.Set(string(ak), u)
	// Fetch the data for the custom key.
	tier, ok = cache.Get(string(ak))
	if !ok {
		t.Fatal("Expected the entry to exist.")
	}
	if tier != u.Tier {
		t.Fatalf("Expected tier %+v, got %+v", u.Tier, tier)
	}
}
