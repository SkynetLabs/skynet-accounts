package api

import (
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	jwt2 "github.com/lestrrat-go/jwx/jwt"
	"gitlab.com/NebulousLabs/errors"
)

// userAndTokenByRequestToken scans the request for an authentication token,
// fetches the corresponding user from the database and returns both user and
// token.
func (api *API) userAndTokenByRequestToken(req *http.Request) (*database.User, jwt2.Token, error) {
	token, err := tokenFromRequest(req)
	if err != nil {
		return nil, nil, errors.AddContext(err, "error fetching token from request")
	}
	sub, _, _, err := jwt.TokenFields(token)
	if err != nil {
		return nil, nil, errors.AddContext(err, "error decoding token from request")
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub)
	if err != nil {
		return nil, nil, errors.AddContext(err, "error fetching user from database")
	}
	return u, token, nil
}

// userAndTokenByAPIKey extracts the APIKey from the request and validates it.
// It then returns the user who owns it and a token for that user.
// It first checks the headers and then the query.
// This method accesses the database.
func (api *API) userAndTokenByAPIKey(req *http.Request) (*database.User, jwt2.Token, error) {
	ak, err := apiKeyFromRequest(req)
	if err != nil {
		return nil, nil, err
	}
	akr, err := api.staticDB.APIKeyByKey(req.Context(), ak.String())
	if err != nil {
		return nil, nil, err
	}
	// If we're dealing with a public API key, we need to validate that this
	// request is a GET for a covered skylink.
	if akr.Public {
		sl, err := database.ExtractSkylinkHash(req.RequestURI)
		if err != nil || !akr.CoversSkylink(sl) {
			return nil, nil, database.ErrInvalidAPIKey
		}
	}
	u, err := api.staticDB.UserByID(req.Context(), akr.UserID)
	if err != nil {
		return nil, nil, err
	}
	t, err := jwt.TokenForUser(u.Email, u.Sub)
	return u, t, err
}

// apiKeyFromRequest extracts the API key from the request and returns it.
// This function does not differentiate between APIKey and APIKey.
// It first checks the headers and then the query.
func apiKeyFromRequest(r *http.Request) (*database.APIKey, error) {
	// Check the headers for an API key.
	akStr := r.Header.Get(APIKeyHeader)
	// If there is no API key in the headers, try the query.
	if akStr == "" {
		akStr = r.FormValue("apiKey")
	}
	if akStr == "" {
		return nil, ErrNoAPIKey
	}
	return database.NewAPIKeyFromString(akStr)
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// It first checks the authorization header and then the cookies.
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
