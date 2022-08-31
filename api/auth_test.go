package api

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/types"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestAPIKeyFromRequest ensures that apiKeyFromRequest works as expected.
func TestAPIKeyFromRequest(t *testing.T) {
	req := &http.Request{
		Form:   make(map[string][]string),
		Header: make(map[string][]string),
	}

	// API key from request with no API key.
	_, err := apiKeyFromRequest(req)
	if err != ErrNoAPIKey {
		t.Fatalf("Expected '%s', got '%s'.", ErrNoAPIKey, err)
	}

	// API key from headers.
	akStr := randomAPIKeyString()
	req.Header.Set(APIKeyHeader, akStr)
	ak, err := apiKeyFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if ak.String() != akStr {
		t.Fatalf("Expected '%s', got '%s'.", akStr, ak)
	}
}

// TestTokenFromRequest ensures that tokenFromRequest works as expected.
func TestTokenFromRequest(t *testing.T) {
	jwt.AccountsJWKSFile = "../jwt/fixtures/jwks.json"
	err := jwt.LoadAccountsKeySet(logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	tk, err := jwt.TokenForUser(types.NewEmail(t.Name()+"@siasky.net"), t.Name()+"_sub", 0)
	if err != nil {
		t.Fatal(err)
	}
	tkBytes, err := jwt.TokenSerialize(tk)
	if err != nil {
		t.Fatal(err)
	}

	req := &http.Request{
		Header: make(map[string][]string),
	}

	// Token from request with no token.
	_, err = tokenFromRequest(req)
	if err == nil || !errors.Contains(err, ErrNoToken) {
		t.Fatalf("Expected '%s', got %v", ErrNoToken.Error(), err)
	}

	// Token from request with a cookie.
	encodedValue, err := secureCookie.Encode(CookieName, string(tkBytes))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    encodedValue,
		HttpOnly: true,
		Path:     "/",
		Expires:  time.Now().Add(time.Hour),
		MaxAge:   3600,
		Secure:   true, // do not send over insecure channels, e.g. HTTP
		SameSite: 1,    // https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00
	}
	req.AddCookie(cookie)
	tk, err = tokenFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	tkB, err := jwt.TokenSerialize(tk)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(tkB, tkBytes) {
		t.Log(string(tkB), "\n", string(tkBytes))
		t.Fatal("Token mismatch.")
	}

	// Token from request with a header and a cookie. Expect the header to take
	// precedence.
	tk2, err := jwt.TokenForUser(types.NewEmail(t.Name()+"2@siasky.net"), t.Name()+"2_sub", 0)
	if err != nil {
		t.Fatal(err)
	}
	tkBytes2, err := jwt.TokenSerialize(tk2)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+string(tkBytes2))
	tk, err = tokenFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	tkB, err = jwt.TokenSerialize(tk)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tkB, tkBytes) {
		t.Fatal("Cookie token got precedence over header token.")
	}

	if !bytes.Equal(tkB, tkBytes2) {
		t.Fatal("Token mismatch.")
	}

	// Invalid token. ValidateToken is tested elsewhere, all we aim for here is
	// to make sure it's being called.
	invalidToken := base64.StdEncoding.EncodeToString(fastrand.Bytes(len(tkBytes)))
	req.Header.Set("Authorization", "Bearer "+invalidToken)
	_, err = tokenFromRequest(req)
	if err == nil {
		t.Fatal("Invalid token passed validation. Token:", invalidToken)
	}
}

// randomAPIKeyString is a helper.
func randomAPIKeyString() string {
	return base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(fastrand.Bytes(database.PubKeySize))
}
