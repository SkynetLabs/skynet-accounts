package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	jwt2 "github.com/lestrrat-go/jwx/jwt"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TODO Test the methods here which are still untested.
// 	- add integration tests

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

// userAndTokenByAPIKey extracts the APIKey or PubAPIKey from the requests and
// validates it. It then returns the user who owns it and a token for that user.
// It first checks the headers and then the query.
// This method accesses the database.
func (api *API) userAndTokenByAPIKey(req *http.Request) (*database.User, jwt2.Token, error) {
	akStr, err := apiKeyFromRequest(req)
	if err != nil {
		return nil, nil, err
	}
	// We should only check for a PubAPIKey if this is a GET request for a valid
	// skylink. We ignore the errors here because the API key might not be a
	// public one.
	if req.Method == http.MethodGet {
		pak := database.PubAPIKey(akStr)
		sl, err := database.ExtractSkylinkHash(req.RequestURI)
		if err == nil && sl != "" && pak.IsValid() {
			uID, err := api.userIDForPubAPIKey(req.Context(), pak, sl)
			if err == nil {
				return api.userAndTokenByUserID(req.Context(), uID)
			}
		}
	}
	// Check if this is a valid APIKey.
	ak := database.APIKey(akStr)
	if !ak.IsValid() {
		return nil, nil, ErrInvalidAPIKey
	}
	uID, err := api.userIDForAPIKey(req.Context(), ak)
	if err != nil {
		return nil, nil, ErrInvalidAPIKey
	}
	return api.userAndTokenByUserID(req.Context(), uID)
}

// userAndTokenByUserID is a helper method that fetches a given user from the
// database based on their Key, issues a JWT token for them, and returns both
// of those.
func (api *API) userAndTokenByUserID(ctx context.Context, uid primitive.ObjectID) (*database.User, jwt2.Token, error) {
	u, err := api.staticDB.UserByID(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	t, err := jwt.TokenForUser(u.Email, u.Sub)
	return u, t, err
}

// userIDForAPIKey looks up the given APIKey and returns the Key of the user that
// issued it.
func (api *API) userIDForAPIKey(ctx context.Context, ak database.APIKey) (primitive.ObjectID, error) {
	akRec, err := api.staticDB.APIKeyGetRecord(ctx, ak)
	if err != nil {
		return primitive.ObjectID{}, err
	}
	return akRec.UserID, nil
}

// userIDForPubAPIKey looks up the given PubAPIKey, validates that the target
// skylink is covered by it, and returns the Key of the user that issued the
// PubAPIKey.
func (api *API) userIDForPubAPIKey(ctx context.Context, pak database.PubAPIKey, sl string) (primitive.ObjectID, error) {
	pakRec, err := api.staticDB.PubAPIKeyGetRecord(ctx, pak)
	if err != nil {
		return primitive.ObjectID{}, err
	}
	for _, s := range pakRec.Skylinks {
		if sl == s {
			return pakRec.UserID, nil
		}
	}
	return primitive.ObjectID{}, database.ErrUserNotFound
}

// apiKeyFromRequest extracts the API key from the request and returns it.
// This function does not differentiate between APIKey and PubAPIKey.
// It first checks the headers and then the query.
func apiKeyFromRequest(r *http.Request) (string, error) {
	// Check the headers for an API key.
	akStr := r.Header.Get(APIKeyHeader)
	// If there is no API key in the headers, try the query.
	if akStr == "" {
		akStr = r.FormValue("apiKey")
	}
	if akStr == "" {
		return "", ErrNoAPIKey
	}
	return akStr, nil
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
