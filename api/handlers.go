package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"time"

	"github.com/SkynetLabs/skynet-accounts/build"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/hash"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/lib"
	"github.com/SkynetLabs/skynet-accounts/metafetcher"
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

	// accountRecoveryDTO defines the payload we expect when a user is trying to
	// change their password.
	accountRecoveryDTO struct {
		Token           string `json:"token"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	// credentialsDTO defines the standard credentials package we expect.
	credentialsDTO struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// userUpdateDTO defines the fields of the User record that can be changed
	// externally, e.g. by calling `PUT /user`.
	userUpdateDTO struct {
		Email    string `json:"email,omitempty"`
		StripeID string `json:"stripeCustomerId,omitempty"`
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
	resp := LimitsPublic{
		UserLimits: api.staticTierLimits,
	}
	api.WriteJSON(w, resp)
}

// loginGET generates a login challenge for the caller.
func (api *API) loginGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var pk database.PubKey
	err := pk.LoadString(req.FormValue("pubKey"))
	if err != nil {
		api.WriteError(w, errors.New("invalid pubKey provided"), http.StatusBadRequest)
		return
	}
	_, err = api.staticDB.UserByPubKey(req.Context(), pk)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, errors.New("no user with this pubkey"), http.StatusBadRequest)
		return
	}
	ch, err := api.staticDB.NewChallenge(req.Context(), pk, database.ChallengeTypeLogin)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ch)
}

// loginPOST starts a user session by issuing a cookie
func (api *API) loginPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the body, we might need to use it several times.
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "empty request body"), http.StatusBadRequest)
		return
	}

	// Since we don't want to have separate endpoints for logging in with
	// credentials and token, we'll do both here.
	//
	// Check whether credentials are provided. Those trump the token because a
	// user with a valid token might want to relog. No need to force them to
	// log out first.
	var payload credentialsDTO
	err = json.Unmarshal(body, &payload)
	if err == nil && payload.Email != "" && payload.Password != "" {
		api.loginPOSTCredentials(w, req, payload.Email, payload.Password)
		return
	}

	// Check for a challenge response in the request.
	var chr database.ChallengeResponse
	err = chr.LoadFromRequest(bytes.NewBuffer(body))
	if err == nil {
		api.loginPOSTChallengeResponse(w, req, chr)
		return
	}

	// In case credentials were not found try to log the user by detecting a
	// token.
	api.loginPOSTToken(w, req)
}

// loginPOSTChallengeResponse is a helper that handles logins with a challenge.
func (api *API) loginPOSTChallengeResponse(w http.ResponseWriter, req *http.Request, chr database.ChallengeResponse) {
	ctx := req.Context()
	pk, _, err := api.staticDB.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeLogin)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to validate challenge response"), http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserByPubKey(ctx, pk)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	api.loginUser(w, u, false)
}

// loginPOSTCredentials is a helper that handles logins with credentials.
func (api *API) loginPOSTCredentials(w http.ResponseWriter, req *http.Request, email, password string) {
	// Fetch the user with that email, if they exist.
	u, err := api.staticDB.UserByEmail(req.Context(), email)
	if err != nil {
		api.staticLogger.Debugf("Error fetching a user with email '%s': %+v\n", email, err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	// Check if the password matches.
	err = hash.Compare(password, []byte(u.PasswordHash))
	if err != nil {
		api.WriteError(w, errors.New("password mismatch"), http.StatusUnauthorized)
		return
	}
	api.loginUser(w, u, false)
}

// loginPOSTToken is a helper that handles logins via a token attached to the
// request.
func (api *API) loginPOSTToken(w http.ResponseWriter, req *http.Request) {
	// Fetch a JWT token from the request. This token will tell us who the user
	// is and until when their current session is going to stay valid.
	tokenStr, err := tokenFromRequest(req)
	if err != nil {
		api.staticLogger.Debugln("Error fetching token from request:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	token, err := jwt.ValidateToken(tokenStr)
	if err != nil {
		api.staticLogger.Debugln("Error validating token:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	// Write a secure cookie containing the JWT token of the user. This allows
	// us to verify the user's identity and permissions (i.e. tier) without
	// requesting their credentials or accessing the DB.
	err = writeCookie(w, tokenStr, token.Expiration().UTC().Unix())
	if err != nil {
		api.staticLogger.Debugln("Error writing cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// loginUser is a helper method that generates a JWT for the user and writes the
// login cookie.
func (api *API) loginUser(w http.ResponseWriter, u *database.User, returnUser bool) {
	// Generate a JWT.
	tk, tkBytes, err := jwt.TokenForUser(u.Email, u.Sub)
	if err != nil {
		api.staticLogger.Debugf("Error creating a token for user: %+v\n", err)
		err = errors.AddContext(err, "failed to create a token for user")
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Write the JWT to an encrypted cookie.
	err = writeCookie(w, string(tkBytes), tk.Expiration().UTC().Unix())
	if err != nil {
		api.staticLogger.Debugln("Error writing cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if returnUser {
		api.WriteJSON(w, u)
	} else {
		api.WriteSuccess(w)
	}
}

// logoutPOST ends a user session by removing a cookie
func (api *API) logoutPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	_, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.staticLogger.Debugln("Error fetching token from context:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	err = writeCookie(w, "", time.Now().UTC().Unix()-1)
	if err != nil {
		api.staticLogger.Debugln("Error deleting cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// registerGET generates a registration challenge for the caller.
func (api *API) registerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var pk database.PubKey
	err := pk.LoadString(req.FormValue("pubKey"))
	if err != nil {
		api.WriteError(w, errors.New("invalid pubKey provided"), http.StatusBadRequest)
		return
	}
	// Check if this pubkey is already associated with a user.
	_, err = api.staticDB.UserByPubKey(req.Context(), pk)
	if !errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, errors.New("pubkey already registered"), http.StatusBadRequest)
		return
	}
	ch, err := api.staticDB.NewChallenge(req.Context(), pk, database.ChallengeTypeRegister)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ch)
}

// registerPOST registers a new user based on a challenge-response.
func (api *API) registerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the body, we might need to use it several times.
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "empty request body"), http.StatusBadRequest)
		return
	}
	// Get the challenge response.
	var chr database.ChallengeResponse
	err = chr.LoadFromRequest(bytes.NewBuffer(body))
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "missing or invalid challenge response"), http.StatusBadRequest)
		return
	}
	ctx := req.Context()
	pk, _, err := api.staticDB.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to validate challenge response"), http.StatusBadRequest)
		return
	}
	// Parse the request's body.
	var payload credentialsDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	// Validate the email address.
	e, err := mail.ParseAddress(payload.Email)
	if err != nil {
		api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
		return
	}
	// Strip any names from the email and leave just the address.
	// The password is optional.
	payload.Email = e.Address
	u, err := api.staticDB.UserCreatePK(ctx, payload.Email, payload.Password, "", pk, database.TierFree)
	if errors.Contains(err, database.ErrUserAlreadyExists) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	err = api.staticMailer.SendAddressConfirmationEmail(ctx, u.Email, u.EmailConfirmationToken)
	if err != nil {
		api.staticLogger.Debugln(errors.AddContext(err, "failed to send address confirmation email"))
	}
	api.loginUser(w, u, true)
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
	// TODO Remove this when we remove Kratos.
	// Check if the user's details have changed and update them if necessary.
	// We only do it here, instead of baking this into UserBySub because we only
	// care about this information being correct when we're going to present it
	// to the user, e.g. on the Dashboard.
	_, email, err := jwt.UserDetailsFromJWT(req.Context())
	if err != nil {
		api.staticLogger.Traceln("Failed to get user details from JWT:", err)
	}
	if err == nil && email != u.Email {
		u.Email = email
		err = api.staticDB.UserSave(req.Context(), u)
		if err != nil {
			api.staticLogger.Traceln("Failed to update user in DB:", err)
		}
	}
	api.WriteJSON(w, u)
}

// userLimitsGET returns the speed limits which apply to this user.
func (api *API) userLimitsGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	t, err := tokenFromRequest(req)
	if err != nil {
		api.WriteJSON(w, database.UserLimits[database.TierAnonymous])
		return
	}
	tk, err := jwt.ValidateToken(t)
	if err != nil {
		api.WriteJSON(w, database.UserLimits[database.TierAnonymous])
		return
	}
	s, exists := tk.Get("sub")
	if !exists {
		api.staticLogger.Warnln("Token without a sub.")
		api.WriteJSON(w, database.UserLimits[database.TierAnonymous])
		return
	}
	sub := s.(string)
	// If the user is not cached, or they were cached too long ago we'll fetch
	// their data from the DB.
	tier, ok := api.staticUserTierCache.Get(sub)
	if !ok {
		u, err := api.staticDB.UserBySub(req.Context(), sub, false)
		if err != nil {
			api.staticLogger.Debugf("Failed to fetch user from DB for sub '%s'. Error: %s", sub, err.Error())
			api.WriteJSON(w, database.UserLimits[database.TierAnonymous])
			return
		}
		api.staticUserTierCache.Set(u)
	}
	tier, ok = api.staticUserTierCache.Get(sub)
	if !ok {
		build.Critical("Failed to fetch user from UserTierCache right after setting it.")
	}
	api.WriteJSON(w, database.UserLimits[tier])
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

// userDELETE deletes the user and all of their data.
func (api *API) userDELETE(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ctx := req.Context()
	sub, _, _, err := jwt.TokenFromContext(ctx)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(ctx, sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	err = api.staticDB.UserDelete(ctx, u)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userPOST creates a new user.
func (api *API) userPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Parse the request's body.
	var payload credentialsDTO
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	err = json.Unmarshal(b, &payload)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	// Validate the email address.
	a, err := mail.ParseAddress(payload.Email)
	if err != nil {
		api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
		return
	}
	// Strip any names from the email and leave just the address.
	payload.Email = a.Address
	if payload.Password == "" {
		api.WriteError(w, errors.New("password is required"), http.StatusBadRequest)
		return
	}
	// We are generating the sub here and not in UserCreate because there are
	// many reasons to call UserCreate but this handler is the only place (so
	// far) that should be allowed to call it without a sub. The reason for that
	// is that the users created here are the only users we do not need to link
	// to CockroachDB via their subs.
	sub, err := lib.GenerateUUID()
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to generate user sub"), http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserCreate(req.Context(), payload.Email, payload.Password, sub, database.TierFree)
	if errors.Contains(err, database.ErrUserAlreadyExists) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	err = api.staticMailer.SendAddressConfirmationEmail(req.Context(), u.Email, u.EmailConfirmationToken)
	if err != nil {
		api.staticLogger.Debugln(errors.AddContext(err, "failed to send address confirmation email"))
	}
	api.loginUser(w, u, true)
}

// userPUT allows changing some user information.
// This method receives its parameters as a JSON object.
func (api *API) userPUT(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ctx := req.Context()
	sub, _, _, err := jwt.TokenFromContext(ctx)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}

	// Read and parse the request body.
	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		err = errors.AddContext(err, "failed to read request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	var payload userUpdateDTO
	err = json.Unmarshal(bodyBytes, &payload)
	if err != nil {
		err = errors.AddContext(err, "failed to parse request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if payload.Email == "" && payload.StripeID == "" {
		// The payload is empty, nothing to do.
		api.WriteSuccess(w)
		return
	}

	// Fetch the user from the DB.
	u, err := api.staticDB.UserBySub(ctx, sub, false)
	if errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}

	if payload.StripeID != "" {
		// Check if this user already has a Stripe customer ID.
		if u.StripeID != "" {
			err = errors.New("this user already has a Stripe customer id")
			api.WriteError(w, err, http.StatusConflict)
			return
		}
		// Verify that no other user owns this StripeID.
		su, err := api.staticDB.UserByStripeID(ctx, payload.StripeID)
		if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		if err == nil && su.Sub != sub {
			err = errors.New("this stripe customer id belongs to another user")
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		// Set the StripeID.
		u.StripeID = payload.StripeID
	}

	var changedEmail bool
	if payload.Email != "" {
		// Validate the new email.
		a, err := mail.ParseAddress(payload.Email)
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "invalid email address"), http.StatusBadRequest)
			return
		}
		// Strip any names from the email and leave just the address.
		payload.Email = a.Address
		// Check if another user already has this email address.
		eu, err := api.staticDB.UserByEmail(ctx, payload.Email)
		if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		if err == nil && eu.Sub != sub {
			err = errors.New("this email is already in use")
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		// Set the new email and set it up for a confirmation.
		u.Email = payload.Email
		u.EmailConfirmationTokenExpiration = time.Now().UTC().Add(database.EmailConfirmationTokenTTL).Truncate(time.Millisecond)
		u.EmailConfirmationToken, err = lib.GenerateUUID()
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to generate a token"), http.StatusInternalServerError)
			return
		}
		changedEmail = true
	}

	// Save the changes.
	err = api.staticDB.UserSave(ctx, u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Send a confirmation email if the user's email address was changed.
	if changedEmail {
		err = api.staticMailer.SendAddressConfirmationEmail(ctx, u.Email, u.EmailConfirmationToken)
		if err != nil {
			api.staticLogger.Debugln(errors.AddContext(err, "failed to send address confirmation email"))
		}
	}
	api.loginUser(w, u, true)
}

// userPubKeyRegisterGET generates an update challenge for the caller.
func (api *API) userPubKeyRegisterGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}

	ctx := req.Context()
	pk, err := hex.DecodeString(req.FormValue("pubKey"))
	if err != nil || len(pk) != database.PubKeySize {
		api.WriteError(w, errors.New("invalid pubKey provided"), http.StatusBadRequest)
		return
	}
	_, err = api.staticDB.UserByPubKey(ctx, pk)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, errors.New("failed to fetch user from the DB"), http.StatusInternalServerError)
		return
	}
	if err == nil {
		api.WriteError(w, errors.New("pubkey already registered"), http.StatusBadRequest)
		return
	}
	ch, err := api.staticDB.NewChallenge(ctx, pk, database.ChallengeTypeUpdate)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	uu := &database.UnconfirmedUserUpdate{
		Sub:         sub,
		ChallengeID: ch.ID,
		ExpiresAt:   ch.ExpiresAt.Truncate(time.Millisecond),
	}
	err = api.staticDB.StoreUnconfirmedUserUpdate(ctx, uu)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to store unconfirmed user update"), http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ch)
}

// userPubKeyRegisterPOST updates the user's pubKey based on a challenge-response.
func (api *API) userPubKeyRegisterPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}

	ctx := req.Context()
	// Get the challenge response.
	var chr database.ChallengeResponse
	err = chr.LoadFromRequest(req.Body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "missing or invalid challenge response"), http.StatusBadRequest)
		return
	}
	pk, chID, err := api.staticDB.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeUpdate)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to validate challenge response"), http.StatusBadRequest)
		return
	}
	// Check if the pubkey from the UnconfirmedUserUpdate is already associated
	// with a user. That might have happened between the challenge creation and
	// the current moment.
	pku, err := api.staticDB.UserByPubKey(ctx, pk)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		err = errors.AddContext(err, "failed to verify that the pubKey is not already in use")
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err == nil && pku.Sub != sub {
		api.WriteError(w, errors.New("this pubKey already belongs to another user"), http.StatusBadRequest)
		return
	}
	uu, err := api.staticDB.FetchUnconfirmedUserUpdate(ctx, chID)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to fetch unconfirmed user update"), http.StatusInternalServerError)
		return
	}
	if uu.Sub != sub {
		api.staticLogger.Warnf("Potential attempt to modify another user's pubKey. Sub of challenge requester '%s', sub of response submitter '%s'", uu.Sub, sub)
		api.WriteError(w, errors.New("user's sub doesn't match update sub"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserBySub(ctx, sub, false)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u.PubKeys = append(u.PubKeys, pk)
	err = api.staticDB.UserSave(ctx, u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	err = api.staticDB.DeleteUnconfirmedUserUpdate(ctx, chID)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.loginUser(w, u, true)
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

// userConfirmGET validates the given confirmation token and confirms that the
// account under which this token was issued really owns the email address to
// which this token was sent.
// The user doesn't need to be logged in.
func (api *API) userConfirmGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	token := req.Form.Get("token")
	u, err := api.staticDB.UserConfirmEmail(req.Context(), token)
	if errors.Contains(err, database.ErrInvalidToken) || errors.Contains(err, database.ErrUserNotFound) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.loginUser(w, u, false)
}

// userReconfirmPOST allows the user to request a new email address confirmation
// email, in case the previous one didn't arrive for some reason.
// The user needs to be logged in.
func (api *API) userReconfirmPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
	u.EmailConfirmationTokenExpiration = time.Now().UTC().Add(database.EmailConfirmationTokenTTL).Truncate(time.Millisecond)
	u.EmailConfirmationToken, err = lib.GenerateUUID()
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to generate a token"), http.StatusInternalServerError)
		return
	}
	err = api.staticDB.UserSave(req.Context(), u)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to generate a new confirmation token"), http.StatusInternalServerError)
		return
	}
	err = api.staticMailer.SendAddressConfirmationEmail(req.Context(), u.Email, u.EmailConfirmationToken)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to send the new confirmation token"), http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userRecoverRequestPOST allows the user to request an account recovery. This
// creates a password-reset token that allows the user to change their password
// without logging in.
// The user doesn't need to be logged in.
func (api *API) userRecoverRequestPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	email := req.PostFormValue("email")
	if email == "" {
		api.WriteError(w, errors.New("missing required parameter 'email'"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByEmail(req.Context(), email)
	if errors.Contains(err, database.ErrUserNotFound) {
		// Someone tried to recover an account with an email that's not in our
		// database. It's possible that this is a user who forgot which email
		// they used when they signed up. Email them, so they know.
		errSend := api.staticMailer.SendAccountAccessAttemptedEmail(req.Context(), email)
		if errSend != nil {
			api.staticLogger.Warningln(errors.AddContext(err, "failed to send an email"))
		}
		// We don't want to give a potential attacker information about the
		// emails in our database, so we will respond that we've sent the email.
		// If they used the wrong email, they will get an email that indicates
		// that, otherwise they will get nothing.
		api.WriteSuccess(w)
		return
	}
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to fetch the user with this email"), http.StatusInternalServerError)
		return
	}
	// Verify that the user's email is confirmed.
	if u.EmailConfirmationToken != "" {
		api.WriteError(w, errors.New("user's email is not confirmed. it cannot be used for account recovery"), http.StatusBadRequest)
		return
	}
	// Generate a new recovery token and add it to the user's account.
	u.RecoveryToken, err = lib.GenerateUUID()
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to generate a token"), http.StatusInternalServerError)
		return
	}
	err = api.staticDB.UserSave(req.Context(), u)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to create a token"), http.StatusInternalServerError)
		return
	}
	// Send the token to the user via an email.
	err = api.staticMailer.SendRecoverAccountEmail(req.Context(), u.Email, u.RecoveryToken)
	if err != nil {
		// The token was successfully generated and added to the user's account,
		// but we failed to send it to the user. We will try to remove it.
		u.RecoveryToken = ""
		if errRem := api.staticDB.UserSave(req.Context(), u); errRem != nil {
			api.WriteError(w, errors.AddContext(err, "failed to send recovery email. no token has been added to the account. please try again"), http.StatusInternalServerError)
			return
		}
		// We failed to remove the token we added. The user needs to be notified.
		api.WriteError(w, errors.AddContext(err, "failed to send recovery email. please try again"), http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userRecoverPOST allows the user to change their password without logging in.
// They need to provide a valid password-reset token.
// The user doesn't need to be logged in.
func (api *API) userRecoverPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Parse the request's body.
	var payload accountRecoveryDTO
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	err = json.Unmarshal(b, &payload)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	if payload.Password == "" || payload.ConfirmPassword == "" || payload.Token == "" {
		api.WriteError(w, errors.New("missing required parameter"), http.StatusBadRequest)
		return
	}
	if payload.Password != payload.ConfirmPassword {
		api.WriteError(w, errors.New("passwords don't match"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByRecoveryToken(req.Context(), payload.Token)
	if err != nil {
		api.WriteError(w, errors.New("no such user"), http.StatusBadRequest)
		return
	}
	passHash, err := hash.Generate(payload.Password)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to hash password"), http.StatusInternalServerError)
		return
	}
	u.PasswordHash = string(passHash)
	u.RecoveryToken = ""
	err = api.staticDB.UserSave(req.Context(), u)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to save password"), http.StatusInternalServerError)
		return
	}
	api.loginUser(w, u, false)
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
		// Queue the skylink to have its metadata fetched and updated in the DB.
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
	err = req.ParseForm()
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
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
		// Queue the skylink to have its metadata fetched. We do not specify a user
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

// userUploadsDELETE unpins all uploads of a skylink uploaded by the user.
func (api *API) userUploadsDELETE(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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
	startOfTime := time.Time{}
	numUploads, storageUsed, _, _, err := api.staticDB.UserUploadStats(ctx, u.ID, startOfTime)
	if err != nil {
		api.staticLogger.Debugln("Failed to get user's upload bandwidth used:", err)
		return
	}
	quota := database.UserLimits[u.Tier]
	quotaExceeded := numUploads > quota.MaxNumberUploads || storageUsed > quota.Storage
	if quotaExceeded != u.QuotaExceeded {
		u.QuotaExceeded = quotaExceeded
		err = api.staticDB.UserSave(ctx, u)
		if err != nil {
			api.staticLogger.Warnf("Failed to save user. User: %+v, err: %s", u, err.Error())
		}
		api.staticUserTierCache.Set(u)
	}
}

// wellKnownJwksGET returns our public JWKS, so people can use that to verify
// the authenticity of the JWT tokens we issue.
func (api *API) wellKnownJwksGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteJSON(w, jwt.AccountsPublicJWKS)
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
