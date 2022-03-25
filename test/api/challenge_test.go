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
	r, b, _ := at.Get("/user/pubkey/register", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	queryParams := url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/user/pubkey/register", queryParams)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), database.ErrInvalidPublicKey.Error()) {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, database.ErrInvalidPublicKey.Error(), r.StatusCode, string(b))
	}

	// Request a challenge with a pubKey that belongs to another user.
	_, pk2 := crypto.GenerateKeyPair()
	_, err = at.DB.UserCreatePK(at.Ctx, name+"_other@siasky.net", "", name+"_other_sub", pk2[:], database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	queryParams = url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(pk2[:]))
	r, b, _ = at.Get("/user/pubkey/register", queryParams)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "pubkey already registered") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "pubkey already registered", r.StatusCode, string(b))
	}

	// Request a challenge for setting the user's pubKey.
	sk, pk := crypto.GenerateKeyPair()
	queryParams = url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(pk[:]))
	r, b, err = at.Get("/user/pubkey/register", queryParams)
	var ch database.Challenge
	err = json.Unmarshal(b, &ch)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get a challenge. Status '%s', body '%s', error '%s'", r.Status, string(b), err)
	}
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Try to solve it without passing the solution.
	r, b, _ = at.Post("/user/pubkey/register", nil, nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "missing or invalid challenge response") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "missing or invalid challenge response", r.StatusCode, string(b))
	}

	// Try to solve it without being logged in.
	at.ClearCredentials()
	response := append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/user/pubkey/register", nil, bodyParams)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
	}

	// Try to solve the challenge while logged in as a different user.
	// NOTE: This will consume the challenge and the user will need to request
	// a new one.
	r, b, err = at.UserPOST(name+"_user3@siasky.net", name+"_pass")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal(r.Status, err, string(b))
	}
	at.SetCookie(test.ExtractCookie(r))
	r, b, _ = at.Post("/user/pubkey/register", nil, bodyParams)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "user's sub doesn't match update sub") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "user's sub doesn't match update sub", r.StatusCode, string(b))
	}

	// Request a new challenge with the original test user.
	at.SetCookie(c)
	queryParams = url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(pk[:]))
	r, b, err = at.Get("/user/pubkey/register", queryParams)
	err = json.Unmarshal(b, &ch)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal("Failed to get a challenge:", err, r.Status, string(b))
	}
	chBytes, err = hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	bodyParams = url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, err = at.Post("/user/pubkey/register", nil, bodyParams)
	if err != nil {
		t.Fatalf("Failed to confirm the update. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
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
	var ch database.Challenge

	// Request a new challenge.
	queryParams := url.Values{}
	queryParams.Set("pubKey", hex.EncodeToString(pk[:]))
	r, b, err := at.Get("/user/pubkey/register", queryParams)
	err = json.Unmarshal(b, &ch)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal("Failed to get a challenge:", err, r.Status, string(b))
	}
	chBytes, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		t.Fatal("Invalid challenge:", err)
	}
	// Solve the challenge.
	response := append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, err = at.Post("/user/pubkey/register", nil, bodyParams)
	if err != nil {
		t.Fatalf("Failed to confirm the update. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
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
	r, b, err = at.Delete("/user/pubkey/"+hex.EncodeToString(pk[:]), nil)
	if err == nil || r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected to fail with 401. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	at.SetCookie(c)
	// Call DELETE with an invalid key.
	r, b, err = at.Delete("/user/pubkey/INVALID_KEY", nil)
	if err == nil || r.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	_, pk1 := crypto.GenerateKeyPair()
	// Call DELETE with a key that doesn't belong to this user.
	r, b, err = at.Delete("/user/pubkey/"+hex.EncodeToString(pk1[:]), nil)
	if err == nil || r.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	// Call DELETE with correct parameters.
	r, b, err = at.Delete("/user/pubkey/"+hex.EncodeToString(pk[:]), nil)
	if err != nil || r.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected to succeed. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
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
	r, b, err = at.Delete("/user/pubkey/"+hex.EncodeToString(pk[:]), nil)
	if err == nil || r.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected to fail with 400. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
}
