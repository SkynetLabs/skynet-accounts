package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// CookieName is the name of the cookie where we store the user's JWT token.
	CookieName = "skynet-jwt"
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
	_sc *securecookie.SecureCookie = nil
)

// secureCookie extracts the hash and block keys from env vars and returns a
// usable `securecookie` struct.
func secureCookie() *securecookie.SecureCookie {
	if _sc == nil {
		var hashKey = []byte(os.Getenv(envCookieHashKey))
		var blockKey = []byte("") // []byte(os.Getenv(envCookieEncKey))
		_sc = securecookie.New(hashKey, blockKey)
	}
	return _sc
}

// writeCookie is a helper function that writes the given JWT token as a
// secure cookie.
func writeCookie(w http.ResponseWriter, token string) error {
	if token == "" {
		return errors.New("invalid token")
	}
	ts := strings.Split(token, ".")
	if len(ts) != 3 {
		return errors.New("invalid token")
	}
	var b []byte
	_, err := base64.StdEncoding.Decode([]byte(ts[1]), b)
	if err != nil {
		return errors.AddContext(err, "failed to decode token payload")
	}
	type claims struct {
		Exp     int64 `json:"exp"`
		Session struct {
			Identity struct {
				Traits struct {
					Email string `json:"email"`
					Name  struct {
						First string `json:"first"`
						Last  string `json:"last"`
					} `json:"name"`
				} `json:"traits"`
			} `json:"identity"`
		} `json:"session"`
	}
	var cl claims
	err = json.Unmarshal(b, &cl)
	if err != nil {
		return errors.AddContext(err, "failed to parse claims")
	}
	content, err := json.Marshal(cl)
	if err != nil {
		return errors.AddContext(err, "failed to marshal back to JSON")
	}
	encodedValue, err := secureCookie().Encode(CookieName, string(content))
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
		Expires:  time.Unix(cl.Exp, 0),
		MaxAge:   cookieValidity,
		Secure:   true, // do not send over insecure channels, e.g. HTTP
		SameSite: 1,    // https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00
	}
	http.SetCookie(w, cookie)
	return nil
}
