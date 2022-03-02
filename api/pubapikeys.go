package api

import (
	"net/http"
	"strconv"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

type (
	// PubAPIKeyPOST describes the request body for creating a new PubAPIKey
	PubAPIKeyPOST struct {
		Skylinks []string
	}
	// PubAPIKeyPUT describes the request body for updating a PubAPIKey
	PubAPIKeyPUT struct {
		Key      database.PubAPIKey
		Skylinks []string
	}
	// PubAPIKeyPATCH describes the request body for updating a PubAPIKey by
	// providing only the requested changes
	PubAPIKeyPATCH struct {
		Key    database.PubAPIKey
		Add    []string
		Remove []string
	}
)

// userAPIKeyGET lists all PubAPI keys associated with the user.
func (api *API) userPubAPIKeyGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	paks, err := api.staticDB.PubAPIKeyList(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, paks)
}

// userAPIKeyDELETE removes a PubAPI key.
func (api *API) userPubAPIKeyDELETE(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	pakID := ps.ByName("id")
	err := api.staticDB.PubAPIKeyDelete(req.Context(), *u, pakID)
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

// userPubAPIKeyPOST creates a new PubAPI key for the user.
func (api *API) userPubAPIKeyPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body PubAPIKeyPOST
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	pakRec, err := api.staticDB.PubAPIKeyCreate(req.Context(), *u, body.Skylinks)
	if errors.Contains(err, database.ErrMaxNumAPIKeysExceeded) {
		err = errors.AddContext(err, "the maximum number of API keys a user can create is "+strconv.Itoa(database.MaxNumAPIKeysPerUser))
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, pakRec)
}

// userPubAPIKeyPUT updates a PubAPI key.
func (api *API) userPubAPIKeyPUT(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body PubAPIKeyPUT
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.PubAPIKeyUpdate(req.Context(), *u, body.Key, body.Skylinks)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userPubAPIKeyPATCH patches a PubAPI key. The difference between PUT and PATCH is
func (api *API) userPubAPIKeyPATCH(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body PubAPIKeyPATCH
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.PubAPIKeyPatch(req.Context(), *u, body.Key, body.Add, body.Remove)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
