package api

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/SkynetLabs/skynet-accounts/types"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// testRegistration validates the registration challenge-response flow.
func testRegistration(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := test.DBNameForTest(t.Name())
	sk, pk := crypto.GenerateKeyPair()

	// Request a challenge without a pubkey.
	_, status, err := at.RegisterGET(nil)
	if status != http.StatusBadRequest || err == nil || !strings.Contains(err.Error(), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), status, err)
	}

	// Request a challenge with an invalid pubkey.
	_, status, err = at.RegisterGET(fastrand.Bytes(10)[:])
	if status != http.StatusBadRequest || err == nil || !strings.Contains(err.Error(), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), status, err)
	}

	// Request a challenge with a valid pubkey.
	ch, status, err := at.RegisterGET(pk[:])
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Solve the challenge.
	response := append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	sig := ed25519.Sign(sk[:], response)
	emailStr := types.NewEmail(name + "@siasky.net")
	u, status, err := at.RegisterPOST(response, sig, emailStr.String())
	if err != nil {
		t.Fatalf("Failed to register. Status %d, error '%s'", status, err)
	}
	if u.Email != emailStr {
		t.Fatalf("Expected email '%s', got '%s'.", emailStr, u.Email)
	}
	// Make sure the user exists in the database.
	u1, err := at.DB.UserByPubKey(at.Ctx, pk[:])
	if err != nil {
		t.Fatal("Failed to fetch user from DB:", err)
	}
	if u1.Email != emailStr {
		t.Fatalf("Expected user with email '%s', got '%s'.", emailStr, u1.Email)
	}

	// Try to request another registration challenge with the same pubkey.
	_, status, err = at.RegisterGET(pk[:])
	if status != http.StatusBadRequest || err == nil || !strings.Contains(err.Error(), "pubkey already registered") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "pubkey already registered", status, err)
	}
}

