package api

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoWriter is a custom http.ResponseWriter to commit or abort transactions.
type MongoWriter struct {
	logger *logrus.Logger
	sctx   mongo.SessionContext
	w      http.ResponseWriter
}

// NewMongoWriter creates the MongoWriter and starts a transaction.
// Returns an error if it fails to start a transaction.
func NewMongoWriter(w http.ResponseWriter, sctx mongo.SessionContext, logger *logrus.Logger) (MongoWriter, error) {
	return MongoWriter{
		logger: logger,
		sctx:   sctx,
		w:      w,
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
