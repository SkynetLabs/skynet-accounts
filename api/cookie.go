package api

import (
	"net/http"
	"os"
	"time"

	"github.com/gorilla/securecookie"
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
)

var (
	secureCookie = func() *securecookie.SecureCookie {
		var hashKey = []byte(os.Getenv(envCookieHashKey))
		var blockKey = []byte(os.Getenv(envCookieEncKey))
		return securecookie.New(hashKey, blockKey)
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
