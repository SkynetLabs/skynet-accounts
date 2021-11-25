package api

import (
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
)

// TestUserGETFromUser ensures the UserGETFromUser method correctly converts
// from database.User to UserGET.
func TestUserGETFromUser(t *testing.T) {
	var u *database.User
	var uGET *UserGET

	// Call with a nil value. Expect not to panic.
	uGET = UserGETFromUser(u)
	if uGET != nil {
		t.Fatal("Expected nil.")
	}

	u = &database.User{}
	// Call with a user without a confirmation token.
	u.EmailConfirmationToken = ""
	uGET = UserGETFromUser(u)
	if uGET == nil {
		t.Fatal("Unexpected nil.")
	}
	if !uGET.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be true.")
	}

	// Call with a user with a confirmation token.
	u.EmailConfirmationToken = "token"
	uGET = UserGETFromUser(u)
	if uGET == nil {
		t.Fatal("Unexpected nil.")
	}
	if uGET.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be false.")
	}
}
