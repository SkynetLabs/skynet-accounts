package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/user", validate(api.userHandlerGET))
	//api.staticRouter.PUT("/user", validate(api.userHandlerPUT))
	//api.staticRouter.POST("/track/upload/:skylink", validate(api.trackUploadHandler))
	//api.staticRouter.POST("/track/download/:skylink", validate(api.trackDownloadHandler))
}

// validate ensures that the user making the request has logged in.
func validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		t, err := tokenFromRequest(req)
		if err != nil {
			WriteError(w, err, http.StatusBadRequest)
			return
		}
		token, err := ValidateToken(t)
		if err != nil {
			WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := context.WithValue(req.Context(), ctxValue("token"), token)
		h(w, req.WithContext(ctx), ps)
	}
}
