package test

import (
	"context"
	"reflect"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
	"gitlab.com/NebulousLabs/errors"
)

// TestDatabase_UserBySub ensures UserBySub works as expected.
// This method also test UserCreate.
func TestDatabase_UserBySub(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A random sub that shouldn't exist in the DB.
	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	// Test finding a non-existent user. This should fail.
	_, err = db.UserBySub(ctx, sub, false)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v\n", err)
	}

	// Add a user to find.
	u, err := db.UserCreate(nil, sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(nil, user)
	}(u)

	// Test finding an existent user. This should pass.
	u1, err := db.UserBySub(ctx, u.Sub, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, u1) {
		t.Fatalf("User not equal to original: %v vs %v\n", u, u1)
	}
}

// TestDatabase_UserByID ensures UserByID works as expected.
func TestDatabase_UserByID(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Test finding a non-existent user. This should fail.
	_, err = db.UserByID(ctx, "5fac383fdafc482e510627c3")
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v\n", err)
	}

	// Add a user to find.
	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	u, err := db.UserCreate(nil, sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)

	// Test finding an existent user. This should pass.
	u1, err := db.UserByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, u1) {
		t.Fatalf("User not equal to original: %v vs %v\n", u, u1)
	}
}

// TestDatabase_UserUpdate ensures UserUpdate works as expected.
func TestDatabase_UserUpdate(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	u, err := db.UserCreate(nil, sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)

	// Test changing the user's tier.
	u.Tier = database.TierPremium5
	err = db.UserUpdate(ctx, u)
	if err != nil {
		t.Fatal("Failed to update user:", err)
	}
	u1, err := db.UserByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal("Failed to load user:", err)
	}
	if u1.Tier != database.TierPremium5 {
		t.Fatalf("Expected tier '%d', got '%d'\n", database.TierPremium5, u1.Tier)
	}
}

// TestDatabase_UserDelete ensures UserDelete works as expected.
func TestDatabase_UserDelete(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	// Add a user to delete.
	u, err := db.UserCreate(nil, sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Make sure the user is there.
	fu, err := db.UserByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if fu == nil {
		t.Fatal("expected to find a user but didn't")
	}
	// Delete the user.
	err = db.UserDelete(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the user is not there anymore.
	_, err = db.UserByID(ctx, u.ID.Hex())
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatal(err)
	}
}

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "27017",
	}
}
