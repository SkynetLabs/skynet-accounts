package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.sia.tech/siad/build"
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
		{name: "Health", test: testHandlerHealthGET},
		{name: "UserCreate", test: testHandlerUserPOST},
		{name: "LoginLogout", test: testHandlerLoginPOST},
		{name: "UserEdit", test: testUserPUT},
		{name: "UserAddPubKey", test: testUserAddPubKey},
		{name: "UserDelete", test: testUserDELETE},
		{name: "UserLimits", test: testUserLimits},
		{name: "UserDeleteUploads", test: testUserUploadsDELETE},
		{name: "UserConfirmReconfirmEmail", test: testUserConfirmReconfirmEmailGET},
		{name: "UserAccountRecovery", test: testUserAccountRecovery},
		{name: "StandardTrackingFlow", test: testTrackingAndStats},
		{name: "StandardUserFlow", test: testUserFlow},
		{name: "Challenge-Response/Registration", test: testRegistration},
		{name: "Challenge-Response/Login", test: testLogin},
		{name: "APIKeysFlow", test: testAPIKeysFlow},
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
	bodyParams := url.Values{}
	bodyParams.Set("password", password)
	_, _, err := at.Post("/user", nil, bodyParams)
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
	bodyParams = url.Values{}
	bodyParams.Set("email", emailAddr)
	_, b, err = at.Post("/user", nil, bodyParams)
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
	bodyParams = url.Values{}
	bodyParams.Set("email", emailAddr)
	bodyParams.Set("password", password)
	_, b, err = at.Post("/login", nil, bodyParams)
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
	bodyParams := url.Values{}
	bodyParams.Set("email", emailAddr)
	bodyParams.Set("password", password)
	// Try logging in with a non-existent user.
	_, _, err := at.Post("/login", nil, bodyParams)
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
	r, _, err := at.Post("/login", nil, bodyParams)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the response contains a login cookie.
	c := test.ExtractCookie(r)
	if c == nil {
		t.Fatal("Expected a cookie.")
	}
	// Make sure the returned cookie is usable for making requests.
	at.SetCookie(c)
	defer at.ClearCredentials()
	// Make sure the response contains a valid JWT.
	_, err = jwt.ValidateToken(r.Header.Get("Skynet-Token"))
	if err != nil {
		t.Fatal("Missing or invalid token. Error:", err)
	}
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
	at.SetCookie(test.ExtractCookie(r))
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
	bodyParams = url.Values{}
	bodyParams.Set("email", emailAddr)
	bodyParams.Set("password", "bad password")
	_, _, err = at.Post("/login", nil, bodyParams)
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

	at.SetCookie(c)
	defer at.ClearCredentials()

	// Call unauthorized.
	at.ClearCredentials()
	_, _, err = at.Put("/user", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%s'", unauthorized, err)
	}
	at.SetCookie(c)
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
	params.Set("email", u.Email)
	params.Set("password", pw)
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
	at.SetCookie(c)
	defer at.ClearCredentials()
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
	at.ClearCredentials()
	r, _, _ = at.Delete("/user", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d, got %d", http.StatusUnauthorized, r.StatusCode)
	}
	// Delete the user.
	at.SetCookie(c)
	defer at.ClearCredentials()
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
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected %d, got %d.", http.StatusUnauthorized, r.StatusCode)
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
	at.SetCookie(c)
	defer at.ClearCredentials()

	// Call /user/limits with a cookie. Expect FreeTier response.
	tl, _, err := at.UserLimits()
	if err != nil {
		t.Fatal(err)
	}
	if tl.TierID != database.TierFree {
		t.Fatalf("Expected to get the results for tier id %d, got %d", database.TierFree, tl.TierID)
	}
	if tl.TierName != database.UserLimits[database.TierFree].TierName {
		t.Fatalf("Expected tier name '%s', got '%s'", database.UserLimits[database.TierFree].TierName, tl.TierName)
	}
	if tl.DownloadBandwidth != database.UserLimits[database.TierFree].DownloadBandwidth {
		t.Fatalf("Expected download bandwidth '%d', got '%d'", database.UserLimits[database.TierFree].DownloadBandwidth, tl.DownloadBandwidth)
	}

	// Call /user/limits without a cookie. Expect FreeAnonymous response.
	at.ClearCredentials()
	tl, _, err = at.UserLimits()
	if err != nil {
		t.Fatal(err)
	}
	if tl.TierID != database.TierAnonymous {
		t.Fatalf("Expected to get the results for tier id %d, got %d", database.TierAnonymous, tl.TierID)
	}
	if tl.TierName != database.UserLimits[database.TierAnonymous].TierName {
		t.Fatalf("Expected tier name '%s', got '%s'", database.UserLimits[database.TierAnonymous].TierName, tl.TierName)
	}
	if tl.DownloadBandwidth != database.UserLimits[database.TierAnonymous].DownloadBandwidth {
		t.Fatalf("Expected download bandwidth '%d', got '%d'", database.UserLimits[database.TierAnonymous].DownloadBandwidth, tl.DownloadBandwidth)
	}

	// Create a new user which we'll use to test the quota limits. We can't use
	// the existing one because their status is already cached.
	u2, c, err := test.CreateUserAndLogin(at, t.Name()+"2")
	if err != nil {
		t.Fatal("Failed to create a user and log in:", err)
	}
	defer func() {
		if err = u2.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()
	at.SetCookie(c)
	defer at.ClearCredentials()
	// Upload a very large file, which exceeds the user's storage limit. This
	// should cause their QuotaExceed flag to go up and their speeds to drop to
	// anonymous levels. Their tier should remain Free.
	dbu2 := *u2.User
	filesize := database.UserLimits[database.TierFree].Storage + 1
	sl, _, err := test.CreateTestUpload(at.Ctx, at.DB, &dbu2, filesize)
	if err != nil {
		t.Fatal(err)
	}
	// Make a specific call to trackUploadPOST in order to trigger the
	// checkUserQuotas method. This wil register the upload a second time but
	// that doesn't affect the test.
	_, err = at.TrackUpload(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	// We need to try this several times because we'll only get the right result
	// after the background goroutine that updates user's quotas has had time to
	// run.
	err = build.Retry(10, 200*time.Millisecond, func() error {
		// Check the user's limits. We expect the tier to be Free but the limits to
		// match Anonymous.
		tl, _, err = at.UserLimits()
		if err != nil {
			return errors.AddContext(err, "failed to call /user/limits")
		}
		if tl.TierID != database.TierFree {
			return fmt.Errorf("Expected to get the results for tier id %d, got %d", database.TierFree, tl.TierID)
		}
		if tl.TierName != database.UserLimits[database.TierFree].TierName {
			return fmt.Errorf("Expected tier name '%s', got '%s'", database.UserLimits[database.TierFree].TierName, tl.TierName)
		}
		if tl.DownloadBandwidth != database.UserLimits[database.TierAnonymous].DownloadBandwidth {
			return fmt.Errorf("Expected download bandwidth '%d', got '%d'", database.UserLimits[database.TierAnonymous].DownloadBandwidth, tl.DownloadBandwidth)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

	at.SetCookie(c)
	defer at.ClearCredentials()

	// Create an upload.
	skylink, _, err := test.CreateTestUpload(at.Ctx, at.DB, u.User, 128%skynet.KiB)
	// Make sure it shows up for this user.
	_, b, err := at.Get("/user/uploads", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ups api.UploadsGET
	err = json.Unmarshal(b, &ups)
	if err != nil {
		t.Fatal(err)
	}
	// We expect to have a single upload, and we expect it to be of this skylink.
	if len(ups.Items) != 1 || ups.Items[0].Skylink != skylink.Skylink {
		t.Fatalf("Expected to have a single upload of %s, got %+v", skylink.Skylink, ups)
	}
	// Try to delete the upload without passing a JWT cookie.
	at.ClearCredentials()
	_, b, err = at.Delete("/user/uploads/"+skylink.Skylink, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error %s, got %s. Body: %s", unauthorized, err, string(b))
	}
	at.SetCookie(c)
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

	defer at.ClearCredentials()

	// Confirm the user
	params := url.Values{}
	params.Set("token", u.EmailConfirmationToken)
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
	at.ClearCredentials()
	_, b, err = at.Post("/user/reconfirm", nil, nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", unauthorized, err, string(b))
	}
	// Reset the confirmation field, so we can continue testing with the same
	// user.
	at.SetCookie(c)
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
	params.Set("token", "this is not a valid token")
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
	params.Set("token", u.EmailConfirmationToken)
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

	defer at.ClearCredentials()

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
	params.Set("email", attemptedEmail)
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
	params.Set("email", u.Email)
	_, b, err = at.Post("/user/recover/request", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Confirm the email.
	queryParams := url.Values{}
	queryParams.Set("token", u.EmailConfirmationToken)
	_, b, err = at.Get("/user/confirm", queryParams)
	if err != nil {
		t.Fatal(err, string(b))
	}
	// Request recovery with a valid email. We expect there to be a single email
	// with the recovery token.
	bodyParams := url.Values{}
	bodyParams.Set("email", u.Email)
	_, b, err = at.Post("/user/recover/request", nil, bodyParams)
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
	// Try without a token:
	params = url.Values{}
	params.Set("password", newPassword)
	params.Set("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try without a password.
	params = url.Values{}
	params.Set("token", token)
	params.Set("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try without a confirmation.
	params = url.Values{}
	params.Set("token", token)
	params.Set("password", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try with mismatched password and confirmation.
	params = url.Values{}
	params.Set("token", token)
	params.Set("password", newPassword)
	params.Set("confirmPassword", "not the same as the password")
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try with an invalid token.
	params = url.Values{}
	params.Set("token", hex.EncodeToString(fastrand.Bytes(32)))
	params.Set("password", newPassword)
	params.Set("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%s'. Body: '%s'", badRequest, err, string(b))
	}
	// Try to use the token we got to recover the account.
	params = url.Values{}
	params.Set("token", token)
	params.Set("password", newPassword)
	params.Set("confirmPassword", newPassword)
	_, b, err = at.Post("/user/recover", nil, params)
	if err != nil {
		t.Log(token)
		t.Fatal(err, string(b))
	}
	// Make sure the user's password is now successfully changed.
	params = url.Values{}
	params.Set("email", u.Email)
	params.Set("password", newPassword)
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
	params.Set("token", token)
	params.Set("password", newPassword)
	params.Set("confirmPassword", newPassword)
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

	at.SetCookie(c)
	defer at.ClearCredentials()

	// Generate a random skylink.
	skylink, err := skymodules.NewSkylinkV1(crypto.HashBytes(fastrand.Bytes(32)), 0, 32)
	if err != nil {
		t.Fatal(err)
	}
	expectedStats := database.UserStats{}

	// Call trackUpload without a cookie.
	at.ClearCredentials()
	_, err = at.TrackUpload(skylink.String())
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%v'", unauthorized, err)
	}
	at.SetCookie(c)
	// Call trackUpload with an invalid skylink.
	_, err = at.TrackUpload("INVALID_SKYLINK")
	if err == nil || !strings.Contains(err.Error(), badRequest) {
		t.Fatalf("Expected '%s', got '%v'", badRequest, err)
	}
	// Call trackUpload with a valid skylink.
	_, err = at.TrackUpload(skylink.String())
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations. We won't adjust anything based on size because
	// the metafetcher won't be running during testing.
	expectedStats.NumUploads++
	expectedStats.BandwidthUploads += skynet.BandwidthUploadCost(0)
	expectedStats.RawStorageUsed += skynet.RawStorageUsed(0)

	// Call trackRegistryRead without a cookie.
	at.ClearCredentials()
	_, err = at.TrackRegistryRead()
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%v'", unauthorized, err)
	}
	at.SetCookie(c)
	// Call trackRegistryRead.
	_, err = at.TrackRegistryRead()
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations.
	expectedStats.NumRegReads++
	expectedStats.BandwidthRegReads += skynet.CostBandwidthRegistryRead

	// Call trackRegistryWrite without a cookie.
	at.ClearCredentials()
	_, err = at.TrackRegistryWrite()
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%v'", unauthorized, err)
	}
	at.SetCookie(c)
	// Call trackRegistryWrite.
	_, err = at.TrackRegistryWrite()
	if err != nil {
		t.Fatal(err)
	}
	// Adjust the expectations.
	expectedStats.NumRegWrites++
	expectedStats.BandwidthRegWrites += skynet.CostBandwidthRegistryWrite

	// Call userStats without a cookie.
	at.ClearCredentials()
	_, b, err := at.Get("/user/stats", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected error '%s', got '%v'", unauthorized, err)
	}
	at.SetCookie(c)
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
	emailAddr := name + "@siasky.net"
	password := hex.EncodeToString(fastrand.Bytes(16))
	queryParams := url.Values{}
	queryParams.Set("email", emailAddr)
	queryParams.Set("password", password)
	// Create a user.
	u, err := test.CreateUser(at, queryParams.Get("email"), queryParams.Get("password"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	// Log in with that user in order to make sure it exists.
	bodyParams := url.Values{}
	bodyParams.Set("email", emailAddr)
	bodyParams.Set("password", password)
	r, _, err := at.Post("/login", nil, bodyParams)
	if err != nil {
		t.Fatal("Login failed. Error ", err.Error())
	}
	// Grab the Skynet cookie, so we can make authenticated calls.
	at.SetCookie(test.ExtractCookie(r))
	defer at.ClearCredentials()
	if at.Cookie == nil {
		t.Fatalf("Failed to extract cookie from request. Cookies found: %+v", r.Cookies())
	}
	// Make sure the response contains a valid JWT.
	tk := r.Header.Get("Skynet-Token")
	if _, err = jwt.ValidateToken(tk); err != nil {
		t.Fatal("Missing or invalid token. Error:", err)
	}
	// Make sure we can make calls with this token.
	c := at.Cookie
	at.SetToken(tk)
	_, _, err = at.Get("/user", nil)
	if err != nil {
		t.Fatal("Failed to fetch user data with token:", err.Error())
	}
	at.SetCookie(c)
	// Change the user's email.
	newEmail := name + "_new@siasky.net"
	r, b, err := at.UserPUT(newEmail, "", "")
	if err != nil {
		t.Fatalf("Failed to update user. Error: %s. Body: %s", err.Error(), string(b))
	}
	// Grab the new cookie. It has changed because of the user edit.
	at.SetCookie(test.ExtractCookie(r))
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
	at.SetCookie(test.ExtractCookie(r))
	// Try to get the user, expect a 401.
	_, b, err = at.Get("/user", nil)
	if err == nil || !strings.Contains(err.Error(), unauthorized) {
		t.Fatalf("Expected to get %s, got %s. Body: %s", unauthorized, err, string(b))
	}
}
