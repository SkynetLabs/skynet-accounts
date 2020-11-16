package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/NebulousLabs/skynet-accounts/build"

	"github.com/julienschmidt/httprouter"
)

// API is ...
type API struct {
	DB     *database.DB
	Router *httprouter.Router
}

// New returns a new initialised API.
func New() *API {
	router := httprouter.New()
	router.RedirectTrailingSlash = true

	api := &API{
		Router: router,
	}
	api.buildHTTPRoutes()

	db, err := database.New(context.Background())
	api.DB = db
	if err != nil {
		log.Println("Failed to connect to the database! Running without a database.")
		// Assign an empty database instance. In its methods we can check for
		// the availability of the DB and return errors instead of crashing.
		api.DB = &database.DB{}
	}

	return api
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
