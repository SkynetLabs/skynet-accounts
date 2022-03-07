package api

import (
	"context"
	"encoding/json"
	"io"
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
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// LimitBodySizeSmall defines a size limit for requests that we don't expect
	// to contain a lot of data.
	LimitBodySizeSmall = 4 * skynet.KiB
)

type (
	// ChallengePublic is the response of GET /login, GET /register,
	// GET /user/pubkey/register
	ChallengePublic struct {
		// Challenge is a hex-encoded representation of the []byte challenge.
		Challenge string `bson:"challenge" json:"challenge"`
	}
	// DownloadsGET is the response of GET /user/downloads
	DownloadsGET struct {
		Items    []database.DownloadResponse `json:"items"`
		Offset   int                         `json:"offset"`
		PageSize int                         `json:"pageSize"`
		Count    int                         `json:"count"`
	}
	// HealthGET is the response type of GET /health
	HealthGET struct {
		DBAlive bool `json:"dbAlive"`
	}
	// LimitsGET provides public information of the various limits this
	// portal has.
	// This is the response of GET /limits
	LimitsGET struct {
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
	// UploadsGET is the response of GET /user/uploads
	UploadsGET struct {
		Items    []database.UploadResponse `json:"items"`
		Offset   int                       `json:"offset"`
		PageSize int                       `json:"pageSize"`
		Count    int                       `json:"count"`
	}
	// UserGET defines a representation of the User struct returned by all
	// handlers. This allows us to tweak the fields of the struct before
	// returning it.
	UserGET struct {
		database.User
		EmailConfirmed bool `json:"emailConfirmed"`
	}
	// UserLimitsGET is response of GET /user/limits
	UserLimitsGET struct {
		TierID int `json:"tierID"`
		database.TierLimits
	}

	// accountRecoveryPOST defines the payload we expect when a user is trying
	// to change their password.
	accountRecoveryPOST struct {
		Token           string `json:"token"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	// credentialsPOST defines the standard credentials package we expect.
	credentialsPOST struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// userUpdatePUT defines the fields of the User record that can be changed
	// externally, e.g. by calling `PUT /user`.
	userUpdatePUT struct {
		Email    string `json:"email,omitempty"`
		Password string `json:"password,omitempty"`
		StripeID string `json:"stripeCustomerId,omitempty"`
	}
)

// healthGET returns the status of the service
func (api *API) healthGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var status HealthGET
	err := api.staticDB.Ping(req.Context())
	status.DBAlive = err == nil
	api.WriteJSON(w, status)
}

// limitsGET returns the speed limits of this portal.
func (api *API) limitsGET(_ *database.User, w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	resp := LimitsGET{
		UserLimits: api.staticTierLimits,
	}
	api.WriteJSON(w, resp)
}

// loginGET generates a login challenge for the caller.
func (api *API) loginGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var pk database.PubKey
	err := pk.LoadString(req.FormValue("pubKey"))
	if err != nil {
		api.WriteError(w, database.ErrInvalidPublicKey, http.StatusBadRequest)
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
	api.WriteJSON(w, ChallengePublic{ch.Challenge})
}

// loginPOST starts a user session by issuing a cookie
func (api *API) loginPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the body, we might need to use it several times.
	body, err := ioutil.ReadAll(io.LimitReader(req.Body, LimitBodySizeSmall))
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to read request body"), http.StatusBadRequest)
		return
	}

	// Since we don't want to have separate endpoints for logging in with
	// credentials and token, we'll do both here.
	//
	// Check whether credentials are provided. Those trump the token because a
	// user with a valid token might want to relog. No need to force them to
	// log out first.
	var payload credentialsPOST
	err = json.Unmarshal(body, &payload)
	if err == nil && payload.Email != "" && payload.Password != "" {
		api.loginPOSTCredentials(w, req, payload.Email, payload.Password)
		return
	}

	// Check for a challenge response in the request's body.
	var chr database.ChallengeResponse
	err = chr.LoadFromBytes(body)
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
	token, err := tokenFromRequest(req)
	if err != nil {
		api.staticLogger.Debugln("Error fetching token from request:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	tokenBytes, err := jwt.TokenSerialize(token)
	if err != nil {
		api.staticLogger.Debugln("Error serializing token:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Write a secure cookie containing the JWT token of the user. This allows
	// us to verify the user's identity and permissions (i.e. tier) without
	// requesting their credentials or accessing the DB.
	err = writeCookie(w, string(tokenBytes), token.Expiration().UTC().Unix())
	if err != nil {
		api.staticLogger.Debugln("Error writing cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Skynet-Token", string(tokenBytes))
	api.WriteSuccess(w)
}

// loginUser is a helper method that generates a JWT for the user and writes the
// login cookie.
func (api *API) loginUser(w http.ResponseWriter, u *database.User, returnUser bool) {
	// Generate a JWT.
	tk, err := jwt.TokenForUser(u.Email, u.Sub)
	if err != nil {
		api.staticLogger.Debugf("Error creating a token for user: %+v\n", err)
		err = errors.AddContext(err, "failed to create a token for user")
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	tkBytes, err := jwt.TokenSerialize(tk)
	if err != nil {
		api.staticLogger.Debugln("Failed to serialize token:", err)
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
	w.Header().Set("Skynet-Token", string(tkBytes))
	if returnUser {
		api.WriteJSON(w, UserGETFromUser(u))
	} else {
		api.WriteSuccess(w)
	}
}

// logoutPOST ends a user session by removing a cookie
func (api *API) logoutPOST(_ *database.User, w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// Remove the user's cookie. We achieve that by overwriting the cookie with
	// a new one, which has its expiration time in the past. The browser will
	// remove it for us.
	err := writeCookie(w, "", time.Now().UTC().Unix()-1)
	if err != nil {
		api.staticLogger.Debugln("Error deleting cookie:", err)
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// registerGET generates a registration challenge for the caller.
func (api *API) registerGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Check if the registrations are open.
	val, err := api.staticDB.ReadConfigValue(req.Context(), database.ConfValRegistrationsDisabled)
	if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
		return
	}
	if val == database.ConfValTrue {
		api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
		return
	}
	var pk database.PubKey
	err = pk.LoadString(req.FormValue("pubKey"))
	if err != nil {
		api.WriteError(w, database.ErrInvalidPublicKey, http.StatusBadRequest)
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
	api.WriteJSON(w, ChallengePublic{ch.Challenge})
}

// registerPOST registers a new user based on a challenge-response.
func (api *API) registerPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Check if the registrations are open.
	val, err := api.staticDB.ReadConfigValue(req.Context(), database.ConfValRegistrationsDisabled)
	if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
		return
	}
	if val == database.ConfValTrue {
		api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
		return
	}
	// Get the body, we might need to use it several times.
	body, err := ioutil.ReadAll(io.LimitReader(req.Body, LimitBodySizeSmall))
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "empty request body"), http.StatusBadRequest)
		return
	}
	// Get the challenge response.
	var chr database.ChallengeResponse
	err = chr.LoadFromBytes(body)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "missing or invalid challenge response"), http.StatusBadRequest)
		return
	}
	// Parse the request's body.
	var payload credentialsPOST
	err = json.Unmarshal(body, &payload)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	parsed, err := mail.ParseAddress(payload.Email)
	if err != nil || payload.Email != parsed.Address {
		api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
		return
	}
	// The password is optional and that's why we do not verify it.
	ctx := req.Context()
	pk, _, err := api.staticDB.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeRegister)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to validate challenge response"), http.StatusBadRequest)
		return
	}
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
func (api *API) userGET(u *database.User, w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteJSON(w, UserGETFromUser(u))
}

// userLimitsGET returns the speed limits which apply to this user.
//
// NOTE: This handler needs to use the noAuth middleware in order to be able to
// optimise its calls to the DB and the use of caching.
func (api *API) userLimitsGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// First check for an API key.
	ak, err := apiKeyFromRequest(req)
	respAnon := UserLimitsGET{
		TierID:     database.TierAnonymous,
		TierLimits: database.UserLimits[database.TierAnonymous],
	}
	if err == nil {
		u, err := api.staticDB.UserByAPIKey(req.Context(), ak)
		if err != nil {
			api.staticLogger.Traceln("Error while fetching user by API key:", err)
			api.WriteJSON(w, respAnon)
			return
		}
		resp := UserLimitsGET{
			TierID:     u.Tier,
			TierLimits: database.UserLimits[u.Tier],
		}
		// If the quota is exceeded we should keep the user's tier but report
		// anonymous-level speeds.
		if u.QuotaExceeded {
			resp.TierLimits = database.UserLimits[database.TierAnonymous]
			// Keep the original tier name.
			resp.TierLimits.TierName = database.UserLimits[u.Tier].TierName
		}
		api.WriteJSON(w, resp)
		return
	}
	token, err := tokenFromRequest(req)
	if err != nil {
		api.WriteJSON(w, respAnon)
		return
	}
	s, exists := token.Get("sub")
	if !exists {
		api.staticLogger.Warnln("Token without a sub.")
		api.WriteJSON(w, respAnon)
		return
	}
	sub := s.(string)
	// If the user is not cached, or they were cached too long ago we'll fetch
	// their data from the DB.
	tier, qe, ok := api.staticUserTierCache.Get(sub)
	if !ok {
		u, err := api.staticDB.UserBySub(req.Context(), sub)
		if err != nil {
			api.staticLogger.Debugf("Failed to fetch user from DB for sub '%s'. Error: %s", sub, err.Error())
			api.WriteJSON(w, respAnon)
			return
		}
		api.staticUserTierCache.Set(u)
		// Populate the tier and qe values, while simultaneously making sure
		// that we can read the record from the cache.
		tier, qe, ok = api.staticUserTierCache.Get(sub)
		if !ok {
			build.Critical("Failed to fetch user from UserTierCache right after setting it.")
		}
	}
	resp := UserLimitsGET{
		TierID:     tier,
		TierLimits: database.UserLimits[tier],
	}
	// If the quota is exceeded we should keep the user's tier but report
	// anonymous-level speeds.
	if qe {
		resp.TierLimits = database.UserLimits[database.TierAnonymous]
		// Keep the original tier name.
		resp.TierLimits.TierName = database.UserLimits[tier].TierName
	}
	api.WriteJSON(w, resp)
}

// userStatsGET returns statistics about an existing user.
func (api *API) userStatsGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	us, err := api.staticDB.UserStats(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, us)
}

// userDELETE deletes the user and all of their data.
func (api *API) userDELETE(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := api.staticDB.UserDelete(req.Context(), u)
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
func (api *API) userPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Check if the registrations are open.
	val, err := api.staticDB.ReadConfigValue(req.Context(), database.ConfValRegistrationsDisabled)
	if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
		return
	}
	if val == database.ConfValTrue {
		api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
		return
	}
	// Parse the request's body.
	var payload credentialsPOST
	err = parseRequestBodyJSON(req.Body, LimitBodySizeSmall, &payload)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to parse request body"), http.StatusBadRequest)
		return
	}
	if payload.Email == "" {
		api.WriteError(w, errors.New("email is required"), http.StatusBadRequest)
		return
	}
	parsed, err := mail.ParseAddress(payload.Email)
	if err != nil || payload.Email != parsed.Address {
		api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
		return
	}
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
func (api *API) userPUT(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Read and parse the request body.
	var payload userUpdatePUT
	err := parseRequestBodyJSON(req.Body, LimitBodySizeSmall, &payload)
	if err != nil {
		err = errors.AddContext(err, "failed to parse request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if payload == (userUpdatePUT{}) {
		// The payload is empty, nothing to do.
		api.WriteError(w, errors.New("empty request"), http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	if payload.Password != "" {
		// Check if the registrations are open. If they are not then changing
		// passwords is also not allowed.
		val, err := api.staticDB.ReadConfigValue(ctx, database.ConfValRegistrationsDisabled)
		if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
			api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
			return
		}
		if val == database.ConfValTrue {
			api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
			return
		}

		pwHash, err := hash.Generate(payload.Password)
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to hash password"), http.StatusInternalServerError)
			return
		}
		u.PasswordHash = string(pwHash)
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
		if err == nil && su.Sub != u.Sub {
			err = errors.New("this stripe customer id belongs to another user")
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		// Set the StripeID.
		u.StripeID = payload.StripeID
	}

	var changedEmail bool
	if payload.Email != "" {
		parsed, err := mail.ParseAddress(payload.Email)
		if err != nil || payload.Email != parsed.Address {
			api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
			return
		}
		// Check if another user already has this email address.
		eu, err := api.staticDB.UserByEmail(ctx, payload.Email)
		if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		if err == nil && eu.Sub != u.Sub {
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
func (api *API) userPubKeyRegisterGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ctx := req.Context()
	var pk database.PubKey
	err := pk.LoadString(req.FormValue("pubKey"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	_, err = api.staticDB.UserByPubKey(ctx, pk)
	// Our happy case is getting database.ErrUserNotFound. Anything else is a
	// problem - either another user is using the pubkey or we failed to verify
	// that that is not the case.
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
		Sub:         u.Sub,
		ChallengeID: ch.ID,
		ExpiresAt:   ch.ExpiresAt.Truncate(time.Millisecond),
	}
	err = api.staticDB.StoreUnconfirmedUserUpdate(ctx, uu)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to store unconfirmed user update"), http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, ChallengePublic{ch.Challenge})
}

// userPubKeyRegisterPOST updates the user's pubKey based on a challenge-response.
func (api *API) userPubKeyRegisterPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ctx := req.Context()
	// Get the challenge response.
	var chr database.ChallengeResponse
	err := chr.LoadFromReader(io.LimitReader(req.Body, LimitBodySizeSmall))
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "missing or invalid challenge response"), http.StatusBadRequest)
		return
	}
	pk, chID, err := api.staticDB.ValidateChallengeResponse(ctx, chr, database.ChallengeTypeUpdate)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to validate challenge response"), http.StatusBadRequest)
		return
	}
	// Check if the pubkey is already associated with the current user.
	if u.HasKey(pk) {
		// This pubkey already belongs to the user. Log them in and return.
		api.loginUser(w, u, true)
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
	if err == nil && pku.Sub != u.Sub {
		api.WriteError(w, errors.New("this pubKey already belongs to another user"), http.StatusBadRequest)
		return
	}
	uu, err := api.staticDB.FetchUnconfirmedUserUpdate(ctx, chID)
	if err != nil {
		api.WriteError(w, errors.AddContext(err, "failed to fetch unconfirmed user update"), http.StatusInternalServerError)
		return
	}
	if uu.Sub != u.Sub {
		api.staticLogger.Warnf("Potential attempt to modify another user's pubKey. Sub of challenge requester '%s', sub of response submitter '%s'", uu.Sub, u.Sub)
		api.WriteError(w, errors.New("user's sub doesn't match update sub"), http.StatusBadRequest)
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
func (api *API) userUploadsGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err := errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ups, total, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	response := UploadsGET{
		Items:    ups,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// userDownloadsGET returns all downloads made by the current user.
func (api *API) userDownloadsGET(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err := errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	downs, total, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	response := DownloadsGET{
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
func (api *API) userConfirmGET(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
func (api *API) userReconfirmPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var err error
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
func (api *API) userRecoverRequestPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Check if the registrations are open. If they are not then account
	// recovery is also disabled.
	val, err := api.staticDB.ReadConfigValue(req.Context(), database.ConfValRegistrationsDisabled)
	if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
		return
	}
	if val == database.ConfValTrue {
		api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
		return
	}

	// Read and parse the request body.
	var payload struct {
		Email string `json:"email"`
	}
	err = parseRequestBodyJSON(req.Body, LimitBodySizeSmall, &payload)
	if err != nil {
		err = errors.AddContext(err, "failed to parse request body")
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if payload.Email == "" {
		api.WriteError(w, errors.New("missing required parameter 'email'"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByEmail(req.Context(), payload.Email)
	if errors.Contains(err, database.ErrUserNotFound) {
		// Someone tried to recover an account with an email that's not in our
		// database. It's possible that this is a user who forgot which email
		// they used when they signed up. Email them, so they know.
		errSend := api.staticMailer.SendAccountAccessAttemptedEmail(req.Context(), payload.Email)
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
func (api *API) userRecoverPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Check if the registrations are open. If they are not then account
	// recovery is also disabled.
	val, err := api.staticDB.ReadConfigValue(req.Context(), database.ConfValRegistrationsDisabled)
	if err != nil && !errors.Contains(err, mongo.ErrNoDocuments) {
		api.WriteError(w, errors.AddContext(err, "failed to read from configuration"), http.StatusInternalServerError)
		return
	}
	if val == database.ConfValTrue {
		api.WriteError(w, errors.New("registrations are currently disabled"), http.StatusNotImplemented)
		return
	}

	// Parse the request's body.
	var payload accountRecoveryPOST
	err = parseRequestBodyJSON(req.Body, LimitBodySizeSmall, &payload)
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
func (api *API) trackUploadPOST(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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
func (api *API) trackDownloadPOST(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := req.ParseForm()
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
func (api *API) trackRegistryReadPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	_, err := api.staticDB.RegistryReadCreate(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// trackRegistryWritePOST registers a new registry write in the system.
func (api *API) trackRegistryWritePOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	_, err := api.staticDB.RegistryWriteCreate(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userUploadsDELETE unpins all uploads of a skylink uploaded by the user.
func (api *API) userUploadsDELETE(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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

// wellKnownJWKSGET returns our public JWKS, so people can use that to verify
// the authenticity of the JWT tokens we issue.
func (api *API) wellKnownJWKSGET(_ *database.User, w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteJSON(w, jwt.AccountsPublicJWKS)
}

// UserGETFromUser converts a database.User struct to a UserGET struct.
func UserGETFromUser(u *database.User) *UserGET {
	if u == nil {
		return nil
	}
	return &UserGET{
		User:           *u,
		EmailConfirmed: u.EmailConfirmationToken == "",
	}
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

// parseRequestBodyJSON reads a limited portion of the body and decodes it into
// the given obj. The purpose of this is to prevent DoS attacks that rely on
// excessively large request bodies.
func parseRequestBodyJSON(body io.ReadCloser, maxBodySize int64, objRef interface{}) error {
	return json.NewDecoder(io.LimitReader(body, maxBodySize)).Decode(&objRef)
}
