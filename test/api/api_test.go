package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

// TestResponseWriter is a testing ResponseWriter implementation.
type TestResponseWriter struct {
	Buffer bytes.Buffer
	Status int
}

// Header implementation.
func (w TestResponseWriter) Header() http.Header {
	return http.Header{}
}

// Write implementation.
func (w TestResponseWriter) Write(b []byte) (int, error) {
	return w.Buffer.Write(b)
}

// WriteHeader implementation.
func (w TestResponseWriter) WriteHeader(statusCode int) {
	w.Status = statusCode
}

// TestWithDBSession ensures that database transactions are started, committed,
// and aborted properly.
func TestWithDBSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	testAPI, err := api.New(db, nil, &logrus.Logger{}, nil)
	if err != nil {
		t.Fatal("Failed to instantiate API.", err)
	}

	emailSuccess := t.Name() + "success@siasky.net"
	emailSuccessJSON := t.Name() + "success_json@siasky.net"
	emailFailure := t.Name() + "failure@siasky.net"

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
			t.Fatalf("Expected email %s, got %s.", emailSuccess, u.Email)
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

	var rw TestResponseWriter
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

// TestUserTierCache ensures out tier cache works as expected.
func TestUserTierCache(t *testing.T) {
	t.Parallel()

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

	email := test.DBNameForTest(t.Name()) + "@siasky.net"
	password := hex.EncodeToString(fastrand.Bytes(16))
	u, err := test.CreateUser(at, email, password)
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
	params := url.Values{}
	params.Add("email", email)
	params.Add("password", password)
	r, _, err := at.Post("/login", nil, params)
	if err != nil {
		t.Fatal(err)
	}
	at.Cookie = test.ExtractCookie(r)
	// Get the user's limit. Since they are on a Pro account but their
	// SubscribedUntil is set in the past, we expect to get TierFree.
	_, b, err := at.Get("/user/limits", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ul database.TierLimits
	err = json.Unmarshal(b, &ul)
	if err != nil {
		t.Fatal(err)
	}
	if ul.TierName != database.UserLimits[database.TierFree].TierName {
		t.Fatalf("Expected tier '%s', got '%s'", database.UserLimits[database.TierFree].TierName, ul.TierName)
	}
	// Now set their SubscribedUntil in the future, so their subscription tier
	// is active.
	u.SubscribedUntil = time.Now().UTC().Add(365 * 24 * time.Hour)
	err = at.DB.UserSave(at.Ctx, u.User)
	if err != nil {
		t.Fatal(err)
	}
	// Register a test upload that exceeds the user's allowed storage, so their
	// QuotaExceeded flag will get raised.
	sl, _, err := test.CreateTestUpload(at.Ctx, at.DB, u.User, database.UserLimits[u.Tier].Storage+1)
	if err != nil {
		t.Fatal(err)
	}
	// Make a specific call to trackUploadPOST in order to trigger the
	// checkUserQuotas method. This wil register the upload a second time but
	// that doesn't affect the test.
	_, _, err = at.Post("/track/upload/"+sl.Skylink, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Sleep for a short time in order to make sure that the background
	// goroutine that updates user's quotas has had time to run.
	time.Sleep(2 * time.Second)
	// We expect to get TierAnonymous.
	_, b, err = at.Get("/user/limits", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, &ul)
	if err != nil {
		t.Fatal(err)
	}
	if ul.TierName != database.UserLimits[database.TierAnonymous].TierName {
		t.Fatalf("Expected tier '%s', got '%s'", database.UserLimits[database.TierAnonymous].TierName, ul.TierName)
	}
	// Delete the uploaded file, so the user's quota recovers.
	// This call should invalidate the tier cache.
	_, _, err = at.Delete("/user/uploads/"+sl.Skylink, nil)
	time.Sleep(2 * time.Second)
	// We expect to get TierPremium20.
	_, b, err = at.Get("/user/limits", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, &ul)
	if err != nil {
		t.Fatal(err)
	}
	if ul.TierName != database.UserLimits[database.TierPremium20].TierName {
		t.Fatalf("Expected tier '%s', got '%s'", database.UserLimits[database.TierPremium20].TierName, ul.TierName)
	}
}
