package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/user"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// userHandlerGET returns information about an existing user.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Limit to the user themselves.
	if err := req.ParseForm(); err != nil {
		WriteError(w, errors.New("Failed to parse parameters."), http.StatusBadRequest)
	}
	email := req.Form.Get("email")
	if email == "" {
		WriteError(w, errors.New("No email provided."), http.StatusBadRequest)
		return
	}
	users, err := api.DB.UserFindAllByField(req.Context(), "email", email)
	if err != nil && err != database.ErrUserNotFound {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if errors.Contains(err, database.ErrUserNotFound) || len(users) == 0 {
		WriteError(w, database.ErrUserNotFound, http.StatusNotFound)
		return
	}
	WriteJSON(w, users[0])
}

// userHandlerPOST creates a new user.
func (api *API) userHandlerPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if err := req.ParseMultipartForm(64 * 1_000_000); err != nil {
		WriteError(w, errors.New("Failed to parse multipart parameters."), http.StatusBadRequest)
		return
	}
	email := (user.Email)(req.PostFormValue("email"))
	if !email.Validate() {
		WriteError(w, user.ErrInvalidEmail, http.StatusBadRequest)
		return
	}
	user := &user.User{
		FirstName: req.PostFormValue("firstName"),
		LastName:  req.PostFormValue("lastName"),
		Email:     email,
	}
	ins, err := api.DB.UserSave(req.Context(), user)
	if err != nil {
		WriteError(w, errors.AddContext(err, "failed to save user"), http.StatusInternalServerError)
		return
	}
	if ins {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	WriteJSON(w, user)
}

// userHandlerPUT updates an existing user.
func (api *API) userHandlerPUT(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userChangePasswordHandler changes a user's password, given the old one is known.
func (api *API) userChangePasswordHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userLoginHandler starts a new session for a user.
func (api *API) userLoginHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	fmt.Println("User login.")
	//req.ParseForm()
	//req.Form.Set("async", "true")
	//api.renterDownloadHandler(w, req, ps)
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userPasswordResetRequestHandler starts the password recovery routine.
// This involves sending an email with a password reset link to the user.
func (api *API) userPasswordResetRequestHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
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

// userPasswordResetCompleteHandler completes the password recovery routine by
// changing the user's password to the newly provided one, given that the
// provided recovery code is valid and unused. It also marks the recovery code
// as used.
func (api *API) userPasswordResetCompleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}
