package main

import (
	"os"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

// TestParseEnvironmentVariables ensures that we properly parse and validate all
// required environment variables.
func TestParseEnvironmentVariables(t *testing.T) {
	logger := logrus.New()

	// Missing PORTAL_DOMAIN
	err := os.Setenv(envPortal, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envPortal) {
		t.Fatal("Failed to error out on invalid", envPortal)
	}
	err = os.Setenv(envPortal, "siasky.net")
	if err != nil {
		t.Fatal(err)
	}

	// Missing SERVER_DOMAIN
	err = os.Setenv(envServerDomain, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if email.ServerLockID != database.PortalName {
		t.Fatalf("Expected ServerLockID to be %s, got %s", database.PortalName, email.ServerLockID)
	}
	err = os.Setenv(envServerDomain, "test.siasky.net")
	if err != nil {
		t.Fatal(err)
	}

	// Invalid ACCOUNTS_JWT_TTL
	err = os.Setenv(envJWTTTL, "invalid TTL value")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if err == nil || !strings.Contains(err.Error(), "failed to parse env var "+envJWTTTL) {
		t.Fatal("Failed to error out on invalid", envJWTTTL)
	}
	// Zero ACCOUNTS_JWT_TTL
	err = os.Setenv(envJWTTTL, "0")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if err == nil || !strings.Contains(err.Error(), "env var is set to zero, which is an invalid value (must be positive or unset)") {
		t.Fatal("Failed to error out on zero", envJWTTTL)
	}
	err = os.Setenv(envJWTTTL, "")
	if err != nil {
		t.Fatal(err)
	}

	// Missing ACCOUNTS_EMAIL_URI
	err = os.Setenv(envEmailURI, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if err == nil || !strings.Contains(err.Error(), envEmailURI+" is empty") {
		t.Fatal("Failed to error out on empty", envEmailURI)
	}
	// Invalid ACCOUNTS_EMAIL_URI
	err = os.Setenv(envEmailURI, "this is not empty but it's also not valid")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseEnvironmentVariables(logger)
	if err == nil || !strings.Contains(err.Error(), "invalid email URI") {
		t.Log(err)
		t.Fatal("Failed to error out on invalid", envEmailURI)
	}
	emailURIValue := "smtps://disabled@example.net:not-a-password@smtp.gmail.com:465/?skip_ssl_verify=false"
	err = os.Setenv(envEmailURI, emailURIValue)
	if err != nil {
		t.Fatal(err)
	}
	// All values should be correct now.
	emailURI, err := parseEnvironmentVariables(logger)
	if err != nil {
		t.Fatal(err)
	}
	if emailURI != emailURIValue {
		t.Fatalf("Expected email URI '%s', got '%s'", emailURIValue, emailURI)
	}
}

// TestLoadDBCredentials ensures that we validate that all required environment
// variables are present.
func TestLoadDBCredentials(t *testing.T) {
	originals, err := loadDBCredentials()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		e1 := os.Setenv(envDBUser, originals.User)
		e2 := os.Setenv(envDBPass, originals.Password)
		e3 := os.Setenv(envDBHost, originals.Host)
		e4 := os.Setenv(envDBPort, originals.Port)
		e := errors.Compose(e1, e2, e3, e4)
		if e != nil {
			t.Error(e)
		}
	}()
	// Missing user
	err = os.Unsetenv(envDBUser)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadDBCredentials()
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envDBUser) {
		t.Log(err)
		t.Fatal("Failed to error out on missing", envDBUser)
	}
	err = os.Setenv(envDBUser, originals.User)
	if err != nil {
		t.Fatal(err)
	}
	// Missing password
	err = os.Unsetenv(envDBPass)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadDBCredentials()
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envDBPass) {
		t.Fatal("Failed to error out on missing", envDBPass)
	}
	err = os.Setenv(envDBPass, originals.Password)
	if err != nil {
		t.Fatal(err)
	}
	// Missing host
	err = os.Unsetenv(envDBHost)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadDBCredentials()
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envDBHost) {
		t.Fatal("Failed to error out on missing", envDBHost)
	}
	err = os.Setenv(envDBHost, originals.Host)
	if err != nil {
		t.Fatal(err)
	}
	// Missing port
	err = os.Unsetenv(envDBPort)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadDBCredentials()
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envDBPort) {
		t.Fatal("Failed to error out on missing", envDBPort)
	}
	err = os.Setenv(envDBPort, originals.Port)
	if err != nil {
		t.Fatal(err)
	}
}
