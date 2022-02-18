package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	jwt2 "github.com/lestrrat-go/jwx/jwt"

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
	api.staticRouter.GET("/register", api.WithDBSession(api.noAuth(api.registerGET)))
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
		var u *database.User
		var token jwt2.Token
		// Check for an API key. We only return an error if an invalid API key
		// is provided.
		ak, err := apiKeyFromRequest(req)
		if err == nil {
			// We have an API key. Let's generate a token based on it.
			token, err = api.tokenFromAPIKey(req.Context(), ak)
			u, err = api.staticDB.UserByAPIKey(req.Context(), ak)
			if err != nil {
				api.staticLogger.Debugf("Error fetching user for API key %s. Error: %s", ak, err)
				api.WriteError(w, errors.AddContext(err, "failed to fetch user by API key"), http.StatusUnauthorized)
				return
			}
		} else {
			// No API key. Let's check for a token in the request.
			token, err = tokenFromRequest(req)
			if err != nil {
				api.staticLogger.Debugln("Error fetching token from request:", err)
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
			sub, _, _, err := jwt.TokenFields(token)
			if err != nil {
				api.staticLogger.Debugln("Error decoding token from request:", err)
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
			u, err = api.staticDB.UserBySub(req.Context(), sub)
			if errors.Contains(err, database.ErrUserNotFound) {
				api.staticLogger.Debugln("User that created this token no longer exists:", err)
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
			if err != nil {
				api.staticLogger.Debugln("Error fetching user by token from request:", err)
				api.WriteError(w, err, http.StatusInternalServerError)
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
	c, err := r.Cookie(CookieName)
	hasCookie := err == nil && c != nil
	api.staticLogger.Tracef("Processing request: %v %v, Auth: %v, Skynet Cookie: %v, Referer: %v, Host: %v, RemoreAddr: %v", r.Method, r.URL, hasAuth, hasCookie, r.Referer(), r.Host, r.RemoteAddr)
}

// tokenFromAPIKey returns a token, generated for the owner of the given API key.
func (api *API) tokenFromAPIKey(ctx context.Context, ak database.APIKey) (jwt2.Token, error) {
	u, err := api.staticDB.UserByAPIKey(ctx, ak)
	if err != nil {
		return nil, err
	}
	tk, err := jwt.TokenForUser(u.Email, u.Sub)
	return tk, err
}

// apiKeyFromRequest extracts the API key from the request and returns it.
// It first checks the headers and then the query.
func apiKeyFromRequest(r *http.Request) (database.APIKey, error) {
	// Check the headers for an API key.
	akStr := r.Header.Get(APIKeyHeader)
	// If there is no API key in the headers, try the query.
	if akStr == "" {
		akStr = r.FormValue("apiKey")
	}
	if akStr == "" {
		return "", ErrNoAPIKey
	}
	ak := database.APIKey(akStr)
	if !ak.IsValid() {
		return "", ErrInvalidAPIKey
	}
	return ak, nil
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// It first checks the request headers and then the cookies.
// The token is validated before being returned.
func tokenFromRequest(r *http.Request) (jwt2.Token, error) {
	var tokenStr string
	// Check the headers for a token.
	parts := strings.Split(r.Header.Get("Authorization"), "Bearer")
	if len(parts) == 2 {
		tokenStr = strings.TrimSpace(parts[1])
	} else {
		// Check the cookie for a token.
		cookie, err := r.Cookie(CookieName)
		if errors.Contains(err, http.ErrNoCookie) {
			return nil, ErrNoToken
		}
		if err != nil {
			return nil, errors.AddContext(err, "cookie exists but it's not valid")
		}
		err = secureCookie.Decode(CookieName, cookie.Value, &tokenStr)
		if err != nil {
			return nil, errors.AddContext(err, "failed to decode token")
		}
	}
	token, err := jwt.ValidateToken(tokenStr)
	if err != nil {
		return nil, errors.AddContext(err, "failed to validate token")
	}
	return token, nil
}
