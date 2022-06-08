package api

import (
	"net/http"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/types"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type (
	// UploaderInfo gives information about a user who created an upload.
	UploaderInfo struct {
		UserID   primitive.ObjectID
		Email    types.Email
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
	if !database.ValidSkylink(skylink) {
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
	// Get the user data of all uploaders.
	uploaders := make(map[primitive.ObjectID]database.User)
	for _, up := range ups {
		if _, exist := uploaders[up.UserID]; exist {
			continue
		}
		u, err := api.staticDB.UserByID(ctx, up.UserID)
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
	upInfos := make([]UploadInfo, len(ups))
	for i, up := range ups {
		u := uploaders[up.UserID]
		upInfos[i] = UploadInfo{
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
	api.WriteJSON(w, upInfos)
}
