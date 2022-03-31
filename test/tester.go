package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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
)

type (
	// AccountsTester is a simple testing kit for accounts. It starts a testing
	// instance of the service and provides simplified ways to call the handlers.
	AccountsTester struct {
		Ctx    context.Context
		DB     *database.DB
		Logger *logrus.Logger
		APIKey string
		Cookie *http.Cookie
		Token  string

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

// NewAccountsTester creates and starts a new AccountsTester service.
// Use the Close method for a graceful shutdown.
func NewAccountsTester(dbName string) (*AccountsTester, error) {
	ctx := context.Background()
	logger := logrus.New()
	logger.Out = ioutil.Discard

	// Initialise the environment.
	jwt.PortalName = testPortalAddr
	jwt.AccountsJWKSFile = pathToJWKSFile
	err := jwt.LoadAccountsKeySet(logger)
	if err != nil {
		return nil, errors.AddContext(err, fmt.Sprintf("failed to load JWKS file from %s", jwt.AccountsJWKSFile))
	}

	// Connect to the database.
	db, err := database.NewCustomDB(ctx, dbName, DBTestCredentials(), logger)
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
		Ctx:    ctxWithCancel,
		DB:     db,
		Logger: logger,
		cancel: cancel,
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

// Get executes a GET request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Get(endpoint string, params url.Values) (*http.Response, []byte, error) {
	return at.request(http.MethodGet, endpoint, params, nil, nil)
}

// Delete executes a DELETE request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Delete(endpoint string, params url.Values) (*http.Response, []byte, error) {
	return at.request(http.MethodDelete, endpoint, params, nil, nil)
}

// Post executes a POST request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
// TODO Remove the url.Values in favour of a simple map.
func (at *AccountsTester) Post(endpoint string, params url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
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

// Put executes a PUT request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Put(endpoint string, params url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
	b, err := json.Marshal(bodyParams)
	if err != nil {
		return &http.Response{}, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	return at.request(http.MethodPut, endpoint, params, b, nil)
}

// Patch executes a PATCH request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Patch(endpoint string, params url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
	b, err := json.Marshal(bodyParams)
	if err != nil {
		return &http.Response{}, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	return at.request(http.MethodPatch, endpoint, params, b, nil)
}

// CreateUserPost is a helper method that creates a new user.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) CreateUserPost(emailAddr, password string) (*http.Response, []byte, error) {
	params := url.Values{}
	params.Set("email", emailAddr)
	params.Set("password", password)
	return at.Post("/user", nil, params)
}

// UserPUT is a helper.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) UserPUT(email, password, stipeID string) (*http.Response, []byte, error) {
	serviceURL := testPortalAddr + ":" + testPortalPort + "/user"
	b, err := json.Marshal(map[string]string{
		"email":            email,
		"password":         password,
		"stripeCustomerId": stipeID,
	})
	if err != nil {
		return &http.Response{}, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	req, err := http.NewRequest(http.MethodPut, serviceURL, bytes.NewBuffer(b))
	if err != nil {
		return &http.Response{}, nil, err
	}
	return at.executeRequest(req)
}

// request is a helper method that puts together and executes an HTTP
// request. It attaches the current cookie, if one exists.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) request(method string, endpoint string, queryParams url.Values, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	if queryParams == nil {
		queryParams = url.Values{}
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + queryParams.Encode()
	req, err := http.NewRequest(method, serviceURL, bytes.NewBuffer(body))
	if err != nil {
		return &http.Response{}, nil, err
	}
	for name, val := range headers {
		req.Header.Set(name, val)
	}
	return at.executeRequest(req)
}

// executeRequest is a helper method which executes a test request and processes
// the response by extracting the body from it and handling non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) executeRequest(req *http.Request) (*http.Response, []byte, error) {
	if req == nil {
		return &http.Response{}, nil, errors.New("invalid request")
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
	r, err := client.Do(req)
	if err != nil {
		return &http.Response{}, nil, err
	}
	return processResponse(r)
}

// HealthGet executes a GET /health.
func (at *AccountsTester) HealthGet() (api.HealthGET, int, error) {
	r, b, err := at.request(http.MethodGet, "/health", nil, nil, nil)
	if err != nil {
		return api.HealthGET{}, r.StatusCode, err
	}
	var resp api.HealthGET
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return api.HealthGET{}, 0, errors.AddContext(err, "failed to marshal the body JSON")
	}
	return resp, r.StatusCode, nil
}

// UserAPIKeysDELETE performs a `DELETE /user/apikeys/:id` request.
func (at *AccountsTester) UserAPIKeysDELETE(id primitive.ObjectID) (int, error) {
	r, _, err := at.request(http.MethodDelete, "/user/apikeys/"+id.Hex(), nil, nil, nil)
	return r.StatusCode, err
}

// UserAPIKeysGET performs a `GET /user/apikeys/:id` request.
func (at *AccountsTester) UserAPIKeysGET(id primitive.ObjectID) (api.APIKeyResponse, int, error) {
	r, b, err := at.request(http.MethodGet, "/user/apikeys/"+id.Hex(), nil, nil, nil)
	if err != nil {
		return api.APIKeyResponse{}, r.StatusCode, err
	}
	if r.StatusCode != http.StatusOK {
		return api.APIKeyResponse{}, r.StatusCode, errors.New(string(b))
	}
	var result api.APIKeyResponse
	err = json.Unmarshal(b, &result)
	if err != nil {
		return api.APIKeyResponse{}, 0, errors.AddContext(err, "failed to parse response")
	}
	return result, r.StatusCode, nil
}

