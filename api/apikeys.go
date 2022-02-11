package api

import (
	"net/http"
	"strconv"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// userAPIKeyPOST creates a new API key for the user.
func (api *API) userAPIKeyPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	ak, err := api.staticDB.APIKeyCreate(req.Context(), *u)
	if errors.Contains(err, database.ErrMaxNumAPIKeysExceeded) {
		err = errors.AddContext(err, "the maximum number of API keys a user can create is "+strconv.Itoa(database.MaxNumAPIKeysPerUser))
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Make the Key visible in JSON form. We do that with an anonymous struct
	// because we don't envision that being needed anywhere else in the project.
	akWithKey := struct {
		database.APIKeyRecord
		Key database.APIKey `bson:"key" json:"key"`
	}{
		*ak,
		ak.Key,
	}
	api.WriteJSON(w, akWithKey)
}

// userAPIKeyGET lists all API keys associated with the user.
func (api *API) userAPIKeyGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	aks, err := api.staticDB.APIKeyList(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, aks)
}

// userAPIKeyDELETE removes an API key.
func (api *API) userAPIKeyDELETE(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	akID := ps.ByName("id")
	err = api.staticDB.APIKeyDelete(req.Context(), *u, akID)
	if err == mongo.ErrNoDocuments {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
