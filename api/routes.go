package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/jwt"
	jwt2 "github.com/lestrrat-go/jwx/jwt"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// APIKeyHeader holds the name of the header we use for API keys. This
	// header name matches the established standard used by Swagger and others.
	APIKeyHeader = "X-API-KEY"
	// ErrNoAPIKey is an error returned when we expect an API key but we don't
	// find one.
	ErrNoAPIKey = errors.New("no api key found")
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
	api.staticRouter.GET("/user/pubkey/register", api.WithDBSession(api.validate(api.userPubKeyRegisterGET)))
	api.staticRouter.POST("/user/pubkey/register", api.WithDBSession(api.validate(api.userPubKeyRegisterPOST)))
	api.staticRouter.GET("/user/uploads", api.WithDBSession(api.validate(api.userUploadsGET)))
	api.staticRouter.DELETE("/user/uploads/:skylink", api.WithDBSession(api.validate(api.userUploadsDELETE)))
	api.staticRouter.GET("/user/downloads", api.WithDBSession(api.validate(api.userDownloadsGET)))

	// Endpoints for user API keys.
	api.staticRouter.POST("/user/apikey", api.WithDBSession(api.validate(api.userAPIKeyPOST)))
	api.staticRouter.GET("/user/apikey", api.WithDBSession(api.validate(api.userAPIKeyGET)))
	api.staticRouter.DELETE("/user/apikey/:apiKey", api.WithDBSession(api.validate(api.userAPIKeyDELETE)))

	// Endpoints for email communication with the user.
	api.staticRouter.GET("/user/confirm", api.WithDBSession(api.noValidate(api.userConfirmGET))) // TODO POST
	api.staticRouter.POST("/user/reconfirm", api.WithDBSession(api.validate(api.userReconfirmPOST)))
	api.staticRouter.POST("/user/recover/request", api.WithDBSession(api.noValidate(api.userRecoverRequestPOST)))
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
		var token jwt2.Token
		// Check for an API key. We only return an error if an invalid API key
		// is provided.
		ak, err := apiKeyFromRequest(req)
		if err == nil {
			// We have an API key. Let's generate a token based on it.
			token, err = api.tokenFromAPIKey(req.Context(), ak)
			if err != nil {
				api.staticLogger.Debugln("Error generating token for API key:", err)
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
		} else {
			// No API key. Let's check for a token in the request.
			token, _, err = tokenFromRequest(req)
			if err != nil {
				api.staticLogger.Debugln("Error fetching token from request:", err)
				api.WriteError(w, err, http.StatusUnauthorized)
				return
			}
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

// tokenFromAPIKey returns a token, generated for the owner of the given API key.
func (api *API) tokenFromAPIKey(ctx context.Context, ak string) (jwt2.Token, error) {
	u, err := api.staticDB.UserByAPIKey(ctx, ak)
	if err != nil {
		return nil, err
	}
	tk, _, err := jwt.TokenForUser(u.Email, u.Sub)
	return tk, err
}

// apiKeyFromRequest extracts the API key from the request and returns it.
// It first checks the headers and then the query.
func apiKeyFromRequest(r *http.Request) (string, error) {
	// Check the headers for an API key.
	ak := r.Header.Get(APIKeyHeader)
	// If there is no API key in the headers, try the query.
	if ak == "" {
		ak = r.Form.Get("api_key")
	}
	if ak == "" {
		return "", ErrNoAPIKey
	}
	return ak, nil
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// It first checks the request headers and then the cookies.
// The token is validated before being returned.
func tokenFromRequest(r *http.Request) (jwt2.Token, string, error) {
	var tokenStr string
	// Check the headers for a token.
	parts := strings.Split(r.Header.Get("Authorization"), "Bearer")
	if len(parts) == 2 {
		tokenStr = strings.TrimSpace(parts[1])
	} else {
		// Check the cookie for a token.

		cookie, err := r.Cookie(CookieName)
		if errors.Contains(err, http.ErrNoCookie) {
			return nil, "", errors.New("no authorisation token found")
		}
		if err != nil {
			return nil, "", errors.AddContext(err, "cookie exists but it's not valid")
		}
		err = secureCookie.Decode(CookieName, cookie.Value, &tokenStr)
		if err != nil {
			return nil, "", err
		}
	}
	token, err := jwt.ValidateToken(tokenStr)
	if err != nil {
		return nil, "", err
	}
	return token, tokenStr, nil
}
