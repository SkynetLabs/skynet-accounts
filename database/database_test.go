package database

import (
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestDB_passwordHashAndSalt ensures passwordHashAndSalt() works as expected.
func TestDB_passwordHashAndSalt(t *testing.T) {
	initEnv()
	db := &DB{}

	// HAPPY CASE
	password := string(fastrand.Bytes(24))
	pw, salt, err := db.passwordHashAndSalt(password)
	if err != nil {
		t.Fatalf("failed to set password to '%salt' (%v)\n", password, []byte(password))
	}

	u := &User{
		Email:    "foo@bar.baz",
		Password: pw,
		Salt:     salt,
	}
	if err = u.VerifyPassword(password); err != nil {
		t.Fatal("failed to verify password", err)
	}

	// FAILURE CASE:
	// Ensure failing to set a password doesn't affect the user's salt.
	salt = u.Salt
	db.staticDep = &test.DependencyHashPassword{}
	_, _, err = db.passwordHashAndSalt("some_new_pass")
	if err == nil || !strings.Contains(err.Error(), "DependencyHashPassword") {
		t.Fatalf("expected to fail with %s but got %v\n", "DependencyHashPassword", err)
	}
}
