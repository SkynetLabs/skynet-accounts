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
			WriteError(w, errors.AddContext(err, "user not found, failed to create"), http.StatusFailedDependency)
			return
		}
	}
	WriteJSON(w, u)
}
