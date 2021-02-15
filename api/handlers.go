package api

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"gitlab.com/NebulousLabs/errors"

	"github.com/julienschmidt/httprouter"
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
		api.WriteError(w, err, http.StatusBadRequest)
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
		// Queue the skylink to have its meta data fetched and updated in the
		// DB, as well as the user's used space to be updated.
		api.staticMF.Queue <- metafetcher.Message{
			UserID:    u.ID,
			SkylinkID: skylink.ID,
		}
	} else {
		err = api.staticDB.UserUpdateUsedStorage(req.Context(), u.ID, skylink.Size)
		if err != nil {
			// Log the error but return success - the record will be corrected
			// later when we rescan the user's used space.
			api.staticLogger.Debug("Failed to update user's used space:", err)
		}
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
	st := ps.ByName("status")
	rg := ps.ByName("range")
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
	api.staticLogger.Tracef("Downloading skylink %v, status %v, range %v\n", skylink, st, rg)
	_, err = api.staticDB.DownloadCreate(req.Context(), *u, *skylink)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if skylink.Size == 0 {
		// Zero size means that we haven't fetched the skyfile's size yet.
		// Queue the skylink to have its meta data fetched and updated in the
		// DB, as well as the user's used space to be updated.
		api.staticMF.Queue <- metafetcher.Message{
			UserID:    u.ID,
			SkylinkID: skylink.ID,
		}
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

// trackRegistryWriteHandler registers a new registry read in the system.
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
