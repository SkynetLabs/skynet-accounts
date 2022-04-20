package test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/metafetcher"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.sia.tech/siad/build"
)

var (
	testPortalAddr = "http://127.0.0.1"
	testPortalPort = "6000"
	pathToJWKSFile = "../../jwt/fixtures/jwks.json"

	// dontFollowRedirectsCheckRedirectFn is a function that instructs http.Client
	// to return with the last user response, instead of following a redirect.
	dontFollowRedirectsCheckRedirectFn = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
)

type (
	// AccountsTester is a simple testing kit for accounts. It starts a testing
	// instance of the service and provides simplified ways to call the handlers.
	AccountsTester struct {
		Ctx             context.Context
		DB              *database.DB
		Logger          *logrus.Logger
		APIKey          string
		Cookie          *http.Cookie
		Token           string
		FollowRedirects bool

		cancel context.CancelFunc
	}
)

// ExtractCookie is a helper method which extracts the login cookie from a
// response, so we can use it with future requests while testing.
func ExtractCookie(r *http.Response) *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == api.CookieName {
			return c
		}
	}
	return nil
}

// NewDatabase returns a new DB connection based on the passed parameters.
func NewDatabase(ctx context.Context, dbName string) (*database.DB, error) {
	return database.NewCustomDB(ctx, SanitizeName(dbName), DBTestCredentials(), NewDiscardLogger())
}

// NewAccountsTester creates and starts a new AccountsTester service.
// Use the Close method for a graceful shutdown.
func NewAccountsTester(dbName string) (*AccountsTester, error) {
	ctx := context.Background()
	logger := NewDiscardLogger()

	// Initialise the environment.
	jwt.PortalName = testPortalAddr
	jwt.AccountsJWKSFile = pathToJWKSFile
	err := jwt.LoadAccountsKeySet(logger)
	if err != nil {
		return nil, errors.AddContext(err, fmt.Sprintf("failed to load JWKS file from %s", jwt.AccountsJWKSFile))
	}

	// Connect to the database.
	db, err := NewDatabase(ctx, dbName)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to the DB")
	}

	// Start a noop mail sender in a background thread.
	sender, err := email.NewSender(ctx, db, logger, &DependencySkipSendingEmails{}, FauxEmailURI)
	if err != nil {
		return nil, errors.AddContext(err, "failed to create an email sender")
	}
	sender.Start()

	ctxWithCancel, cancel := context.WithCancel(ctx)
	// The meta fetcher will fetch metadata for all skylinks. This is needed, so
	// we can determine their size.
	mf := metafetcher.New(ctxWithCancel, db, logger)

	// The server API encapsulates all the modules together.
	server, err := api.New(db, mf, logger, email.NewMailer(db))
	if err != nil {
		cancel()
		return nil, errors.AddContext(err, "failed to build the API")
	}

	// Start the HTTP server in a goroutine and gracefully stop it once the
	// cancel function is called and the context is closed.
	srv := &http.Server{
		Addr:    ":" + testPortalPort,
		Handler: server,
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
	go func() {
		select {
		case <-ctxWithCancel.Done():
			_ = srv.Shutdown(context.TODO())
		}
	}()

	at := &AccountsTester{
		Ctx:             ctxWithCancel,
		DB:              db,
		FollowRedirects: true,
		Logger:          logger,
		cancel:          cancel,
	}
	// Wait for the accounts tester to be fully ready.
	err = build.Retry(50, time.Millisecond, func() error {
		_, _, e := at.HealthGet()
		return e
	})
	if err != nil {
		return nil, errors.AddContext(err, "failed to start accounts tester in the given time")
	}
	return at, nil
}

// NewDiscardLogger returns a new logger that sends all output to ioutil.Discard.
func NewDiscardLogger() *logrus.Logger {
	logger := logrus.New()
	logger.Out = ioutil.Discard
	return logger
}

