package api

import (
	"net/http"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"go.mongodb.org/mongo-driver/mongo"
)

// userAPIKeyPOST creates a new API key for the user.
func (api *API) userAPIKeyPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ak, err := api.staticDB.APIKeyCreate(req.Context(), *u)
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
func (api *API) userAPIKeyGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	aks, err := api.staticDB.APIKeyList(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, aks)
}

// userAPIKeyDELETE removes an API key.
func (api *API) userAPIKeyDELETE(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID := ps.ByName("id")
	err := api.staticDB.APIKeyDelete(req.Context(), *u, akID)
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