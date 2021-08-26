package jwt

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestJWT(t *testing.T) {
	logger := logrus.StandardLogger()
	tkBytes, err := TokenForUserSerialized(logger, "user@example.com", "this_is_a_sub")
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
	fmt.Println(parts[1])

	body, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "this_is_a_sub", "this_is_A_FAKE_sub", 1))
	t.Log(string(body))
	parts[1] = base64.StdEncoding.EncodeToString(body)
	forgedTkStr := strings.Join(parts, ".")
	_, err = ValidateToken(logger, forgedTkStr)
	if err == nil {
		t.Fatalf("expected error '%s', got <nil>", "verification error")
	}
}