// SanitizeName sanitizes the input for all kinds of unwanted characters and
// replaces those with underscores.
// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions
func SanitizeName(s string) string {
	re := regexp.MustCompile(`[/\\.\s"$*<>:|?]`)
	cleanDBName := re.ReplaceAllString(s, "_")
	// 64 characters is MongoDB's limit on database names.
	// See https://docs.mongodb.com/manual/reference/limits/#mongodb-limit-Length-of-Database-Names
	if len(cleanDBName) > 64 {
		cleanDBName = cleanDBName[:64]
	}
	return cleanDBName
}

// ClearCredentials removes any credentials stored by this tester, such as a
// cookie, token, etc.
func (at *AccountsTester) ClearCredentials() {
	at.APIKey = ""
	at.Cookie = nil
	at.Token = ""
}

// Close performs a graceful shutdown of the AccountsTester service.
func (at *AccountsTester) Close() error {
	at.cancel()
	if at.DB != nil {
		err := at.DB.Disconnect(at.Ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetAPIKey ensures that all subsequent requests are going to use the given
// API key for authentication.
func (at *AccountsTester) SetAPIKey(ak string) {
	at.ClearCredentials()
	at.APIKey = ak
}

// SetCookie ensures that all subsequent requests are going to use the given
// cookie for authentication.
func (at *AccountsTester) SetCookie(c *http.Cookie) {
	at.ClearCredentials()
	at.Cookie = c
}

// SetToken ensures that all subsequent requests are going to use the given
// token for authentication.
func (at *AccountsTester) SetToken(t string) {
	at.ClearCredentials()
	at.Token = t
}

// SetFollowRedirects configures the tester to either follow HTTP redirects or
// not. The default is to follow them.
func (at *AccountsTester) SetFollowRedirects(f bool) {
	at.FollowRedirects = f
}

// post executes a POST Request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) post(endpoint string, params url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
	if params == nil {
		params = url.Values{}
	}
	bodyMap := make(map[string]string)
	for k, v := range bodyParams {
		if len(v) == 0 {
			continue
		}
		bodyMap[k] = v[0]
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return &http.Response{}, nil, err
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodPost, serviceURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return &http.Response{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return at.executeRequest(req)
}

// Request is a helper method that puts together and executes an HTTP
// Request. It attaches the current cookie, if one exists.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Request(method string, endpoint string, queryParams url.Values, body []byte, headers map[string]string, obj interface{}) (*http.Response, error) {
	if queryParams == nil {
		queryParams = url.Values{}
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + queryParams.Encode()
	req, err := http.NewRequest(method, serviceURL, bytes.NewBuffer(body))
	if err != nil {
		return &http.Response{StatusCode: http.StatusInternalServerError}, err
	}
	for name, val := range headers {
		req.Header.Set(name, val)
	}
	r, b, err := at.executeRequest(req)
	// Define a list of response codes we assume are "good". We are going to
	// return an error if the response returns a code that's not on this list.
	acceptedResponseCodes := map[int]bool{
		http.StatusOK:                true,
		http.StatusNoContent:         true,
		http.StatusTemporaryRedirect: true,
		http.StatusPermanentRedirect: true,
	}
	// Use the response's body as error response on bad response codes.
	if !acceptedResponseCodes[r.StatusCode] {
		if err == nil {
			return r, errors.New(string(b))
		}
		return r, errors.AddContext(err, string(b))
	}
	if r.StatusCode == http.StatusOK {
		// Process the body
		err = json.Unmarshal(b, &obj)
		if err != nil {
			return r, errors.AddContext(err, "failed to marshal the body JSON")
		}
	}
	return r, err
}

// executeRequest is a helper method which executes a test Request and processes
// the response by extracting the body from it and handling non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) executeRequest(req *http.Request) (*http.Response, []byte, error) {
	if req == nil {
		return &http.Response{}, nil, errors.New("invalid Request")
	}
	if at.APIKey != "" {
		req.Header.Set(api.APIKeyHeader, at.APIKey)
	}
	if at.Cookie != nil {
		req.Header.Set("Cookie", at.Cookie.String())
	}
	if at.Token != "" {
		req.Header.Set("Authorization", "Bearer "+at.Token)
	}
	client := http.Client{}
	if !at.FollowRedirects {
		client.CheckRedirect = dontFollowRedirectsCheckRedirectFn
	}
	r, err := client.Do(req)
	if err != nil {
		return &http.Response{}, nil, err
	}
	return processResponse(r)
}

// processResponse is a helper method which extracts the body from the response
// and handles non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func processResponse(r *http.Response) (*http.Response, []byte, error) {
	body, err := ioutil.ReadAll(r.Body)
	_ = r.Body.Close()
	// For convenience, whenever we have a non-OK status we'll wrap it in an
	// error.
	if r.StatusCode < 200 || r.StatusCode > 299 {
		err = errors.Compose(err, errors.New(r.Status))
	}
	return r, body, err
}

// HealthGet executes a GET /health.
func (at *AccountsTester) HealthGet() (api.HealthGET, int, error) {
	var resp api.HealthGET
	r, err := at.Request(http.MethodGet, "/health", nil, nil, nil, &resp)
	return resp, r.StatusCode, err
}

/*** Login and logout helpers ***/

// LoginCredentialsPOST logs the user in and returns a response.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) LoginCredentialsPOST(emailAddr, password string) (*http.Response, []byte, error) {
	params := url.Values{}
	params.Set("email", emailAddr)
	params.Set("password", password)
	return at.post("/login", nil, params)
}

// LoginPubKeyGET performs `GET /login`
func (at *AccountsTester) LoginPubKeyGET(pk database.PubKey) (api.ChallengePublic, int, error) {
	params := url.Values{}
	if pk != nil {
		params.Set("pubKey", hex.EncodeToString(pk[:]))
	}
	var resp api.ChallengePublic
	r, err := at.Request(http.MethodGet, "/login", params, nil, nil, &resp)
	return resp, r.StatusCode, err
}

// LoginPubKeyPOST performs `POST /login`
func (at *AccountsTester) LoginPubKeyPOST(response, signature []byte, emailStr string) (*http.Response, []byte, error) {
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(signature))
	bodyParams.Set("email", emailStr)
	return at.post("/login", nil, bodyParams)
}

