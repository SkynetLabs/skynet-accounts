package api

import (
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/health", api.noValidate(api.healthGET))
	api.staticRouter.GET("/limits", api.noValidate(api.limitsGET))

	api.staticRouter.GET("/login", api.WithDBSession(api.noValidate(api.loginGET)))
	api.staticRouter.POST("/login", api.WithDBSession(api.noValidate(api.loginPOST)))
	api.staticRouter.POST("/logout", api.WithDBSession(api.validate(api.logoutPOST)))
	api.staticRouter.GET("/register", api.WithDBSession(api.noValidate(api.registerGET)))
	api.staticRouter.POST("/register", api.WithDBSession(api.noValidate(api.registerPOST)))

	// Endpoints at which Nginx reports portal usage.
	api.staticRouter.POST("/track/upload/:skylink", api.WithDBSession(api.validate(api.trackUploadPOST)))
	api.staticRouter.POST("/track/download/:skylink", api.WithDBSession(api.validate(api.trackDownloadPOST)))
	api.staticRouter.POST("/track/registry/read", api.WithDBSession(api.validate(api.trackRegistryReadPOST)))
	api.staticRouter.POST("/track/registry/write", api.WithDBSession(api.validate(api.trackRegistryWritePOST)))

	api.staticRouter.POST("/user", api.WithDBSession(api.noValidate(api.userPOST))) // This will be removed in the future.
	api.staticRouter.GET("/user", api.WithDBSession(api.validate(api.userGET)))
	api.staticRouter.PUT("/user", api.WithDBSession(api.validate(api.userPUT)))
	api.staticRouter.DELETE("/user", api.WithDBSession(api.validate(api.userDELETE)))
	api.staticRouter.GET("/user/limits", api.noValidate(api.userLimitsGET))
	api.staticRouter.GET("/user/stats", api.validate(api.userStatsGET))
	api.staticRouter.GET("/user/addpubkey", api.validate(api.userAddPubKeyGET))
	api.staticRouter.POST("/user/addpubkey", api.validate(api.userAddPubKeyPOST))
	api.staticRouter.GET("/user/uploads", api.WithDBSession(api.validate(api.userUploadsGET)))
	api.staticRouter.DELETE("/user/uploads/:skylink", api.WithDBSession(api.validate(api.userUploadsDELETE)))
	api.staticRouter.GET("/user/downloads", api.WithDBSession(api.validate(api.userDownloadsGET)))

	// Endpoints for email communication with the user.
	api.staticRouter.GET("/user/confirm", api.WithDBSession(api.noValidate(api.userConfirmGET)))
	api.staticRouter.POST("/user/reconfirm", api.WithDBSession(api.validate(api.userReconfirmPOST)))
	api.staticRouter.GET("/user/recover", api.WithDBSession(api.noValidate(api.userRecoverGET)))
	api.staticRouter.POST("/user/recover", api.WithDBSession(api.noValidate(api.userRecoverPOST)))

	api.staticRouter.POST("/stripe/webhook", api.WithDBSession(api.noValidate(api.stripeWebhookPOST)))
	api.staticRouter.GET("/stripe/prices", api.noValidate(api.stripePricesGET))

	api.staticRouter.GET("/.well-known/jwks.json", api.noValidate(api.wellKnownJwksGET))
}

// noValidate is a pass-through method used for decorating the request and
// logging relevant data.
func (api *API) noValidate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.logRequest(req)
		h(w, req, ps)
	}
}

// validate ensures that the user making the request has logged in.
func (api *API) validate(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.logRequest(req)
		tokenStr, err := tokenFromRequest(req)
		if err != nil {
			api.staticLogger.Debugln("Error fetching token from request:", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		token, err := jwt.ValidateToken(tokenStr)
		if err != nil {
			api.staticLogger.Debugln("Error validating token:", err)
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := jwt.ContextWithToken(req.Context(), token)
		h(w, req.WithContext(ctx), ps)
	}
}

// logRequest logs information about the current request.
func (api *API) logRequest(r *http.Request) {
	hasAuth := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer")
	c, err := r.Cookie(CookieName)
	hasCookie := err == nil && c != nil
	api.staticLogger.Tracef("Processing request: %v %v, Auth: %v, Skynet Cookie: %v, Referer: %v, Host: %v, RemoreAddr: %v", r.Method, r.URL, hasAuth, hasCookie, r.Referer(), r.Host, r.RemoteAddr)
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

// userFromRequest returns a user object based on the JWT within the request.
// Note that this method does not rely on a token being stored in the context.
func (api *API) userFromRequest(r *http.Request) *database.User {
	t, err := tokenFromRequest(r)
	if err != nil {
		return nil
	}
	token, err := jwt.ValidateToken(t)
	if err != nil {
		return nil
	}
	tokenMap, err := token.AsMap(r.Context())
	if err != nil {
		return nil
	}
	sub, exists := tokenMap["sub"]
	if !exists {
		return nil
	}
	u, err := api.staticDB.UserBySub(r.Context(), sub.(string), false)
	if err != nil {
		return nil
	}
	return u
}