// testLogin validates the login challenge-response flow.
func testLogin(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := test.DBNameForTest(t.Name())
	sk, pkk := crypto.GenerateKeyPair()
	var pk = database.PubKey(pkk[:])

	// Register a user via challenge-response, so we have a test user with a
	// pubkey that we can login with.
	ch, _, err := at.RegisterGET(pk)
	if err != nil {
		t.Fatal("Failed to get a challenge:", err)
	}
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}
	response := append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	sig := ed25519.Sign(sk[:], response)
	emailStr := types.NewEmail(name + "@siasky.net")
	u, status, err := at.RegisterPOST(response, sig, emailStr.String())
	if err != nil {
		t.Fatalf("Failed to validate the response. Status %d, error '%s'", status, err)
	}

	// Request a challenge without a pubkey.
	_, status, err = at.LoginPubKeyGET(nil)
	if status != http.StatusBadRequest || err == nil || !strings.Contains(err.Error(), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), status, err)
	}

	// Request a challenge with an invalid pubkey.
	ch, status, err = at.LoginPubKeyGET(fastrand.Bytes(10)[:])
	if status != http.StatusBadRequest || err == nil || !strings.Contains(err.Error(), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), status, err)
	}

	// Request a challenge with a valid pubkey.
	ch, status, err = at.LoginPubKeyGET(pk[:])
	chBytes, err = hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	sig = ed25519.Sign(sk[:], response)
	emailStr = types.NewEmail(name + "@siasky.net")
	r, b, err := at.LoginPubKeyPOST(response, sig, emailStr.String())
	if err != nil {
		t.Fatalf("Failed to login. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	// Make sure we have a valid cookie returned and that it's for the same user.
	at.SetCookie(test.ExtractCookie(r))
	u, _, err = at.UserGET()
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != emailStr {
		t.Fatalf("Expected user with email %s, got %s", emailStr, u.Email)
	}
}

// testUserAddPubKey tests the ability of update user's pubKey.
func testUserAddPubKey(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	u, c, err := test.CreateUserAndLogin(at, name)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()
	at.SetCookie(c)
	defer at.ClearCredentials()

	// Request a challenge without a pubkey.
	_, status, _ := at.UserPubkeyRegisterGET("")
	if status != http.StatusBadRequest {
		t.Fatalf("Expected %d, got %d", http.StatusBadRequest, status)
	}

	// Request a challenge with an invalid pubkey.
	_, status, _ = at.UserPubkeyRegisterGET(string(fastrand.Bytes(10)))
	if status != http.StatusBadRequest {
		t.Fatalf("Expected %d, got %d", http.StatusBadRequest, status)
	}

	// Request a challenge with a pubKey that belongs to another user.
	_, pk2 := crypto.GenerateKeyPair()
	_, err = at.DB.UserCreatePK(at.Ctx, types.NewEmail(name+"_other@siasky.net"), "", name+"_other_sub", pk2[:], database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	_, status, err = at.UserPubkeyRegisterGET(string(pk2[:]))
	if status != http.StatusBadRequest {
		t.Fatalf("Expected %d, got %d", http.StatusBadRequest, status)
	}

	// Request a challenge for setting the user's pubKey.
	sk, pk := crypto.GenerateKeyPair()
	ch, status, err := at.UserPubkeyRegisterGET(hex.EncodeToString(pk[:]))
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Try to solve it without passing the solution.
	_, status, err = at.UserPubkeyRegisterPOST(nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("Expected %d, got %d", http.StatusBadRequest, status)
	}

	// Try to solve it without being logged in.
	at.ClearCredentials()
	response := append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	_, status, err = at.UserPubkeyRegisterPOST(response, ed25519.Sign(sk[:], response))
	if status != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d, error '%s'", http.StatusUnauthorized, status, err)
	}

	// Try to solve the challenge while logged in as a different user.
	// NOTE: This will consume the challenge and the user will need to request
	// a new one.
	r, b, err := at.UserPOST(types.NewEmail(name+"_user3@siasky.net").String(), name+"_pass")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal(r.Status, err, string(b))
	}
	at.SetCookie(test.ExtractCookie(r))
	_, status, err = at.UserPubkeyRegisterPOST(response, ed25519.Sign(sk[:], response))
	if status != http.StatusBadRequest {
		t.Fatalf("Expected %d, got %d", http.StatusBadRequest, status)
	}

	// Request a new challenge with the original test user.
	at.SetCookie(c)
	ch, status, err = at.UserPubkeyRegisterGET(hex.EncodeToString(pk[:]))
	if err != nil || status != http.StatusOK {
		t.Fatal("Failed to get a challenge:", err, r.Status, err)
	}
	chBytes, err = hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	_, status, err = at.UserPubkeyRegisterPOST(response, ed25519.Sign(sk[:], response))
	if err != nil {
		t.Fatalf("Failed to confirm the update. Status %d, error '%s'", status, err)
	}

	// Make sure the user's pubKey is properly set.
	u3, err := at.DB.UserBySub(at.Ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(u3.PubKeys) == 0 {
		t.Fatal("Expected at least one pubkey assigned, got none.")
	}
	if !bytes.Equal(u3.PubKeys[0], pk[:]) {
		t.Fatalf("Expected pubKey '%s', got '%s',", hex.EncodeToString(pk[:]), hex.EncodeToString(u3.PubKeys[0]))
	}
}

// testUserDeletePubKey ensures that users can delete pubkeys from their
// accounts.
func testUserDeletePubKey(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	u, c, err := test.CreateUserAndLogin(at, name)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()
	at.SetCookie(c)
	defer at.ClearCredentials()

	sk, pk := crypto.GenerateKeyPair()

	// Request a new challenge.
	ch, status, err := at.UserPubkeyRegisterGET(hex.EncodeToString(pk[:]))
	if err != nil || status != http.StatusOK {
		t.Fatal("Failed to get a challenge:", err, status)
	}
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}
	// Solve the challenge.
	response := append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	_, status, err = at.UserPubkeyRegisterPOST(response, ed25519.Sign(sk[:], response))
	if err != nil {
		t.Fatalf("Failed to confirm the update. Status %d, error '%s'", status, err)
	}
	// Make sure the user's pubKey is properly set.
	u1, err := at.DB.UserBySub(at.Ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(u1.PubKeys) != 1 {
		t.Fatal("Expected one pubkey assigned, got none.")
	}
	if !bytes.Equal(u1.PubKeys[0], pk[:]) {
		t.Fatalf("Expected pubKey '%s', got '%s',", hex.EncodeToString(pk[:]), hex.EncodeToString(u1.PubKeys[0]))
	}

	// Call DELETE without a cookie.
	at.ClearCredentials()
	status, err = at.UserPubkeyDELETE(pk[:])
	if err == nil || status != http.StatusUnauthorized {
		t.Fatalf("Expected to fail with 401. Status %d, error '%s'", status, err)
	}
	at.SetCookie(c)
	// Call DELETE with an invalid key.
	status, err = at.UserPubkeyDELETE([]byte("INVALID_KEY"))
	if err == nil || status != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, error '%s'", status, err)
	}
	_, pk1 := crypto.GenerateKeyPair()
	// Call DELETE with a key that doesn't belong to this user.
	status, err = at.UserPubkeyDELETE(pk1[:])
	if err == nil || status != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, error '%s'", status, err)
	}
	// Call DELETE with correct parameters.
	status, err = at.UserPubkeyDELETE(pk[:])
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("Expected to succeed. Status %d, error '%s'", status, err)
	}
	// Verify that the key was deleted.
	u2, err := at.DB.UserBySub(at.Ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(u2.PubKeys) > 0 {
		t.Fatal("Expected no public keys, got", len(u2.PubKeys))
	}
	// Call DELETE with the already deleted key.
	status, err = at.UserPubkeyDELETE(pk[:])
	if err == nil || status != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, error '%s'", status, err)
	}
}
