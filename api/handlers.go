package api

import (
	"net/http"
	"strconv"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"gitlab.com/NebulousLabs/errors"

	"github.com/julienschmidt/httprouter"
)

// userHandler returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %v\n", req)
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
func (api *API) userUploadsHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %v\n", req)
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
	offset, _ := strconv.Atoi(ps.ByName("offset"))
	limit, _ := strconv.Atoi(ps.ByName("limit"))
	ups, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, limit)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	api.WriteJSON(w, ups)
}

// userDownloadsHandler returns all downloads made by the current user.
func (api *API) userDownloadsHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %v\n", req)
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	offset, _ := strconv.Atoi(ps.ByName("offset"))
	limit, _ := strconv.Atoi(ps.ByName("limit"))
	ups, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, limit)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	api.WriteJSON(w, ups)
}

// trackUploadHandler registers a new upload in the system.
func (api *API) trackUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %v\n", req)
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
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
	api.staticLogger.Tracef("Processing request: %v\n", req)
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
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
	_, err = api.staticDB.DownloadCreate(req.Context(), *u, *skylink)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