// UserAPIKeysLIST performs a `GET /user/apikeys` request.
func (at *AccountsTester) UserAPIKeysLIST() ([]api.APIKeyResponse, int, error) {
	r, b, err := at.request(http.MethodGet, "/user/apikeys", nil, nil, nil)
	if err != nil {
		return nil, r.StatusCode, err
	}
	if r.StatusCode != http.StatusOK {
		return nil, r.StatusCode, errors.New(string(b))
	}
	result := make([]api.APIKeyResponse, 0)
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, 0, errors.AddContext(err, "failed to parse response")
	}
	return result, r.StatusCode, nil
}

// UserAPIKeysPOST performs a `POST /user/apikeys` request.
func (at *AccountsTester) UserAPIKeysPOST(body api.APIKeyPOST) (api.APIKeyResponseWithKey, int, error) {
	bb, err := json.Marshal(body)
	if err != nil {
		return api.APIKeyResponseWithKey{}, http.StatusBadRequest, err
	}
	r, b, err := at.request(http.MethodPost, "/user/apikeys", nil, bb, nil)
	if err != nil {
		return api.APIKeyResponseWithKey{}, r.StatusCode, err
	}
	if r.StatusCode != http.StatusOK {
		return api.APIKeyResponseWithKey{}, r.StatusCode, errors.New(string(b))
	}
	var result api.APIKeyResponseWithKey
	err = json.Unmarshal(b, &result)
	if err != nil {
		return api.APIKeyResponseWithKey{}, 0, errors.AddContext(err, "failed to parse response")
	}
	return result, r.StatusCode, nil
}

// UserAPIKeysPUT performs a `PUT /user/apikeys` request.
func (at *AccountsTester) UserAPIKeysPUT(akID primitive.ObjectID, body api.APIKeyPUT) (int, error) {
	bb, err := json.Marshal(body)
	if err != nil {
		return http.StatusBadRequest, err
	}
	r, b, err := at.request(http.MethodPut, "/user/apikeys/"+akID.Hex(), nil, bb, nil)
	if err != nil {
		return r.StatusCode, err
	}
	if r.StatusCode != http.StatusNoContent {
		return r.StatusCode, errors.New(string(b))
	}
	return r.StatusCode, nil
}

// UserAPIKeysPATCH performs a `PATH /user/apikeys` request.
func (at *AccountsTester) UserAPIKeysPATCH(akID primitive.ObjectID, body api.APIKeyPATCH) (int, error) {
	bb, err := json.Marshal(body)
	if err != nil {
		return http.StatusBadRequest, err
	}
	r, b, err := at.request(http.MethodPatch, "/user/apikeys/"+akID.Hex(), nil, bb, nil)
	if err != nil {
		return r.StatusCode, err
	}
	if r.StatusCode != http.StatusNoContent {
		return r.StatusCode, errors.New(string(b))
	}
	return r.StatusCode, nil
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

// TrackDownload performs a `POST /track/download/:skylink` request.
func (at *AccountsTester) TrackDownload(skylink string, bytes int64) (int, error) {
	form := url.Values{}
	form.Set("bytes", fmt.Sprint(bytes))
	r, _, err := at.request(http.MethodPost, "/track/download/"+skylink, form, nil, nil)
	return r.StatusCode, err
}

// TrackUpload performs a `POST /track/upload/:skylink` request.
func (at *AccountsTester) TrackUpload(skylink string, ip string) (int, error) {
	form := url.Values{}
	form.Set("ip", ip)
	r, _, err := at.request(http.MethodPost, "/track/upload/"+skylink, form, nil, nil)
	return r.StatusCode, err
}

// TrackRegistryRead performs a `POST /track/registry/read` request.
func (at *AccountsTester) TrackRegistryRead() (int, error) {
	r, _, err := at.request(http.MethodPost, "/track/registry/read", nil, nil, nil)
	return r.StatusCode, err
}

// TrackRegistryWrite performs a `POST /track/registry/write` request.
func (at *AccountsTester) TrackRegistryWrite() (int, error) {
	r, _, err := at.request(http.MethodPost, "/track/registry/write", nil, nil, nil)
	return r.StatusCode, err
}

// UserLimits performs a `GET /user/limits` request.
func (at *AccountsTester) UserLimits(unit string, headers map[string]string) (api.UserLimitsGET, int, error) {
	queryParams := url.Values{}
	if unit != "" {
		queryParams.Set("unit", unit)
	}
	r, b, err := at.request(http.MethodGet, "/user/limits", queryParams, nil, headers)
	if err != nil {
		return api.UserLimitsGET{}, r.StatusCode, err
	}
	if r.StatusCode != http.StatusOK {
		return api.UserLimitsGET{}, r.StatusCode, errors.New(string(b))
	}
	var resp api.UserLimitsGET
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return api.UserLimitsGET{}, 0, errors.AddContext(err, "failed to marshal the body JSON")
	}
	return resp, r.StatusCode, nil
}

// UserLimitsSkylink performs a `GET /user/limits/:skylink` request.
func (at *AccountsTester) UserLimitsSkylink(sl string, unit, apikey string, headers map[string]string) (api.UserLimitsGET, int, error) {
	queryParams := url.Values{}
	queryParams.Set("unit", unit)
	queryParams.Set("apiKey", apikey)
	if !database.ValidSkylinkHash(sl) {
		return api.UserLimitsGET{}, 0, database.ErrInvalidSkylink
	}
	r, b, err := at.request(http.MethodGet, "/user/limits/"+sl, queryParams, nil, headers)
	if err != nil {
		return api.UserLimitsGET{}, r.StatusCode, err
	}
	if r.StatusCode != http.StatusOK {
		return api.UserLimitsGET{}, r.StatusCode, errors.New(string(b))
	}
	var resp api.UserLimitsGET
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return api.UserLimitsGET{}, 0, errors.AddContext(err, "failed to marshal the body JSON")
	}
	return resp, r.StatusCode, nil
}
