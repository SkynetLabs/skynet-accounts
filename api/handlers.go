package api

import (
	"context"
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
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

	// userUpdateData defines the fields of the User record that can be changed
	// externally, e.g. by calling `PUT /user`.
	userUpdateData struct {
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

// loginPOST starts a user session by issuing a cookie
func (api *API) loginPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Since we don't want to have separate endpoints for logging in with
	// credentials and token, we'll do both here.
	//
	// Check whether credentials are provided. Those trump the token because a
	// user with a valid token might want to relog. No need to force them to
	// log out first.
	email := req.PostFormValue("email")
	pw := req.PostFormValue("password")
	if email != "" && pw != "" {
		api.loginPOSTCredentials(w, req, email, pw)
		return
	}
	// In case credentials were not found try to log the user by detecting a
	// token.
	api.loginPOSTToken(w, req)
}

// loginPOSTCredentials is a helper that handles logins with credentials.
func (api *API) loginPOSTCredentials(w http.ResponseWriter, req *http.Request, email, password string) {
	// Fetch the user with that email, if they exist.
	u, err := api.staticDB.UserByEmail(req.Context(), email, false)
	if err != nil && !errors.Contains(err, database.ErrUserNotFound) {
		api.staticLogger.Tracef("Error fetching a user with email '%s': %+v\n", email, err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	// If the user is not in the DB there is a chance that it's in CockroachDB.
	// Try to fetch it from there and if it's there, create a version of it in
	// MongoDB. Then try to login against this new user record, as the hash of
	// the password will be moved from CockroachDB and fully usable.
	if errors.Contains(err, database.ErrUserNotFound) {
		cru, err := database.CockroachUserByEmail(api.staticCockroachDB, email)
		if err != nil {
			api.staticLogger.Warnf("Failed to fetch user from CockroachDB: %s", err)
			api.WriteError(w, database.ErrUserNotFound, http.StatusUnauthorized)
			return
		}
		// We need to generate an ID for the user because UserSave won't be able
		// to create one for us.
		var id primitive.ObjectID
		copy(id[:], fastrand.Bytes(12))
		u = &database.User{
			ID:           id,
			Email:        cru.Email,
			PasswordHash: cru.PassHash,
			Sub:          cru.Sub,
			CreatedAt:    cru.CreatedAt,
		}
		err = api.staticDB.UserSave(req.Context(), u)
		if err != nil {
			api.staticLogger.Warnf("Failed to move user from CockroachDB: %s", err)
			api.WriteError(w, database.ErrUserNotFound, http.StatusUnauthorized)
			return
		}
		api.staticLogger.Debugf("User with email %s moved from CockroachDB.", u.Email)
	}
	// Check if we somehow have an empty password hash and try to fill that.
	if u.PasswordHash == "" {
		cru, err := database.CockroachUserByEmail(api.staticCockroachDB, email)
		if err != nil {
			api.staticLogger.Warnf("Failed to fetch user from CockroachDB: %s", err)
			api.WriteError(w, database.ErrUserNotFound, http.StatusUnauthorized)
			return
		}
		u.PasswordHash = cru.PassHash
		err = api.staticDB.UserSave(req.Context(), u)
		if err != nil {
			api.staticLogger.Warnf("Failed to fill user password hash from CockroachDB: %s", err)
			api.WriteError(w, database.ErrUserNotFound, http.StatusUnauthorized)
			return
		}
		api.staticLogger.Debugf("User with email %s had their password hash filled from CockroachDB.", u.Email)
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
		api.staticLogger.Traceln("Error fetching token from request:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	token, err := jwt.ValidateToken(tokenStr)
	if err != nil {
		api.staticLogger.Traceln("Error validating token:", err)
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	// Write a secure cookie containing the JWT token of the user. This allows
	// us to verify the user's identity and permissions (i.e. tier) without
	// requesting their credentials or accessing the DB.
	err = writeCookie(w, tokenStr, token.Expiration().UTC().Unix())
	if err != nil {
		api.staticLogger.Traceln("Error writing cookie:", err)
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
		api.staticLogger.Tracef("Error creating a token for user: %+v\n", err)
		err = errors.AddContext(err, "failed to create a token for user")
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Write the JWT to an encrypted cookie.
	err = writeCookie(w, string(tkBytes), tk.Expiration().UTC().Unix())
	if err != nil {
		api.staticLogger.Traceln("Error writing cookie:", err)
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
		api.staticLogger.Debugln("Failed to get user details from JWT:", err)
	}
	if err == nil && email != u.Email {
		u.Email = email
		err = api.staticDB.UserSave(req.Context(), u)
		if err != nil {
			api.staticLogger.Debugln("Failed to update user in DB:", err)
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

// userPOST creates a new user.
func (api *API) userPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	email := req.PostFormValue("email")
	// Validate the email address.
	a, err := mail.ParseAddress(email)
	if err != nil {
		api.WriteError(w, errors.New("invalid email provided"), http.StatusBadRequest)
		return
	}
	// Strip any names from the email and leave just the address.
	email = a.Address
	pw := req.PostFormValue("password")
	if pw == "" {
		api.WriteError(w, errors.New("password is required"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserCreate(req.Context(), email, pw, "", database.TierFree)
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
	api.WriteJSON(w, u)
}

// userPUT allows changing some user information.
// This method receives its parameters as a JSON object.
func (api *API) userPUT(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := jwt.TokenFromContext(req.Context())
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
	defer func() { _ = req.Body.Close() }()
	var payload userUpdateData
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
	u, err := api.staticDB.UserBySub(req.Context(), sub, false)
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
		su, err := api.staticDB.UserByStripeID(req.Context(), payload.StripeID)
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
		eu, err := api.staticDB.UserByEmail(req.Context(), payload.Email, false)
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
	err = api.staticDB.UserSave(req.Context(), u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Send a confirmation email if the user's email address was changed.
	if changedEmail {
		err = api.staticMailer.SendAddressConfirmationEmail(req.Context(), u.Email, u.EmailConfirmationToken)
		if err != nil {
			api.staticLogger.Debugln(errors.AddContext(err, "failed to send address confirmation email"))
		}
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

// userRecoverGET allows the user to request an account recovery. This creates
// a password-reset token that allows the user to change their password without
// logging in.
// The user doesn't need to be logged in.
func (api *API) userRecoverGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	email := req.Form.Get("email")
	if email == "" {
		api.WriteError(w, errors.New("missing required parameter 'email'"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByEmail(req.Context(), email, false)
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
		// The token was successfully generated and added to the user's account
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
	if err := req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	recoveryToken := req.Form.Get("token")
	password := req.Form.Get("password")
	confirmPassword := req.Form.Get("confirmPassword")
	if recoveryToken == "" || password == "" || confirmPassword == "" {
		api.WriteError(w, errors.New("missing required parameter"), http.StatusBadRequest)
		return
	}
	if password != confirmPassword {
		api.WriteError(w, errors.New("passwords don't match"), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByRecoveryToken(req.Context(), recoveryToken)
	if err != nil {
		api.WriteError(w, errors.New("no such user"), http.StatusBadRequest)
		return
	}
	passHash, err := hash.Generate(password)
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
		// Queue the skylink to have its meta data fetched and updated in the DB.
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
		// Queue the skylink to have its meta data fetched. We do not specify a user
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
			api.staticLogger.Infof("Failed to save user. User: %+v, err: %s", u, err.Error())
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
