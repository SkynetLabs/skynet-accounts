package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	// TODO Rate-limit these from the start to prevent brute force and other forms of abuse.
	api.staticRouter.POST("/login", api.userLoginHandler)
	api.staticRouter.POST("/user", (api.userHandlerPOST))
	api.staticRouter.GET("/user/:id", Validate(api.userHandlerGET))
	api.staticRouter.PUT("/user/:id", Validate(api.userHandlerPUT))
	api.staticRouter.POST("/user/:id/password", Validate(api.userChangePasswordHandler))
	//api.staticRouter.POST("/password/reset/request", api.passwordResetRequestHandler)
	//api.staticRouter.POST("/password/reset/verify", api.passwordResetCompleteHandler)
	//api.staticRouter.POST("/password/reset/complete", api.passwordResetCompleteHandler)
}

// Validate ensures that the user making the request has logged in.
func Validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		token, err := ValidateToken(extractToken(req))
		if err != nil {
			WriteError(w, errors.New("Unauthorized"), http.StatusUnauthorized)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := context.WithValue(req.Context(), ctxValue("token"), token)
		h(w, req.Clone(ctx), ps)
	}
}
