package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/jwt"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// LimitsPublic provides public information of the various limits this
	// portal has.
	LimitsPublic struct {
		UserLimits []TierLimitsPublic `json:"userLimits"`
	}
	// TierLimitsPublic is a DTO specifically designed to inform the public
	// about the different limits of each account tier.
	TierLimitsPublic struct {
		TierName          string `json:"tierName"`
		UploadBandwidth   int    `json:"uploadBandwidth"`   // bits per second
		DownloadBandwidth int    `json:"downloadBandwidth"` // bits per second
		MaxUploadSize     int64  `json:"maxUploadSize"`     // the max size of a single upload in bytes
		MaxNumberUploads  int    `json:"maxNumberUploads"`
		RegistryDelay     int    `json:"registryDelay"` // ms
		Storage           int64  `json:"storageLimit"`
	}
)

// healthGET returns the status of the service
func (api *API) healthGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	status := struct {
		DBAlive bool `json:"dbAlive"`
	}{}
	err := api.staticDB.Ping(req.Context())
	status.DBAlive = err == nil
	api.WriteJSON(w, status)
}

// limitsGET returns the speed limits of this portal.
func (api *API) limitsGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	ul := make([]TierLimitsPublic, len(database.UserLimits))
	for i, t := range database.UserLimits {
		ul[i] = TierLimitsPublic{
			TierName:          t.TierName,
			UploadBandwidth:   t.UploadBandwidth * 8,   // convert from bytes
			DownloadBandwidth: t.DownloadBandwidth * 8, // convert from bytes
			MaxUploadSize:     t.MaxUploadSize,
			MaxNumberUploads:  t.MaxNumberUploads,
			RegistryDelay:     t.RegistryDelay,
			Storage:           t.Storage,
		}
	}
	resp := LimitsPublic{
		UserLimits: ul,
	}
	api.WriteJSON(w, resp)
}

