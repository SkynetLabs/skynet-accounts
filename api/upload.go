package api

import (
	"net/http"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type (
	// UploaderInfo gives information about a user who created an upload.
	UploaderInfo struct {
		UserID   primitive.ObjectID
		Email    string
		Sub      string
		StripeID string
	}

	// UploadInfo gives information about a given upload.
	UploadInfo struct {
		Skylink    string
		UploaderIP string
		UploadedAt time.Time
		UploaderInfo
	}
)

// uploadInfoGET returns detailed information about all uploads of a given
// skylink.
func (api *API) uploadInfoGET(_ *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	skylink := ps.ByName("skylink")
	if !database.ValidSkylinkHash(skylink) {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	sl, err := api.staticDB.Skylink(ctx, skylink)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to get skylink"), http.StatusInternalServerError)
		return
	}
	// Get all uploads of this skylink.
	ups, err := api.staticDB.UploadsBySkylinkID(ctx, sl.ID)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to get uploads"), http.StatusInternalServerError)
		return
	}
	// Get a set of all users that created these uploads.
	uploaderSet := make(map[primitive.ObjectID]interface{})
	for _, up := range ups {
		uploaderSet[up.UserID] = struct{}{}
	}
	// Fet those users' data.
	uploaders := make(map[primitive.ObjectID]database.User)
	for upID := range uploaderSet {
		u, err := api.staticDB.UserByID(ctx, upID)
		if errors.Contains(err, database.ErrUserNotFound) {
			continue
		}
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to get uploader"), http.StatusInternalServerError)
			return
		}
		uploaders[u.ID] = *u
	}
	// Create the final list of hydrated uploads.
	upsHydrated := make([]UploadInfo, len(ups))
	for i, up := range ups {
		u := uploaders[up.UserID]
		upsHydrated[i] = UploadInfo{
			Skylink:    skylink,
			UploaderIP: up.UploaderIP,
			UploadedAt: up.Timestamp,
			UploaderInfo: UploaderInfo{
				UserID:   u.ID,
				Email:    u.Email,
				Sub:      u.Sub,
				StripeID: u.StripeID,
			},
		}
	}
	api.WriteJSON(w, upsHydrated)
}
