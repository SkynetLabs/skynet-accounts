package database

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/mongo"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// TestValidateChallengeResponse is a unit test using a database.
func TestValidateChallengeResponse(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
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
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err == nil || !strings.Contains(err.Error(), "unexpected challenge type") {
		t.Fatalf("Expected error 'unexpected challenge type', got '%v'.", err)
	}

	// Try to solve the challenge with the correct response but with the wrong
	// expectations.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeUpdate)
	if err == nil || !strings.Contains(err.Error(), "unexpected challenge type") {
		t.Fatalf("Expected error 'unexpected challenge type', got '%v'.", err)
	}

	// Try to solve it with an invalid type.
	response = append(chBytes, append([]byte("invalid_type"), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err == nil || !strings.Contains(err.Error(), "invalid challenge type") {
		t.Fatalf("Expected error 'invalid challenge type', got '%v'.", err)
	}

	// Try to solve it with the wrong secret key.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(fastrand.Bytes(64), response),
	}
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error 'invalid signature', got '%v'.", err)
	}

	// Try to solve it with a bad recipient.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte("bad-recipient.net")...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(fastrand.Bytes(64), response),
	}
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
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
	_, _, err = db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err == nil || !strings.Contains(err.Error(), "challenge not found") {
		t.Fatalf("Expected error 'challenge not found', got '%v'.", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	chr = database.ChallengeResponse{
		Response:  response,
		Signature: ed25519.Sign(sk[:], response),
	}
	pk2, _, err := db.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
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

// TestUnconfirmedUserUpdate ensures the entire flow for unconfirmed user
// updates works as expected.
func TestUnconfirmedUserUpdate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}
	_, pk := crypto.GenerateKeyPair()

	// Create a new challenge.
	ch, err := db.NewChallenge(ctx, pk[:], database.ChallengeTypeRegister)
	if err != nil {
		t.Fatal("Failed to create challenge", err)
	}

	uu := &database.UnconfirmedUserUpdate{
		Sub:         dbName + "_sub",
		ChallengeID: ch.ID,
		ExpiresAt:   ch.ExpiresAt.Truncate(time.Millisecond),
	}

	// Store the update.
	err = db.StoreUnconfirmedUserUpdate(ctx, uu)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch the update.
	uu2, err := db.FetchUnconfirmedUserUpdate(ctx, ch.ID)
	if err != nil {
		t.Fatal()
	}
	// Verify we fetched the right thing.
	if uu2.ChallengeID != uu.ChallengeID || uu2.Sub != uu.Sub || uu2.ExpiresAt != uu.ExpiresAt {
		t.Fatalf("Expected '%+v',\ngot '%+v'.", uu, uu2)
	}
	// Delete the update.
	err = db.DeleteUnconfirmedUserUpdate(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that it's gone.
	_, err = db.FetchUnconfirmedUserUpdate(ctx, ch.ID)
	if err != mongo.ErrNoDocuments {
		t.Fatalf("Expected '%s', got '%s'.", mongo.ErrNoDocuments, err)
	}
}
