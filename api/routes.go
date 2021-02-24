package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.POST("/login", api.loginHandler)
	api.staticRouter.POST("/logout", api.validate(api.logoutHandler))

	api.staticRouter.POST("/track/upload/:skylink", api.validate(api.trackUploadHandler))
	api.staticRouter.POST("/track/download/:skylink", api.validate(api.trackDownloadHandler))
	api.staticRouter.POST("/track/registry/read", api.validate(api.trackRegistryReadHandler))
	api.staticRouter.POST("/track/registry/write", api.validate(api.trackRegistryWriteHandler))

	api.staticRouter.GET("/user", api.validate(api.userHandler))
	api.staticRouter.PUT("/user", api.validate(api.userPutHandler))
	api.staticRouter.GET("/user/stats", api.validate(api.userStatsHandler))
	api.staticRouter.GET("/user/uploads", api.validate(api.userUploadsHandler))
	api.staticRouter.GET("/user/downloads", api.validate(api.userDownloadsHandler))

	api.staticRouter.POST("/stripe/checkout/success", api.validate(api.stripeCheckoutHandler))
	api.staticRouter.POST("/stripe/webhook", api.stripeWebhookHandler)
}

// validate ensures that the user making the request has logged in.
func (api *API) validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.staticLogger.Tracef("Processing request: %+v", req)
		tokenStr, err := tokenFromRequest(req)
		if err != nil {
			api.staticLogger.Traceln("Error fetching token from request:", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		token, err := ValidateToken(api.staticLogger, tokenStr)
		if err != nil {
			api.staticLogger.Traceln("Error validating token:", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := context.WithValue(req.Context(), ctxValue("token"), token)
		h(w, req.WithContext(ctx), ps)
	}
}
