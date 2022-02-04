package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.sia.tech/siad/crypto"
)

const (
	// Helper consts for checking the returned errors.
	badRequest   = "400 Bad Request"
	unauthorized = "401 Unauthorized"
)

type subtest struct {
	name string
	test func(t *testing.T, at *test.AccountsTester)
}

// TestHandlers is a meta test that sets up a test instance of accounts and runs
// a suite of tests that ensure all handlers behave as expected.
func TestHandlers(t *testing.T) {
	dbName := test.DBNameForTest(t.Name())
	at, err := test.NewAccountsTester(dbName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	// Specify subtests to run
	tests := []subtest{
		// GET /health
		{name: "Health", test: testHandlerHealthGET},
		// POST /user, POST /login
		{name: "UserCreate", test: testHandlerUserPOST},
		// POST /login, POST /logout, POST /user
		{name: "LoginLogout", test: testHandlerLoginPOST},
		// PUT /user
		{name: "UserEdit", test: testUserPUT},
		// PUT /user
		{name: "UserAddPubKey", test: testUserAddPubKey},
		// DELETE /user
		{name: "UserDelete", test: testUserDELETE},
		// GET /user/limits
		{name: "UserLimits", test: testUserLimits},
		// DELETE /user/uploads/:skylink, GET /user/uploads
		{name: "UserDeleteUploads", test: testUserUploadsDELETE},
		// GET /user/confirm, POST /user/reconfirm
		{name: "UserConfirmReconfirmEmail", test: testUserConfirmReconfirmEmailGET},
		// GET /user/recover, POST /user/recover, POST /login
		{name: "UserAccountRecovery", test: testUserAccountRecovery},
		// POST /track/upload/:skylink, POST /track/download/:skylink, POST /track/registry/read, POST /track/registry/write, GET /user/stats
		{name: "StandardTrackingFlow", test: testTrackingAndStats},
		// POST /user, POST /login, PUT /user, GET /user, POST /logout
		{name: "StandardUserFlow", test: testUserFlow},
		// GET /register, POST /register
		{name: "Challenge-Response/Registration", test: testRegistration},
		// GET /register, POST /register, GET /login, POST /login
		{name: "Challenge-Response/Login", test: testLogin},
		// GET /user/apikeys, POST /user/apikeys, DELETE /user/apikeys/:apiKey
		{name: "APIKeysFlow", test: testAPIKeysFlow},
		// POST /user/apikeys, GET /user/stats?api_key
		{name: "APIKeysUsage", test: testAPIKeysUsage},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t, at)
		})
	}
}

// testHandlerHealthGET tests the /health handler.
func testHandlerHealthGET(t *testing.T, at *test.AccountsTester) {
	_, b, err := at.Get("/health", nil)
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
	name := test.DBNameForTest(t.Name())
	emailAddr := name + "@siasky.net"
	password := hex.EncodeToString(fastrand.Bytes(16))
	// Try to create a user with a missing email.
	params := url.Values{}
	params.Add("password", password)
	_, _, err := at.Post("/user", nil, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'", badRequest, err)
	}
	// Try to create a user with an empty email.
	_, b, err := at.CreateUserPost("", "password")
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try to create a user with an invalid email.
	_, b, err = at.CreateUserPost("invalid", "password")
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try to create a user with an empty password.
	params = url.Values{}
	params.Add("email", emailAddr)
	_, b, err = at.Post("/user", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'. Body: '%s", badRequest, err, string(b))
	}
	// Create a user.
	_, b, err = at.CreateUserPost(emailAddr, password)
	if err != nil {
		t.Fatalf("User creation failed. Error: '%s'. Body: '%s' ", err.Error(), string(b))
	}
	// Make sure the user exists in the DB.
	u, err := at.DB.UserByEmail(at.Ctx, emailAddr)
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
	params = url.Values{}
	params.Add("email", emailAddr)
	params.Add("password", password)
	_, b, err = at.Post("/login", nil, params)
	if err != nil {
		t.Fatalf("Login failed. Error: '%s'. Body: '%s'", err.Error(), string(b))
	}
	// try to create a user with an already taken email
	_, b, err = at.CreateUserPost(emailAddr, "password")
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
}

