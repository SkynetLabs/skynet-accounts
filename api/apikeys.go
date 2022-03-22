package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type (
	// Revive complains about these names stuttering but we like them as they
	// are, so we'll disable revive for a moment here.
	//revive:disable

	// APIKeyPOST describes the body of a POST request that creates an API key
	APIKeyPOST struct {
		Public   bool     `json:"public,string"`
		Skylinks []string `json:"skylinks"`
	}
	// APIKeyPUT describes the request body for updating an API key
	APIKeyPUT struct {
		Skylinks []string
	}
	// APIKeyPATCH describes the request body for updating an API key by
	// providing only the requested changes
	APIKeyPATCH struct {
		Add    []string
		Remove []string
	}
	// APIKeyResponse is an API DTO which mirrors database.APIKey.
	APIKeyResponse struct {
		ID        primitive.ObjectID `json:"id"`
		UserID    primitive.ObjectID `json:"-"`
		Public    bool               `json:"public,string"`
		Key       database.APIKey    `json:"-"`
		Skylinks  []string           `json:"skylinks"`
		CreatedAt time.Time          `json:"createdAt"`
	}
	// APIKeyResponseWithKey is an API DTO which mirrors database.APIKey but
	// also reveals the value of the Key field. This should only be used on key
	// creation.
	APIKeyResponseWithKey struct {
		APIKeyResponse
		Key database.APIKey `json:"key"`
	}

	//revive:enable
)

// Validate checks if the request and its parts are valid.
func (akp APIKeyPOST) Validate() error {
	if !akp.Public && len(akp.Skylinks) > 0 {
		return errors.New("public API keys cannot refer to skylinks")
	}
	var errs []error
	for _, s := range akp.Skylinks {
		if !database.ValidSkylinkHash(s) {
			errs = append(errs, errors.New("invalid skylink: "+s))
		}
	}
	if len(errs) > 0 {
		return errors.Compose(errs...)
	}
	return nil
}

//revive:disable

// APIKeyResponseFromAPIKey creates a new APIKeyResponse from the given API key.
func APIKeyResponseFromAPIKey(ak database.APIKeyRecord) *APIKeyResponse {
	return &APIKeyResponse{
		ID:        ak.ID,
		UserID:    ak.UserID,
		Public:    ak.Public,
		Key:       ak.Key,
		Skylinks:  ak.Skylinks,
		CreatedAt: ak.CreatedAt,
	}
}

// APIKeyResponseWithKeyFromAPIKey creates a new APIKeyResponseWithKey from the
// given API key.
func APIKeyResponseWithKeyFromAPIKey(ak database.APIKeyRecord) *APIKeyResponseWithKey {
	return &APIKeyResponseWithKey{
		APIKeyResponse: APIKeyResponse{
			ID:        ak.ID,
			UserID:    ak.UserID,
			Public:    ak.Public,
			Key:       ak.Key,
			Skylinks:  ak.Skylinks,
			CreatedAt: ak.CreatedAt,
		},
		Key: ak.Key,
	}
}

//revive:enable

// userAPIKeyPOST creates a new API key for the user.
func (api *API) userAPIKeyPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body APIKeyPOST
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err := body.Validate(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ak, err := api.staticDB.APIKeyCreate(req.Context(), *u, body.Public, body.Skylinks)
	if errors.Contains(err, database.ErrMaxNumAPIKeysExceeded) {
		err = errors.AddContext(err, "the maximum number of API keys a user can create is "+strconv.Itoa(database.MaxNumAPIKeysPerUser))
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, APIKeyResponseWithKeyFromAPIKey(*ak))
}

// userAPIKeyGET returns a single API key.
func (api *API) userAPIKeyGET(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ak, err := api.staticDB.APIKeyGet(req.Context(), akID)
	// If there is no such API key or it doesn't exist, return a 404.
	if errors.Contains(err, mongo.ErrNoDocuments) || (err == nil && ak.UserID != u.ID) {
		api.WriteError(w, nil, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, APIKeyResponseFromAPIKey(ak))
}

// userAPIKeyLIST lists all API keys associated with the user.
func (api *API) userAPIKeyLIST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	aks, err := api.staticDB.APIKeyList(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]*APIKeyResponse, 0, len(aks))
	for _, ak := range aks {
		resp = append(resp, APIKeyResponseFromAPIKey(ak))
	}
	api.WriteJSON(w, resp)
}

// userAPIKeyDELETE removes an API key.
func (api *API) userAPIKeyDELETE(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyDelete(req.Context(), *u, akID)
	if err == mongo.ErrNoDocuments {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userAPIKeyPUT updates an API key. Only possible for public API keys.
func (api *API) userAPIKeyPUT(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	var body APIKeyPUT
	err = parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyUpdate(req.Context(), *u, akID, body.Skylinks)
	if errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userAPIKeyPATCH patches an API key. The difference between PUT and PATCH is
// that PATCH only specifies the changes while PUT provides the expected list of
// covered skylinks. Only possible for public API keys.
func (api *API) userAPIKeyPATCH(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	var body APIKeyPATCH
	err = parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyPatch(req.Context(), *u, akID, body.Add, body.Remove)
	if errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
