package database

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// TestValidateChallengeResponse is a unit test using a database.
func TestValidateChallengeResponse(t *testing.T) {
	ctx := context.Background()
	dbName := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sk, pk := crypto.GenerateKeyPair()

	// Create a new challenge.
	ch, err := db.NewChallenge(ctx, pk[:], database.ChallengeTypeRegister)
	if err != nil {
		t.Fatal("Failed to create challenge", err)
	}

	// Get the challenge bytes.
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge", err)
	}

	// Try to solve it with the wrong type.
	response := append(chBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	chr := database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	_, err = db.ValidateChallengeResponse(ctx, chr)
	if err == nil || !strings.Contains(err.Error(), "challenge not found") {
		t.Fatalf("Expected error 'challenge not found', got '%v'.", err)
	}

	// Try to solve it with an invalid type.
	response = append(chBytes, append([]byte("invalid_type"), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	_, err = db.ValidateChallengeResponse(ctx, chr)
	if err == nil || !strings.Contains(err.Error(), "invalid challenge type") {
		t.Fatalf("Expected error 'invalid challenge type', got '%v'.", err)
	}

	// Try to solve it with the wrong secret key.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(fastrand.Bytes(64), response),
	}
	_, err = db.ValidateChallengeResponse(ctx, chr)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error 'invalid signature', got '%v'.", err)
	}

	// Try to solve it with a bad recipient.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte("bad-recipient.net")...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(fastrand.Bytes(64), response),
	}
	_, err = db.ValidateChallengeResponse(ctx, chr)
	if err == nil || !strings.Contains(err.Error(), "invalid recipient") {
		t.Fatalf("Expected error 'invalid recipient', got '%v'.", err)
	}

	// Try to solve the wrong challenge.
	wrongBytes := fastrand.Bytes(database.ChallengeSize)
	response = append(wrongBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	_, err = db.ValidateChallengeResponse(ctx, chr)
	if err == nil || !strings.Contains(err.Error(), "challenge not found") {
		t.Fatalf("Expected error 'challenge not found', got '%v'.", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	pk2, err := db.ValidateChallengeResponse(ctx, chr)
	if err != nil {
		t.Fatal("Failed to validate the response", err)
	}
	if subtle.ConstantTimeCompare(pk[:], pk2[:]) != 1 {
		t.Fatalf("Expected pubkey '%s', got '%s'.", hex.EncodeToString(pk[:]), hex.EncodeToString(pk2[:]))
	}

	// Create a new challenge.
	ch, err = db.NewChallenge(ctx, pk[:], database.ChallengeTypeRegister)
	if err != nil {
		t.Fatal("Failed to create second challenge", err)
	}
}