// testHandlerLoginPOST tests the /login endpoint.
func testHandlerLoginPOST(t *testing.T, at *test.AccountsTester) {
	emailAddr := test.DBNameForTest(t.Name()) + "@siasky.net"
	password := hex.EncodeToString(fastrand.Bytes(16))
	params := url.Values{}
	params.Add("email", emailAddr)
	params.Add("password", password)
	// Try logging in with a non-existent user.
	_, _, err := at.Post("/login", nil, params)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'", unauthorized, err)
	}
	u, err := test.CreateUser(at, emailAddr, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()
	// Login with an existing user.
	r, _, err := at.Post("/login", nil, params)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the response contains a login cookie.
	c := test.ExtractCookie(r)
	if c == nil {
		t.Fatal("Expected a cookie.")
	}
	// Make sure the returned cookie is usable for making requests.
	at.Cookie = c
	defer func() { at.Cookie = nil }()
	_, b, err := at.Get("/user", nil)
	if err != nil || !strings.Contains(string(b), emailAddr) {
		t.Fatal("Expected to be able to fetch the user with this cookie.")
	}
	// test /logout while we're here.
	r, b, err = at.Post("/logout", nil, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Expect the returned cookie to be already expired.
	at.Cookie = test.ExtractCookie(r)
	if at.Cookie == nil {
		t.Fatal("Expected to have a cookie.")
	}
	if at.Cookie.Expires.After(time.Now().UTC()) {
		t.Fatal("Expected the cookie to have expired already.")
	}
	// Expect to be unable to get the user with this cookie.
	_, _, err = at.Get("/user", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatal("Expected to be unable to fetch the user with this cookie.")
	}
	// Try logging out again. This should fail with a 401.
	_, _, err = at.Post("/logout", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected %s, got %s", unauthorized, err)
	}
	// Try logging in with a bad password.
	badPassParams := url.Values{}
	badPassParams.Add("email", emailAddr)
	badPassParams.Add("password", "bad password")
	_, _, err = at.Post("/login", nil, badPassParams)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'", unauthorized, err)
	}
}

