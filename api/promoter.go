package api

import (
	"fmt"
	"net/http"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// PromoterSetTierPOST describes the body of a POST request that sets the
	// user's tier.
	PromoterSetTierPOST struct {
		Tier int `json:"tier"`
	}
)

// _promoterSetTierPOST sets the given user's tier.
func (api *API) _promoterSetTierPOST(_ *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub := ps.ByName("sub")
	var body PromoterSetTierPOST
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if body.Tier < database.TierFree || body.Tier >= database.TierMaxReserved {
		api.WriteError(w, fmt.Errorf("invalid tier %d", body.Tier), http.StatusBadRequest)
		return
	}
	ctx := req.Context()
	u, err := api.staticDB.UserBySub(ctx, sub)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	err = api.staticDB.UserSetTier(ctx, u, body.Tier)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
