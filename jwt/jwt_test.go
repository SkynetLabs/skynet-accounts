package jwt

import (
	"context"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/lib"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

// TestJWT ensures we can generate and validate JWTs. It also ensures that we
// accurately reject forged tokens.
func TestJWT(t *testing.T) {
	err := LoadAccountsKeySet(logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	email := t.Name() + "@siasky.net"
	sub := "this is a sub"
	fakeSub := "fake sub"
	_, tkBytes, err := TokenForUser(email, sub)
	if err != nil {
		t.Fatal("failed to generate token:", err)
	}

	// Happy case.
	_, err = ValidateToken(string(tkBytes))
	if err != nil {
		t.Fatal("failed to validate token:", err)
	}

	// Change the data and ensure the validation will fail.
	parts := strings.Split(string(tkBytes), ".")
	body, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), sub, fakeSub, 1))
	parts[1] = base64.StdEncoding.EncodeToString(body)
	forgedTkStr := strings.Join(parts, ".")
	_, err = ValidateToken(forgedTkStr)
	if err == nil {
		t.Fatalf("expected error '%s', got <nil>", "verification error")
	}
}

// TestValidateToken_Expired specifically tests that ValidateToken properly
// detects expired token.
func TestValidateToken_Expired(t *testing.T) {
	err := LoadAccountsKeySet(logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	email := t.Name() + "@siasky.net"
	sub := "this is a sub"
	// Fetch the tools we need in order to craft a custom token.
	key, found := AccountsJWKS.Get(0)
	if !found {
		t.Fatal("No JWKS available.")
	}
	var sigAlgo jwa.SignatureAlgorithm
	for _, sa := range jwa.SignatureAlgorithms() {
		if string(sa) == key.Algorithm() {
			sigAlgo = sa
			break
		}
	}
	if sigAlgo == "" {
		t.Fatal("Failed to determine signature algorithm.")
	}
	// Craft a token with custom expiration time that has already passed.
	session := tokenSession{
		Active: true,
		Identity: tokenIdentity{
			Traits: tokenTraits{
				Email: email,
			},
		},
	}
	now := time.Now().UTC()
	tk := jwt.New()
	err1 := tk.Set("exp", now.Unix()-1)
	err2 := tk.Set("iat", now.Unix()-10)
	err3 := tk.Set("iss", JWTPortalName)
	err4 := tk.Set("sub", sub)
	err5 := tk.Set("session", session)
	err = errors.Compose(err1, err2, err3, err4, err5)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := jwt.Sign(tk, sigAlgo, key)
	if err != nil {
		t.Fatal("Failed to sign token.")
	}
	_, err = ValidateToken(string(bytes))
	if err != ErrTokenExpired {
		t.Fatalf("Expected an ErrTokenExpired, got %v", err)
	}
}

// TestTokenFromContext ensures that TokenFromContext works as expected.
// Note that this test does not cover validating the token's signature, as that
// is handled when the token is inserted into the context by api.validate().
func TestTokenFromContext(t *testing.T) {
	err := LoadAccountsKeySet(logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	// Create a test token.
	email := "user@email.com"
	sub, err := lib.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	tk, _, err := TokenForUser(email, sub)
	if err != nil {
		t.Fatal("failed to generate token:", err)
	}
	// Embed the token in a new context.
	ctx := ContextWithToken(context.Background(), tk)
	// Happy case.
	subNew, emailNew, tkNew, err := TokenFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if subNew != sub {
		t.Fatalf("Expected sub to be %s, got %s.", sub, subNew)
	}
	if emailNew != email {
		t.Fatalf("Expected email to be %s, got %s.", email, emailNew)
	}
	if !reflect.DeepEqual(tk, tkNew) {
		t.Fatal("Fetched token is different from the original.")
	}
	// Test missing context.
	_, _, _, err = TokenFromContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to parse token from context") {
		t.Fatalf("Expected error 'failed to parse token from context', got %v.", err)
	}
}
