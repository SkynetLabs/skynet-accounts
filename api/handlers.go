package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"gitlab.com/NebulousLabs/errors"

	"github.com/julienschmidt/httprouter"
)

// userHandler returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	WriteJSON(w, u)
}

// userUploadsHandler returns all uploads made by the current user.
func (api *API) userUploadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err = req.ParseForm(); err != nil {
		WriteError(w, err, http.StatusBadRequest)
	}
	offset, err1 := fetchOffset(req.Form)
	limit, err2 := fetchLimit(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		WriteError(w, err, http.StatusBadRequest)
	}
	ups, total, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, limit)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.UploadsResponseDTO{
		Items:      ups,
		Offset:     offset,
		PageSize:   limit,
		TotalCount: total,
	}
	WriteJSON(w, response)
}

// userDownloadsHandler returns all downloads made by the current user.
func (api *API) userDownloadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err = req.ParseForm(); err != nil {
		WriteError(w, err, http.StatusBadRequest)
	}
	offset, err1 := fetchOffset(req.Form)
	limit, err2 := fetchLimit(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		WriteError(w, err, http.StatusBadRequest)
	}
	downs, total, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, limit)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.DownloadsResponseDTO{
		Items:      downs,
		Offset:     offset,
		PageSize:   limit,
		TotalCount: total,
	}
	WriteJSON(w, response)
}

// trackUploadHandler registers a new upload in the system.
func (api *API) trackUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	sl := ps.ByName("skylink")
	if sl == "" {
		WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.UploadCreate(req.Context(), *u, *skylink)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
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
	WriteSuccess(w)
}

// trackDownloadHandler registers a new download in the system.
func (api *API) trackDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	sl := ps.ByName("skylink")
	if sl == "" {
		WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.DownloadCreate(req.Context(), *u, *skylink)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	WriteSuccess(w)
}

// fetchOffset extracts the offset from the params and validates its value.
func fetchOffset(form url.Values) (int, error) {
	offset, _ := strconv.Atoi(form.Get("offset"))
	if offset < 0 {
		return 0, errors.New("Invalid offset")
	}
	return offset, nil
}

// fetchLimit extracts the offset from the params and validates its value.
func fetchLimit(form url.Values) (int, error) {
	limit, _ := strconv.Atoi(form.Get("limit"))
	if limit < 0 {
		return 0, errors.New("Invalid limit")
	}
	if limit == 0 {
		limit = database.DefaultPageSize
	}
	return limit, nil
}
