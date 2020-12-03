package api

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/dgrijalva/jwt-go"
)

type (
	// skynetClaims is a helper struct that defines what claims will be used
	// with our JWT tokens.
	skynetClaims struct {
		UserID string `json:"user_id"`
		Tier   int    `json:"tier"`

		jwt.StandardClaims
	}
)

// IssueToken creates a new JWT token for this user.
// This method uses the env var JWT_SECRET.
func IssueToken(u *database.User) (string, error) {
	claims := skynetClaims{
		UserID: u.ID.Hex(),
		Tier:   u.Tier,
		StandardClaims: jwt.StandardClaims{
			//Id:        "",
			Issuer: "Skynet Accounts", // TODO If we know the portal's name we can use it here.
			//Audience:  "",
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Second * time.Duration(TokenValiditySeconds)).Unix(),
			//NotBefore: 0,
			//Subject: "",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := t.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateToken verifies the validity of a JWT token, both in terms of validity
// of the signature and expiration time.
func ValidateToken(t string) (*jwt.Token, error) {
	token, err := jwt.Parse(t, func(token *jwt.Token) (interface{}, error) {
		//Make sure that the token method conform to "SigningMethodHMAC"
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New(fmt.Sprintf("unexpected signing method: %v", token.Header["alg"]))
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token is invalid")
	}
	return token, nil
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// It checks the header for a Bearer token and, if not found, checks for a cookie.
// Returns an empty string if there is no token.
func tokenFromRequest(r *http.Request) string {
	// Check the headers for a token.
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}

	// Check the cookie for a token.
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	value := make(map[string]string)
	err = secureCookie().Decode(cookieName, cookie.Value, &value)
	if err != nil {
		return ""
	}
	return value["token"]
}

// tokenFromContext is a helper function that extracts the JWT token from the
// context and returns the contained user id, claims and the token itself.
func tokenFromContext(req *http.Request) (id string, claims jwt.MapClaims, token *jwt.Token, err error) {
	t, ok := req.Context().Value(ctxValue("token")).(*jwt.Token)
	if !ok {
		err = errors.New("failed to get token")
		return
	}
	if reflect.ValueOf(t.Claims).Kind() != reflect.ValueOf(jwt.MapClaims{}).Kind() {
		err = errors.New("the token does not contain the claims we expect")
		return
	}
	claims = t.Claims.(jwt.MapClaims)
	if reflect.ValueOf(claims["user_id"]).Kind() != reflect.String {
		err = errors.New("the token does not contain the user_id we expect")
	}
	id = claims["user_id"].(string)
	token = t
	return
}
