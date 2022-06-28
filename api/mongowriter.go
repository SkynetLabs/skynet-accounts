package api

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// writeConflictErrMsg is the error message MongoDB issues when a
	// transaction needs to be reverted because of a write conflict.
	writeConflictErrMsg = "(WriteConflict) WriteConflict error: this operation conflicted with another operation. Please retry your operation or multi-document transaction."
)

type (
	// MongoSessionContext defines the minimal session context interface that
	// we are using. This interface facilitates testing and should be expanded
	// whenever we need additional functionality from mongo.SessionContext.
	MongoSessionContext interface {
		context.Context
		StartTransaction(...*options.TransactionOptions) error
		AbortTransaction(context.Context) error
		CommitTransaction(context.Context) error
	}
	// MongoWriter is a custom http.ResponseWriter that handles MongoDB
	// transactions.
	MongoWriter struct {
		logger *logrus.Logger
		sctx   MongoSessionContext
		// w is the currently active response writer. In case of a successful
		// operation it will be the response writer, otherwise it will be the ew.
		w http.ResponseWriter
		// ew is an error writer buffer in which we'll store the data written to
		// the writer in case the operation is not successful. Later we'll be
		// able to either retrieve this data (if we can't retry anymore) or
		// discard it (if we want to retry the call).
		ew *bufferResponseWriter
	}

	// bufferResponseWriter will hold anything written to it in memory.
	// We use it on error to temporarily store the content until we decide
	// whether to retry or give up on the operation.
	bufferResponseWriter struct {
		Buffer bytes.Buffer
		Status int
	}
)

// NewMongoWriter creates a MongoWriter and starts a transaction.
// Returns an error if it fails to start a transaction.
//
// If the response is a successful one (status code 2xx) then MongoWriter will
// directly write to the given http.ResponseWriter. Otherwise, it will write to
// an internal buffer writer, leaving the original http.ResponseWriter intact,
// so the request can be retried. The status of the internal buffer writer can
// be inspected via the ErrorStatus, ErrorBuffer, and FailedWithWriteConflict
// methods.
func NewMongoWriter(w http.ResponseWriter, sctx MongoSessionContext, logger *logrus.Logger) (MongoWriter, error) {
	return MongoWriter{
		logger: logger,
		sctx:   sctx,
		w:      w,
		ew:     &bufferResponseWriter{},
	}, sctx.StartTransaction()
}

// Header implements the ResponseWriter interface.
func (mw *MongoWriter) Header() http.Header {
	return mw.w.Header()
}

// Write implements the ResponseWriter interface.
func (mw *MongoWriter) Write(bytes []byte) (int, error) {
	return mw.w.Write(bytes)
}

// WriteHeader writes the header and finalises the transaction.
func (mw *MongoWriter) WriteHeader(statusCode int) {
	if statusCode < 200 || statusCode > 299 {
		// This is an error state, write all further content to the error writer.
		mw.w = mw.ew
		err := mw.sctx.AbortTransaction(mw.sctx)
		if err != nil {
			mw.logger.Warningln("Failed to abort transaction:", err)
		}
	} else {
		err := mw.sctx.CommitTransaction(mw.sctx)
		if err != nil {
			mw.logger.Warningln("Failed to commit transaction:", err)
		}
	}
	mw.w.WriteHeader(statusCode)
}

// ErrorBuffer returns the data stored in the error buffer.
func (mw *MongoWriter) ErrorBuffer() []byte {
	return mw.ew.Buffer.Bytes()
}

// ErrorStatus returns the status code with which the last call errored out,
// if any.
func (mw *MongoWriter) ErrorStatus() int {
	return mw.ew.Status
}

// FailedWithWriteConflict informs us whether the MongoWriter received a MongoDB
// WriteConflict error.
func (mw *MongoWriter) FailedWithWriteConflict() bool {
	return mw.ew.Status != 0 && strings.Contains(mw.ew.Buffer.String(), writeConflictErrMsg)
}

// Header implementation.
func (w *bufferResponseWriter) Header() http.Header {
	return http.Header{}
}

// Write implementation.
func (w *bufferResponseWriter) Write(b []byte) (int, error) {
	return w.Buffer.Write(b)
}

// WriteHeader implementation.
func (w *bufferResponseWriter) WriteHeader(statusCode int) {
	w.Status = statusCode
}
