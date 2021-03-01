package api

import (
	"encoding/json"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// StripeAPIKey is our API key for communicating with Stripe. It's read
	// from the `.env` file on service start.
	StripeAPIKey = ""
)

// API is ...
type API struct {
	staticDB     *database.DB
	staticMF     *metafetcher.MetaFetcher
	staticRouter *httprouter.Router
	staticLogger *logrus.Logger
}

// ctxValue is a helper type which makes it safe to register values in the
// context. If we don't use a custom unexported type it's easy for others
// to get our value or accidentally overwrite it.
type ctxValue string

// New returns a new initialised API.
func New(db *database.DB, mf *metafetcher.MetaFetcher, logger *logrus.Logger) (*API, error) {
	if db == nil {
		return nil, errors.New("no DB provided")
	}
	if logger == nil {
		logger = logrus.New()
	}
	router := httprouter.New()
	router.RedirectTrailingSlash = true

	api := &API{
		staticDB:     db,
		staticMF:     mf,
		staticRouter: router,
		staticLogger: logger,
	}
	api.buildHTTPRoutes()
	return api, nil
}

// Router exposed the internal httprouter struct.
func (api *API) Router() *httprouter.Router {
	return api.staticRouter
}

// WriteError an error to the API caller.
func (api *API) WriteError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	api.staticLogger.Debugln(code, err)
	encodingErr := json.NewEncoder(w).Encode(err)
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
	api.staticLogger.Debugln(http.StatusOK)
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
