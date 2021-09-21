package jwt

import (
	"context"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/lib"
	"github.com/sirupsen/logrus"
)

// TestJWT ensures we can generate and validate JWTs. It also ensures that we
// accurately reject forged tokens.
func TestJWT(t *testing.T) {
	logger := logrus.StandardLogger()
	err := LoadAccountsKeySet(logger)
	if err != nil {
		t.Fatal(err)
	}
	_, tkBytes, err := TokenForUser("user@example.com", "this_is_a_sub")
	if err != nil {
		t.Fatal("failed to generate token:", err)
	}

	// Happy case.
	_, err = ValidateToken(logger, string(tkBytes))
	if err != nil {
		t.Fatal("failed to validate token:", err)
	}

	// Change the data and ensure the validation will fail.
	parts := strings.Split(string(tkBytes), ".")
	body, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "this_is_a_sub", "this_is_A_FAKE_sub", 1))
	parts[1] = base64.StdEncoding.EncodeToString(body)
	forgedTkStr := strings.Join(parts, ".")
	_, err = ValidateToken(logger, forgedTkStr)
	if err == nil {
		t.Fatalf("expected error '%s', got <nil>", "verification error")
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
