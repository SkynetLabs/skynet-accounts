package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

/*
TODO
	- panic handler
	- method not allowed
	- OPTIONS & CORS - https://github.com/julienschmidt/httprouter#automatic-options-responses-and-cors
*/

const (
	// DefaultTimeoutDB defines the longest a DB operation can take before
	// triggering a timeout. In seconds.
	DefaultTimeoutDB = 10
	// DefaultTimeoutRequest defines the longest an API request can take before
	// triggering a timeout. In seconds.
	DefaultTimeoutRequest = 30

	// TokenValiditySeconds determines the duration of JWT tokens.
	TokenValiditySeconds = 24 * 3600
)

// API is ...
type API struct {
	staticDB     *database.DB
	staticRouter *httprouter.Router
}

// ctxValue is a helper type which makes it safe to register values in the
// context. If we don't use a custom unexported type it's easy for others
// to get our value or accidentally overwrite it.
type ctxValue string

// New returns a new initialised API.
func New(db *database.DB) (*API, error) {
	if db == nil {
		return nil, errors.New("no DB provided")
	}
	router := httprouter.New()
	router.RedirectTrailingSlash = true

	api := &API{
		staticDB:     db,
		staticRouter: router,
	}
	api.buildHTTPRoutes()
	return api, nil
}

// Router exposed the internal httprouter struct.
func (api *API) Router() *httprouter.Router {
	return api.staticRouter
}

// WriteError an error to the API caller.
func WriteError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if build.DEBUG {
		log.Println(code, err)
	}
	encodingErr := json.NewEncoder(w).Encode(err)
	if _, isJsonErr := encodingErr.(*json.SyntaxError); isJsonErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API error response:", encodingErr)
	}
}

// WriteJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead. The Content-Type of the response header is set
// accordingly.
func WriteJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(obj)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API response:", err)
	}
}

// WriteSuccess writes the HTTP header with status 204 No Content to the
// ResponseWriter. WriteSuccess should only be used to indicate that the
// requested action succeeded AND there is no data to return.
func WriteSuccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