// LogoutPOST performs `POST /logout`
func (at *AccountsTester) LogoutPOST() (*http.Response, []byte, error) {
	return at.post("/logout", nil, nil)
}

/*** Registration helpers ***/

// RegisterGET performs `GET /register`
func (at *AccountsTester) RegisterGET(pk database.PubKey) (api.ChallengePublic, int, error) {
	query := url.Values{}
	if pk != nil {
		query.Set("pubKey", hex.EncodeToString(pk[:]))
	}
	var resp api.ChallengePublic
	r, err := at.Request(http.MethodGet, "/register", query, nil, nil, &resp)
	return resp, r.StatusCode, err
}

// RegisterPOST performs `POST /register`
func (at *AccountsTester) RegisterPOST(response, signature []byte, email string) (api.UserGET, int, error) {
	bodyParams := url.Values{}
	bodyParams.Set("response", hex.EncodeToString(response))
	bodyParams.Set("signature", hex.EncodeToString(signature))
	bodyParams.Set("email", email)
	r, b, err := at.post("/register", nil, bodyParams)
	if err != nil {
		return api.UserGET{}, r.StatusCode, errors.AddContext(err, string(b))
	}
	if r.StatusCode != http.StatusOK {
		return api.UserGET{}, r.StatusCode, errors.New(string(b))
	}
	var result api.UserGET
	err = json.Unmarshal(b, &result)
	if err != nil {
		return api.UserGET{}, 0, errors.AddContext(err, "failed to parse response")
	}
	return result, r.StatusCode, nil
}

/*** Track helpers ***/

// TrackDownload performs a `POST /track/download/:skylink` Request.
func (at *AccountsTester) TrackDownload(skylink string, bytes int64) (int, error) {
	form := url.Values{}
	form.Set("bytes", fmt.Sprint(bytes))
	r, err := at.Request(http.MethodPost, "/track/download/"+skylink, form, nil, nil, nil)
	return r.StatusCode, err
}

