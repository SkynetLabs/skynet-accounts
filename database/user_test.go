package database

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/test"

	"gitlab.com/NebulousLabs/fastrand"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestEmail_Validate ensures email validation functions as expected.
// See https://gist.github.com/cjaoude/fd9910626629b53c4d25
func TestEmail_Validate(t *testing.T) {
	t.Parallel()

	valid := []string{
		"email@example.com",
		"firstname.lastname@example.com",
		"email@subdomain.example.com",
		"first_name+lastname@example.com",
		"1234567890@example.com",
		"email@example-one.com",
		"_______@example.com",
		"email@example.name",
		"email@example.museum",
		"email@example.co.jp",
		"firstname-lastname@example.com",
	}
	invalid := []string{
		"",
		"plainaddress",
		"#@%^%#$@#$@#.com",
		"@example.com",
		"Joe Smith <email@example.com>",
		"email.example.com",
		"email@example@example.com",
		"あいうえお@example.com",
		"email@example.com (Joe Smith)",
		"email@example",
		"email@111.222.333.44444",
		// Strange Invalid Addresses:
		"”(),:;<>[\\]@example.com",
		"just”not”right@example.com",
		"this\\ is\"really\"not\allowed@example.com",
	}
	for _, email := range valid {
		if !(Email)(email).Validate() {
			t.Errorf("Expected '%s' to be valid\n", email)
		}
	}
	for _, email := range invalid {
		if (Email)(email).Validate() {
			t.Errorf("Expected '%s' to be invalid\n", email)
		}
	}
}

// TestUser_saltAndPepper ensures saltAndPepper() works as expected.
// Do not run in parallel to other tests - this test changes the environment.
func TestUser_saltAndPepper(t *testing.T) {
	initEnv()
	salt := []byte("this is a salt")
	pepper := []byte("this is some pepper")
	u := &User{
		ID:        primitive.ObjectID{},
		FirstName: "Foo",
		LastName:  "Bar",
		Email:     "foo@bar.baz",
		Salt:      salt,
	}

	oldPepper, ok := os.LookupEnv(envPepper)
	if ok {
		defer func() { _ = os.Setenv(envPepper, oldPepper) }()
	}
	err := os.Setenv(envPepper, string(pepper))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(u.saltAndPepper(), append(salt, pepper...)) {
		t.Fatal("unexpected salt&pepper returned")
	}
}

// TestUser_SetPassword ensures SetPassword() works as expected.
func TestUser_SetPassword(t *testing.T) {
	initEnv()
	salt := fastrand.Bytes(saltSize)
	u := &User{
		ID:        primitive.ObjectID{},
		FirstName: "Foo",
		LastName:  "Bar",
		Email:     "foo@bar.baz",
		Salt:      salt,
	}

	// HAPPY CASE
	pw := string(fastrand.Bytes(24))
	err := u.SetPassword(pw)
	if err != nil {
		t.Fatalf("failed to set password to '%s' (%v)\n", pw, []byte(pw))
	}
	if err = u.VerifyPassword(pw); err != nil {
		t.Fatal("failed to verify password", err)
	}
	// ensure the salt has changed
	if bytes.Equal(u.Salt, salt) {
		t.Fatal("salt wasn't regenerated")
	}

	// FAILURE CASE:
	// Ensure failing to set a password doesn't affect the user's salt.
	salt = u.Salt
	u.dep = &test.DependencyHashPassword{}
	err = u.SetPassword("some_new_pass")
	if err == nil || !strings.Contains(err.Error(), "DependencyHashPassword") {
		t.Fatalf("expected to fail with  %s but got %v\n", "DependencyHashPassword", err)
	}
	if !bytes.Equal(u.Salt, salt) {
		t.Fatal("expected the user's salt to not change")
	}
}

// TestUser_VerifyPassword ensures VerifyPassword() works as expected.
func TestUser_VerifyPassword(t *testing.T) {
	initEnv()
	u := &User{
		ID:        primitive.ObjectID{},
		FirstName: "Foo",
		LastName:  "Bar",
		Email:     "foo@bar.baz",
	}
	pw := string(fastrand.Bytes(saltSize))
	err := u.SetPassword(pw)
	if err != nil {
		t.Fatal("failed to set password", err)
	}

	// HAPPY CASE
	err = u.VerifyPassword(pw)
	if err != nil {
		t.Fatal("failed to verify password", err)
	}

	// FAILURE CASE: Bad password
	err = u.VerifyPassword("wrong_pass")
	if err == nil {
		t.Fatal("expected to fail to verify password")
	}

	// FAILURE CASE: Wrong salt
	u.Salt = fastrand.Bytes(saltSize)
	err = u.VerifyPassword(pw)
	if err == nil {
		t.Fatal("expected to fail to verify password")
	}
}

// initEnv is a helper method that initialises some environment variables for
// testing purposes, so we don't run into build.Critical and build.Severe.
func initEnv() {
	_ = os.Setenv(envPepper, string(fastrand.Bytes(saltSize)))
}
