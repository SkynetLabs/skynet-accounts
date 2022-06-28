package api

import (
	"context"
	"crypto/subtle"
	"io/ioutil"
	"math"
	"net/http"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MockSessionContext status values.
const (
	statusNotStarted = iota
	statusStarted
	statusCommitted
	statusAborted
)

type (
	// MockSessionContext is a mock of mongo.SessionContext
	MockSessionContext struct {
		context.Context
		txID   int
		status int
		mu     sync.Mutex
	}
)

// NewMockSessionContext returns a new MockSessionContext.
func NewMockSessionContext(ctx context.Context) *MockSessionContext {
	return &MockSessionContext{ctx, 0, statusNotStarted, sync.Mutex{}}
}

// StartTransaction starts a transaction if none exists already.
func (msc *MockSessionContext) StartTransaction(...*options.TransactionOptions) error {
	msc.mu.Lock()
	defer msc.mu.Unlock()
	if msc.txID != 0 {
		return errors.New("transaction already exists")
	}
	msc.txID = fastrand.Intn(math.MaxInt)
	msc.status = statusStarted
	return nil
}

// AbortTransaction aborts a transaction, if one exists.
func (msc *MockSessionContext) AbortTransaction(context.Context) error {
	msc.mu.Lock()
	defer msc.mu.Unlock()
	if msc.txID == 0 || msc.status == statusNotStarted {
		return errors.New("no transaction")
	}
	msc.status = statusAborted
	return nil
}

// CommitTransaction commits a transaction, if one exists.
func (msc *MockSessionContext) CommitTransaction(context.Context) error {
	msc.mu.Lock()
	defer msc.mu.Unlock()
	if msc.txID == 0 || msc.status == statusNotStarted {
		return errors.New("no transaction")
	}
	msc.status = statusCommitted
	return nil
}

// TestMongoWriter ensures that MongoWrites functions as expected.
func TestMongoWriter(t *testing.T) {
	ctx := context.Background()
	testLogger := logrus.Logger{}
	testLogger.SetOutput(ioutil.Discard)

	/* Happy path */

	testRW := &bufferResponseWriter{}
	testSC := NewMockSessionContext(ctx)
	// Create a new MongoWriter and open a new transaction with the given session.
	mw, err := NewMongoWriter(testRW, testSC, &testLogger)
	if err != nil {
		t.Fatal(err)
	}
	// Expect testSc to contain a started transaction.
	if testSC.txID == 0 {
		t.Fatal("No transaction started.")
	}
	if testSC.status != statusStarted {
		t.Fatalf("Expected txn status %d, got %d", statusStarted, testSC.status)
	}
	// Write a success code and some content.
	mw.WriteHeader(http.StatusOK)
	contentOK := "this is OK content"
	n, err := mw.Write([]byte(contentOK))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(contentOK) {
		t.Fatalf("Expected %d bytes to be written, got %d", len(contentOK), n)
	}
	// Expect to find the content in the original response writer, i.e. testRW.
	if subtle.ConstantTimeCompare(testRW.Buffer.Bytes(), []byte(contentOK)) != 1 {
		t.Fatalf("Expected buffer content to be '%s', got '%s'", contentOK, string(testRW.Buffer.Bytes()))
	}
	// Expect the transaction to be successfully committed.
	if testSC.status != statusCommitted {
		t.Fatalf("Expected status %d, got %d", statusStarted, testSC.status)
	}

	/* Error path */

	testRW = &bufferResponseWriter{}
	testSC = NewMockSessionContext(ctx)
	// Create a new MongoWriter and open a new transaction with the given session.
	// Expect testSc to contain a started transaction.
	mw, err = NewMongoWriter(testRW, testSC, &testLogger)
	if err != nil {
		t.Fatal(err)
	}
	if testSC.txID == 0 {
		t.Fatal("No transaction started.")
	}
	if testSC.status != statusStarted {
		t.Fatalf("Expected txn status %d, got %d", statusStarted, testSC.status)
	}
	// Write an error code and some content.
	mw.WriteHeader(http.StatusInternalServerError)
	contentErr := "this is error content"
	n, err = mw.Write([]byte(contentErr))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(contentErr) {
		t.Fatalf("Expected %d bytes to be written, got %d", len(contentErr), n)
	}
	// Expect the internal error status to be correct.
	if mw.ErrorStatus() != http.StatusInternalServerError {
		t.Fatalf("Expected error status to be %d, got %d", http.StatusInternalServerError, mw.ErrorStatus())
	}
	// Expect to find the content in the error buffer.
	if subtle.ConstantTimeCompare(mw.ErrorBuffer(), []byte(contentErr)) != 1 {
		t.Fatalf("Expected error buffer content to be '%s', got '%s'", string(mw.ErrorBuffer()), contentOK)
	}
	// Expect the transaction to be successfully aborted.
	if testSC.status != statusAborted {
		t.Fatalf("Expected status %d, got %d", statusAborted, testSC.status)
	}
	// Expect MongoWriter to properly signal that the error was NOT caused by a
	// WriteConflict.
	if mw.FailedWithWriteConflict() {
		t.Fatal("Expected no WriteConflict indication but got one.")
	}

	/* WriteConflict error path */

	testRW = &bufferResponseWriter{}
	testSC = NewMockSessionContext(ctx)
	// Create a new MongoWriter and open a new transaction with the given session.
	// Expect testSc to contain a started transaction.
	mw, err = NewMongoWriter(testRW, testSC, &testLogger)
	if err != nil {
		t.Fatal(err)
	}
	if testSC.txID == 0 {
		t.Fatal("No transaction started.")
	}
	if testSC.status != statusStarted {
		t.Fatalf("Expected txn status %d, got %d", statusStarted, testSC.status)
	}
	// Write an error code and some content.
	mw.WriteHeader(http.StatusInternalServerError)
	n, err = mw.Write([]byte(writeConflictErrMsg))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(writeConflictErrMsg) {
		t.Fatalf("Expected %d bytes to be written, got %d", len(writeConflictErrMsg), n)
	}
	// Expect the internal error status to be correct.
	if mw.ErrorStatus() != http.StatusInternalServerError {
		t.Fatalf("Expected error status to be %d, got %d", http.StatusInternalServerError, mw.ErrorStatus())
	}
	// Expect to find the content in the error buffer.
	if subtle.ConstantTimeCompare(mw.ErrorBuffer(), []byte(writeConflictErrMsg)) != 1 {
		t.Fatalf("Expected error buffer content to be '%s', got '%s'", string(mw.ErrorBuffer()), contentOK)
	}
	// Expect the transaction to be successfully aborted.
	if testSC.status != statusAborted {
		t.Fatalf("Expected status %d, got %d", statusAborted, testSC.status)
	}
	// Expect MongoWriter to properly signal that the error was caused by a
	// WriteConflict.
	if !mw.FailedWithWriteConflict() {
		t.Fatal("Expected a WriteConflict indication, didn't get one.")
	}
}
