package jwt

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	jwt2 "github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// accountsJWKS is the public RS key set used by accounts for JWT
	// validation.
	accountsJWKS jwk.Set = nil
	// TODO This Pub one is WIP - we need a better approach for this. But this will do for now.
	accountsJWKSPub jwk.Set = nil

	// oathkeeperPubKeys is the public RS key set exposed by Oathkeeper for JWT
	// validation. It's available at oathkeeperPubKeyURL.
	oathkeeperPubKeys jwk.Set = nil

	// OathkeeperAddr holds the domain + port on which we can find Oathkeeper.
	// The point of this var is to be overridable via .env.
	OathkeeperAddr = "oathkeeper:4456"

	// JWTPortalName is the issuing service we are using for our JWTs. This
	// value can be overwritten by main.go is PORTAL_NAME is set.
	JWTPortalName = "https://siasky.net"

	// JWTTTL defines the lifetime of the JWT token in seconds.
	JWTTTL = 720 * 3600
)

type (
	// ctxValue is a helper type which makes it safe to register values in the
	// context. If we don't use a custom unexported type it's easy for others
	// to get our value or accidentally overwrite it.
	ctxValue string

	// tokenSession is the bare minimum we need in the `session` claim in our
	// JWTs.
	tokenSession struct {
		Active   bool          `json:"active"`
		Identity tokenIdentity `json:"identity"`
	}
	tokenIdentity struct {
		Traits tokenTraits `json:"traits"`
	}
	tokenTraits struct {
		Email string `json:"email"`
	}
)

// ContextWithToken returns a copy of the given context that contains a token.
func ContextWithToken(ctx context.Context, token *jwt.Token) context.Context {
	return context.WithValue(ctx, ctxValue("token"), token)
}

// TokenForUser creates a JWT token for the given user.
//
// The tokens generated by this function are a slimmed down version of the ones
// described in ValidateToken's docstring.
func TokenForUser(logger *logrus.Logger, email, sub string) (jwt.Token, error) {
	t, err := TokenForUserSerialized(logger, email, sub)
	if err != nil {
		return nil, errors.AddContext(err, "failed to sign token")
	}
	return jwt.Parse(t)
}

// TokenForUserSerialized creates a serialized JWT token for the given user.
//
// The tokens generated by this function are a slimmed down version of the ones
// described in ValidateToken's docstring.
func TokenForUserSerialized(logger *logrus.Logger, email, sub string) ([]byte, error) {
	keySet, err := accountsKeySet(logger)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch the JWKS")
	}
	key, found := keySet.Get(0)
	if !found {
		return nil, errors.New("JWKS is empty")
	}
	t, err := tokenForUser(email, sub)
	if err != nil {
		return nil, errors.AddContext(err, "failed to build token")
	}
	sigAlgo := algoFromKey(key)
	if sigAlgo == "" {
		return nil, errors.New("failed to determine signature algorithm")
	}

	return jwt.Sign(t, sigAlgo, key)
}

// algoFromKey is a helper method that returns the signature algorithm of the
// given key or an empty string on failure.
func algoFromKey(key jwk.Key) (sigAlgo jwa.SignatureAlgorithm) {
	for _, sa := range jwa.SignatureAlgorithms() {
		if string(sa) == key.Algorithm() {
			return sa
		}
	}
	return ""
}

// TokenFromContext extracts the JWT token from the
// context and returns the contained user sub, claims and the token itself.
// The sub is the user id used in Kratos.
//
// Example claims structure:
//
// map[
//    exp:1.607594172e+09
//    iat:1.607593272e+09
//    iss:https://siasky.net/
//    jti:1e5872ae-71d8-49ec-a550-4fc6163cbbf2
//    nbf:1.607593272e+09
//    sub:695725d4-a345-4e68-919a-7395cb68484c
//    session:map[
//        active:true
//        authenticated_at:2020-12-09T16:09:35.004003Z
//        issued_at:2020-12-09T16:09:35.004042Z
//        expires_at:2020-12-10T16:09:35.004003Z
//        id:9911ad26-e47f-4ec4-86a1-fbbc7fd5073e
//        identity:map[
//            id:695725d4-a345-4e68-919a-7395cb68484c
//            recovery_addresses:[
//                map[
//                    id:e2d847e1-1885-4edf-bccb-64b527b30096
//                    value:ivaylo@nebulous.tech
//                    via:email
//                ]
//            ]
//            schema_id:default
//            schema_url:https://siasky.net/secure/.ory/kratos/public/schemas/default
//            traits:map[
//                email:ivaylo@nebulous.tech
//                name:map[
//                    first:Ivaylo
//                    last:Novakov
//                ]—
//            ]
//            verifiable_addresses:[
//                map[
//                    id:953b0c1a-def9-4fa2-af23-fb36c00768d2
//                    status:pending
//                    value:ivaylo@nebulous.tech
//                    verified:true
//                    verified_at:2020-12-09T16:09:35.004042Z
//                    via:email
//                ]
//            ]
//        ]
//    ]
// ]
func TokenFromContext(ctx context.Context) (sub string, claims jwt2.MapClaims, token *jwt2.Token, err error) {
	t, ok := ctx.Value(ctxValue("token")).(*jwt2.Token)
	if !ok {
		err = errors.New("failed to get token")
		return
	}
	if reflect.ValueOf(t.Claims).Kind() != reflect.ValueOf(jwt2.MapClaims{}).Kind() {
		err = errors.New("the token does not contain the claims we expect")
		return
	}
	claims = t.Claims.(jwt2.MapClaims)
	if reflect.ValueOf(claims["sub"]).Kind() != reflect.String {
		err = errors.New("the token does not contain the sub we expect")
		return
	}
	subEntry, ok := claims["sub"]
	if !ok {
		claims = nil
		err = errors.New("jwt claims don't contain a valid sub")
		return
	}
	sub = subEntry.(string)
	token = t
	return
}

