package api

import (
	"net/http"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/jwt"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
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

	api.staticRouter.POST("/stripe/webhook", api.stripeWebhookHandler)
	api.staticRouter.GET("/stripe/prices", api.stripePricesHandler)
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
		token, err := jwt.ValidateToken(api.staticLogger, tokenStr)
		if err != nil {
			api.staticLogger.Traceln("Error validating token:", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := jwt.ContextWithToken(req.Context(), token)
		h(w, req.WithContext(ctx), ps)
	}
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// It first checks the request headers and then the cookies.
func tokenFromRequest(r *http.Request) (string, error) {
	// Check the headers for a token.
	authHeader := r.Header.Get("Authorization")
	parts := strings.Split(authHeader, "Bearer")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), nil
	}
	// Check the cookie for a token.
	cookie, err := r.Cookie(CookieName)
	if errors.Contains(err, http.ErrNoCookie) {
		return "", errors.New("no cookie found")
	}
	if err != nil {
		return "", errors.AddContext(err, "cookie exists but it's not valid")
	}
	var value string
	err = secureCookie.Decode(CookieName, cookie.Value, &value)
	if err != nil {
		return "", err
	}
	return value, nil
}
