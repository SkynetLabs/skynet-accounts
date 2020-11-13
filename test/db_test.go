package test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"gitlab.com/NebulousLabs/errors"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/NebulousLabs/skynet-accounts/user"
)

// initEnv sets the environment variables to what we have defined in Makefile.
func initEnv() {
	e1 := os.Setenv("SKYNET_DB_HOST", "localhost")
	e2 := os.Setenv("SKYNET_DB_PORT", "27017") // DEBUG
	//e2 := os.Setenv("SKYNET_DB_PORT", "37017")
	e3 := os.Setenv("SKYNET_DB_USER", "admin")
	e4 := os.Setenv("SKYNET_DB_PASS", "ivolocalpass")
	if err := errors.Compose(e1, e2, e3, e4); err != nil {
		panic(err)
	}
}

// TestDB_UserFindByID ensures UserFindByID works as expected.
func TestDB_UserFindByID(t *testing.T) {
	initEnv()
	username := t.Name()
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Test finding a non-existent user. This should fail.
	_, err = db.UserFindByID(ctx, "5fac383fdafc482e510627c3")
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v\n", err)
	}

	// Add a user to find.
	u := &user.User{
		FirstName: username,
		LastName:  "Pratchett",
		Email:     (user.Email)(username + "@pratchett.com"),
	}
	if err = insertUser(db, u); err != nil {
		t.Fatal(err)
	}
	defer func(uid string) {
		_, _ = db.UserDeleteByID(nil, uid)
	}(u.ID.Hex())

	// Test finding an existent user. This should pass.
	u1, err := db.UserFindByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, u1) {
		t.Fatalf("User not equal to original: %v vs %v\n", u, u1)
	}
}

// TestDB_UserSave ensures UserSave works as expected.
func TestDB_UserSave(t *testing.T) {
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
	if err = insertUser(db, u); err != nil {
		t.Fatal(err)
	}
	defer func(uid string) {
		_, _ = db.UserDeleteByID(nil, uid)
	}(u.ID.Hex())

	// Test changing the user's names.
	u.FirstName += "_changed"
	u.LastName += "_also_changed"
	ins, err := db.UserSave(ctx, u)
	if err != nil {
		t.Fatal("Failed to update user:", err)
	}
	if ins {
		t.Fatal("Did not expect a new user to be created.")
	}
	u1, err := db.UserFindByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal("Failed to load user:", err)
	}
	if u.FirstName != u1.FirstName || u.LastName != u1.LastName {
		t.Fatalf("Expected names '%s' and '%s', got '%s' and '%s'\n", u.FirstName, u.LastName, u1.FirstName, u1.LastName)
	}

	// Test changing the user's email to a non-existent email. This should work.
	u.Email = "new@email.com"
	ins, err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal("Failed to update user:", err)
	}
	if ins {
		t.Fatal("Did not expect a new user to be created.")
	}
	u1, err = db.UserFindByID(ctx, u.ID.Hex())
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
	if err = insertUser(db, nu); err != nil {
		t.Fatal(err)
	}
	defer func(uid string) {
		_, _ = db.UserDeleteByID(nil, uid)
	}(nu.ID.Hex())
	u.Email = nu.Email
	_, err = db.UserSave(ctx, u)
	if !errors.Contains(err, database.ErrEmailAlreadyUsed) {
		t.Fatalf("Expected error ErrEmailAlreadyUsed but got %v\n", err)
	}
}

// TestDB_UserDeleteByID ensures UserDeleteByID works as expected.
func TestDB_UserDeleteByID(t *testing.T) {
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
	if err = insertUser(db, u); err != nil {
		t.Fatal(err)
	}
	defer func(uid string) {
		_, _ = db.UserDeleteByID(nil, uid)
	}(u.ID.Hex())
	// Make sure the user is there.
	fu, err := db.UserFindByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if fu == nil {
		t.Fatal("expected to find a user but didn't")
	}
	// Delete the user.
	ok, err := db.UserDeleteByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to be able to delete user but it was not found")
	}
	// Make sure the user is not there anymore.
	_, err = db.UserFindByID(ctx, u.ID.Hex())
	if err != database.ErrUserNotFound {
		t.Fatal(err)
	}
}

// insertUser is a helper that inserts a new user.
func insertUser(db *database.DB, u *user.User) error {
	ins, err := db.UserSave(nil, u)
	if err != nil {
		return err
	}
	if !ins {
		return errors.New("Expected a new user to be created.")
	}
	return nil
}