// testUserPUT tests the PUT /user endpoint.
func testUserPUT(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	u, c, err := test.CreateUserAndLogin(at, name)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
			panic(err)
		}
	}()

	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Call unauthorized.
	at.Cookie = nil
	_, _, err = at.Put("/user", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Update the user's Stripe ID.
	stripeID := name + "_stripe_id"
	_, b, err := at.UserPUT("", "", stripeID)
	if err != nil {
		t.Fatal(err, string(b))
	}
	var u2 database.User
	err = json.Unmarshal(b, &u2)
	if err != nil {
		t.Fatal(err)
	}
	if u2.StripeID != stripeID {
		t.Fatalf("Expected the user to have StripeID %s, got %s", stripeID, u2.StripeID)
	}
	// Try to update the StripeID again. Expect this to fail.
	r, b, err := at.UserPUT("", "", stripeID)
	if err == nil || !strings.Contains(err.Error(), "409 Conflict") || r.StatusCode != http.StatusConflict {
		t.Fatalf("Expected to get error '409 Conflict' and status 409, got '%s' and %d. Body: '%s'", err, r.StatusCode, string(b))
	}

	// Update the user's password with an empty one. Expect this to succeed but
	// not change anything.
	r, b, _ = at.UserPUT("", "", "")
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request, got %d", r.StatusCode)
	}
	// Fetch the user from the DB again and make sure their password hash hasn't
	// changed.
	uSamePassHash, err := at.DB.UserByID(at.Ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if uSamePassHash.PasswordHash != u.PasswordHash {
		t.Fatal("Expected the user's password to not change but it did.")
	}
	pw := hex.EncodeToString(fastrand.Bytes(12))
	_, b, err = at.UserPUT("", pw, "")
	if err != nil {
		t.Fatal(err)
	}
	// Fetch the user from the DB again and make sure their password hash has
	// changed.
	uNewPassHash, err := at.DB.UserByID(at.Ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if uNewPassHash.PasswordHash == u.PasswordHash {
		t.Fatal("Expected the user's password to change but it did not.")
	}
	// Check if we can login with the new password.
	params := url.Values{}
	params.Add("email", u.Email)
	params.Add("password", pw)
	// Try logging in with a non-existent user.
	_, _, err = at.Post("/login", nil, params)
	if err != nil {
		t.Fatal(err)
	}

	// Update the user's email.
	emailAddr := name + "_new@siasky.net"
	r, b, err = at.UserPUT(emailAddr, "", "")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatal(r.StatusCode, string(b), err)
	}
	// Fetch the user from the DB because we want to be sure that their email
	// is marked as unconfirmed which is not reflected in the JSON
	// representation of the object.
	u3, err := at.DB.UserByEmail(at.Ctx, emailAddr)
	if err != nil {
		t.Fatal(err)
	}
	if u3.Email != emailAddr {
		t.Fatalf("Expected the user to have email %s, got %s", emailAddr, u3.Email)
	}
	if u3.EmailConfirmationToken == "" {
		t.Fatalf("Expected the user to have a non-empty confirmation token, got '%s'", u3.EmailConfirmationToken)
	}
	// Expect to find a confirmation email queued for sending.
	filer := bson.M{"to": emailAddr}
	_, msgs, err := at.DB.FindEmails(at.Ctx, filer, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "Please verify your email address" {
		t.Fatal("Expected to find a single confirmation email but didn't.")
	}
}

// testUserDELETE tests the DELETE /user endpoint.
func testUserDELETE(t *testing.T, at *test.AccountsTester) {
	u, c, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	// Delete the user.
	at.Cookie = c
	defer func() { at.Cookie = nil }()
	r, _, err := at.Delete("/user", nil)
	if err != nil || r.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected %d success, got %d '%s'", http.StatusNoContent, r.StatusCode, err)
	}
	// Make sure the use doesn't exist anymore.
	_, err = at.DB.UserByEmail(at.Ctx, u.Email)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error '%s', got '%s'.", database.ErrUserNotFound, err)
	}
	// Create the user again.
	u, c, err = test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	// Create some data for this user.
	sl, _, err := test.CreateTestUpload(at.Ctx, at.DB, u.User, 128)
	if err != nil {
		t.Fatal(err)
	}
	err = at.DB.DownloadCreate(at.Ctx, *u.User, *sl, 128)
	if err != nil {
		t.Fatal(err)
	}
	_, err = at.DB.RegistryWriteCreate(at.Ctx, *u.User)
	if err != nil {
		t.Fatal(err)
	}
	_, err = at.DB.RegistryReadCreate(at.Ctx, *u.User)
	if err != nil {
		t.Fatal(err)
	}
	// Try to delete the user without a cookie.
	at.Cookie = nil
	r, _, _ = at.Delete("/user", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d, got %d", http.StatusUnauthorized, r.StatusCode)
	}
	// Delete the user.
	at.Cookie = c
	defer func() { at.Cookie = nil }()
	r, _, err = at.Delete("/user", nil)
	if err != nil || r.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected %d success, got %d '%s'", http.StatusNoContent, r.StatusCode, err)
	}
	// Make sure the user doesn't exist anymore.
	_, err = at.DB.UserByEmail(at.Ctx, u.Email)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error '%s', got '%s'.", database.ErrUserNotFound, err)
	}
	// Make sure that the data is gone.
	stats, err := at.DB.UserStats(at.Ctx, *u.User)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NumUploads != 0 || stats.NumDownloads != 0 || stats.NumRegReads != 0 || stats.NumRegWrites != 0 {
		t.Fatalf("Expected all user stats to be zero, got uploads %d, downloads %d, registry reads %d, registry writes %d,",
			stats.NumUploads, stats.NumDownloads, stats.NumRegReads, stats.NumRegWrites)
	}
	// Try to delete the same user again.
	r, _, _ = at.Delete("/user", nil)
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected %d Not Found, got %d.", http.StatusNotFound, r.StatusCode)
	}
}

// testUserLimits tests the /user/limits endpoint.
func testUserLimits(t *testing.T, at *test.AccountsTester) {
	u, c, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Call /user/limits with a cookie. Expect FreeTier response.
	_, b, err := at.Get("/user/limits", nil)
	if err != nil {
		t.Fatal(err)
	}
	var tl database.TierLimits
	err = json.Unmarshal(b, &tl)
	if err != nil {
		t.Fatal(err)
	}
	if tl.TierName != database.UserLimits[database.TierFree].TierName {
		t.Fatalf("Expected to get the results for %s, got %s", database.UserLimits[database.TierFree].TierName, tl.TierName)
	}

	// Call /user/limits without a cookie. Expect FreeAnonymous response.
	at.Cookie = nil
	_, b, err = at.Get("/user/limits", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, &tl)
	if err != nil {
		t.Fatal(err)
	}
	if tl.TierName != database.UserLimits[database.TierAnonymous].TierName {
		t.Fatalf("Expected to get the results for %s, got %s", database.UserLimits[database.TierAnonymous].TierName, tl.TierName)
	}
}

