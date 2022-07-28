package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/types"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	// ErrTimePeriodTooLong is returned when the user requests unacceptably long
	// time period.
	ErrTimePeriodTooLong = errors.New("given time period is too long")
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
	// SkylinksList represents a list of skylinks.
	SkylinksList struct {
		Skylinks   []string `json:"skylinks"`
		TotalCount int      `json:"totalCount"`
	}
)

// _uploadInfoGET returns detailed information about all uploads of a given
// skylink.
func (api *API) _uploadInfoGET(_ *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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

// _uploadedSkylinksGET lists all uploads from the given time range.
func (api *API) _uploadedSkylinksGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	from, err := parseInt64Param(req, "from")
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	to, err := parseInt64Param(req, "to")
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	offset, err := fetchOffset(req.Form)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	pageSize, err := fetchPageSize(req.Form, DefaultPageSizeLarge)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	dayInSecs := int64(24 * 3600)
	defaultPeriod := 3 * dayInSecs
	maxPeriod := 30 * dayInSecs
	if to == 0 && from == 0 {
		to = time.Now().UTC().Unix()
		from = to - defaultPeriod
	} else if to == 0 {
		to = from + defaultPeriod
	} else if from == 0 {
		from = to - defaultPeriod
	}
	if to-from > maxPeriod {
		api.WriteError(w, ErrTimePeriodTooLong, http.StatusBadRequest)
		return
	}
	// Fetch all uploads from the period.
	uploads, totalCount, err := api.staticDB.UploadsByPeriod(req.Context(), time.Unix(from, 0), time.Unix(to, 0), offset, pageSize)
	if errors.Contains(err, database.ErrInvalidTimePeriod) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to fetch uploads from DB"), http.StatusInternalServerError)
		return
	}
	resp := SkylinksList{
		Skylinks:   CollectUniqueSkylinks(uploads),
		TotalCount: int(totalCount),
	}
	api.WriteJSON(w, resp)
}

// CollectUniqueSkylinks is a helper that iterates over upload responses and
// returns a deduplicated list of the uploaded skylinks.
func CollectUniqueSkylinks(uploads []database.UploadResponse) []string {
	// Add all skylinks to a set, so they are deduplicated.
	slsMap := make(map[string]struct{})
	for _, u := range uploads {
		slsMap[u.Skylink] = struct{}{}
	}
	// Collect them in a slice.
	sls := make([]string, 0, len(slsMap))
	for s := range slsMap {
		sls = append(sls, s)
	}
	return sls
}

// parseInt64Param is a helper that parses an integer parameter.
func parseInt64Param(req *http.Request, name string) (int64, error) {
	var val int64
	var err error
	if str := req.FormValue(name); str != "" {
		val, err = strconv.ParseInt(str, 10, 0)
		if err != nil {
			return 0, errors.AddContext(err, fmt.Sprintf("invalid '%s' value", name))
		}
	}
	return val, nil
}
