package database

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/jwt"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/crypto"
	"golang.org/x/crypto/ed25519"
)

// TestChallengeResponse_LoadFromRequest tests the LoadFromReader method of
// ChallengeResponse.
func TestChallengeResponse_LoadFromRequest(t *testing.T) {
	var chr ChallengeResponse
	// Generate some valid data.
	sk, _ := crypto.GenerateKeyPair()
	response := append(fastrand.Bytes(ChallengeSize), append([]byte(ChallengeTypeLogin), []byte(jwt.JWTPortalName)...)...)
	signature := ed25519.Sign(sk[:], response)
	payload := challengeResponseDTO{
		Signature: hex.EncodeToString(fastrand.Bytes(16)),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r := &http.Request{
		Body: ioutil.NopCloser(bytes.NewReader(payloadBytes)),
	}
	// No "response" field.
	err = chr.LoadFromReader(r.Body)
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid response", err)
	}
	// Invalid response.
	payload = challengeResponseDTO{
		Response: hex.EncodeToString(fastrand.Bytes(16)),
	}
	payloadBytes, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(payloadBytes))
	err = chr.LoadFromReader(r.Body)
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid response", err)
	}
	// Missing signature.
	payload = challengeResponseDTO{
		Response: hex.EncodeToString(response),
	}
	payloadBytes, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(payloadBytes))
	err = chr.LoadFromReader(r.Body)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid signature", err)
	}
	// Invalid signature.
	payload = challengeResponseDTO{
		Response:  hex.EncodeToString(response),
		Signature: hex.EncodeToString(fastrand.Bytes(16)),
	}
	payloadBytes, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(payloadBytes))
	err = chr.LoadFromReader(r.Body)
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Expected error '%s', got '%s'", "invalid signature", err)
	}
	// Valid response and valid signature.
	payload = challengeResponseDTO{
		Response:  hex.EncodeToString(response),
		Signature: hex.EncodeToString(signature),
	}
	payloadBytes, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(payloadBytes))
	err = chr.LoadFromReader(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if subtle.ConstantTimeCompare(chr.Response, response) != 1 || subtle.ConstantTimeCompare(chr.Signature, signature) != 1 {
		t.Fatalf("Expected '%s' and '%s',\ngot '%s' and '%s'",
			hex.EncodeToString(response), hex.EncodeToString(signature),
			hex.EncodeToString(chr.Response), hex.EncodeToString(chr.Signature))
	}
}

// TestPubKey_String tests the String method of PubKey.
func TestPubKey_String(t *testing.T) {
	pk := PubKey(make([]byte, PubKeySize))
	// Expect a non-initialised pubkey to have an empty string representation.
	pkStr := pk.String()
	if pkStr != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("Expected '%s', got '%s'.", "0000000000000000000000000000000000000000000000000000000000000000", pkStr)
	}
	// Initialise the pubkey.
	b := fastrand.Bytes(PubKeySize)
	copy(pk[:], b)
	// Expect the string representation of a pubkey to be its hex-encoded bytes.
	pkStr = pk.String()
	if pkStr != hex.EncodeToString(b) {
		t.Fatalf("Expected '%s', got '%s'.", hex.EncodeToString(b), pkStr)
	}
}

// TestPubKey_LoadString tests the LoadString method of PubKey.
func TestPubKey_LoadString(t *testing.T) {
	var pk PubKey
	// Expect the loading to fail if the input is empty.
	err := pk.LoadString("")
	if err == nil {
		t.Fatal("Expected error 'invalid pubKey provided', got 'nil'.")
	}
	// Expect the loading to fail if the key does not contain exactly PubKeySize
	// bytes.
	err = pk.LoadString(hex.EncodeToString(fastrand.Bytes(PubKeySize - 1)))
	if err == nil {
		t.Fatal("Expected error 'invalid pubKey provided', got 'nil'.")
	}
	err = pk.LoadString(hex.EncodeToString(fastrand.Bytes(PubKeySize + 1)))
	if err == nil {
		t.Fatal("Expected error 'invalid pubKey provided', got 'nil'.")
	}
	// Expect the loading to fail if the input is not valid hex data.
	err = pk.LoadString(strings.Repeat("Z", PubKeySize*2))
	if err == nil {
		t.Fatal("Expected error 'invalid pubKey provided', got 'nil'.")
	}
	// Expect the loading to succeed when the size is right and the content is
	// of the correct type.
	pk2 := PubKey(fastrand.Bytes(PubKeySize))
	err = pk.LoadString(pk2.String())
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected pubkey.
	if subtle.ConstantTimeCompare(pk[:], pk2[:]) != 1 {
		t.Fatalf("Expected '%s', got '%s'", hex.EncodeToString(pk2[:]), hex.EncodeToString(pk[:]))
	}
}