// TrackUpload performs a `POST /track/upload/:skylink` Request.
func (at *AccountsTester) TrackUpload(skylink string, ip string) (int, error) {
	form := url.Values{}
	form.Set("ip", ip)
	r, err := at.Request(http.MethodPost, "/track/upload/"+skylink, form, nil, nil, nil)
	return r.StatusCode, err
}

// TrackRegistryRead performs a `POST /track/registry/read` Request.
func (at *AccountsTester) TrackRegistryRead() (int, error) {
	r, err := at.Request(http.MethodPost, "/track/registry/read", nil, nil, nil, nil)
	return r.StatusCode, err
}

// TrackRegistryWrite performs a `POST /track/registry/write` Request.
func (at *AccountsTester) TrackRegistryWrite() (int, error) {
	r, err := at.Request(http.MethodPost, "/track/registry/write", nil, nil, nil, nil)
	return r.StatusCode, err
}

/*** User helpers ***/

// UserDELETE performs `DELETE /user`
func (at *AccountsTester) UserDELETE() (int, error) {
	r, err := at.Request(http.MethodDelete, "/user", nil, nil, nil, nil)
	return r.StatusCode, err
}

// UserGET performs `GET /user`
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) UserGET() (api.UserGET, int, error) {
	var resp api.UserGET
	r, err := at.Request(http.MethodGet, "/user", nil, nil, nil, &resp)
	return resp, r.StatusCode, err
}

// UserConfirmGET performs `GET /user/confirm`
func (at *AccountsTester) UserConfirmGET(confirmationToken string) (int, error) {
	qp := url.Values{}
	qp.Set("token", confirmationToken)
	r, err := at.Request(http.MethodGet, "/user/confirm", qp, nil, nil, nil)
	return r.StatusCode, err
}

// UserPOST is a helper method that creates a new user.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) UserPOST(emailAddr, password string) (*http.Response, []byte, error) {
	params := url.Values{}
	params.Set("email", emailAddr)
	params.Set("password", password)
	return at.post("/user", nil, params)
}

// UserPUT is a helper method which updates the entire user record.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) UserPUT(email, name, password, profilePic, stipeID string) (api.UserGET, int, error) {
	b, err := json.Marshal(map[string]string{
		"email":            email,
		"name":             name,
		"password":         password,
		"profilePic":       profilePic,
		"stripeCustomerId": stipeID,
	})
	if err != nil {
		return api.UserGET{}, http.StatusBadRequest, err
	}
	var resp api.UserGET
	r, err := at.Request(http.MethodPut, "/user", nil, b, nil, &resp)
	return resp, r.StatusCode, err
}

// UserReconfirmPOST performs `POST /user/reconfirm`
func (at *AccountsTester) UserReconfirmPOST() (*http.Response, []byte, error) {
	return at.post("/user/reconfirm", nil, nil)
}

// UserUploadsGET performs `GET /user/uploads`
func (at *AccountsTester) UserUploadsGET() (api.UploadsGET, int, error) {
	var result api.UploadsGET
	r, err := at.Request(http.MethodGet, "/user/uploads", nil, nil, nil, &result)
	return result, r.StatusCode, err
}

/*** User API keys helpers ***/

// UserAPIKeysDELETE performs a `DELETE /user/apikeys/:id` Request.
func (at *AccountsTester) UserAPIKeysDELETE(id primitive.ObjectID) (int, error) {
	r, err := at.Request(http.MethodDelete, "/user/apikeys/"+id.Hex(), nil, nil, nil, nil)
	return r.StatusCode, err
}

// UserAPIKeysGET performs a `GET /user/apikeys/:id` Request.
func (at *AccountsTester) UserAPIKeysGET(id primitive.ObjectID) (api.APIKeyResponse, int, error) {
	var result api.APIKeyResponse
	r, err := at.Request(http.MethodGet, "/user/apikeys/"+id.Hex(), nil, nil, nil, &result)
	return result, r.StatusCode, err
}

