package api

import (
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// userHandler returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if errors.Contains(err, database.ErrUserNotFound) {
		u, err = api.staticDB.UserCreate(req.Context(), sub, database.TierFree)
		if err != nil {
			WriteError(w, errors.AddContext(err, "user not found, failed to create"), http.StatusFailedDependency)
			return
		}
	}
	WriteJSON(w, u)
}

// userUploadsHandler returns all uploads made by the current user.
func (api *API) userUploadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	ups, err := api.staticDB.UploadsByUser(req.Context(), *u)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
	}
	WriteJSON(w, ups)
}

// userDownloadsHandler returns all downloads made by the current user.
func (api *API) userDownloadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	ups, err := api.staticDB.DownloadsByUser(req.Context(), *u)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
	}
	WriteJSON(w, ups)
}

// trackUploadHandler registers a new upload in the system.
func (api *API) trackUploadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sl := req.PostFormValue("skylink")
	if sl == "" {
		WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if err == database.ErrInvalidSkylink {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
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
func (api *API) trackDownloadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sl := req.PostFormValue("skylink")
	if sl == "" {
		WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if err == database.ErrInvalidSkylink {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
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
