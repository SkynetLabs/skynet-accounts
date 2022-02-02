package api

import (
	"net/http"

	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ak)
}

// userAPIKeyGET lists one or all API keys associated with the user.
func (api *API) userAPIKeyGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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

	// If there's an API key ID supplied we'll return a single key.
	if id := ps.ByName("keyID"); id != "" {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			if err != nil {
				api.WriteError(w, err, http.StatusBadRequest)
				return
			}
		}
		ak, err := api.staticDB.APIKeyFetch(req.Context(), *u, oid)
		if err != nil {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		api.WriteJSON(w, ak)
		return
	}

	// If there is no API key ID supplied we'll list all keys for this user.
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
	id, err := primitive.ObjectIDFromHex(ps.ByName("keyID"))
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "Invalid API key ID provided."), http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyDelete(req.Context(), *u, id)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