// UserDetailsFromJWT extracts the user details from the JWT token embedded in
// the context. We do it that way, so we can call this from anywhere in the code.
func UserDetailsFromJWT(ctx context.Context) (sub, email string, err error) {
	if ctx == nil {
		err = errors.New("Invalid context")
		return
	}
	_, claims, _, err := TokenFromContext(ctx)
	if err != nil {
		return
	}
	if reflect.ValueOf(claims["sub"]).Kind() != reflect.String {
		err = errors.New("the token does not contain the sub we expect")
		return
	}
	sub = claims["sub"].(string)
	// Validate the chain of inset maps claims->session->identity->traits->name
	// and then extract the data we need.
	if reflect.ValueOf(claims["session"]).Kind() != reflect.Map {
		err = errors.New("the token does not contain the sessions we expect")
		return
	}
	session := claims["session"].(map[string]interface{})
	if reflect.ValueOf(session["identity"]).Kind() != reflect.Map {
		err = errors.New("the token does not contain the identity we expect")
		return
	}
	id := session["identity"].(map[string]interface{})
	if reflect.ValueOf(id["traits"]).Kind() != reflect.Map {
		err = errors.New("the token does not contain the traits we expect")
		return
	}
	tr := id["traits"].(map[string]interface{})
	if reflect.ValueOf(tr["name"]).Kind() != reflect.Map {
		err = errors.New("the token does not contain the names we expect")
		return
	}
	email = tr["email"].(string)
	return
}

// ValidateToken verifies the validity of a JWT token, both in terms of validity
// of the signature and expiration time.
//
// Example token:
//
// Header:
//
// {
//  "alg": "RS256",
//  "kid": "a2aa9739-d753-4a0d-87ee-61f101050277",
//  "typ": "JWT"
// }
//
// Payload:
//
// {
//  "exp": 1607594172,
//  "iat": 1607593272,
//  "iss": "https://siasky.net/",
//  "jti": "1e5872ae-71d8-49ec-a550-4fc6163cbbf2",
//  "nbf": 1607593272,
//  "sub": "695725d4-a345-4e68-919a-7395cb68484c"
//  "session": {
//    "active": true,
//    "authenticated_at": "2020-12-09T16:09:35.004003Z",
//    "expires_at": "2020-12-10T16:09:35.004003Z",
//    "issued_at": "2020-12-09T16:09:35.004042Z"
//    "id": "9911ad26-e47f-4ec4-86a1-fbbc7fd5073e",
//    "identity": {
//      "id": "695725d4-a345-4e68-919a-7395cb68484c",
//      "recovery_addresses": [
//        {
//          "id": "e2d847e1-1885-4edf-bccb-64b527b30096",
//          "value": "ivaylo@nebulous.tech",
//          "via": "email"
//        }
//      ],
//      "schema_id": "default",
//      "schema_url": "https://siasky.net/secure/.ory/kratos/public/schemas/default",
//      "traits": {
//        "email": "ivaylo@nebulous.tech",
//        "name": {
//          "first": "Ivaylo",
//          "last": "Novakov"
//        }
//      },
//      "verifiable_addresses": [
//        {
//          "id": "953b0c1a-def9-4fa2-af23-fb36c00768d2",
//          "status": "pending",
//          "value": "ivaylo@nebulous.tech",
//          "verified": false,
//          "verified_at": null,
//          "via": "email"
//        }
//      ]
//    },
//  },
// }
func ValidateToken(logger *logrus.Logger, t string) (jwt.Token, error) {
	_, err := accountsKeySet(logger)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch JWT key set")
	}

	// TODO This approach is a mess. We need a proper approach to fetching the
	//  pub JWKS. If possible, we want to use a single file and just strip the
	//  sensitive parts of the private key. Or maybe that's wrong? This about it.
	token, err := jwt.Parse([]byte(t), jwt.WithKeySet(accountsJWKSPub))
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse and validate JWT")
	}
	return token, nil // TODO Re-enable Oathkeeper as fallback.
	// if err == nil {
	// 	// return nil, err
	// 	return token, nil
	// }
	//
	// // try to parse the token as an oathkeeper token
	// keySet, err = oathkeeperPublicKeys(logger)
	// if err != nil {
	// 	return nil, errors.AddContext(err, "failed to fetch Oathkeeper's JWKS")
	// }
	//
	// token, err = jwt.Parse([]byte(t), jwt.WithKeySet(*keySet))
	// if err != nil {
	// 	return nil, err
	// }
	// return token, nil
}

