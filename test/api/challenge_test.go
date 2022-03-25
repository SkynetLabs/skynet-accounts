package api

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
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
	r, b, _ := at.Get("/register", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	queryParams := url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/register", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	params := url.Values{}
	params.Set("pubKey", hex.EncodeToString(pk[:]))

	// Request a challenge with a valid pubkey.
	_, b, err := at.Get("/register", params)
	var chBytes []byte
	{
		var ch database.Challenge
		err = json.Unmarshal(b, &ch)
		if err != nil {
			t.Fatal("Failed to get a challenge:", err)
		}
		chBytes, err = hex.DecodeString(ch.Challenge)
		if err != nil {
			t.Fatal("Invalid challenge:", err)
		}
	}

	// Solve the challenge.
	response := append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	bodyParams.Set("email", name+"@siasky.net")
	r, b, err = at.Post("/register", nil, bodyParams)
	if err != nil {
		t.Fatalf("Failed to register. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	var u database.User
	err = json.Unmarshal(b, &u)
	if err != nil {
		t.Fatal("Failed to unmarshal returned user:", err)
	}
	if u.Email != bodyParams.Get("email") {
		t.Fatalf("Expected email '%s', got '%s'.", bodyParams.Get("email"), u.Email)
	}
	// Make sure the user exists in the database.
	u1, err := at.DB.UserByPubKey(at.Ctx, pk[:])
	if err != nil {
		t.Fatal("Failed to fetch user from DB:", err)
	}
	if u1.Email != bodyParams.Get("email") {
		t.Fatalf("Expected user with email '%s', got '%s'.", bodyParams.Get("email"), u1.Email)
	}

	// Try to request another registration challenge with the same pubkey.
	queryParams = url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(pk[:]))
	r, b, _ = at.Get("/register", queryParams)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "pubkey already registered") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "pubkey already registered", r.StatusCode, string(b))
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
	queryParams := url.Values{}
	queryParams.Set("pubKey", pk.String())
	_, b, err := at.Get("/register", queryParams)
	var ch database.Challenge
	err = json.Unmarshal(b, &ch)
	if err != nil {
		t.Fatal("Failed to get a challenge:", err)
	}
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}
	response := append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	bodyParams.Set("email", name+"@siasky.net")
	r, b, err := at.Post("/register", nil, bodyParams)
	if err != nil {
		t.Fatalf("Failed to validate the response. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}

	// Request a challenge without a pubkey.
	r, b, _ = at.Get("/login", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	queryParams = url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/login", queryParams)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	// Request a challenge with a valid pubkey.
	queryParams = url.Values{}
	queryParams.Set("pubKey", pk.String())
	_, b, err = at.Get("/login", queryParams)
	err = json.Unmarshal(b, &ch)
	if err != nil {
		t.Fatal("Failed to get a challenge:", err)
	}
	chBytes, err = hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	bodyParams = url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	bodyParams.Set("email", name+"@siasky.net")
	r, b, err = at.Post("/login", nil, bodyParams)
	if err != nil {
		t.Fatalf("Failed to login. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	// Make sure we have a valid cookie returned and that it's for the same user.
	at.SetCookie(test.ExtractCookie(r))
	u, _, err := at.UserGET()
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != bodyParams.Get("email") {
		t.Fatalf("Expected user with email %s, got %s", bodyParams.Get("email"), u.Email)
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
	_, err = at.DB.UserCreatePK(at.Ctx, name+"_other@siasky.net", "", name+"_other_sub", pk2[:], database.TierFree)
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
	r, bb, err := at.UserPOST(name+"_user3@siasky.net", name+"_pass")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal(r.Status, err, string(bb))
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
	if subtle.ConstantTimeCompare(u3.PubKeys[0], pk[:]) != 1 {
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
	if subtle.ConstantTimeCompare(u1.PubKeys[0], pk[:]) != 1 {
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
