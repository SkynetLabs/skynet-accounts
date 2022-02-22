package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

// TestParseConfiguration ensures that we properly parse and validate all
// required environment variables.
func TestParseConfiguration(t *testing.T) {
	// Fetch current state of env and make sure we restore it on exit.
	{
		keys := []string{
			envPortal,
			envServerDomain,
			envStripeAPIKey,
			envAccountsJWKSFile,
			envJWTTTL,
			envEmailURI,
			envEmailFrom,
			envMaxNumAPIKeysPerUser,
		}
		values := make(map[string]string)
		for _, k := range keys {
			v, ok := os.LookupEnv(k)
			if ok {
				values[k] = v
			}
		}
		defer func() {
			errorList := make([]error, 0)
			for _, k := range keys {
				v, ok := values[k]
				if ok {
					errorList = append(errorList, os.Setenv(k, v))
				} else {
					errorList = append(errorList, os.Unsetenv(k))
				}
			}
			if e := errors.Compose(errorList...); e != nil {
				t.Error(e)
			}
		}()
	}

	logger := logrus.New()

	// Missing PORTAL_DOMAIN
	err := os.Setenv(envPortal, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseConfiguration(logger)
	if err == nil || !strings.Contains(err.Error(), "missing env var "+envPortal) {
		t.Fatal("Failed to error out on invalid", envPortal)
	}
	portal := "siasky.net"
	err = os.Setenv(envPortal, portal)
	if err != nil {
		t.Fatal(err)
	}

	// Missing SERVER_DOMAIN
	err = os.Setenv(envServerDomain, "")
	if err != nil {
		t.Fatal(err)
	}
	conf, err := parseConfiguration(logger)
	if conf.ServerLockID != database.PortalName {
		t.Fatalf("Expected ServerLockID to be %s, got %s", database.PortalName, conf.ServerLockID)
	}
	serverDomain := "test.siasky.net"
	err = os.Setenv(envServerDomain, serverDomain)
	if err != nil {
		t.Fatal(err)
	}

	// Invalid ACCOUNTS_JWT_TTL
	err = os.Setenv(envJWTTTL, "invalid TTL value")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseConfiguration(logger)
	if err == nil || !strings.Contains(err.Error(), "failed to parse env var "+envJWTTTL) {
		t.Fatal("Failed to error out on invalid", envJWTTTL)
	}
	// Zero ACCOUNTS_JWT_TTL
	err = os.Setenv(envJWTTTL, "0")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseConfiguration(logger)
	if err == nil || !strings.Contains(err.Error(), "env var is set to zero, which is an invalid value (must be positive or unset)") {
		t.Fatal("Failed to error out on zero", envJWTTTL)
	}
	ttl := 123
	err = os.Setenv(envJWTTTL, strconv.Itoa(ttl))
	if err != nil {
		t.Fatal(err)
	}

	// Missing ACCOUNTS_EMAIL_URI
	err = os.Setenv(envEmailURI, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseConfiguration(logger)
	if err == nil || !strings.Contains(err.Error(), envEmailURI+" is empty") {
		t.Fatal("Failed to error out on empty", envEmailURI)
	}
	// Invalid ACCOUNTS_EMAIL_URI
	err = os.Setenv(envEmailURI, "this is not empty but it's also not valid")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseConfiguration(logger)
	if err == nil || !strings.Contains(err.Error(), "invalid email URI") {
		t.Log(err)
		t.Fatal("Failed to error out on invalid", envEmailURI)
	}

	// Set all values
	emailFrom := "disabled@example.net"
	emailURIValue := fmt.Sprintf("smtps://%s:not-a-password@smtp.gmail.com:465/?skip_ssl_verify=false", emailFrom)
	err = os.Setenv(envEmailURI, emailURIValue)
	if err != nil {
		t.Fatal(err)
	}
	sk := "sk_live_THIS_IS_A_LIVE_KEY"
	err = os.Setenv(envStripeAPIKey, sk)
	if err != nil {
		t.Fatal(err)
	}

	// All values should be correct now. Make sure we have the correct
	// corresponding values in the returned configuration struct.
	config, err := parseConfiguration(logger)
	if err != nil {
		t.Fatal(err)
	}
	if config.EmailURI != emailURIValue {
		t.Fatalf("Expected %s, got %s", emailURIValue, config.EmailURI)
	}
	if config.EmailFrom != emailFrom {
		t.Fatalf("Expected %s, got %s", emailFrom, config.EmailFrom)
	}
	if config.PortalName != "https://"+portal {
		t.Fatalf("Expected %s, got %s", "https://"+portal, config.PortalName)
	}
	if config.PortalAddressAccounts != "https://account."+portal {
		t.Fatalf("Expected %s, got %s", "https://accounts."+portal, config.PortalAddressAccounts)
	}
	if config.StripeKey != sk {
		t.Fatalf("Expected %s, got %s", sk, config.StripeKey)
	}
	if config.StripeTestMode {
		t.Fatal("Expected live mode.")
	}
	if config.ServerLockID != serverDomain {
		t.Fatalf("Expected %s, got %s", serverDomain, config.ServerLockID)
	}
	if config.ServerLockID != serverDomain {
		t.Fatalf("Expected %s, got %s", serverDomain, config.ServerLockID)
	}
	if config.JWTTTL != ttl {
		t.Fatalf("Expected %d, got %d", ttl, config.JWTTTL)
	}
	if config.MaxAPIKeys != database.MaxNumAPIKeysPerUser {
		t.Fatalf("Expected %d, got %d", database.MaxNumAPIKeysPerUser, config.MaxAPIKeys)
	}

	// Set alternative config values and test their outcomes.

	// Set a test stripe key. Expect the key to change and tp enter test mode.
	sk = "sk_test_THIS_IS_A_TEST_KEY"
	err = os.Setenv(envStripeAPIKey, sk)
	if err != nil {
		t.Fatal(err)
	}
	maxKeys := 321
	err = os.Setenv(envMaxNumAPIKeysPerUser, strconv.Itoa(maxKeys))
	if err != nil {
		t.Fatal(err)
	}

	config, err = parseConfiguration(logger)
	if err != nil {
		t.Fatal(err)
	}
	if config.StripeKey != sk {
		t.Fatalf("Expected %s, got %s", sk, config.StripeKey)
	}
	if !config.StripeTestMode {
		t.Fatal("Expected test mode.")
	}
	if config.MaxAPIKeys != maxKeys {
		t.Fatalf("Expected %d, got %d", maxKeys, config.MaxAPIKeys)
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
	// Ensure the returned values are what we expect.
	creds, err := loadDBCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds.User != os.Getenv(envDBUser) || creds.Password != os.Getenv(envDBPass) ||
		creds.Host != os.Getenv(envDBHost) || creds.Port != os.Getenv(envDBPort) {
		t.Fatalf("Expected %s, %s, %s, %s, got %s, %s, %s, %s.",
			os.Getenv(envDBUser), os.Getenv(envDBPass), os.Getenv(envDBHost), os.Getenv(envDBPort),
			creds.User, creds.Password, creds.Host, creds.Port,
		)
	}
}
