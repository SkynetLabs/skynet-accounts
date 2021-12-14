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
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	params := url.Values{}
	params.Add("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/register", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with a valid pubkey.
	params = url.Values{}
	pkStr := hex.EncodeToString(pk[:])
	params.Add("pubKey", pkStr)
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
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	params.Add("email", name+"@siasky.net")
	r, b, err = at.Post("/register", nil, params)
	if err != nil {
		t.Fatalf("Failed to register. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	var u database.User
	err = json.Unmarshal(b, &u)
	if err != nil {
		t.Fatal("Failed to unmarshal returned user:", err)
	}
	if u.Email != params.Get("email") {
		t.Fatalf("Expected email '%s', got '%s'.", params.Get("email"), u.Email)
	}
	// Make sure the user exists in the database.
	u1, err := at.DB.UserByPubKey(at.Ctx, pk[:])
	if err != nil {
		t.Fatal("Failed to fetch user from DB:", err)
	}
	if u1.Email != params.Get("email") {
		t.Fatalf("Expected user with email '%s', got '%s'.", params.Get("email"), u1.Email)
	}

	// Try to request another registration challenge with the same pubkey.
	params = url.Values{}
	params.Add("pubKey", hex.EncodeToString(pk[:]))
	r, b, _ = at.Get("/register", params)
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
	params := url.Values{}
	params.Add("pubKey", pk.String())
	_, b, err := at.Get("/register", params)
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
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	params.Add("email", name+"@siasky.net")
	r, b, err := at.Post("/register", nil, params)
	if err != nil {
		t.Fatalf("Failed to validate the response. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}

	// Request a challenge without a pubkey.
	r, b, _ = at.Get("/login", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	params = url.Values{}
	params.Add("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/login", params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with a valid pubkey.
	params = url.Values{}
	params.Add("pubKey", pk.String())
	_, b, err = at.Get("/login", params)
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
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	params.Add("email", name+"@siasky.net")
	r, b, err = at.Post("/login", nil, params)
	if err != nil {
		t.Fatalf("Failed to login. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}
	// Make sure we have a valid cookie returned and that it's for the same user.
	at.Cookie = test.ExtractCookie(r)
	_, b, err = at.Get("/user", nil)
	if err != nil {
		t.Fatalf("Failed to fetch user with the given cookie: '%s', error '%s'", string(b), err)
	}
	var u database.User
	err = json.Unmarshal(b, &u)
	if err != nil {
		t.Fatal("Failed to parse user:", err)
	}
	if u.Email != params.Get("email") {
		t.Fatalf("Expected user with email %s, got %s", params.Get("email"), u.Email)
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
	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Request a challenge without a pubkey.
	r, b, _ := at.Get("/user/pubkey/register", nil)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with an invalid pubkey.
	params := url.Values{}
	params.Add("pubKey", hex.EncodeToString(fastrand.Bytes(10)))
	r, b, _ = at.Get("/user/pubkey/register", params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid pubKey provided") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid pubKey provided", r.StatusCode, string(b))
	}

	// Request a challenge with a pubKey that belongs to another user.
	_, pk2 := crypto.GenerateKeyPair()
	_, err = at.DB.UserCreatePK(at.Ctx, name+"_other@siasky.net", "", name+"_other_sub", pk2[:], database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	params = url.Values{}
	params.Add("pubKey", hex.EncodeToString(pk2[:]))
	r, b, _ = at.Get("/user/pubkey/register", params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "pubkey already registered") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "pubkey already registered", r.StatusCode, string(b))
	}

	// Request a challenge for setting the user's pubKey.
	sk, pk := crypto.GenerateKeyPair()
	params = url.Values{}
	params.Add("pubKey", hex.EncodeToString(pk[:]))
	r, b, err = at.Get("/user/pubkey/register", params)
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
	r, b, _ = at.Post("/user/pubkey/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "missing or invalid challenge response") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "missing or invalid challenge response", r.StatusCode, string(b))
	}

	// Try to solve it without being logged in.
	at.Cookie = nil
	response := append(chBytes, append([]byte(database.ChallengeTypeUpdate), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/user/pubkey/register", nil, params)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
	}

	// Try to solve the challenge while logged in as a different user.
	// NOTE: This will consume the challenge and the user will need to request
	// a new one.
	r, b, err = at.CreateUserPost(name+"_user3@siasky.net", name+"_pass")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal(r.Status, err, string(b))
	}
	at.Cookie = test.ExtractCookie(r)
	r, b, _ = at.Post("/user/pubkey/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "user's sub doesn't match update sub") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "user's sub doesn't match update sub", r.StatusCode, string(b))
	}

	// Request a new challenge with the original test user.
	at.Cookie = c
	params = url.Values{}
	params.Add("pubKey", hex.EncodeToString(pk[:]))
	r, b, err = at.Get("/user/pubkey/register", params)
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
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, err = at.Post("/user/pubkey/register", nil, params)
	if err != nil {
		t.Fatalf("Failed to confirm the update. Status %d, body '%s', error '%s'", r.StatusCode, string(b), err)
	}

	// Make sure the user's pubKey is properly set.
	u3, err := at.DB.UserBySub(at.Ctx, u.Sub, false)
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
