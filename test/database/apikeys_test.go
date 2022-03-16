package database

import (
	"context"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
)

// TestAPIKeys ensures the DB operations with API keys work as expected.
func TestAPIKeys(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.UserCreate(ctx, "", "", t.Name(), database.TierFree)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	sl1 := test.RandomSkylink()
	sl2 := test.RandomSkylink()

	// Create a private API key.
	akr1, err := db.APIKeyCreate(ctx, *u, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Create a private API key with skylinks. Expect to fail.
	_, err = db.APIKeyCreate(ctx, *u, false, []string{sl1})
	if err == nil {
		t.Fatal("Managed to create a private API key with skylinks.")
	}
	// Create a public API key
	akr2, err := db.APIKeyCreate(ctx, *u, true, []string{sl1})
	if err != nil {
		t.Fatal(err)
	}
	// Create a public API key without any skylinks.
	akr3, err := db.APIKeyCreate(ctx, *u, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Get an API key.
	akr1a, err := db.APIKeyGet(ctx, akr1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if akr1a.ID.Hex() != akr1.ID.Hex() {
		t.Fatal("Did not get the correct API key!")
	}
	// Get an API key by key.
	akr1a, err = db.APIKeyByKey(ctx, akr1.Key.String())
	if err != nil {
		t.Fatal(err)
	}
	if akr1a.ID.Hex() != akr1.ID.Hex() {
		t.Fatal("Did not get the correct API key by key!")
	}
	// List API keys.
	akrs, err := db.APIKeyList(ctx, *u)
	if err != nil {
		t.Fatal(err)
	}
	if len(akrs) != 3 {
		t.Fatalf("Expected %d API keys, got %d", 3, len(akrs))
	}
	// Check if all keys we expect to exist actually exist.
	found := 0
	for _, akr := range []*database.APIKeyRecord{akr1, akr2, akr3} {
		for _, akrFound := range akrs {
			if akrFound.ID.Hex() == akr.ID.Hex() {
				found++
			}
		}
	}
	if found != 3 {
		t.Fatalf("Expected to find %d API keys we expect, found %d", 3, found)
	}

	// Try to update a general API key. Expect to fail.
	err = db.APIKeyUpdate(ctx, *u, akr1.ID, []string{sl1})
	if err == nil {
		t.Fatal("Expected to be unable to update general API key.")
	}
	err = db.APIKeyPatch(ctx, *u, akr1.ID, []string{sl1}, nil)
	if err == nil {
		t.Fatal("Expected to be unable to patch general API key.")
	}
	// Update a public API key.
	err = db.APIKeyUpdate(ctx, *u, akr2.ID, []string{sl1, sl2})
	if err != nil {
		t.Fatal(err)
	}
	// Verify.
	akr2a, err := db.APIKeyGet(ctx, akr2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !akr2a.CoversSkylink(sl1) || !akr2a.CoversSkylink(sl2) {
		t.Fatal("Expected the API to cover both skylinks.")
	}
	// Patch a public API key.
	err = db.APIKeyPatch(ctx, *u, akr2.ID, nil, []string{sl2})
	if err != nil {
		t.Fatal(err)
	}
	// Verify.
	akr2b, err := db.APIKeyGet(ctx, akr2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !akr2b.CoversSkylink(sl1) || akr2b.CoversSkylink(sl2) {
		t.Fatal("Expected the API to cover one but not the other skylink.")
	}

	// Delete a general API key.
	err = db.APIKeyDelete(ctx, *u, akr1.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Verify.
	akrs, err = db.APIKeyList(ctx, *u)
	if err != nil {
		t.Fatal(err)
	}
	for _, akr := range akrs {
		if akr.ID.Hex() == akr1.ID.Hex() {
			t.Fatal("Expected the API key to be gone but it's not.")
		}
	}
	// Delete a public API key.
	err = db.APIKeyDelete(ctx, *u, akr2.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Verify.
	akrs, err = db.APIKeyList(ctx, *u)
	if err != nil {
		t.Fatal(err)
	}
	for _, akr := range akrs {
		if akr.ID.Hex() == akr2.ID.Hex() {
			t.Fatal("Expected the API key to be gone but it's not.")
		}
	}
}
