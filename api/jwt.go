package api

import (
	"fmt"
	"net/http"
	"os"
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
// This method uses the env var ACCESS_SECRET.
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
	token, err := t.SignedString([]byte(os.Getenv("ACCESS_SECRET")))
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
		return []byte(os.Getenv("ACCESS_SECRET")), nil
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

// extractToken extracts the JWT token from the request and returns it. Returns
// an empty string if there is no token.
func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	strArr := strings.Split(h, " ")
	if len(strArr) == 2 {
		return strArr[1]
	}
	return ""
}
