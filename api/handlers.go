package api

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// userHandlerGET returns information about an existing user.
func (api *API) userHandlerGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
}

// userHandlerPOST creates a new user.
func (api *API) userHandlerPOST(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// TODO Implement
	WriteJSON(w, struct{ msg string }{"Not implemented."})
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
