package api

import "github.com/gorilla/securecookie"

const (
	// CookieName is the name of the cookie where we store the user's JWT token.
	CookieName = "SkynetJWTCookie"
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
		var hashKey = []byte(securecookie.GenerateRandomKey(32)) // TODO env
		var blockKey = []byte(securecookie.GenerateRandomKey(32))
		_sc = securecookie.New(hashKey, blockKey)
	}
	return _sc
}
