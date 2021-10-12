package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/gorilla/securecookie"
	"github.com/joho/godotenv"
)

const (
	// CookieName is the name of the cookie where we store the user's JWT token.
	CookieName = "skynet-jwt"

	// envCookieDomain holds the name of the environment variable for the
	// domain name of the portal
	envCookieDomain = "COOKIE_DOMAIN"
	// envCookieHashKey holds the name of the env var which holds the key we use
	// to hash cookies.
	envCookieHashKey = "COOKIE_HASH_KEY"
	// envCookieEncKey holds the name of the env var which holds the key we use
	// to encrypt cookies.
	envCookieEncKey = "COOKIE_ENC_KEY"
	// secureCookieKeySize defines the size of the hash key and encryption key
	// we want to use for setting our cookie.
	secureCookieKeySize = 32
)

var (
	secureCookie = func() *securecookie.SecureCookie {
		_ = godotenv.Load()
		hashKeyStr := os.Getenv(envCookieHashKey)
		encKeyStr := os.Getenv(envCookieEncKey)
		if build.Release == "testing" {
			if len(hashKeyStr) < secureCookieKeySize {
				hashKeyStr = string(securecookie.GenerateRandomKey(secureCookieKeySize))
			}
			if len(encKeyStr) < secureCookieKeySize {
				encKeyStr = string(securecookie.GenerateRandomKey(secureCookieKeySize))
			}
		}
		if len(hashKeyStr) < secureCookieKeySize || len(encKeyStr) < secureCookieKeySize {
			panic(fmt.Sprintf("Both %s and %s are required environment variables and need to contain at least 32 bytes of hex-encoded entropy. %s, %s", envCookieHashKey, envCookieEncKey, hashKeyStr, encKeyStr))
		}
		// These keys need to be *exactly* 16 or 32 bytes long.
		var hashKey = []byte(hashKeyStr)[:secureCookieKeySize]
		var encKey = []byte(encKeyStr)[:secureCookieKeySize]
		return securecookie.New(hashKey, encKey)
	}()
)

// writeCookie is a helper function that writes the given JWT token as a
// secure cookie.
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie
func writeCookie(w http.ResponseWriter, token string, exp int64) error {
	encodedValue, err := secureCookie.Encode(CookieName, token)
	if err != nil {
		return err
	}
	// Allow this cookie to be used on all subdomains of this domain.
	domain, ok := os.LookupEnv(envCookieDomain)
	if !ok {
		domain = "127.0.0.1"
	}
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    encodedValue,
		HttpOnly: true,
		Path:     "/",
		Domain:   domain,
		MaxAge:   int(exp - time.Now().UTC().Unix()),
		Secure:   true, // do not send over insecure channels, e.g. HTTP
		SameSite: 1,    // https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00
	}
	http.SetCookie(w, cookie)
	return nil
}
