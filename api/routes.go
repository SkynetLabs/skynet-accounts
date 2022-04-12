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
	APIKeyHeader = "Skynet-API-Key" // #nosec
	// ErrAPIKeyNotAllowed is an error returned when an API key was passed to an
	// endpoint that doesn't allow API key use.
	ErrAPIKeyNotAllowed = errors.New("this endpoint does not allow the use of API keys")
	// ErrNoAPIKey is an error returned when we expect an API key but we don't
	// find one.
	ErrNoAPIKey = errors.New("no api key found")
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
	api.staticRouter.POST("/logout", api.withAuth(api.logoutPOST, false))
	api.staticRouter.GET("/register", api.noAuth(api.registerGET))
	api.staticRouter.POST("/register", api.WithDBSession(api.noAuth(api.registerPOST)))

	// Endpoints at which Nginx reports portal usage.
	api.staticRouter.POST("/track/upload/:skylink", api.noAuth(api.trackUploadPOST))
	api.staticRouter.POST("/track/download/:skylink", api.withAuth(api.trackDownloadPOST, true))
	api.staticRouter.POST("/track/registry/read", api.withAuth(api.trackRegistryReadPOST, true))
	api.staticRouter.POST("/track/registry/write", api.withAuth(api.trackRegistryWritePOST, true))

	api.staticRouter.POST("/user", api.noAuth(api.userPOST)) // This will be removed in the future.
	api.staticRouter.GET("/user", api.withAuth(api.userGET, false))
	api.staticRouter.PUT("/user", api.WithDBSession(api.withAuth(api.userPUT, false)))
	api.staticRouter.DELETE("/user", api.withAuth(api.userDELETE, false))
	api.staticRouter.GET("/user/limits", api.noAuth(api.userLimitsGET))
	api.staticRouter.GET("/user/limits/:skylink", api.noAuth(api.userLimitsSkylinkGET))
	api.staticRouter.GET("/user/stats", api.withAuth(api.userStatsGET, false))
	api.staticRouter.DELETE("/user/pubkey/:pubKey", api.WithDBSession(api.withAuth(api.userPubKeyDELETE, false)))
	api.staticRouter.GET("/user/pubkey/register", api.WithDBSession(api.withAuth(api.userPubKeyRegisterGET, false)))
	api.staticRouter.POST("/user/pubkey/register", api.WithDBSession(api.withAuth(api.userPubKeyRegisterPOST, false)))
	api.staticRouter.GET("/user/uploads", api.withAuth(api.userUploadsGET, false))
	api.staticRouter.DELETE("/user/uploads/:skylink", api.withAuth(api.userUploadsDELETE, false))
	api.staticRouter.GET("/user/downloads", api.withAuth(api.userDownloadsGET, false))

	// Endpoints for user API keys.
	api.staticRouter.POST("/user/apikeys", api.WithDBSession(api.withAuth(api.userAPIKeyPOST, false)))
	api.staticRouter.GET("/user/apikeys", api.withAuth(api.userAPIKeyLIST, false))
	api.staticRouter.GET("/user/apikeys/:id", api.withAuth(api.userAPIKeyGET, false))
	api.staticRouter.PUT("/user/apikeys/:id", api.WithDBSession(api.withAuth(api.userAPIKeyPUT, false)))
	api.staticRouter.PATCH("/user/apikeys/:id", api.WithDBSession(api.withAuth(api.userAPIKeyPATCH, false)))
	api.staticRouter.DELETE("/user/apikeys/:id", api.withAuth(api.userAPIKeyDELETE, false))

	// Endpoints for email communication with the user.
	api.staticRouter.GET("/user/confirm", api.WithDBSession(api.noAuth(api.userConfirmGET))) // TODO POST
	api.staticRouter.POST("/user/reconfirm", api.WithDBSession(api.withAuth(api.userReconfirmPOST, false)))
	api.staticRouter.POST("/user/recover/request", api.WithDBSession(api.noAuth(api.userRecoverRequestPOST)))
	api.staticRouter.POST("/user/recover", api.WithDBSession(api.noAuth(api.userRecoverPOST)))

	api.staticRouter.POST("/stripe/billing", api.WithDBSession(api.withAuth(api.stripeBillingPOST, false)))
	api.staticRouter.POST("/stripe/checkout", api.WithDBSession(api.withAuth(api.stripeCheckoutPOST, false)))
	api.staticRouter.GET("/stripe/prices", api.noAuth(api.stripePricesGET))
	api.staticRouter.POST("/stripe/webhook", api.WithDBSession(api.noAuth(api.stripeWebhookPOST)))

	api.staticRouter.GET("/.well-known/jwks.json", api.noAuth(api.wellKnownJWKSGET))

	// Internal endpoint. Never expose these!
	api.staticRouter.GET("/uploadinfo/:skylink", api.noAuth(api.uploadInfoGET))
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
func (api *API) withAuth(h HandlerWithUser, allowsAPIKey bool) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.logRequest(req)
		u, token, err := api.userFromRequest(req, allowsAPIKey)
		if errors.Contains(err, ErrNoAPIKey) || errors.Contains(err, database.ErrInvalidAPIKey) || errors.Contains(err, database.ErrUserNotFound) || errors.Contains(err, ErrAPIKeyNotAllowed) {
			api.WriteError(w, err, http.StatusUnauthorized)
			return
		}
		if err != nil {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		// Embed the verified token in the context of the request.
		ctx := jwt.ContextWithToken(req.Context(), token)
		h(u, w, req.WithContext(ctx), ps)
	}
}

// logRequest logs information about the current request.
func (api *API) logRequest(r *http.Request) {
	hasAuth := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer")
	hasAPIKey := r.Header.Get(APIKeyHeader) != ""
	c, err := r.Cookie(CookieName)
	hasCookie := err == nil && c != nil
	api.staticLogger.Tracef("Processing request: %v %v, Auth: %v, API Key: %v, Cookie: %v, Referer: %v, Host: %v, RemoreAddr: %v",
		r.Method, r.URL, hasAuth, hasAPIKey, hasCookie, r.Referer(), r.Host, r.RemoteAddr)
}
