package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// MaxMultipartMem defines the maximum amount of memory to be used for
	// parsing the request's multipart form. In bytes.
	MaxMultipartMem = 64_000_000
)

// userHandlerGET returns information about an existing user.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Limit to the user themselves.
	id := ps.ByName("id")
	u, err := api.staticDB.UserByID(req.Context(), id)
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
	email := (database.Email)(req.PostFormValue("email"))
	if !email.Validate() {
		WriteError(w, database.ErrInvalidEmail, http.StatusBadRequest)
		return
	}
	u := &database.User{
		FirstName: req.PostFormValue("firstName"),
		LastName:  req.PostFormValue("lastName"),
		Email:     email,
	}
	err := api.staticDB.UserCreate(req.Context(), u)
	if err != nil {
		WriteError(w, errors.AddContext(err, "failed to create user"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	WriteJSON(w, u)
}

// userHandlerPUT updates an existing user.
func (api *API) userHandlerPUT(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := req.ParseMultipartForm(MaxMultipartMem)
	if err != nil {
		WriteError(w, errors.New("Failed to parse multipart parameters."), http.StatusBadRequest)
		return
	}
	var u *database.User
	// Fetch the user by their _id.
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
		email := (database.Email)(req.PostFormValue("email"))
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
