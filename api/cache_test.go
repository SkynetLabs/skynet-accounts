package api

import (
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
)

// TestUserTierCache tests that working with userTierCache works as expected.
func TestUserTierCache(t *testing.T) {
	cache := newUserTierCache()
	u := &database.User{
		Sub:             t.Name(),
		Tier:            database.TierPremium5,
		CreatedAt:       time.Now().UTC().Truncate(time.Millisecond),
		SubscribedUntil: time.Now().UTC().Add(100 * time.Hour).Truncate(time.Millisecond),
		QuotaExceeded:   false,
	}
	// Get the user from the empty cache.
	ce, ok := cache.Get(u.Sub)
	if ok || ce.Tier != database.TierAnonymous {
		t.Fatalf("Expected to get tier %d and %t, got %d and %t.", database.TierAnonymous, false, ce.Tier, ok)
	}
	// Set the user in the cache.
	cache.Set(u.Sub, u)
	// Check again.
	ce, ok = cache.Get(u.Sub)
	if !ok || ce.Tier != u.Tier {
		t.Fatalf("Expected to get tier %d and %t, got %d and %t.", u.Tier, true, ce.Tier, ok)
	}
	if ce.QuotaExceeded != u.QuotaExceeded {
		t.Fatal("Quota exceeded flag doesn't match.")
	}
	u.QuotaExceeded = true
	cache.Set(u.Sub, u)
	ce, ok = cache.Get(u.Sub)
	if !ok || ce.Tier != u.Tier {
		t.Fatalf("Expected to get tier %d and %t, got %d and %t.", u.Tier, true, ce.Tier, ok)
	}
	if ce.QuotaExceeded != u.QuotaExceeded {
		t.Fatal("Quota exceeded flag doesn't match.")
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
	u.SubscribedUntil = time.Now().UTC().Add(timeToMonthRollover).Truncate(time.Millisecond)
	// Update the cache.
	cache.Set(u.Sub, u)
	// Expect the cache entry's ExpiresAt to be after 30 minutes.
	timeIn30 := time.Now().UTC().Add(time.Hour - timeToMonthRollover)
	if ce.ExpiresAt.After(timeIn30) && ce.ExpiresAt.Before(timeIn30.Add(time.Second)) {
		t.Fatalf("Expected ExpiresAt to be within 1 second of %s, but it was %s (off by %d ns)", timeIn30.String(), ce.ExpiresAt.String(), (time.Hour - timeIn30.Sub(ce.ExpiresAt)).Nanoseconds())
	}

	// Create a new API key.
	ak := database.NewAPIKey()
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
	ce, ok = cache.Get(string(ak))
	if !ok {
		t.Fatal("Expected the entry to exist.")
	}
	if ce.Tier != u.Tier {
		t.Fatalf("Expected tier %d, got %d", u.Tier, ce.Tier)
	}
}
