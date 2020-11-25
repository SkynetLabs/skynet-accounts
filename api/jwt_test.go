package api

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/NebulousLabs/skynet-accounts/database"
)

// TestToken ensures that IssueToken() and ValidateToken() function properly.
// TODO This test is too basic and needs to be expanded.
func TestToken(t *testing.T) {
	id, err := primitive.ObjectIDFromHex("5fbd557f0518ba7ae9267e3b")
	if err != nil {
		t.Fatal(err)
	}
	u := &database.User{
		ID:        id,
		FirstName: "Terry",
		LastName:  "Pratchett",
		Email:     "terry@pratchett.com",
		Tier:      2,
	}
	token, err := IssueToken(u)
	if err != nil {
		t.Fatal("failed to issue a token", err)
	}

	_, err = ValidateToken(token)
	if err != nil {
		t.Fatal("failed to verify token", err)
	}
}
