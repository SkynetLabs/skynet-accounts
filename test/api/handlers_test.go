package api

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
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
		// GET /health
		{Name: "health", Test: testHandlerHealthGET},
		// POST /user, POST /login
		{Name: "createUser", Test: testHandlerUserPOST},
		// POST /user, POST /login, PUT /user, GET /user, POST /logout
		{Name: "testUserFlow", Test: testUserFlow},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Test(t, at)
		})
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
		t.Fatal("Failed to unmarshal service's response: ", err)
	}
	// DBAlive should never be false because if we couldn't reach the DB, we
	// wouldn't have made it this far in the test.
	if !status.DBAlive {
		t.Fatal("DB down.")
	}
}

// testHandlerUserPOST tests user creation and login.
func testHandlerUserPOST(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := strings.ReplaceAll(t.Name(), "/", "_")
	params := map[string]string{
		"email":    name + "@siasky.net",
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Create a user.
	_, _, err := at.Post("/user", map[string]string{}, params)
	if err != nil {
		t.Fatal("User creation failed. Error ", err.Error())
	}
	// Make sure the user exists in the DB.
	u, err := at.DB.UserByEmail(at.Ctx, params["email"], false)
	if err != nil {
		t.Fatal("Error while fetching the user from the DB. Error ", err.Error())
	}
	// Clean up the user after the test.
	defer func(user *database.User) {
		err = at.DB.UserDelete(at.Ctx, user)
		if err != nil {
			t.Errorf("Error while cleaning up user: %s", err.Error())
			return
		}
	}(u)
	// Log in with that user in order to make sure it exists.
	_, _, err = at.Post("/login", map[string]string{}, params)
	if err != nil {
		t.Fatal("Login failed. Error ", err.Error())
	}
}

// testUserFlow tests the happy path of a user's everyday life: create, login,
// edit, logout. It focuses on the happy path and leaves the edge cases to the
// per-handler tests.
func testUserFlow(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := strings.ReplaceAll(t.Name(), "/", "_")
	params := map[string]string{
		"email":    name + "@siasky.net",
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Create a user.
	_, _, err := at.Post("/user", map[string]string{}, params)
	if err != nil {
		t.Fatal("User creation failed. Error ", err.Error())
	}
	// Make sure the user exists in the DB.
	u, err := at.DB.UserByEmail(at.Ctx, params["email"], false)
	if err != nil {
		t.Fatal("Error while fetching the user from the DB. Error ", err.Error())
	}
	// Clean up the user after the test.
	defer func(user *database.User) {
		err = at.DB.UserDelete(at.Ctx, user)
		if err != nil {
			t.Errorf("Error while cleaning up user: %s", err.Error())
			return
		}
	}(u)
	// Log in with that user in order to make sure it exists.
	r, _, err := at.Post("/login", map[string]string{}, params)
	if err != nil {
		t.Fatal("Login failed. Error ", err.Error())
	}
	// Grab the Skynet cookie, so we can make authenticated calls.
	at.Cookie = test.ExtractCookie(r)
	if at.Cookie == nil {
		t.Fatalf("Failed to extract cookie from request. Cookies found: %+v", r.Cookies())
	}
	// Change the user's email.
	newEmail := name + "_new@siasky.net"
	r, b, err := at.Put("/user", nil, map[string]string{"email": newEmail})
	if err != nil {
		t.Fatalf("Failed to update user. Error: %s. Body: %s", err.Error(), string(b))
	}
	// Grab the new cookie. It has changed because of the user edit.
	at.Cookie = test.ExtractCookie(r)
	if at.Cookie == nil {
		t.Fatalf("Failed to extract cookie from request. Cookies found: %+v", r.Cookies())
	}
	_, b, err = at.Get("/user", nil)
	if err != nil {
		t.Fatal("Failed to fetch the updated user:", err.Error())
	}
	// Make sure the email is updated.
	u2 := database.User{}
	err = json.Unmarshal(b, &u2)
	if err != nil {
		t.Fatal("Failed to unmarshal user:", err.Error())
	}
	if u2.Email != newEmail {
		t.Fatalf("Email mismatch. Expected %s, got %s", newEmail, u2.Email)
	}
	r, _, err = at.Post("/logout", nil, nil)
	if err != nil {
		t.Fatal("Failed to logout:", err.Error())
	}
	// Grab the new cookie.
	at.Cookie = test.ExtractCookie(r)
	// Try to get the user, expect a 401.
	_, _, err = at.Get("/user", nil)
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("Expected to get %s, got %s", "401 Unauthorized", err)
	}
}
