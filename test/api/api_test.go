package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/SkynetLabs/skynet-accounts/test/dependencies"
	"github.com/SkynetLabs/skynet-accounts/types"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/build"

	"gitlab.com/NebulousLabs/errors"
)

// TestUserTierCache ensures out tier cache works as expected.
func TestUserTierCache(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dbName := test.DBNameForTest(t.Name())
	at, err := test.NewAccountsTester(dbName, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	emailAddr := types.NewEmail(test.DBNameForTest(t.Name()) + "@siasky.net")
	password := hex.EncodeToString(fastrand.Bytes(16))
	u, err := test.CreateUser(at, emailAddr, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = u.Delete(at.Ctx); err != nil {
			t.Error(errors.AddContext(err, "failed to delete user in defer"))
		}
	}()

	// Promote the user to Pro.
	u.Tier = database.TierPremium20
	err = at.DB.UserSave(at.Ctx, u.User)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := at.LoginCredentialsPOST(emailAddr.String(), password)
	if err != nil {
		t.Fatal(err)
	}
	at.SetCookie(test.ExtractCookie(r))
	// Get the user's limit.
	ul, _, err := at.UserLimits("byte", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ul.Sub != u.Sub {
		t.Fatalf("Expected user sub '%s', got '%s'", u.Sub, ul.Sub)
	}
	if ul.TierName != database.UserLimits[database.TierPremium20].TierName {
		t.Fatalf("Expected tier name '%s', got '%s'", database.UserLimits[database.TierPremium20].TierName, ul.TierName)
	}
	if ul.TierID != database.TierPremium20 {
		t.Fatalf("Expected tier id '%d', got '%d'", database.TierPremium20, ul.TierID)
	}
	if ul.TierName != database.UserLimits[database.TierPremium20].TierName {
		t.Fatalf("Expected tier name '%s', got '%s'", database.UserLimits[database.TierPremium20].TierName, ul.TierName)
	}
	if ul.UploadBandwidth != database.UserLimits[database.TierPremium20].UploadBandwidth {
		t.Fatalf("Expected upload bandwidth '%d', got '%d'", database.UserLimits[database.TierPremium20].UploadBandwidth, ul.UploadBandwidth)
	}
	// Register a test upload that exceeds the user's allowed storage, so their
	// QuotaExceeded flag will get raised.
	sl, _, err := test.CreateTestUpload(at.Ctx, at.DB, *u.User, database.UserLimits[u.Tier].Storage+1)
	if err != nil {
		t.Fatal(err)
	}
	// Make a specific call to trackUploadPOST in order to trigger the
	// checkUserQuotas method. This wil register the upload a second time but
	// that doesn't affect the test.
	_, err = at.TrackUpload(sl.Skylink, "")
	if err != nil {
		t.Fatal(err)
	}
	// We need to try this several times because we'll only get the right result
	// after the background goroutine that updates user's quotas has had time to
	// run.
	err = build.Retry(10, 200*time.Millisecond, func() error {
		// We expect to get tier with name and id matching TierPremium20 but with
		// speeds matching TierAnonymous.
		ul, _, err = at.UserLimits("byte", nil)
		if err != nil {
			t.Fatal(err)
		}
		if ul.TierID != database.TierPremium20 {
			return fmt.Errorf("expected tier id '%d', got '%d'", database.TierPremium20, ul.TierID)
		}
		if ul.TierName != database.UserLimits[database.TierPremium20].TierName {
			return fmt.Errorf("expected tier name '%s', got '%s'", database.UserLimits[database.TierPremium20].TierName, ul.TierName)
		}
		if ul.UploadBandwidth != database.UserLimits[database.TierAnonymous].UploadBandwidth {
			return fmt.Errorf("expected upload bandwidth '%d', got '%d'", database.UserLimits[database.TierAnonymous].UploadBandwidth, ul.UploadBandwidth)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Delete the uploaded file, so the user's quota recovers.
	// This call should invalidate the tier cache.
	_, err = at.UploadsDELETE(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(10, 200*time.Millisecond, func() error {
		// We expect to get TierPremium20.
		ul, _, err = at.UserLimits("byte", nil)
		if err != nil {
			return errors.AddContext(err, "failed to call /user/limits")
		}
		if ul.TierID != database.TierPremium20 {
			return fmt.Errorf("expected tier id '%d', got '%d'", database.TierPremium20, ul.TierID)
		}
		if ul.TierName != database.UserLimits[database.TierPremium20].TierName {
			return fmt.Errorf("expected tier name '%s', got '%s'", database.UserLimits[database.TierPremium20].TierName, ul.TierName)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWithDBSession is a test suite that covers WithDBSession.
func TestWithDBSession(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tests := map[string]func(t *testing.T){
		"general":                               testWithDBSessionGeneral,
		"retry WriteConflict with goroutines":   testWithDBSessionRetryOnWriteConflictGoroutines,
		"retry WriteConflict repeated failures": testWithDBSessionRetryOnWriteConflictRepeatedFailures,
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt(t)
		})
	}
}

// testWithDBSessionGeneral ensures that database transactions are started,
// committed, and aborted properly.
func testWithDBSessionGeneral(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}
	testAPI, err := api.New(db, nil, &logrus.Logger{}, nil, "")
	if err != nil {
		t.Fatal("Failed to instantiate API.", err)
	}

	emailSuccess := types.NewEmail(t.Name() + "success@siasky.net")
	emailSuccessJSON := types.NewEmail(t.Name() + "success_json@siasky.net")
	emailFailure := types.NewEmail(t.Name() + "failure@siasky.net")

	// This handler successfully creates a user in the DB and exits with
	// a success status code. We expect the user to exist in the DB after
	// the handler exits and the txn is committed.
	successHandler := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		sctx := r.Context()
		_, err = db.UserCreate(sctx, emailSuccess, "pass", "success sub", database.TierFree)
		if err != nil {
			t.Fatal("Failed to create user.", err)
		}
		// Make sure the user exists while we're still in the txn.
		u, err := db.UserByEmail(sctx, emailSuccess)
		if err != nil {
			t.Fatal("Failed to fetch user from DB.", err)
		}
		if u.Email != emailSuccess {
			t.Fatalf("Expected email '%v', got '%v'.", emailSuccess, u.Email)
		}
		testAPI.WriteSuccess(w)
	}

	// This handler successfully creates a user in the DB and exits with
	// a success status code and a JSON response. We expect the user to exist
	// in the DB after the handler exits and the txn is committed.
	successHandlerJSON := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		sctx := r.Context()
		_, err = db.UserCreate(sctx, emailSuccessJSON, "pass", "success json sub", database.TierFree)
		if err != nil {
			t.Fatal("Failed to create user.", err)
		}
		// Make sure the user exists while we're still in the txn.
		u, err := db.UserByEmail(sctx, emailSuccessJSON)
		if err != nil {
			t.Fatal("Failed to fetch user from DB.", err)
		}
		if u.Email != emailSuccessJSON {
			t.Fatalf("Expected email %s, got %s.", emailSuccessJSON, u.Email)
		}
		testAPI.WriteJSON(w, api.UserGETFromUser(u))
	}

	// This handler successfully creates a user in the DB but exits with
	// an error code. We expect the user to NOT exist in the DB after the
	// handler exits and the txn is aborted.
	failHandler := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		sctx := r.Context()
		_, err = db.UserCreate(sctx, emailFailure, "pass", "failure sub", database.TierFree)
		if err != nil {
			t.Fatal("Failed to create user.", err)
		}
		// Make sure the user exists while we're still in the txn.
		u, err := db.UserByEmail(sctx, emailFailure)
		if err != nil {
			t.Fatal("Failed to fetch user from DB.", err)
		}
		if u.Email != emailFailure {
			t.Fatalf("Expected email %s, got %s.", emailFailure, u.Email)
		}

		// Something fails with the logic following the user creation, so the
		// handler exits with an error.
		testAPI.WriteError(w, errors.New("error"), http.StatusInternalServerError)
	}

	rw := &test.ResponseWriter{}
	var ps httprouter.Params
	req := (&http.Request{}).WithContext(ctx)
	// Call the success handler.
	testAPI.WithDBSession(successHandler)(rw, req, ps)
	// Make sure the success user exists after the handler has returned.
	u, err := db.UserByEmail(ctx, emailSuccess)
	if err != nil {
		t.Fatal("Failed to fetch user from DB.", err)
	}
	if u.Email != emailSuccess {
		t.Fatalf("Expected email %s, got %s.", emailSuccess, u.Email)
	}

	// Call the success JSON handler.
	testAPI.WithDBSession(successHandlerJSON)(rw, req, ps)
	// Make sure the success user exists after the handler has returned.
	u, err = db.UserByEmail(ctx, emailSuccessJSON)
	if err != nil {
		t.Fatal("Failed to fetch user from DB.", err)
	}
	if u.Email != emailSuccessJSON {
		t.Fatalf("Expected email %s, got %s.", emailSuccessJSON, u.Email)
	}

	// Call the failure handler.
	testAPI.WithDBSession(failHandler)(rw, req, ps)
	// Make sure the failure user does not exist after the handler has returned.
	u, err = db.UserByEmail(ctx, emailFailure)
	if err == nil {
		t.Fatal("Fetched a user that shouldn't have existed")
	}
}

// testWithDBSessionRetryOnWriteConflictGoroutines ensures that WithDBSession
// can properly retry requests on MongoDB WriteConflict error.
func testWithDBSessionRetryOnWriteConflictGoroutines(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dbName := test.DBNameForTest(t.Name())
	// This dependency adds a delay within the transaction that updates the
	// user, causing a WriteConflict error.
	dep := dependencies.NewDependencyUserPutMongoDelay()
	at, err := test.NewAccountsTester(dbName, "", dep)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	// Ensure WithDBSession works with requests without bodies.
	// This is a regression test. It panics with a nil pointer if we cannot
	// properly handle requests with nil bodies.
	testAPI, err := api.New(at.DB, nil, at.Logger, nil, "")
	if err != nil {
		t.Fatal("Failed to instantiate API.", err)
	}
	handler := func(_ http.ResponseWriter, _ *http.Request, _ httprouter.Params) {}
	req := (&http.Request{}).WithContext(context.Background())
	testAPI.WithDBSession(handler)(nil, req, nil)

	// Create a test user.
	userEmailStr := types.NewEmail(t.Name() + "@siasky.net").String()
	userPassword := t.Name() + "pass"
	_, b, err := at.UserPOST(userEmailStr, userPassword)
	if err != nil {
		t.Fatal(err, string(b))
	}
	defer func() {
		_, _ = at.UserDELETE()
	}()
	r, b, err := at.LoginCredentialsPOST(userEmailStr, userPassword)
	if err != nil {
		t.Fatal(err, string(b))
	}
	at.SetCookie(test.ExtractCookie(r))

	// Set up several goroutines that will update the user simultaneously.
	// We'll make them all block on a channel and then we'll close the channel,
	// so they all start at the same time. We want to keep the number of
	// conflicting goroutines low because we want the WriteConflict to resolve
	// within the given dxTxnRetryCount attempts.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := at.UserPUT(userEmailStr, "new password", "")
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// testWithDBSessionRetryOnWriteConflictRepeatedFailures ensures that
// WithDBSession will properly retry up to the given number of times and will
// fail after.
func testWithDBSessionRetryOnWriteConflictRepeatedFailures(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dbName := test.DBNameForTest(t.Name())
	// This dependency will cause api.DBTxnRetryCount + 1 failures. This means
	// that our first attempt to call the affected method will fail after
	// retrying api.DBTxnRetryCount times and the next one will succeed after
	// retrying once.
	dep := dependencies.NewDependencyMongoWriteConflictN(api.DBTxnRetryCount + 1)
	at, err := test.NewAccountsTester(dbName, "", dep)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	// Create a test user.
	userEmailStr := types.NewEmail(t.Name() + "@siasky.net").String()
	userPassword := t.Name() + "pass"
	_, b, err := at.UserPOST(userEmailStr, userPassword)
	if err != nil {
		t.Fatal(err, string(b))
	}
	defer func() {
		_, _ = at.UserDELETE()
	}()
	r, b, err := at.LoginCredentialsPOST(userEmailStr, userPassword)
	if err != nil {
		t.Fatal(err, string(b))
	}
	at.SetCookie(test.ExtractCookie(r))

	// Try to update the user. Expect this to fail due to the dependency.
	newpass := "new password"
	_, _, err = at.UserPUT(userEmailStr, newpass, "")
	if err == nil || !strings.Contains(err.Error(), dependencies.DependencyMongoWriteConflictNMessage) {
		t.Fatalf("Expected a '%s' error, got '%s'", dependencies.DependencyMongoWriteConflictNMessage, err)
	}
	// Ensure the password was not updated.
	r, _, _ = at.LoginCredentialsPOST(userPassword, newpass)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected status '%d', got '%d'", http.StatusUnauthorized, r.StatusCode)
	}
	// Try to update the user again. Expect this to pass after one retry.
	_, _, err = at.UserPUT(userEmailStr, newpass, "")
	if err != nil {
		t.Fatal(err)
	}
	// Ensure the password was updated.
	r, _, err = at.LoginCredentialsPOST(userEmailStr, newpass)
	if err != nil || r.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected to log in successfully, got %d '%v'", r.StatusCode, err)
	}
}
