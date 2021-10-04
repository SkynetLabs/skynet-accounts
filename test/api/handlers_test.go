package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/skynet"
	"github.com/NebulousLabs/skynet-accounts/test"
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
		{Name: "userCreate", Test: testHandlerUserPOST},
		// POST /login, POST /logout, POST /user
		{Name: "loginLogout", Test: testHandlerLoginPOST},
		// PUT /user
		{Name: "userEdit", Test: testUserPUT},
		// GET /user/limits
		{Name: "userLimits", Test: testUserLimits},
		// DELETE /user/uploads/:skylink, GET /user/uploads
		{Name: "userDeleteUploads", Test: testUserUploadsDELETE},
		// GET /user/confirm, POST /user/reconfirm
		{Name: "userConfirmReconfirmEmail", Test: testUserConfirmReconfirmEmailGET},
		// GET /user/recover, POST /user/recover, POST /login
		{Name: "userAccountRecovery", Test: testUserAccountRecovery},
		// POST /track/upload/:skylink, POST /track/download/:skylink, POST /track/registry/read, POST /track/registry/write, GET /user/stats
		{Name: "standardTrackingFlow", Test: testTrackingAndStats},
		// POST /user, POST /login, PUT /user, GET /user, POST /logout
		{Name: "standardUserFlow", Test: testUserFlow},
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
	name := strings.ReplaceAll(t.Name(), "/", "_")
	email := name + "@siasky.net"
	params := map[string]string{
		"email":    email,
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Try to create a user without passing params.
	_, _, err := at.Post("/user", nil, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'", badRequest, err)
	}
	// Try to create a user with an invalid email.
	_, _, err = at.Post("/user", nil, map[string]string{"email": "invalid", "password": "pass"})
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'", badRequest, err)
	}
	// Try to create a user with an empty password.
	_, _, err = at.Post("/user", nil, map[string]string{"email": email})
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'", badRequest, err)
	}
	// Create a user.
	_, _, err = at.Post("/user", nil, params)
	if err != nil {
		t.Fatal("User creation failed. Error ", err.Error())
	}
	// Make sure the user exists in the DB.
	u, err := at.DB.UserByEmail(at.Ctx, email, false)
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
	_, _, err = at.Post("/login", nil, params)
	if err != nil {
		t.Fatal("Login failed. Error ", err.Error())
	}
	// try to create a user with an already taken email
	_, _, err = at.Post("/user", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected user creation to fail with '%s', got '%s'", badRequest, err)
	}
}

// testHandlerLoginPOST tests the /login endpoint.
func testHandlerLoginPOST(t *testing.T, at *test.AccountsTester) {
	email := strings.ReplaceAll(t.Name(), "/", "_") + "@siasky.net"
	password := hex.EncodeToString(fastrand.Bytes(16))
	params := map[string]string{
		"email":    email,
		"password": password,
	}
	// Try logging in with a non-existent user.
	_, _, err := at.Post("/login", nil, params)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'", unauthorized, err)
	}
	u, cleanup, err := test.CreateUser(t, at, email, password)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(u)
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
	if err != nil || !strings.Contains(string(b), email) {
		t.Fatal("Expected to be able to fetch the user with this cookie.")
	}
	// Test /logout while we're here.
	r, _, err = at.Post("/logout", nil, nil)
	if err != nil {
		t.Fatal(err)
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
	badPassParams := params
	badPassParams["password"] = "bad password"
	_, _, err = at.Post("/login", nil, badPassParams)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'", unauthorized, err)
	}
}

