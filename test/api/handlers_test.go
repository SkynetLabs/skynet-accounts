package api

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
)

type subtest struct {
	Name string
	Test func(t *testing.T, at *test.AccountsTester)
}

// TestHandlers is a meta test that sets up a test instance of accounts and runs
// a suite of tests that ensure all handlers behave as expected.
func TestHandlers(t *testing.T) {
	at, err := test.NewAccountsTester()
	if err != nil {
		t.Fatal(err)
	}
	defer at.Shutdown()

	// Specify subtests to run
	tests := []subtest{
		{Name: "health", Test: testHandlerHealthGET},
		{Name: "createUser", Test: testHandlerUserPOST},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Test(t, at)
		})
	}
}

// testHandlerUserPOST tests user creation and login.
func testHandlerUserPOST(t *testing.T, at *test.AccountsTester) {
	name := strings.ReplaceAll(t.Name(), "/", "_")
	params := map[string]string{
		"email":    name + "@siasky.net",
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Create a user.
	_, _, err := at.Post("/user", map[string]string{}, params)
	if err != nil {
		t.Fatalf("User creation failed. Error %s", err.Error())
	}
	// Clean up the user after the test.
	defer func() {
		u, err := at.DB.UserByEmail(at.Ctx, params["email"], false)
		if err != nil {
			t.Logf("Error while cleaning up user: %s", err.Error())
			return
		}
		err = at.DB.UserDelete(at.Ctx, u)
		if err != nil {
			t.Logf("Error while cleaning up user: %s", err.Error())
			return
		}
	}()
	// Log in with that user in order to make sure it exists.
	_, _, err = at.Post("/login", map[string]string{}, params)
	if err != nil {
		t.Fatalf("Login failed. Error %s", err.Error())
	}
}

// testHandlerHealthGET tests the /health handler.
func testHandlerHealthGET(t *testing.T, at *test.AccountsTester) {
	_, b, err := at.Get("/health", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	status := struct {
		DBAlive bool `json:"dbAlive"`
	}{}
	err = json.Unmarshal(b, &status)
	if err != nil {
		t.Fatal(err)
	}
	// DBAlive should never be false because if we couldn't reach the DB, we
	// wouldn't have made it this far in the test.
	if !status.DBAlive {
		t.Fatal("DB down.")
	}
}
