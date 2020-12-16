package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/user", validate(api.userHandler))
	//api.staticRouter.PUT("/user", validate(api.userHandlerPUT))
	api.staticRouter.GET("/user/uploads", validate(api.userUploadsHandler))
	api.staticRouter.GET("/user/downloads", validate(api.userDownloadsHandler))
	api.staticRouter.POST("/track/upload", validate(api.trackUploadHandler))
	api.staticRouter.POST("/track/download", validate(api.trackDownloadHandler))
}

// validate ensures that the user making the request has logged in.
func validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		tokenStr, err := tokenFromRequest(req)
		if err != nil {
			fmt.Println("error while fetching token from request", err)
			WriteError(w, err, http.StatusBadRequest)
			return
		}
		token, err := ValidateToken(tokenStr)
		if err != nil {
			fmt.Println("error while validating token", err)
			WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// If we don't have a valid cookie with reasonably long remaining TTL
		// then set one.
		c, err := req.Cookie(CookieName)
		exp, _ := TokenExpiration(token)
		if err != nil || !c.Expires.Equal(time.Unix(exp, 0)) {
			err = writeCookie(w, tokenStr)
			if err != nil {
				logrus.Println("Failed to write cookie:", err)
			}
		}
		// Embed the verified token in the context of the request.
		ctx := context.WithValue(req.Context(), ctxValue("token"), token)
		h(w, req.WithContext(ctx), ps)
	}
}