// testUserPUT tests the PUT /user endpoint.
func testUserPUT(t *testing.T, at *test.AccountsTester) {
	name := strings.ReplaceAll(t.Name(), "/", "_")
	u, c, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
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
	postParams := map[string]string{
		"stripeCustomerId": name + "_stripe_id",
	}
	_, b, err := at.Put("/user", nil, postParams)
	if err != nil {
		t.Fatal(err)
	}
	var u2 database.User
	err = json.Unmarshal(b, &u2)
	if err != nil {
		t.Fatal(err)
	}
	if u2.StripeID != postParams["stripeCustomerId"] {
		t.Fatalf("Expected the user to have StripeID %s, got %s", postParams["stripeCustomerId"], u2.StripeID)
	}
	// Try to update the StripeID again. Expect this to fail.
	r, _, err := at.Put("/user", nil, postParams)
	if err == nil || !strings.Contains(err.Error(), "409 Conflict") || r.StatusCode != http.StatusConflict {
		t.Fatalf("Expected to get error '409 Conflict' and status 409, got '%s' and %d", err, r.StatusCode)
	}

	// Update the user's email.
	postParams = map[string]string{
		"email": name + "_new@siasky.net",
	}
	_, _, err = at.Put("/user", nil, postParams)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch the user from the DB because we want to be sure that their email
	// is marked as unconfirmed which is not reflected in the JSON
	// representation of the object.
	u3, err := at.DB.UserByEmail(at.Ctx, postParams["email"], false)
	if err != nil {
		t.Fatal(err)
	}
	if u3.Email != postParams["email"] {
		t.Fatalf("Expected the user to have email %s, got %s", postParams["email"], u3.Email)
	}
	if u3.EmailConfirmationToken == "" {
		t.Fatalf("Expected the user to have a non-empty confirmation token, got '%s'", u3.EmailConfirmationToken)
	}
	// Expect to find a confirmation email queued for sending.
	filer := bson.M{"to": postParams["email"]}
	_, msgs, err := at.DB.FindEmails(at.Ctx, filer, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "Please verify your email address" {
		t.Fatal("Expected to find a confirmation email but didn't.")
	}
}

// testUserLimits tests the /user/limits endpoint.
func testUserLimits(t *testing.T, at *test.AccountsTester) {
	u, c, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
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
	u, c, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
	at.Cookie = c
	defer func() { at.Cookie = nil }()

	// Create an upload.
	skylink, _, err := test.CreateTestUpload(at.Ctx, at.DB, u, 128%skynet.KiB)
	// Make sure it shows up for this user.
	_, b, err := at.Get("/user/uploads", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ups database.UploadsResponseDTO
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
	_, _, err = at.Delete("/user/uploads/"+skylink.Skylink, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error %s, got %s", unauthorized, err)
	}
	at.Cookie = c
	// Delete it.
	_, _, err = at.Delete("/user/uploads/"+skylink.Skylink, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure it's gone.
	_, b, err = at.Get("/user/uploads", nil)
	if err != nil {
		t.Fatal(err)
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
	u, c, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
	defer func() { at.Cookie = nil }()

	// Confirm the user
	_, _, err = at.Get("/user/confirm", map[string]string{"token": u.EmailConfirmationToken})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the user's email address is confirmed now.
	u2, err := at.DB.UserByEmail(at.Ctx, u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	if u2.EmailConfirmationToken != "" {
		t.Fatal("User is not confirmed.")
	}

	// Make sure `POST /user/reconfirm` requires a cookie.
	at.Cookie = nil
	_, _, err = at.Post("/user/reconfirm", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'", unauthorized, err)
	}
	// Reset the confirmation field, so we can continue testing with the same
	// user.
	at.Cookie = c
	_, _, err = at.Post("/user/reconfirm", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the user's email address is unconfirmed now.
	u3, err := at.DB.UserByEmail(at.Ctx, u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	if u3.EmailConfirmationToken == "" {
		t.Fatal("User is still confirmed.")
	}

	// Call the endpoint without a token.
	_, _, err = at.Get("/user/confirm", nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Call the endpoint with a bad token.
	_, _, err = at.Get("/user/confirm", map[string]string{"token": "this is not a valid token"})
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Call the endpoint with an expired token.
	u.EmailConfirmationTokenExpiration = time.Now().Add(-time.Hour).UTC()
	err = at.DB.UserSave(at.Ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = at.Get("/user/confirm", map[string]string{"token": u.EmailConfirmationToken})
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
}

// testUserAccountRecovery tests the account recovery process.
func testUserAccountRecovery(t *testing.T, at *test.AccountsTester) {
	u, _, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
	defer func() { at.Cookie = nil }()

	// // TEST REQUESTING RECOVERY // //

	// Request recovery without supplying an email.
	_, _, err = at.Get("/user/recover", nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
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
	_, b, err := at.Get("/user/recover", map[string]string{"email": attemptedEmail})
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
	_, _, err = at.Get("/user/recover", map[string]string{"email": u.Email})
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Confirm the email.
	_, b, err = at.Get("/user/confirm", map[string]string{"token": u.EmailConfirmationToken})
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Request recovery with a valid email. We expect there to be a single email
	// with the recovery token.
	_, b, err = at.Get("/user/recover", map[string]string{"email": u.Email})
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
	expectedEndpoint := email.PortalAddress + "/user/recover"
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
	params := map[string]string{
		"password":        newPassword,
		"confirmPassword": newPassword,
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Try without a password.
	params = map[string]string{
		"token":           token,
		"confirmPassword": newPassword,
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Try without a confirmation.
	params = map[string]string{
		"token":    token,
		"password": newPassword,
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Try with mismatched password and confirmation.
	params = map[string]string{
		"token":           token,
		"password":        newPassword,
		"confirmPassword": "not the same as the password",
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Try with an invalid token.
	params = map[string]string{
		"token":           hex.EncodeToString(fastrand.Bytes(32)),
		"password":        newPassword,
		"confirmPassword": newPassword,
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
	// Try to use the token we got to recover the account.
	params = map[string]string{
		"token":           token,
		"password":        newPassword,
		"confirmPassword": newPassword,
	}
	_, b, err = at.Post("/user/recover", nil, params)
	if err != nil {
		t.Log(token)
		t.Fatal(err, string(b))
	}
	// Make sure the user's password is now successfully changed.
	_, b, err = at.Post("/login", nil, map[string]string{"email": u.Email, "password": newPassword})
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Make sure the reset token is removed from the user.
	u2, err := at.DB.UserByEmail(at.Ctx, u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	if u2.RecoveryToken != "" {
		t.Fatalf("Expected recovery token to be empty, got '%s'", u2.RecoveryToken)
	}
	// Make extra sure we cannot sue the token again. This is only to make sure
	// we didn't cache it anywhere or allow it to somehow linger somewhere.
	params = map[string]string{
		"token":           token,
		"password":        newPassword,
		"confirmPassword": newPassword,
	}
	_, _, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'", badRequest, err)
	}
}

// testTrackingAndStats tests all the tracking endpoints and verifies that the
// /user/stats endpoint returns what we expect.
func testTrackingAndStats(t *testing.T, at *test.AccountsTester) {
	u, c, cleanup, err := test.CreateUserAndLogin(t, at)
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer cleanup(u)
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
	_, _, err = at.Post("/track/upload/"+skylink.String(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Call trackUpload with an invalid skylink.
	_, _, err = at.Post("/track/upload/INVALID_SKYLINK", nil, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected error '%s', got '%s'", badRequest, err)
	}
	// Call trackUpload with a valid skylink.
	_, _, err = at.Post("/track/upload/"+skylink.String(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations. We won't adjust anything based on size because
	// the metafetcher won't be running during testing.
	expectedStats.NumUploads++
	expectedStats.BandwidthUploads += skynet.BandwidthUploadCost(0)
	expectedStats.RawStorageUsed += skynet.RawStorageUsed(0)

	// Call trackDownload without a cookie.
	at.Cookie = nil
	downloadParams := map[string]string{"bytes": "100"}
	_, _, err = at.Post("/track/download/"+skylink.String(), downloadParams, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Call trackDownload with an invalid skylink.
	_, _, err = at.Post("/track/download/INVALID_SKYLINK", downloadParams, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected error '%s', got '%s'", badRequest, err)
	}
	// Call trackDownload with a valid skylink and a negative size download
	_, _, err = at.Post("/track/download/"+skylink.String(), map[string]string{"bytes": "-100"}, nil)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected error '%s', got '%s'", badRequest, err)
	}
	// Call trackDownload with a valid skylink.
	_, _, err = at.Post("/track/download/"+skylink.String(), downloadParams, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations.
	expectedStats.NumDownloads++
	expectedStats.BandwidthDownloads += skynet.BandwidthDownloadCost(100)
	expectedStats.TotalDownloadsSize += 100

	// Call trackRegistryRead without a cookie.
	at.Cookie = nil
	_, _, err = at.Post("/track/registry/read", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Call trackRegistryRead.
	_, _, err = at.Post("/track/registry/read", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations.
	expectedStats.NumRegReads++
	expectedStats.BandwidthRegReads += skynet.CostBandwidthRegistryRead

	// Call trackRegistryWrite without a cookie.
	at.Cookie = nil
	_, _, err = at.Post("/track/registry/write", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Call trackRegistryWrite.
	_, _, err = at.Post("/track/registry/write", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations.
	expectedStats.NumRegWrites++
	expectedStats.BandwidthRegWrites += skynet.CostBandwidthRegistryWrite

	// Call userStats without a cookie.
	at.Cookie = nil
	_, _, err = at.Get("/user/stats", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.Cookie = c
	// Call userStats.
	_, b, err := at.Get("/user/stats", nil)
	if err != nil {
		t.Fatal(err)
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
	name := strings.ReplaceAll(t.Name(), "/", "_")
	params := map[string]string{
		"email":    name + "@siasky.net",
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Create a user.
	u, cleanup, err := test.CreateUser(t, at, params["email"], params["password"])
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(u)
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
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected to get %s, got %s", unauthorized, err)
	}
}
