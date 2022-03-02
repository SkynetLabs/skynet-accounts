package api

import (
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// APIKeyHeader holds the name of the header we use for API keys. This
	// header name matches the established standard used by Swagger and others.
	APIKeyHeader = "Skynet-API-Key"
	// ErrNoAPIKey is an error returned when we expect an API key but we don't
	// find one.
	ErrNoAPIKey = errors.New("no api key found")
	// ErrInvalidAPIKey is an error returned when the given API key is invalid.
	ErrInvalidAPIKey = errors.New("invalid api key")
	// ErrNoToken is returned when we expected a JWT token to be provided but it
	// was not.
	ErrNoToken = errors.New("no authorisation token found")
)

type (
	// HandlerWithUser is a wrapper for httprouter.Handle which also includes
	// a user parameter. This allows us to fetch the user making the request
	// just once, during validation.
	HandlerWithUser func(*database.User, http.ResponseWriter, *http.Request, httprouter.Params)
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/health", api.noAuth(api.healthGET))
	api.staticRouter.GET("/limits", api.noAuth(api.limitsGET))

	api.staticRouter.GET("/login", api.WithDBSession(api.noAuth(api.loginGET)))
	api.staticRouter.POST("/login", api.WithDBSession(api.noAuth(api.loginPOST)))
	api.staticRouter.POST("/logout", api.withAuth(api.logoutPOST))
	api.staticRouter.GET("/register", api.noAuth(api.registerGET))
	api.staticRouter.POST("/register", api.WithDBSession(api.noAuth(api.registerPOST)))

	// Endpoints at which Nginx reports portal usage.
	api.staticRouter.POST("/track/upload/:skylink", api.withAuth(api.trackUploadPOST))
	api.staticRouter.POST("/track/download/:skylink", api.withAuth(api.trackDownloadPOST))
	api.staticRouter.POST("/track/registry/read", api.withAuth(api.trackRegistryReadPOST))
	api.staticRouter.POST("/track/registry/write", api.withAuth(api.trackRegistryWritePOST))

	api.staticRouter.POST("/user", api.noAuth(api.userPOST)) // This will be removed in the future.
	api.staticRouter.GET("/user", api.withAuth(api.userGET))
	api.staticRouter.PUT("/user", api.WithDBSession(api.withAuth(api.userPUT)))
	api.staticRouter.DELETE("/user", api.withAuth(api.userDELETE))
	api.staticRouter.GET("/user/limits", api.noAuth(api.userLimitsGET))
	api.staticRouter.GET("/user/limits/:skylink", api.noAuth(api.userLimitsSkylinkGET))
	api.staticRouter.GET("/user/stats", api.withAuth(api.userStatsGET))
	api.staticRouter.GET("/user/pubkey/register", api.WithDBSession(api.withAuth(api.userPubKeyRegisterGET)))
	api.staticRouter.POST("/user/pubkey/register", api.WithDBSession(api.withAuth(api.userPubKeyRegisterPOST)))
	api.staticRouter.GET("/user/uploads", api.withAuth(api.userUploadsGET))
	api.staticRouter.DELETE("/user/uploads/:skylink", api.withAuth(api.userUploadsDELETE))
	api.staticRouter.GET("/user/downloads", api.withAuth(api.userDownloadsGET))

	// Endpoints for user API keys.
	api.staticRouter.POST("/user/apikeys", api.WithDBSession(api.withAuth(api.userAPIKeyPOST)))
	api.staticRouter.GET("/user/apikeys", api.withAuth(api.userAPIKeyGET))
	api.staticRouter.DELETE("/user/apikeys/:id", api.withAuth(api.userAPIKeyDELETE))

	// Endpoints for user public API keys.
	api.staticRouter.POST("/user/pubapikeys", api.WithDBSession(api.withAuth(api.userPubAPIKeyPOST)))
	api.staticRouter.GET("/user/pubapikeys", api.withAuth(api.userPubAPIKeyGET))
	api.staticRouter.PUT("/user/pubapikeys", api.WithDBSession(api.withAuth(api.userPubAPIKeyPUT)))
	api.staticRouter.PATCH("/user/pubapikeys", api.WithDBSession(api.withAuth(api.userPubAPIKeyPATCH)))
	api.staticRouter.DELETE("/user/pubapikeys/:id", api.withAuth(api.userPubAPIKeyDELETE))

	// Endpoints for email communication with the user.
	api.staticRouter.GET("/user/confirm", api.WithDBSession(api.noAuth(api.userConfirmGET))) // TODO POST
	api.staticRouter.POST("/user/reconfirm", api.WithDBSession(api.withAuth(api.userReconfirmPOST)))
	api.staticRouter.POST("/user/recover/request", api.WithDBSession(api.noAuth(api.userRecoverRequestPOST)))
	api.staticRouter.POST("/user/recover", api.WithDBSession(api.noAuth(api.userRecoverPOST)))

	api.staticRouter.POST("/stripe/webhook", api.WithDBSession(api.noAuth(api.stripeWebhookPOST)))
	api.staticRouter.GET("/stripe/prices", api.noAuth(api.stripePricesGET))

	api.staticRouter.GET("/.well-known/jwks.json", api.noAuth(api.wellKnownJWKSGET))
}

// noAuth is a pass-through method used for decorating the request and
// logging relevant data.
func (api *API) noAuth(h HandlerWithUser) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.logRequest(req)
		h(nil, w, req, ps)
	}
}

// withAuth ensures that the user making the request has logged in.
func (api *API) withAuth(h HandlerWithUser) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.logRequest(req)
		// Check for an API key.
		u, token, err := api.userAndTokenByAPIKey(req)
		// If there is an unexpected error, that is a 500.
		if err != nil && !errors.Contains(err, ErrNoAPIKey) && !errors.Contains(err, ErrInvalidAPIKey) && !errors.Contains(err, database.ErrUserNotFound) {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		if err != nil && (errors.Contains(err, ErrInvalidAPIKey) || errors.Contains(err, database.ErrUserNotFound)) {
			api.WriteError(w, errors.AddContext(err, "failed to fetch user by API key"), http.StatusUnauthorized)
			return
		}
		// If there is no API key check for a token.
		if errors.Contains(err, ErrNoAPIKey) {
			u, token, err = api.userAndTokenByRequestToken(req)
			if err != nil {
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
		}
		// Embed the verified token in the context of the request.
		ctx := jwt.ContextWithToken(req.Context(), token)
		h(u, w, req.WithContext(ctx), ps)
	}
}

// logRequest logs information about the current request.
func (api *API) logRequest(r *http.Request) {
	hasAuth := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer")
	hasAPIKey := r.Header.Get(APIKeyHeader) != "" || r.FormValue("apiKey") != ""
	c, err := r.Cookie(CookieName)
	hasCookie := err == nil && c != nil
	api.staticLogger.Tracef("Processing request: %v %v, Auth: %v, API Key: %v, Cookie: %v, Referer: %v, Host: %v, RemoreAddr: %v",
		r.Method, r.URL, hasAuth, hasAPIKey, hasCookie, r.Referer(), r.Host, r.RemoteAddr)
}
