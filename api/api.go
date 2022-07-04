package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/lib"
	"github.com/SkynetLabs/skynet-accounts/metafetcher"
	"gitlab.com/SkynetLabs/skyd/build"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// DBTxnRetryCount specifies the number of times we should retry an API
	// call in case we run into transaction errors.
	DBTxnRetryCount = 5
)

type (
	// API is the central struct which gives us access to all subsystems.
	API struct {
		staticDB            *database.DB
		staticDeps          lib.Dependencies
		staticMF            *metafetcher.MetaFetcher
		staticRouter        *httprouter.Router
		staticLogger        *logrus.Logger
		staticMailer        *email.Mailer
		staticTierLimits    []TierLimitsPublic
		staticUserTierCache *userTierCache
	}

	// errorWrap is a helper type for converting an `error` struct to JSON.
	errorWrap struct {
		Message string `json:"message"`
	}
)

// New returns a new initialised API.
func New(db *database.DB, mf *metafetcher.MetaFetcher, logger *logrus.Logger, mailer *email.Mailer) (*API, error) {
	return NewCustom(db, mf, logger, mailer, &lib.ProductionDependencies{})
}

// NewCustom returns a new initialised API and allows specifying custom
// dependencies.
func NewCustom(db *database.DB, mf *metafetcher.MetaFetcher, logger *logrus.Logger, mailer *email.Mailer, deps lib.Dependencies) (*API, error) {
	if db == nil {
		return nil, errors.New("no DB provided")
	}
	if logger == nil {
		logger = logrus.New()
	}
	router := httprouter.New()
	router.RedirectTrailingSlash = true

	tierLimits := make([]TierLimitsPublic, len(database.UserLimits))
	for i, t := range database.UserLimits {
		tierLimits[i] = TierLimitsPublic{
			TierName:          t.TierName,
			UploadBandwidth:   t.UploadBandwidth * 8,   // convert from bytes
			DownloadBandwidth: t.DownloadBandwidth * 8, // convert from bytes
			MaxUploadSize:     t.MaxUploadSize,
			MaxNumberUploads:  t.MaxNumberUploads,
			RegistryDelay:     t.RegistryDelay,
			Storage:           t.Storage,
		}
	}
	api := &API{
		staticDB:            db,
		staticDeps:          deps,
		staticMF:            mf,
		staticRouter:        router,
		staticLogger:        logger,
		staticMailer:        mailer,
		staticTierLimits:    tierLimits,
		staticUserTierCache: newUserTierCache(),
	}
	api.buildHTTPRoutes()
	return api, nil
}

// ServeHTTP implements the http.Handler interface.
func (api *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	api.staticRouter.ServeHTTP(w, req)
}

// ListenAndServe starts the API server on the given port.
func (api *API) ListenAndServe(port int) error {
	api.staticLogger.Info(fmt.Sprintf("Listening on port %d", port))
	return http.ListenAndServe(fmt.Sprintf(":%d", port), api.staticRouter)
}

// WithDBSession injects a session context into the request context of the
// handler. In case of a MongoDB WriteConflict error, the call is retried up to
// DBTxnRetryCount times or until the request context expires.
func (api *API) WithDBSession(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		numRetriesLeft := DBTxnRetryCount
		var body []byte
		var err error
		if req.Body != nil {
			// Read the request's body and replace its Body io.ReadCloser with a
			// new one based off the read data.
			body, err = io.ReadAll(io.LimitReader(req.Body, LimitBodySizeLarge))
			if err != nil {
				api.WriteError(w, errors.AddContext(err, "failed to read body"), http.StatusBadRequest)
				return
			}
			_ = req.Body.Close()
		}

		// handleFn wraps a full execution of the handler, combined with a retry
		// detection and counting. It also takes care of creating and cancelling
		// Mongo sessions and transactions.
		handleFn := func() (retry bool) {
			req.Body = io.NopCloser(bytes.NewReader(body))
			// Create a new db session
			sess, err := api.staticDB.NewSession()
			if err != nil {
				api.WriteError(w, errors.AddContext(err, "failed to start a new mongo session"), http.StatusInternalServerError)
				return
			}
			// Close session after the handler is done.
			defer sess.EndSession(req.Context())
			// Create session context.
			sctx := mongo.NewSessionContext(req.Context(), sess)
			// Get a special response writer which provide the necessary tools
			// to retry requests on error.
			mw, err := NewMongoWriter(w, sctx, api.staticLogger)
			if err != nil {
				api.WriteError(w, errors.AddContext(err, "failed to start a new transaction"), http.StatusInternalServerError)
				return
			}
			// Create a new request with our session context.
			req = req.WithContext(sctx)
			// Forward the new response writer and request to the handler.
			h(&mw, req, ps)

			// If the call succeeded then we're done because both the status and
			// the response content are already written to the response writer.
			if mw.ErrorStatus() == 0 {
				return false
			}
			// If the call failed with a WriteConflict error and we still have
			// retries left, we'll retry it. Otherwise, we'll write the error to
			// the response writer and finish the call.
			if mw.FailedWithWriteConflict() && numRetriesLeft > 0 {
				select {
				case <-req.Context().Done():
					// If the request context has expired we won't retry anymore.
				default:
					api.staticLogger.Tracef("Retrying call because of WriteConflict (%d out of %d). Request: %+v", numRetriesLeft, DBTxnRetryCount, req)
					numRetriesLeft--
					return true
				}
			}
			// If the call failed with a non-WriteConflict error  or we ran out
			// of retries, we write the error and status to the response writer
			// and finish the call.
			w.WriteHeader(mw.ErrorStatus())
			_, err = w.Write(mw.ErrorBuffer())
			if err != nil {
				api.staticLogger.Warnf("Failed to write to response writer: %+v", err)
			}
			return false
		}

		// Keep retrying the handleFn until it returns a false, indicating that
		// no more retries are needed or possible.
		for handleFn() {
		}
	}
}

// WriteError an error to the API caller.
func (api *API) WriteError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	api.staticLogger.Errorln(code, err)
	encodingErr := json.NewEncoder(w).Encode(errorWrap{Message: err.Error()})
	if _, isJSONErr := encodingErr.(*json.SyntaxError); isJSONErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API error response:", encodingErr)
	}
}

// WriteJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead. The Content-Type of the response header is set
// accordingly.
func (api *API) WriteJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	api.staticLogger.Traceln(http.StatusOK)
	err := json.NewEncoder(w).Encode(obj)
	if err != nil {
		api.staticLogger.Debugln(err)
	}
	if _, isJSONErr := err.(*json.SyntaxError); isJSONErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API response:", err)
	}
}

// WriteSuccess writes the HTTP header with status 204 No Content to the
// ResponseWriter. WriteSuccess should only be used to indicate that the
// requested action succeeded AND there is no data to return.
func (api *API) WriteSuccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
	api.staticLogger.Traceln(http.StatusNoContent)
}
