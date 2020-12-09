package api

import (
	"net/http"
	"os"
	"time"

	"github.com/gorilla/securecookie"
)

const (
	// cookieName is the name of the cookie where we store the user's JWT token.
	cookieName = "SkynetJWTCookie"
	// cookieValidity specifies for how long the cookie is valid. In seconds.
	cookieValidity = 604800 // one week

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
	// _sc holds an instance of `securecookie`, so we don't instantiate it more
	// than once.
	// TODO Do we do that in Go? Do we have a convention for private fields? `_sc`?
	// 	Alternatively, I can make it a field on API.
	_sc *securecookie.SecureCookie = nil
)

// secureCookie extracts the hash and block keys from env vars and returns a
// usable `securecookie` struct.
func secureCookie() *securecookie.SecureCookie {
	if _sc == nil {
		var hashKey = []byte(os.Getenv(envCookieHashKey))
		var blockKey = []byte(os.Getenv(envCookieEncKey))
		_sc = securecookie.New(hashKey, blockKey)
	}
	return _sc
}

// writeJWTCookie is a helper function that writes the given JWT token as a
// secure cookie.
func writeJWTCookie(w http.ResponseWriter, token string) error {
	cookieVal := map[string]string{"token": token}
	encodedValue, err := secureCookie().Encode(cookieName, cookieVal)
	if err != nil {
		return err
	}
	// Allow this cookie to be used on all subdomains of this domain.
	domain, ok := os.LookupEnv(envCookieDomain)
	if !ok {
		domain = "localhost"
	}
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    encodedValue,
		HttpOnly: true,
		Path:     "/",
		Domain:   domain,
		Expires:  time.Now().Add(cookieValidity * time.Second),
		MaxAge:   cookieValidity,
		Secure:   true, // do not send over insecure channels, e.g. HTTP
		SameSite: 1,    // https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00
	}
	http.SetCookie(w, cookie)
	return nil
}
