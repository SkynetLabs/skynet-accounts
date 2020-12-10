package api

import (
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// userHandlerGET returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if errors.Contains(err, database.ErrUserNotFound) {
		u, err = api.staticDB.UserCreate(req.Context(), sub, database.TierFree)
		if err != nil {
			WriteError(w, errors.AddContext(err, "user not found, failed to create"), http.StatusNotFound)
			return
		}
	}
	WriteJSON(w, u)
}

// TODO This will be needed bu only for changing the user's tier and expiration times. It will be driven by payments.
//// userHandlerPUT updates an existing user.
//func (api *API) userHandlerPUT(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
//	sub, _, _, err := tokenFromContext(req)
//	if err != nil {
//		WriteError(w, err, http.StatusInternalServerError)
//		return
//	}
//
//	// Fetch the user by their id.
//	u, err := api.staticDB.UserByID(req.Context(), uid)
//	if err != nil {
//		WriteError(w, errors.AddContext(err, "failed to fetch user"), http.StatusInternalServerError)
//		return
//	}
//	// Get the changes values.
//	if fn := req.PostFormValue("firstName"); fn != "" {
//		u.FirstName = fn
//	}
//	if ln := req.PostFormValue("lastName"); ln != "" {
//		u.LastName = ln
//	}
//	if em := req.PostFormValue("email"); em != "" {
//		// No need for extra validation here, the email will be validated in
//		// the update method before any work is done.
//		u.Email = database.Email(em)
//	}
//	// Persist the changes.
//	err = api.staticDB.UserUpdate(req.Context(), u)
//	if errors.Contains(err, database.ErrInvalidEmail) || errors.Contains(err, database.ErrEmailAlreadyUsed) {
//		WriteError(w, err, http.StatusBadRequest)
//		return
//	}
//	if err != nil {
//		WriteError(w, err, http.StatusInternalServerError)
//		return
//	}
//	w.WriteHeader(http.StatusOK)
//	WriteJSON(w, u)
//}
//
