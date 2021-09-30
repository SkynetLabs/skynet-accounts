package api

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/api"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/test"

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
	db, err := database.New(ctx, test.DBTestCredentials(), logrus.New())
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
		u, err := db.UserByEmail(sctx, emailSuccess, false)
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
		u, err := db.UserByEmail(sctx, emailSuccessJSON, false)
		if err != nil {
			t.Fatal("Failed to fetch user from DB.", err)
		}
		if u.Email != emailSuccessJSON {
			t.Fatalf("Expected email %s, got %s.", emailSuccessJSON, u.Email)
		}
		testAPI.WriteJSON(w, u)
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
		u, err := db.UserByEmail(sctx, emailFailure, false)
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
	u, err := db.UserByEmail(ctx, emailSuccess, false)
	if err != nil {
		t.Fatal("Failed to fetch user from DB.", err)
	}
	if u.Email != emailSuccess {
		t.Fatalf("Expected email %s, got %s.", emailSuccess, u.Email)
	}

	// Call the success JSON handler.
	testAPI.WithDBSession(successHandlerJSON)(rw, req, ps)
	// Make sure the success user exists after the handler has returned.
	u, err = db.UserByEmail(ctx, emailSuccessJSON, false)
	if err != nil {
		t.Fatal("Failed to fetch user from DB.", err)
	}
	if u.Email != emailSuccessJSON {
		t.Fatalf("Expected email %s, got %s.", emailSuccessJSON, u.Email)
	}

	// Call the failure handler.
	testAPI.WithDBSession(failHandler)(rw, req, ps)
	// Make sure the failure user does not exist after the handler has returned.
	u, err = db.UserByEmail(ctx, emailFailure, false)
	if err == nil {
		t.Fatal("Fetched a user that shouldn't have existed")
	}
}