// loginPOST starts a user session by issuing a cookie
func (api *API) loginPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Fetch a JWT token from the request. This token will tell us who the user
	// is and until when their current session is going to stay valid.
	tokenStr, err := tokenFromRequest(req)
	if err != nil {
		api.staticLogger.Traceln("Error fetching token from request:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	token, err := jwt.ValidateToken(api.staticLogger, tokenStr)
	if err != nil {
		api.staticLogger.Traceln("Error validating token:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	// We fetch the expiration time of the token, so we can set the expiration
	// time of the cookie to match it.
	exp := token.Expiration()
	if time.Now().UTC().After(exp) {
		api.WriteError(w, errors.New("token has expired"), http.StatusUnauthorized)
		return
	}
	// Write a secure cookie containing the JWT token of the user. This allows
	// us to verify the user's identity and permissions (i.e. tier) without
	// requesting their credentials or accessing the DB.
	err = writeCookie(w, tokenStr, exp.UTC().Unix())
	if err != nil {
		api.staticLogger.Traceln("Error writing cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// logoutPOST ends a user session by removing a cookie
func (api *API) logoutPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	_, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.staticLogger.Traceln("Error fetching token from context:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	err = writeCookie(w, "", time.Now().UTC().Unix()-1)
	if err != nil {
		api.staticLogger.Traceln("Error deleting cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userGET returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	// Check if the user's details have changed and update them if necessary.
	// We only do it here, instead of baking this into UserBySub because we only
	// care about this information being correct when we're going to present it
	// to the user, e.g. on the Dashboard.
	_, email, err := jwt.UserDetailsFromJWT(req.Context())
	if err != nil {
		api.staticLogger.Debugln("Failed to get user details from JWT:", err)
	}
	if err == nil && email != u.Email {
		u.Email = email
		err = api.staticDB.UserSave(req.Context(), u)
		if err != nil {
			api.staticLogger.Debugln("Failed to update user in DB:", err)
		}
	}
	api.WriteJSON(w, u)
}

// userLimitsGET returns the speed limits which apply to this user.
func (api *API) userLimitsGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	u := api.userFromRequest(req)
	if u == nil || u.QuotaExceeded {
		api.WriteJSON(w, database.UserLimits[database.TierAnonymous])
		return
	}
	api.WriteJSON(w, database.UserLimits[u.Tier])
}

// userStatsGET returns statistics about an existing user.
func (api *API) userStatsGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	us, err := api.staticDB.UserStats(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, us)
}

// userPUT allows changing some user information.
func (api *API) userPUT(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Read body.
	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		err = errors.AddContext(err, "failed to read request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	defer func() { _ = req.Body.Close() }()
	payload := struct {
		StripeID string `json:"stripeCustomerId"`
	}{}
	err = json.Unmarshal(bodyBytes, &payload)
	if err != nil {
		err = errors.AddContext(err, "failed to parse request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if payload.StripeID == "" {
		err = errors.AddContext(err, "empty stripe id")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// Check if this user already has this ID assigned to them.
	if payload.StripeID == u.StripeId {
		// Nothing to do.
		api.WriteJSON(w, u)
		return
	}
	// Check if a user already has this customer id.
	eu, err := api.staticDB.UserByStripeID(req.Context(), payload.StripeID)
	if err != nil && err != database.ErrUserNotFound {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err == nil && eu.ID.Hex() != u.ID.Hex() {
		err = errors.New("this stripe customer id belongs to another user")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// Check if this user already has a Stripe customer ID.
	if u.StripeId != "" {
		err = errors.New("This user already has a Stripe customer id.")
		api.WriteError(w, err, http.StatusUnprocessableEntity)
		return
	}
	// Save the changed Stripe ID to the DB.
	err = api.staticDB.UserSetStripeId(req.Context(), u, payload.StripeID)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// We set this for the purpose of returning the updated value without
	// reading from the DB.
	u.StripeId = payload.StripeID
	api.WriteJSON(w, u)
}

// userUploadsGET returns all uploads made by the current user.
func (api *API) userUploadsGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	if err = req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ups, total, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	response := database.UploadsResponseDTO{
		Items:    ups,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// userUploadDELETE unpins a single upload by this user.
func (api *API) userUploadDELETE(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	uid := ps.ByName("uploadId")
	_, err = api.staticDB.UnpinUpload(req.Context(), uid, *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
	// Now that we've returned results to the caller, we can take care of some
	// administrative details, such as user's quotas check.
	// Note that this call is not affected by the request's context, so we use
	// a separate one.
	go api.checkUserQuotas(context.Background(), u)
}

// userDownloadsGET returns all downloads made by the current user.
func (api *API) userDownloadsGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	if err = req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	downs, total, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	response := database.DownloadsResponseDTO{
		Items:    downs,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// trackUploadPOST registers a new upload in the system.
func (api *API) trackUploadPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	sl := ps.ByName("skylink")
	if sl == "" {
		api.WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.UploadCreate(req.Context(), *u, *skylink)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if skylink.Size == 0 {
		// Zero size means that we haven't fetched the skyfile's size yet.
		// Queue the skylink to have its meta data fetched and updated in the DB.
		go func() {
			api.staticMF.Queue <- metafetcher.Message{
				SkylinkID: skylink.ID,
			}
		}()
	}
	api.WriteSuccess(w)
	// Now that we've returned results to the caller, we can take care of some
	// administrative details, such as user's quotas check.
	// Note that this call is not affected by the request's context, so we use
	// a separate one.
	go api.checkUserQuotas(context.Background(), u)
}

// trackDownloadPOST registers a new download in the system.
func (api *API) trackDownloadPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}

	_ = req.ParseForm()
	downloadedBytes, err := strconv.ParseInt(req.Form.Get("bytes"), 10, 64)
	if err != nil {
		downloadedBytes = 0
		api.staticLogger.Traceln("Failed to parse bytes downloaded:", err)
	}
	if downloadedBytes < 0 {
		api.WriteError(w, errors.New("negative download size"), http.StatusBadRequest)
		return
	}
	// We don't need to track zero-sized downloads. Those are usually additional
	// control requests made by browsers.
	if downloadedBytes == 0 {
		api.WriteSuccess(w)
		return
	}

	sl := ps.ByName("skylink")
	if sl == "" {
		api.WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}

	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	err = api.staticDB.DownloadCreate(req.Context(), *u, *skylink, downloadedBytes)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if skylink.Size == 0 {
		// Zero size means that we haven't fetched the skyfile's size yet.
		// Queue the skylink to have its meta data fetched. We do not specify a user
		// here because this is not an upload, so nobody's used storage needs to be
		// adjusted.
		go func() {
			api.staticMF.Queue <- metafetcher.Message{
				SkylinkID: skylink.ID,
			}
		}()
	}
	api.WriteSuccess(w)
}

// trackRegistryReadPOST registers a new registry read in the system.
func (api *API) trackRegistryReadPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	_, err = api.staticDB.RegistryReadCreate(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// trackRegistryWritePOST registers a new registry write in the system.
func (api *API) trackRegistryWritePOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	_, err = api.staticDB.RegistryWriteCreate(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// skylinkDELETE unpins all uploads of a skylink uploaded by the user.
func (api *API) skylinkDELETE(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	sl := ps.ByName("skylink")
	if !database.ValidSkylinkHash(sl) {
		api.WriteError(w, errors.New("invalid skylink"), http.StatusBadRequest)
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.UnpinUploads(req.Context(), *skylink, *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
	// Now that we've returned results to the caller, we can take care of some
	// administrative details, such as user's quotas check.
	// Note that this call is not affected by the request's context, so we use
	// a separate one.
	go api.checkUserQuotas(context.Background(), u)
}

// checkUserQuotas compares the resources consumed by the user to their quotas
// and sets the QuotaExceeded flag on their account if they exceed any.
func (api *API) checkUserQuotas(ctx context.Context, u *database.User) {
	us, err := api.staticDB.UserStats(ctx, *u)
	if err != nil {
		api.staticLogger.Infof("Failed to fetch user's stats. UID: %s, err: %s", u.ID.Hex(), err.Error())
		return
	}
	q := database.UserLimits[u.Tier]
	quotaExceeded := us.NumUploads > q.MaxNumberUploads || us.TotalUploadsSize > q.Storage
	if quotaExceeded != u.QuotaExceeded {
		u.QuotaExceeded = quotaExceeded
		err = api.staticDB.UserSave(ctx, u)
		if err != nil {
			api.staticLogger.Infof("Failed to save user. User: %+v, err: %s", u, err.Error())
		}
	}
}

// wellKnownJwksGET returns our public JWKS, so people can use that to verify
// the authenticity of the JWT tokens we issue.
func (api *API) wellKnownJwksGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	k, err := jwt.AccountsPublicKeySet(api.staticLogger)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to get the public JWKS"), http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, k)
}

// fetchOffset extracts the offset from the params and validates its value.
func fetchOffset(form url.Values) (int, error) {
	offset, _ := strconv.Atoi(form.Get("offset"))
	if offset < 0 {
		return 0, errors.New("Invalid offset")
	}
	return offset, nil
}

// fetchPageSize extracts the page size from the params and validates its value.
func fetchPageSize(form url.Values) (int, error) {
	pageSize, _ := strconv.Atoi(form.Get("pageSize"))
	if pageSize < 0 {
		return 0, errors.New("Invalid page size")
	}
	if pageSize == 0 {
		pageSize = database.DefaultPageSize
	}
	return pageSize, nil
}
