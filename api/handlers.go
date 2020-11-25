package api

import (
	"fmt"
	"net/http"

	"github.com/dgrijalva/jwt-go"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// MaxMultipartMem defines the maximum amount of memory to be used for
	// parsing the request's multipart form. In bytes.
	MaxMultipartMem = 64_000_000
)

var (
	// ErrAccountUnconfirmed is returned when a suer with an unconfirmed account
	// tried to log in.
	ErrAccountUnconfirmed = errors.New("Unconfirmed.")
)

// userHandlerGET returns information about an existing user.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ok, err := isSelf(req, ps)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if !ok {
		WriteError(w, errors.New("you cannot access other users' info"), http.StatusBadRequest)
		return
	}

	u, err := api.staticDB.UserByID(req.Context(), ps.ByName("id"))
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
func (api *API) userHandlerPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if err := req.ParseMultipartForm(MaxMultipartMem); err != nil {
		WriteError(w, errors.New("Failed to parse multipart parameters."), http.StatusBadRequest)
		return
	}
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
	u := &database.User{
		FirstName: req.PostFormValue("firstName"),
		LastName:  req.PostFormValue("lastName"),
		Email:     email,
		Tier:      database.TierUnconfirmed,
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
func (api *API) userHandlerPUT(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ok, err := isSelf(req, ps)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if !ok {
		WriteError(w, errors.New("you cannot access other users' info"), http.StatusBadRequest)
		return
	}

	err = req.ParseMultipartForm(MaxMultipartMem)
	if err != nil {
		WriteError(w, errors.New("Failed to parse multipart parameters."), http.StatusBadRequest)
		return
	}
	var u *database.User
	// Fetch the user by their id. That is represented by the `_id` key because
	// that is the naming Mongo uses.
	if id := req.PostFormValue("_id"); id != "" {
		u, err = api.staticDB.UserByID(req.Context(), id)
		if err != nil {
			// This is a Bad Request and not an Internal Server Error because
			// the client has supplied an invalid `_id`.
			WriteError(w, errors.AddContext(err, "failed to fetch user"), http.StatusBadRequest)
			return
		}
	}
	// Fetch the user by their email.
	if u == nil {
		email, err := database.NewEmail(req.PostFormValue("email"))
		if err != nil {
			WriteError(w, errors.AddContext(err, "invalid email provided"), http.StatusBadRequest)
			return
		}
		u, err = api.staticDB.UserByEmail(req.Context(), email)
		if err != nil {
			WriteError(w, errors.AddContext(err, "failed to fetch user"), http.StatusBadRequest)
			return
		}
	}
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
func (api *API) userChangePasswordHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userLoginHandler starts a new session for a user.
func (api *API) userLoginHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if err := req.ParseMultipartForm(MaxMultipartMem); err != nil {
		WriteError(w, errors.New("Failed to parse multipart parameters."), http.StatusBadRequest)
		return
	}
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
		fmt.Println(err)
		// TODO WriteError doesn't set the response's error message properly. Or Postman doesn't read it properly?
		WriteError(w, err, http.StatusUnprocessableEntity)
		return
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

// isSelf is a helper function that tells us if the authenticated user is the
// same as the user whose id is being used in the route path.
func isSelf(req *http.Request, ps httprouter.Params) (bool, error) {
	token, ok := req.Context().Value(ctxValue("token")).(*jwt.Token)
	if !ok {
		return false, errors.New("failed to get token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, errors.New("failed to get claims")
	}

	isSelf := ps.ByName("id") != claims["user_id"]
	return isSelf, nil
}
