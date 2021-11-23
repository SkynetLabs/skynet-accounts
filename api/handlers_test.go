package api

import (
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
)

// TestUserDTOFromUser ensures the UserDTOFromUser method correctly converts
// from database.User to UserDTO.
func TestUserDTOFromUser(t *testing.T) {
	var u *database.User
	var uDTO *UserDTO

	// Call with a nil value. Expect not to panic.
	uDTO = UserDTOFromUser(u)
	if uDTO != nil {
		t.Fatal("Expected nil.")
	}

	u = &database.User{}
	// Call with a user without a confirmation token.
	u.EmailConfirmationToken = ""
	uDTO = UserDTOFromUser(u)
	if uDTO == nil {
		t.Fatal("Unexpected nil.")
	}
	if !uDTO.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be true.")
	}

	// Call with a user with a confirmation token.
	u.EmailConfirmationToken = "token"
	uDTO = UserDTOFromUser(u)
	if uDTO == nil {
		t.Fatal("Unexpected nil.")
	}
	if uDTO.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be false.")
	}
}
