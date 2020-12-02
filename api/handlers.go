package api

import (
	"net/http"
	"reflect"

	"github.com/dgrijalva/jwt-go"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// ErrAccountUnconfirmed is returned when a user with an unconfirmed account
	// tried to log in.
	ErrAccountUnconfirmed = errors.New("Unconfirmed.")
)

// userHandlerGET returns information about an existing user.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	uid, _, _, err := jwtToken(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserByID(req.Context(), uid)
	if errors.Contains(err, database.ErrUserNotFound) {
		WriteError(w, database.ErrUserNotFound, http.StatusNotFound)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	WriteJSON(w, u)
}

// userHandlerPOST creates a new user.
func (api *API) userHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	email, err := database.NewEmail(req.PostFormValue("email"))
	if err != nil {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	pw := req.PostFormValue("password")
	if len(pw) == 0 {
		WriteError(w, errors.New("The password cannot be empty."), http.StatusBadRequest)
		return
	}
	u := &database.User{
		FirstName: req.PostFormValue("firstName"),
		LastName:  req.PostFormValue("lastName"),
		Email:     email,
		Tier:      database.TierFree,
	}
	err = u.SetPassword(pw)
	if err != nil {
		WriteError(w, errors.AddContext(err, "failed to set password"), http.StatusInternalServerError)
		return
	}
	err = api.staticDB.UserCreate(req.Context(), u)
	if err != nil {
		WriteError(w, errors.AddContext(err, "failed to create user"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	WriteJSON(w, u)
}

// userHandlerPUT updates an existing user.
func (api *API) userHandlerPUT(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	uid, _, _, err := jwtToken(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	// Fetch the user by their id.
	u, err := api.staticDB.UserByID(req.Context(), uid)
	if err != nil {
		WriteError(w, errors.AddContext(err, "failed to fetch user"), http.StatusInternalServerError)
		return
	}
	// Get the changes values.
	if fn := req.PostFormValue("firstName"); fn != "" {
		u.FirstName = fn
	}
	if ln := req.PostFormValue("lastName"); ln != "" {
		u.LastName = ln
	}
	if em := req.PostFormValue("email"); em != "" {
		// No need for extra validation here, the email will be validated in
		// the update method before any work is done.
		u.Email = database.Email(em)
	}
	// Persist the changes.
	err = api.staticDB.UserUpdate(req.Context(), u)
	if errors.Contains(err, database.ErrInvalidEmail) || errors.Contains(err, database.ErrEmailAlreadyUsed) {
		WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	WriteJSON(w, u)
}

// userChangePasswordHandler changes a user's password, given the old one is known.
func (api *API) userChangePasswordHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	uid, _, _, err := jwtToken(req)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	oldPass := req.PostFormValue("oldPassword")
	newPass := req.PostFormValue("newPassword")
	if oldPass == "" || newPass == "" {
		WriteError(w, errors.New("Both `oldPassword` and `newPassword` are required."), http.StatusBadRequest)
		return
	}
	err = api.staticDB.UserUpdatePassword(req.Context(), uid, oldPass, newPass)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	WriteSuccess(w)
}

// userLoginHandler starts a new session for a user.
func (api *API) userLoginHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	email, err := database.NewEmail(req.PostFormValue("email"))
	if err != nil {
		WriteError(w, database.ErrInvalidEmail, http.StatusBadRequest)
		return
	}
	pw := req.PostFormValue("password")
	if len(pw) == 0 {
		WriteError(w, errors.New("The password cannot be empty."), http.StatusBadRequest)
		return
	}
	u, err := api.staticDB.UserByEmail(req.Context(), email)
	if err != nil {
		// TODO Consider logging this with a unique ID and returning only the ID
		// 	and a vague description. Maybe for all endpoints.
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if u.Tier == database.TierUnconfirmed {
		// TODO This error should be picked up by the FE and it should offer to
		// 	resend the account confirmation email.
		WriteError(w, ErrAccountUnconfirmed, http.StatusUnauthorized)
		return
	}
	err = u.VerifyPassword(pw)
	if err != nil {
		WriteError(w, errors.New("Bad username or password."), http.StatusUnauthorized)
		return
	}
	token, err := IssueToken(u)
	if err != nil {
		// TODO WriteError doesn't set the response's error message properly.
		WriteError(w, err, http.StatusUnprocessableEntity)
		return
	}
	if err = writeJWTCookie(w, token); err != nil {
		log.Println("Failed to write cookie:", err)
	}
	w.WriteHeader(http.StatusOK)
	WriteJSON(w, token)
}

// passwordResetRequestHandler starts the password recovery routine.
// This involves sending an email with a password reset link to the user.
func (api *API) passwordResetRequestHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userPasswordResetVerifyHandler verifies that a password recovery request is
// legitimate by verifying that the provided secret matches the sent one and
// also that the code hasn't expired.
func (api *API) userPasswordResetVerifyHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// passwordResetCompleteHandler completes the password recovery routine by
// changing the user's password to the newly provided one, given that the
// provided recovery code is valid and unused. It also marks the recovery code
// as used.
func (api *API) passwordResetCompleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// jwtToken is a helper function that extracts the JWT token from the context
// and returns the contained user id, claims and the token itself.
func jwtToken(req *http.Request) (id string, claims jwt.MapClaims, token *jwt.Token, err error) {
	t, ok := req.Context().Value(ctxValue("token")).(*jwt.Token)
	if !ok {
		err = errors.New("failed to get token")
		return
	}
	if reflect.ValueOf(t.Claims).Kind() != reflect.ValueOf(jwt.MapClaims{}).Kind() {
		err = errors.New("the token does not contain the claims we expect")
		return
	}
	claims = t.Claims.(jwt.MapClaims)
	if reflect.ValueOf(claims["user_id"]).Kind() != reflect.String {
		err = errors.New("the token does not contain the user_id we expect")
	}
	id = claims["user_id"].(string)
	token = t
	return
}

// writeJWTCookie is a helper function that writes the given JWT token as a
// secure cookie.
func writeJWTCookie(w http.ResponseWriter, token string) error {
	cookieVal := map[string]string{"token": token}
	encoded, err := secureCookie().Encode(CookieName, cookieVal)
	if err != nil {
		return err
	}
	cookie := &http.Cookie{
		Name:  CookieName,
		Value: encoded,
		Path:  "/",
	}
	http.SetCookie(w, cookie)
	return nil
}
