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
		// If set, this cookie will be attached to all requests.
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
		_, _, err = at.Get("/health", nil)
		return err
	})
	if err != nil {
		return nil, errors.AddContext(err, "failed to start accounts tester in the given time")
	}
	return at, nil
}

// Get executes a GET request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Get(endpoint string, params url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodGet, endpoint, params, nil)
}

// Delete executes a DELETE request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Delete(endpoint string, params url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodDelete, endpoint, params, nil)
}

// Post executes a POST request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
// TODO Remove the url.Values in favour of a simple map.
func (at *AccountsTester) Post(endpoint string, params url.Values, bodyParams url.Values) (r *http.Response, body []byte, err error) {
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
		return
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodPost, serviceURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return at.executeRequest(req)
}

// Put executes a PUT request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) Put(endpoint string, params url.Values, putParams url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodPut, endpoint, params, putParams)
}

// Close performs a graceful shutdown of the AccountsTester service.
func (at *AccountsTester) Close() error {
	at.cancel()
	return nil
}

// CreateUserPost is a helper method that creates a new user.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) CreateUserPost(emailAddr, password string) (r *http.Response, body []byte, err error) {
	params := url.Values{}
	params.Add("email", emailAddr)
	params.Add("password", password)
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
		return nil, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	req, err := http.NewRequest(http.MethodPut, serviceURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, nil, err
	}
	return at.executeRequest(req)
}

// request is a helper method that puts together and executes an HTTP
// request. It attaches the current cookie, if one exists.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) request(method string, endpoint string, queryParams url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
	if queryParams == nil {
		queryParams = url.Values{}
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + queryParams.Encode()
	b, err := json.Marshal(bodyParams)
	if err != nil {
		return nil, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	req, err := http.NewRequest(method, serviceURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, nil, err
	}
	return at.executeRequest(req)
}

// executeRequest is a helper method which executes a test request and processes
// the response by extracting the body from it and handling non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *AccountsTester) executeRequest(req *http.Request) (*http.Response, []byte, error) {
	if req == nil {
		return nil, nil, errors.New("invalid request")
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
		return nil, nil, err
	}
	body, err := ioutil.ReadAll(r.Body)
	_ = r.Body.Close()
	// For convenience, whenever we have a non-OK status we'll wrap it in an
	// error.
	if r.StatusCode < 200 || r.StatusCode > 299 {
		err = errors.Compose(err, errors.New(r.Status))
	}
	return r, body, err
}