// testUserUploadsDELETE tests the DELETE /user/uploads/:skylink endpoint.
func testUserUploadsDELETE(t *testing.T, at *test.AccountsTester) {
	u, c, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Create an upload.
	skylink, _, err := test.CreateTestUpload(at.Ctx, at.DB, u.User, 128%skynet.KiB)
	// Make sure it shows up for this user.
	_, b, err := at.Get("/user/uploads", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ups database.UploadsResponse
	err = json.Unmarshal(b, &ups)
	if err != nil {
		t.Fatal(err)
	}
	// We expect to have a single upload, and we expect it to be of this skylink.
	if len(ups.Items) != 1 || ups.Items[0].Skylink != skylink.Skylink {
		t.Fatalf("Expected to have a single upload of %s, got %+v", skylink.Skylink, ups)
	}
	// Try to delete the upload without passing a JWT cookie.
	at.Cookie = nil
	_, b, err = at.Delete("/user/uploads/"+skylink.Skylink, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error %s, got %s. Body: %s", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Delete it.
	_, b, err = at.Delete("/user/uploads/"+skylink.Skylink, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Make sure it's gone.
	_, b, err = at.Get("/user/uploads", nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	err = json.Unmarshal(b, &ups)
	if err != nil {
		t.Fatal(err)
	}
	// We expect to have no uploads.
	if len(ups.Items) != 0 {
		t.Fatalf("Expected to have a no uploads, got %+v", ups)
	}
}

// testUserConfirmReconfirmEmailGET tests the GET /user/confirm  and
// POST /user/reconfirm endpoints. The overlap between the endpoints to great
// that it doesn't make sense to have separate tests.
func testUserConfirmReconfirmEmailGET(t *testing.T, at *test.AccountsTester) {
	u, c, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	defer func() { at.Cookie = nil }()

	// Confirm the user
	params := url.Values{}
	params.Add("token", u.EmailConfirmationToken)
	_, b, err := at.Get("/user/confirm", params)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Make sure the user's email address is confirmed now.
	u2, err := at.DB.UserByEmail(at.Ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if u2.EmailConfirmationToken != "" {
		t.Fatal("User's email is not confirmed.")
	}

	// Make sure `POST /user/reconfirm` requires a cookie.
	at.Cookie = nil
	_, b, err = at.Post("/user/reconfirm", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	// Reset the confirmation field, so we can continue testing with the same
	// user.
	at.Cookie = c
	_, b, err = at.Post("/user/reconfirm", nil, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Make sure the user's email address is unconfirmed now.
	u3, err := at.DB.UserByEmail(at.Ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if u3.EmailConfirmationToken == "" {
		t.Fatal("User is still confirmed.")
	}

	// Call the endpoint without a token.
	_, b, err = at.Get("/user/confirm", nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Call the endpoint with a bad token.
	params = url.Values{}
	params.Add("token", "this is not a valid token")
	_, b, err = at.Get("/user/confirm", params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Call the endpoint with an expired token.
	u.EmailConfirmationTokenExpiration = time.Now().Add(-time.Hour).UTC()
	err = at.DB.UserSave(at.Ctx, u.User)
	if err != nil {
		t.Fatal(err)
	}
	params = url.Values{}
	params.Add("token", u.EmailConfirmationToken)
	_, b, err = at.Get("/user/confirm", params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
}

// testUserAccountRecovery tests the account recovery process.
func testUserAccountRecovery(t *testing.T, at *test.AccountsTester) {
	u, _, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	defer func() { at.Cookie = nil }()

	// // TEST REQUESTING RECOVERY // //

	// Request recovery without supplying an email.
	_, b, err := at.Post("/user/recover/request", nil, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Request recovery with an unknown email address. We don't want to leak
	// that this email is not used by any account, so we expect to receive an OK
	// 200. We also expect an email to be sent to the address, informing them
	// that someone attempted to recover an account using their email address.
	// We do this because it's possible that the owner of the address is the
	// person requesting a recovery and they just forgot which email they used
	// to sign up. While we can't tell them that, we can indicate tht recovery
	// process works as expected and they should try their other emails.
	attemptedEmail := hex.EncodeToString(fastrand.Bytes(16)) + "@siasky.net"
	params := url.Values{}
	params.Add("email", attemptedEmail)
	_, b, err = at.Post("/user/recover/request", nil, params)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Check for the email we expect.
	filter := bson.M{"to": attemptedEmail}
	_, msgs, err := at.DB.FindEmails(at.Ctx, filter, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "Account access attempted" {
		t.Fatalf("Expected to find a single email with subject '%s', got %v", "Account access attempted", msgs)
	}
	// Request recovery with a valid but unconfirmed email.
	params = url.Values{}
	params.Add("email", u.Email)
	_, b, err = at.Post("/user/recover/request", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Confirm the email.
	params = url.Values{}
	params.Add("token", u.EmailConfirmationToken)
	_, b, err = at.Get("/user/confirm", params)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Request recovery with a valid email. We expect there to be a single email
	// with the recovery token.
	params = url.Values{}
	params.Add("email", u.Email)
	_, b, err = at.Post("/user/recover/request", nil, params)
	if err != nil {
		t.Fatal(err, string(b))
	}
	filter = bson.M{
		"to":      u.Email,
		"subject": "Recover access to your account",
	}
	_, msgs, err = at.DB.FindEmails(at.Ctx, filter, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected to find a single email with subject '%s', got %v", "Recover access to your account", len(msgs))
	}
	// Scan the message body for the recovery link.
	linkPattern := regexp.MustCompile("<a\\shref=\"(?P<recEndpoint>.*?)\\?token=(?P<token>.*?)\">")
	match := linkPattern.FindStringSubmatch(msgs[0].Body)
	if len(match) != 3 {
		t.Fatalf("Expected to get %d matches, got %d", 3, len(match))
	}
	result := make(map[string]string)
	for i, name := range linkPattern.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	expectedEndpoint := email.PortalAddressAccounts + "/user/recover"
	if re, exists := result["recEndpoint"]; !exists || !strings.Contains(re, expectedEndpoint) {
		t.Fatalf("Expected to find a link to '%s', got '%s'", expectedEndpoint, re)
	}
	token, exists := result["token"]
	if !exists {
		t.Fatal("Expected to find a token but didn't.")
	}

	// // TEST EXECUTING RECOVERY // //

	newPassword := hex.EncodeToString(fastrand.Bytes(16))
	// params := map[string]string{
	// 	"token":           token,
	// 	"password":        newPassword,
	// 	"confirmPassword": newPassword,
	// }
	// Try without a token:
	params = url.Values{}
	params.Add("password", newPassword)
	params.Add("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try without a password.
	params = url.Values{}
	params.Add("token", token)
	params.Add("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try without a confirmation.
	params = url.Values{}
	params.Add("token", token)
	params.Add("password", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try with mismatched password and confirmation.
	params = url.Values{}
	params.Add("token", token)
	params.Add("password", newPassword)
	params.Add("confirmPassword", "not the same as the password")
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try with an invalid token.
	params = url.Values{}
	params.Add("token", hex.EncodeToString(fastrand.Bytes(32)))
	params.Add("password", newPassword)
	params.Add("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try to use the token we got to recover the account.
	params = url.Values{}
	params.Add("token", token)
	params.Add("password", newPassword)
	params.Add("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err != nil {
		t.Log(token)
		t.Fatal(err, string(b))
	}
	// Make sure the user's password is now successfully changed.
	params = url.Values{}
	params.Add("email", u.Email)
	params.Add("password", newPassword)
	_, b, err = at.Post("/login", nil, params)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Make sure the reset token is removed from the user.
	u2, err := at.DB.UserByEmail(at.Ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if u2.RecoveryToken != "" {
		t.Fatalf("Expected recovery token to be empty, got '%s'", u2.RecoveryToken)
	}
	// Make extra sure we cannot sue the token again. This is only to make sure
	// we didn't cache it anywhere or allow it to somehow linger somewhere.
	params = url.Values{}
	params.Add("token", token)
	params.Add("password", newPassword)
	params.Add("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
}

// testTrackingAndStats tests all the tracking endpoints and verifies that the
// /user/stats endpoint returns what we expect.
func testTrackingAndStats(t *testing.T, at *test.AccountsTester) {
	u, c, err := test.CreateUserAndLogin(at, t.Name())
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Generate a random skylink.
	skylink, err := skymodules.NewSkylinkV1(crypto.HashBytes(fastrand.Bytes(32)), 0, 32)
	if err != nil {
		t.Fatal(err)
	}
	expectedStats := database.UserStats{}

	// Call trackUpload without a cookie.
	at.Cookie = nil
	_, b, err := at.Post("/track/upload/"+skylink.String(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Call trackUpload with an invalid skylink.
	_, b, err = at.Post("/track/upload/INVALID_SKYLINK", nil, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Call trackUpload with a valid skylink.
	_, b, err = at.Post("/track/upload/"+skylink.String(), nil, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Adjust the expectations. We won't adjust anything based on size because
	// the metafetcher won't be running during testing.
	expectedStats.NumUploads++
	expectedStats.BandwidthUploads += skynet.BandwidthUploadCost(0)
	expectedStats.RawStorageUsed += skynet.RawStorageUsed(0)

	// Call trackDownload without a cookie.
	at.Cookie = nil
	params := url.Values{}
	params.Add("bytes", "100")
	_, b, err = at.Post("/track/download/"+skylink.String(), params, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'. Body: '%s", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Call trackDownload with an invalid skylink.
	_, b, err = at.Post("/track/download/INVALID_SKYLINK", params, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Call trackDownload with a valid skylink and a negative size download
	params = url.Values{}
	params.Add("bytes", "-100")
	_, b, err = at.Post("/track/download/"+skylink.String(), params, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Call trackDownload with a valid skylink.
	params = url.Values{}
	params.Add("bytes", "100")
	_, b, err = at.Post("/track/download/"+skylink.String(), params, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Adjust the expectations.
	expectedStats.NumDownloads++
	expectedStats.BandwidthDownloads += skynet.BandwidthDownloadCost(100)
	expectedStats.TotalDownloadsSize += 100

	// Call trackRegistryRead without a cookie.
	at.Cookie = nil
	_, b, err = at.Post("/track/registry/read", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Call trackRegistryRead.
	_, b, err = at.Post("/track/registry/read", nil, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Adjust the expectations.
	expectedStats.NumRegReads++
	expectedStats.BandwidthRegReads += skynet.CostBandwidthRegistryRead

	// Call trackRegistryWrite without a cookie.
	at.Cookie = nil
	_, b, err = at.Post("/track/registry/write", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Call trackRegistryWrite.
	_, b, err = at.Post("/track/registry/write", nil, nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Adjust the expectations.
	expectedStats.NumRegWrites++
	expectedStats.BandwidthRegWrites += skynet.CostBandwidthRegistryWrite

	// Call userStats without a cookie.
	at.Cookie = nil
	_, b, err = at.Get("/user/stats", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	at.Cookie = c
	// Call userStats.
	_, b, err = at.Get("/user/stats", nil)
	if err != nil {
		t.Fatal(err, string(b))
	}
	var serverStats database.UserStats
	err = json.Unmarshal(b, &serverStats)
	if err != nil {
		t.Fatalf("Failed to unmarshall user stats: %s", err.Error())
	}
	if !reflect.DeepEqual(serverStats, expectedStats) {
		t.Fatalf("Expected\n%+v\ngot\n%+v", expectedStats, serverStats)
	}
}

// testUserFlow tests the happy path of a user's everyday life: create, login,
// edit, logout. It focuses on the happy path and leaves the edge cases to the
// per-handler tests.
func testUserFlow(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := test.DBNameForTest(t.Name())
	params := url.Values{}
	params.Add("email", name+"@siasky.net")
	params.Add("password", hex.EncodeToString(fastrand.Bytes(16)))
	// Create a user.
	u, err := test.CreateUser(at, params.Get("email"), params.Get("password"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	// Log in with that user in order to make sure it exists.
	r, _, err := at.Post("/login", nil, params)
	if err != nil {
		t.Fatal("Login failed. Error ", err.Error())
	}
	// Grab the Skynet cookie, so we can make authenticated calls.
	at.Cookie = test.ExtractCookie(r)
	defer func() { at.Cookie = nil }()
	if at.Cookie == nil {
		t.Fatalf("Failed to extract cookie from request. Cookies found: %+v", r.Cookies())
	}
	// Change the user's email.
	newEmail := name + "_new@siasky.net"
	r, b, err := at.UserPUT(newEmail, "", "")
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
	_, b, err = at.Get("/user", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected to get %s, got %s. Body: %s", unauthorized, err, string(b))
	}
}
