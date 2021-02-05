package api

import (
	"context"
	"net/http"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/user", api.validate(api.userHandler))
	//api.staticRouter.PUT("/user", validate(api.userHandlerPUT))
	api.staticRouter.GET("/user/uploads", api.validate(api.userUploadsHandler))
	api.staticRouter.GET("/user/downloads", api.validate(api.userDownloadsHandler))
	api.staticRouter.POST("/track/upload/:skylink", api.validate(api.trackUploadHandler))
	api.staticRouter.POST("/track/download/:skylink", api.validate(api.trackDownloadHandler))
}

// validate ensures that the user making the request has logged in.
func (api *API) validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		tokenStr, err := tokenFromRequest(req)
		if err != nil {
			api.staticLogger.Traceln("error while fetching token from request", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		api.staticLogger.Debugln(" >>> Token from request:", tokenStr)
		token, err := ValidateToken(api.staticLogger, tokenStr)
		if err != nil {
			api.staticLogger.Traceln(errors.AddContext(err, "error while validating token"))
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// If we don't have a valid cookie with reasonably long remaining TTL
		// then set one.
		c, err := req.Cookie(CookieName)
		exp, _ := tokenExpiration(token)
		if err != nil || !c.Expires.Equal(time.Unix(exp, 0)) {
			err = writeCookie(w, tokenStr, exp)
			if err != nil {
				logrus.Println("Failed to write cookie:", err)
			}
		}
		// Embed the verified token in the context of the request.
		ctx := context.WithValue(req.Context(), ctxValue("token"), token)
		h(w, req.WithContext(ctx), ps)
	}
}
