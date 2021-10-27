package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// testRegistration validates the registration challenge-response flow.
func testRegistration(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := strings.ReplaceAll(t.Name(), "/", "_")
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

	// Try to solve it with the wrong type.
	response := append(chBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "failed to validate challenge response") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "failed to validate challenge response", r.StatusCode, string(b))
	}

	// Try to solve it with an invalid type.
	response = append(chBytes, append([]byte("invalid_type"), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "invalid challenge type") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "invalid challenge type", r.StatusCode, string(b))
	}

	// Try to solve it with the wrong secret key.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(fastrand.Bytes(ed25519.PrivateKeySize), response)))
	r, b, _ = at.Post("/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "failed to validate challenge response") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "failed to validate challenge response", r.StatusCode, string(b))
	}

	// Try to solve the wrong challenge.
	wrongBytes := fastrand.Bytes(database.ChallengeSize)
	response = append(wrongBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(fastrand.Bytes(64), response)))
	r, b, _ = at.Post("/register", nil, params)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "failed to validate challenge response") {
		t.Fatalf("Expected %d '%s', got %d '%s'",
			http.StatusBadRequest, "failed to validate challenge response", r.StatusCode, string(b))
	}

	// Solve the challenge.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
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
	name := strings.ReplaceAll(t.Name(), "/", "_")
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

	// Try to solve it with the wrong type.
	response = append(chBytes, append([]byte(database.ChallengeTypeRegister), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, err = at.Post("/login", nil, params)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
	}

	// Try to solve it with an invalid type.
	response = append(chBytes, append([]byte("invalid_type"), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/login", nil, params)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
	}

	// Try to solve it with the wrong secret key.
	response = append(chBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(fastrand.Bytes(ed25519.PrivateKeySize), response)))
	r, b, _ = at.Post("/login", nil, params)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
	}

	// Try to solve the wrong challenge.
	wrongBytes := fastrand.Bytes(database.ChallengeSize)
	response = append(wrongBytes, append([]byte(database.ChallengeTypeLogin), []byte(database.PortalName)...)...)
	params = url.Values{}
	params.Add("response", hex.EncodeToString(response))
	params.Add("signature", hex.EncodeToString(ed25519.Sign(sk[:], response)))
	r, b, _ = at.Post("/login", nil, params)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d , got %d '%s', error '%s'",
			http.StatusUnauthorized, r.StatusCode, string(b), err)
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
	var u2 database.User
	err = json.Unmarshal(b, &u2)
	if err != nil {
		t.Fatal("Failed to parse user:", err)
	}
	if u2.Email != params.Get("email") {
		t.Fatalf("Expected user with email %s, got %s", params.Get("email"), u2.Email)
	}
}