// UserAPIKeysLIST performs a `GET /user/apikeys` Request.
func (at *AccountsTester) UserAPIKeysLIST() ([]api.APIKeyResponse, int, error) {
	result := make([]api.APIKeyResponse, 0)
	r, err := at.Request(http.MethodGet, "/user/apikeys", nil, nil, nil, &result)
	return result, r.StatusCode, err
}

// UserAPIKeysPOST performs a `POST /user/apikeys` Request.
func (at *AccountsTester) UserAPIKeysPOST(body api.APIKeyPOST) (api.APIKeyResponseWithKey, int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return api.APIKeyResponseWithKey{}, http.StatusBadRequest, err
	}
	var result api.APIKeyResponseWithKey
	r, err := at.Request(http.MethodPost, "/user/apikeys", nil, b, nil, &result)
	return result, r.StatusCode, err
}

// UserAPIKeysPUT performs a `PUT /user/apikeys` Request.
func (at *AccountsTester) UserAPIKeysPUT(akID primitive.ObjectID, body api.APIKeyPUT) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return http.StatusBadRequest, err
	}
	r, err := at.Request(http.MethodPut, "/user/apikeys/"+akID.Hex(), nil, b, nil, nil)
	return r.StatusCode, err
}

// UserAPIKeysPATCH performs a `PATCH /user/apikeys` Request.
func (at *AccountsTester) UserAPIKeysPATCH(akID primitive.ObjectID, body api.APIKeyPATCH) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return http.StatusBadRequest, err
	}
	r, err := at.Request(http.MethodPatch, "/user/apikeys/"+akID.Hex(), nil, b, nil, nil)
	return r.StatusCode, err
}

/*** User limits helpers ***/

// UserLimits performs a `GET /user/limits` Request.
func (at *AccountsTester) UserLimits(unit string, headers map[string]string) (api.UserLimitsGET, int, error) {
	queryParams := url.Values{}
	queryParams.Set("unit", unit)
	var resp api.UserLimitsGET
	r, err := at.Request(http.MethodGet, "/user/limits", queryParams, nil, headers, &resp)
	return resp, r.StatusCode, err
}

// UserLimitsSkylink performs a `GET /user/limits/:skylink` Request.
func (at *AccountsTester) UserLimitsSkylink(sl string, unit, apikey string, headers map[string]string) (api.UserLimitsGET, int, error) {
	queryParams := url.Values{}
	queryParams.Set("unit", unit)
	queryParams.Set("apiKey", apikey)
	if !database.ValidSkylinkHash(sl) {
		return api.UserLimitsGET{}, 0, database.ErrInvalidSkylink
	}
	var resp api.UserLimitsGET
	r, err := at.Request(http.MethodGet, "/user/limits/"+sl, queryParams, nil, headers, &resp)
	return resp, r.StatusCode, err
}

/*** User pubkeys helpers ***/

// UserPubkeyDELETE performs `DELETE /user/pubkey/:pubKey`
func (at *AccountsTester) UserPubkeyDELETE(pk database.PubKey) (int, error) {
	r, err := at.Request(http.MethodDelete, "/user/pubkey/"+hex.EncodeToString(pk[:]), nil, nil, nil, nil)
	return r.StatusCode, err
}

// UserPubkeyRegisterGET performs a `GET /user/pubkey/register` Request.
func (at *AccountsTester) UserPubkeyRegisterGET(pubKey string) (api.ChallengePublic, int, error) {
	query := url.Values{}
	query.Add("pubKey", pubKey)
	var result api.ChallengePublic
	r, err := at.Request(http.MethodGet, "/user/pubkey/register", query, nil, nil, &result)
	return result, r.StatusCode, err
}

// UserPubkeyRegisterPOST performs a `POST /user/pubkey/register` Request.
func (at *AccountsTester) UserPubkeyRegisterPOST(response, signature []byte) (api.UserGET, int, error) {
	body := database.ChallengeResponseRequest{
		Response:  hex.EncodeToString(response),
		Signature: hex.EncodeToString(signature),
	}
	b, err := json.Marshal(body)
	if err != nil {
		return api.UserGET{}, http.StatusBadRequest, err
	}
	var result api.UserGET
	r, err := at.Request(http.MethodPost, "/user/pubkey/register", nil, b, nil, &result)
	return result, r.StatusCode, err
}

