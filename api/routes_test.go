package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/sirupsen/logrus"
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

	// API key from request form.
	token := t.Name()
	req.Form.Add("api_key", token)
	tk, err := apiKeyFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if tk != token {
		t.Fatalf("Expected '%s', got '%s'.", token, tk)
	}

	// API key from headers. Expect this to take precedence over request form.
	token2 := t.Name() + "2"
	req.Header.Set(APIKeyHeader, token2)
	tk, err = apiKeyFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if tk == token {
		t.Fatal("Form token took precedence over headers token.")
	}
	if tk != token2 {
		t.Fatalf("Expected '%s', got '%s'.", token2, tk)
	}
}

// TestTokenFromRequest ensures that tokenFromRequest works as expected.
func TestTokenFromRequest(t *testing.T) {
	// FIXME This is a weird and ugly workaround for the fact that I don't know
	// 	how to refer to a file relative to the project's root dir.
	jwt.AccountsJWKSFile = "../jwt/fixtures/jwks.json"
	err := jwt.LoadAccountsKeySet(logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	_, tkBytes, err := jwt.TokenForUser(t.Name()+"@siasky.net", t.Name()+"_sub")
	if err != nil {
		t.Fatal(err)
	}

	req := &http.Request{
		Header: make(map[string][]string),
	}

	// Token from request with no token.
	_, _, err = tokenFromRequest(req)
	if err == nil || !strings.Contains(err.Error(), "no authorisation token found") {
		t.Fatalf("Expected 'no authorisation token found', got %v", err)
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
	_, tkStr, err := tokenFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if tkStr != string(tkBytes) {
		t.Fatalf("Expected '%s', got '%s'", string(tkBytes), tkStr)
	}

	// Token from request with a header and a cookie. Expect the header to take
	// precedence.
	_, tkBytes2, err := jwt.TokenForUser(t.Name()+"2@siasky.net", t.Name()+"2_sub")
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+string(tkBytes2))
	_, tkStr, err = tokenFromRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if tkStr == string(tkBytes) {
		t.Fatal("Cookie token got precedence over header token.")
	}
	if tkStr != string(tkBytes2) {
		t.Fatalf("Expected '%s', got '%s'", string(tkBytes2), tkStr)
	}

	// Invalid token. ValidateToken is tested elsewhere, all we aim for here is
	// to make sure it's being called.
	invalidToken := base64.StdEncoding.EncodeToString(fastrand.Bytes(len(tkBytes)))
	req.Header.Set("Authorization", "Bearer "+invalidToken)
	_, _, err = tokenFromRequest(req)
	if err == nil {
		t.Fatal("Invalid token passed validation. Token:", invalidToken)
	}
}
