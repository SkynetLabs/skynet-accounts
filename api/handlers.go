package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// loginHandler starts a user session by issuing a cookie
func (api *API) loginHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	tokenStr, err := tokenFromRequest(req)
	if err != nil {
		api.staticLogger.Traceln("Error fetching token from request:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	token, err := ValidateToken(api.staticLogger, tokenStr)
	if err != nil {
		api.staticLogger.Traceln("Error validating token:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	exp, err := tokenExpiration(token)
	if err != nil {
		api.staticLogger.Traceln("Error checking token expiration:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	err = writeCookie(w, tokenStr, exp)
	if err != nil {
		api.staticLogger.Traceln("Error writing cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// logoutHandler ends a user session by removing a cookie
func (api *API) logoutHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	_, _, _, err := tokenFromContext(req)
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

// userHandler returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, u)
}

// userStatsHandler returns statistics about an existing user.
func (api *API) userStatsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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
	ud, err := api.staticDB.UserStats(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ud)
}

// userPutHandler allows changing some user information.
func (api *API) userPutHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	if err = req.ParseForm(); err != nil {
		api.WriteError(w, errors.New("bad form data"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Read and validate the parameters. Set them on the user struct.
	stripeId := req.Form.Get("stripeCustomerId")
	if stripeId != "" {
		// Check if a user already has this customer id.
		eu, err := api.staticDB.UserByStripeID(req.Context(), stripeId)
		if err != nil && err != database.ErrUserNotFound {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		if err == nil && eu.ID.Hex() != u.ID.Hex() {
			err = errors.New("this stripe customer id belongs to another user")
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		u.StripeId = stripeId
	}
	// Save the changed user to the DB.
	err = api.staticDB.UserSave(req.Context(), u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, u)
}

// userUploadsHandler returns all uploads made by the current user.
func (api *API) userUploadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	ups, total, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.UploadsResponseDTO{
		Items:    ups,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// userDownloadsHandler returns all downloads made by the current user.
func (api *API) userDownloadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	downs, total, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.DownloadsResponseDTO{
		Items:    downs,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// trackUploadHandler registers a new upload in the system.
func (api *API) trackUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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
				UploaderID: u.ID,
				SkylinkID:  skylink.ID,
			}
		}()
	}
	api.WriteSuccess(w)
}

// trackDownloadHandler registers a new download in the system.
func (api *API) trackDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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

// trackRegistryReadHandler registers a new registry read in the system.
func (api *API) trackRegistryReadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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

// trackRegistryWriteHandler registers a new registry write in the system.
func (api *API) trackRegistryWriteHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
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

// stripeCheckoutHandler ...
func (api *API) stripeCheckoutHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}

	sid := ps.ByName("session_id")
	if sid == "" {
		api.WriteError(w, errors.New("missing parameter 'session_id'"), http.StatusBadRequest)
		return
	}

	// TODO Implement
	fmt.Println(sid, u)

	api.WriteSuccess(w)
}

// stripeWebhookHandler ...
func (api *API) stripeWebhookHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.staticLogger.Debugf("WH >>> ps: %v", ps)
	api.staticLogger.Debugf("WH >>> body: %v", string(bodyBytes))
	api.WriteSuccess(w)
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