/*** User recovery helpers ***/

// UserRecoverPOST performs `POST /user/recover`
func (at *AccountsTester) UserRecoverPOST(tk, pw, confirmPW string) (int, error) {
	body := url.Values{}
	body.Set("token", tk)
	body.Set("password", pw)
	body.Set("confirmPassword", confirmPW)
	r, _, err := at.post("/user/recover", nil, body)
	if err != nil {
		return r.StatusCode, err
	}
	if r.StatusCode != http.StatusNoContent {
		return r.StatusCode, errors.New("unexpected status code")
	}
	return r.StatusCode, nil
}

// UserRecoverRequestPOST performs `POST /user/recover/Request`
func (at *AccountsTester) UserRecoverRequestPOST(email string) (int, error) {
	body := url.Values{}
	body.Set("email", email)
	r, _, err := at.post("/user/recover/Request", nil, body)
	if err != nil {
		return r.StatusCode, err
	}
	if r.StatusCode != http.StatusNoContent {
		return r.StatusCode, errors.New("unexpected status code")
	}
	return r.StatusCode, nil
}

/*** Uploads and downloads helpers ***/

// UploadsDELETE performs `DELETE /user/uploads/:skylink`
func (at *AccountsTester) UploadsDELETE(skylink string) (int, error) {
	r, err := at.Request(http.MethodDelete, "/user/uploads/"+skylink, nil, nil, nil, nil)
	return r.StatusCode, err
}

/*** Various user helpers ***/

// UserStats performs a `GET /user/stats` Request.
func (at *AccountsTester) UserStats(unit string, headers map[string]string) (database.UserStats, int, error) {
	queryParams := url.Values{}
	queryParams.Set("unit", unit)
	var resp database.UserStats
	r, err := at.Request(http.MethodGet, "/user/stats", queryParams, nil, headers, &resp)
	return resp, r.StatusCode, err
}

// UploadInfo performs a `GET /uploadinfo/:skylink` request.
func (at *AccountsTester) UploadInfo(sl string) ([]api.UploadInfo, int, error) {
	if !database.ValidSkylinkHash(sl) {
		return nil, http.StatusBadRequest, database.ErrInvalidSkylink
	}
	var resp []api.UploadInfo
	r, err := at.Request(http.MethodGet, "/uploadinfo/"+sl, nil, nil, nil, &resp)
	if err != nil {
		return nil, r.StatusCode, err
	}
	return resp, r.StatusCode, nil
}

/*** Stripe helpers ***/

// StripeBillingPOST performs a `POST /stripe/billing`
func (at *AccountsTester) StripeBillingPOST() (http.Header, int, error) {
	r, err := at.Request(http.MethodPost, "/stripe/billing", nil, nil, nil, nil)
	// We ignore the temporary redirect error because it's the expected
	// behaviour of this endpoint.
	if err != nil && !strings.Contains(err.Error(), "307 Temporary Redirect") {
		return nil, r.StatusCode, err
	}
	return r.Header, r.StatusCode, nil
}

// StripeCheckoutPOST performs a `POST /stripe/checkout`
func (at *AccountsTester) StripeCheckoutPOST(price string) (string, int, error) {
	body := struct {
		Price string
	}{
		price,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", http.StatusBadRequest, errors.AddContext(err, "failed to serialize request body")
	}
	resp := struct {
		SessionID string
	}{}
	r, err := at.Request(http.MethodPost, "/stripe/checkout", nil, bodyBytes, nil, &resp)
	return resp.SessionID, r.StatusCode, err
}

// StripePricesGET performs a `GET /stripe/prices`
func (at *AccountsTester) StripePricesGET() ([]api.StripePrice, int, error) {
	resp := make([]api.StripePrice, 0)
	r, err := at.Request(http.MethodGet, "/stripe/prices", nil, nil, nil, &resp)
	if err != nil {
		return nil, r.StatusCode, err
	}
	return resp, r.StatusCode, nil
}