// accountsKeySet checks whether we have the needed key set cached and if we
// don't it fetches it and caches it for us. The method returns the first key in
// the set or ann error.
//
// Note that this key set includes both the public *and* private keys.
//
// See https://tools.ietf.org/html/rfc7517
// See https://auth0.com/blog/navigating-rs256-and-jwks/
// See http://self-issued.info/docs/draft-ietf-oauth-json-web-token.html
// Encoding RSA pub key: https://play.golang.org/p/mLpOxS-5Fy
func accountsKeySet(logger *logrus.Logger) (jwk.Set, error) {
	if accountsJWKS == nil {
		b, err := ioutil.ReadFile("../jwks.json") // DEBUG
		// b, err := ioutil.ReadFile(accountsPubKeysFile)
		if err != nil {
			logger.Warningln("ERROR while reading accounts JWKS", err)
			return nil, err
		}
		set := jwk.NewSet()
		err = json.Unmarshal(b, set)
		if err != nil {
			logger.Warningln("ERROR while parsing accounts JWKS", err)
			logger.Warningln("JWKS string:", string(b))
			return nil, err
		}
		accountsJWKS = set

		b, err = ioutil.ReadFile("../jwks_pub.json") // DEBUG
		// b, err := ioutil.ReadFile(accountsPubKeysFile)
		if err != nil {
			logger.Warningln("ERROR while reading accounts JWKS PUB", err)
			return nil, err
		}
		set1 := jwk.NewSet()
		err = json.Unmarshal(b, set1)
		if err != nil {
			logger.Warningln("ERROR while parsing accounts JWKS PUB", err)
			logger.Warningln("JWKS string:", string(b))
			return nil, err
		}
		accountsJWKSPub = set1
	}
	return accountsJWKS, nil
}

// oathkeeperPublicKeys checks whether we have the
// needed public key cached and if we don't it fetches it and caches it for us.
//
// See https://tools.ietf.org/html/rfc7517
// See https://auth0.com/blog/navigating-rs256-and-jwks/
// See http://self-issued.info/docs/draft-ietf-oauth-json-web-token.html
// Encoding RSA pub key: https://play.golang.org/p/mLpOxS-5Fy
func oathkeeperPublicKeys(logger *logrus.Logger) (jwk.Set, error) {
	if oathkeeperPubKeys == nil {
		oathkeeperPubKeyURL := "http://" + OathkeeperAddr + "/.well-known/jwks.json"
		logger.Traceln("fetching JWKS from oathkeeper:", oathkeeperPubKeyURL)
		r, err := http.Get(oathkeeperPubKeyURL) // #nosec G107: Potential HTTP request made with variable url
		if err != nil {
			logger.Warningln("ERROR while fetching JWKS from oathkeeper", err)
			return nil, err
		}
		defer func() { _ = r.Body.Close() }()
		var b []byte
		b, err = ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Warningln("ERROR while reading JWKS from oathkeeper", err)
			return nil, err
		}
		set := jwk.NewSet()
		err = json.Unmarshal(b, set)
		if err != nil {
			logger.Warningln("ERROR while parsing JWKS from oathkeeper", err)
			logger.Warningln("JWKS string:", string(b))
			return nil, err
		}
		oathkeeperPubKeys = set
	}
	return oathkeeperPubKeys, nil
}

// tokenForUser is a helper method that puts together an unsigned token based
// on the provided values.
func tokenForUser(email, sub string) (jwt.Token, error) {
	if email == "" || sub == "" {
		return nil, errors.New("Email and Sub cannot be empty.")
	}
	session := tokenSession{
		Active: true,
		Identity: tokenIdentity{
			Traits: tokenTraits{
				Email: email,
			},
		},
	}
	now := time.Now().UTC()
	t := jwt.New()
	err1 := t.Set("exp", now.Unix()+int64(JWTTTL))
	err2 := t.Set("iat", now.Unix())
	err3 := t.Set("iss", JWTPortalName)
	err4 := t.Set("sub", sub)
	err5 := t.Set("session", session)
	err := errors.Compose(err1, err2, err3, err4, err5)
	if err != nil {
		return nil, err
	}
	return t, nil
}
