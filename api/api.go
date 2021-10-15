package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

type (
	// API is the central struct which gives us access to all subsystems.
	API struct {
		staticDB            *database.DB
		staticMF            *metafetcher.MetaFetcher
		staticRouter        *httprouter.Router
		staticLogger        *logrus.Logger
		staticMailer        *email.Mailer
		staticTierLimits    []TierLimitsPublic
		staticUserTierCache *userTierCache
		staticCockroachDB   *sql.DB
	}

	// errorWrap is a helper type for converting an `error` struct to JSON.
	errorWrap struct {
		Message string `json:"message"`
	}
)

// New returns a new initialised API.
func New(db *database.DB, crdb *sql.DB, mf *metafetcher.MetaFetcher, logger *logrus.Logger, mailer *email.Mailer) (*API, error) {
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
		staticMF:            mf,
		staticRouter:        router,
		staticLogger:        logger,
		staticMailer:        mailer,
		staticTierLimits:    tierLimits,
		staticUserTierCache: newUserTierCache(),
		staticCockroachDB:   crdb,
	}
	api.buildHTTPRoutes()
	return api, nil
}

// Router exposed the internal httprouter struct.
func (api *API) Router() *httprouter.Router {
	return api.staticRouter
}

// WithDBSession injects a session context into the request context of the handler.
func (api *API) WithDBSession(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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

		// Get the special response writer.
		mw, err := newMongoWriter(w, sctx, api.staticLogger)
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to start a new transaction"), http.StatusInternalServerError)
			return
		}

		// Create a new request with our session context.
		req = req.WithContext(sctx)

		// Forward the new response writer and request to the handler.
		h(&mw, req, ps)
	}
}

// WriteError an error to the API caller.
func (api *API) WriteError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	api.staticLogger.Debugln(code, err)
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
	api.staticLogger.Debugln(http.StatusNoContent)
}
