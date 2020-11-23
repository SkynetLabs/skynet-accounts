package database

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/user"

	"gitlab.com/NebulousLabs/errors"
)

// TestDatabase_UserByEmail ensures UserByEmail works as expected.
// This method also test UserCreate.
func TestDatabase_UserByEmail(t *testing.T) {
	initEnv()
	username := t.Name()
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Test finding a non-existent user. This should fail.
	_, err = db.UserByEmail(ctx, "noexist@foo.bar")
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v\n", err)
	}

	// Add a user to find.
	u := &user.User{
		FirstName: username,
		LastName:  "Pratchett",
		Email:     (user.Email)(username + "@pratchett.com"),
	}
	if err = db.UserCreate(nil, u); err != nil {
		t.Fatal(err)
	}
	defer func(user *user.User) {
		_ = db.UserDelete(nil, user)
	}(u)

	// Test finding an existent user. This should pass.
	u1, err := db.UserByEmail(ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, u1) {
		t.Fatalf("User not equal to original: %v vs %v\n", u, u1)
	}
}

// TestDatabase_UserByID ensures UserByID works as expected.
func TestDatabase_UserByID(t *testing.T) {
	initEnv()
	username := t.Name()
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Test finding a non-existent user. This should fail.
	_, err = db.UserByID(ctx, "5fac383fdafc482e510627c3")
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v\n", err)
	}

	// Add a user to find.
	u := &user.User{
		FirstName: username,
		LastName:  "Pratchett",
		Email:     (user.Email)(username + "@pratchett.com"),
	}
	if err = db.UserCreate(nil, u); err != nil {
		t.Fatal(err)
	}
	defer func(user *user.User) {
		_ = db.UserDelete(nil, user)
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
	initEnv()
	username := t.Name()
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	u := &user.User{
		FirstName: username,
		LastName:  "Pratchett",
		Email:     (user.Email)(username + "@pratchett.com"),
	}
	if err = db.UserCreate(nil, u); err != nil {
		t.Fatal(err)
	}
	defer func(user *user.User) {
		_ = db.UserDelete(nil, user)
	}(u)

	// Test changing the user's names.
	u.FirstName += "_changed"
	u.LastName += "_also_changed"
	err = db.UserUpdate(ctx, u)
	if err != nil {
		t.Fatal("Failed to update user:", err)
	}
	u1, err := db.UserByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal("Failed to load user:", err)
	}
	if u.FirstName != u1.FirstName || u.LastName != u1.LastName {
		t.Fatalf("Expected names '%s' and '%s', got '%s' and '%s'\n", u.FirstName, u.LastName, u1.FirstName, u1.LastName)
	}

	// Test changing the user's email to a non-existent email. This should work.
	u.Email = "new@email.com"
	err = db.UserUpdate(ctx, u)
	if err != nil {
		t.Fatal("Failed to update user:", err)
	}
	u1, err = db.UserByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal("Failed to load user:", err)
	}
	if u.Email != u1.Email {
		t.Fatalf("Expected the email to be '%s', got '%s'\n", u.Email, u1.Email)
	}

	// Test changing the user's email to an existing email. This should fail.
	nu := &user.User{
		FirstName: "Some",
		LastName:  "Guy",
		Email:     "existing@email.com",
	}
	if err = db.UserCreate(nil, nu); err != nil {
		t.Fatal(err)
	}
	defer func(user *user.User) {
		_ = db.UserDelete(nil, user)
	}(u)
	u.Email = nu.Email
	err = db.UserCreate(ctx, u)
	if !errors.Contains(err, database.ErrEmailAlreadyUsed) {
		t.Fatalf("Expected error ErrEmailAlreadyUsed but got %v\n", err)
	}
}

// TestDatabase_UserDelete ensures UserDelete works as expected.
func TestDatabase_UserDelete(t *testing.T) {
	initEnv()
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Add a user to delete.
	u := &user.User{
		FirstName: "Ivaylo",
		LastName:  "Novakov",
		Email:     "ivaylo@nebulous.tech",
	}
	if err = db.UserCreate(nil, u); err != nil {
		t.Fatal(err)
	}
	defer func(user *user.User) {
		_ = db.UserDelete(nil, user)
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

// initEnv sets the environment variables to what we have defined in Makefile.
func initEnv() {
	e1 := os.Setenv("SKYNET_DB_HOST", "localhost")
	//e2 := os.Setenv("SKYNET_DB_PORT", "27017") // DEBUG
	e2 := os.Setenv("SKYNET_DB_PORT", "37017")
	e3 := os.Setenv("SKYNET_DB_USER", "admin")
	e4 := os.Setenv("SKYNET_DB_PASS", "ivolocalpass")
	if err := errors.Compose(e1, e2, e3, e4); err != nil {
		panic(err)
	}
}
