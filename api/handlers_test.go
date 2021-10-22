package api

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// TestChallengeResponseFromRequest is a simple unit test.
func TestChallengeResponseFromRequest(t *testing.T) {
	// Generate some valid data.
	sk, _ := crypto.GenerateKeyPair()
	response := append(fastrand.Bytes(database.ChallengeSize), append([]byte(database.ChallengeTypeLogin), []byte(jwt.JWTPortalName)...)...)
	signature := ed25519.Sign(sk[:], response)
	r := &http.Request{PostForm: url.Values{}}
	// No "response" field.
	_, err := challengeResponseFromRequest(r)
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid response", err)
	}
	// Invalid response.
	r.PostForm.Set("response", hex.EncodeToString(fastrand.Bytes(16)))
	_, err = challengeResponseFromRequest(r)
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid response", err)
	}
	// Missing signature.
	r.PostForm.Set("response", hex.EncodeToString(response))
	_, err = challengeResponseFromRequest(r)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid signature", err)
	}
	// Invalid signature.
	r.PostForm.Set("signature", hex.EncodeToString(fastrand.Bytes(16)))
	_, err = challengeResponseFromRequest(r)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid signature", err)
	}
	// Valid response and valid signature.
	r.PostForm.Set("signature", hex.EncodeToString(signature))
	chr, err := challengeResponseFromRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if subtle.ConstantTimeCompare(chr.Response, response) != 1 || subtle.ConstantTimeCompare(chr.Signature, signature) != 1 {
		t.Fatalf("Expected '%s' and '%s',\ngot '%s' and '%s'",
			hex.EncodeToString(response), hex.EncodeToString(signature),
			hex.EncodeToString(chr.Response), hex.EncodeToString(chr.Signature))
	}
}
