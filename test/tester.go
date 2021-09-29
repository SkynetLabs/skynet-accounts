package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/api"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/jwt"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
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
// Use the Shutdown method for a graceful shutdown.
func NewAccountsTester() (*AccountsTester, error) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logrus.New()

	// Initialise the environment.
	email.ServerLockID = "siasky.test"
	email.PortalAddress = testPortalAddr
	jwt.JWTPortalName = testPortalAddr
	jwt.AccountsJWKSFile = pathToJWKSFile
	err := jwt.LoadAccountsKeySet(logger)
	if err != nil {
		cancel()
		return nil, errors.AddContext(err, fmt.Sprintf("failed to load JWKS file from %s", jwt.AccountsJWKSFile))
	}

	// Connect to the database.
	db, err := database.New(ctx, DBTestCredentials(), logger)
	if err != nil {
		cancel()
		return nil, errors.AddContext(err, "failed to connect to the DB")
	}

	// Start a noop mail sender in a background thread.
	sender, err := email.NewSender(ctx, db, logger, &DependencySkipSendingEmails{}, FauxEmailURI)
	if err != nil {
		cancel()
		return nil, errors.AddContext(err, "failed to create an email sender")
	}
	sender.Start()

	// The meta fetcher will fetch metadata for all skylinks. This is needed, so
	// we can determine their size.
	mf := metafetcher.New(ctx, db, logger)

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
		Handler: server.Router(),
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = srv.Shutdown(context.TODO())
		}
	}()

	return &AccountsTester{
		Ctx:    ctx,
		DB:     db,
		Logger: logger,
		cancel: cancel,
	}, nil
}

// Get executes a GET request against the test service.
func (at *AccountsTester) Get(endpoint string, params map[string]string) (r *http.Response, body []byte, err error) {
	return at.executeRequest(http.MethodGet, endpoint, params, nil)
}

// Delete executes a DELETE request against the test service.
func (at *AccountsTester) Delete(endpoint string, params map[string]string) (r *http.Response, body []byte, err error) {
	return at.executeRequest(http.MethodDelete, endpoint, params, nil)
}

// Post executes a POST request against the test service.
func (at *AccountsTester) Post(endpoint string, params map[string]string, postParams map[string]string) (r *http.Response, body []byte, err error) {
	vals := url.Values{}
	for k, v := range postParams {
		vals[k] = append(vals[k], v)
	}
	serviceUrl := testPortalAddr + ":" + testPortalPort + endpoint + "?" + encodeValues(params)
	req, err := http.NewRequest(http.MethodPost, serviceUrl, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if at.Cookie != nil {
		req.Header.Set("Cookie", at.Cookie.String())
	}
	c := http.Client{}
	r, err = c.Do(req)
	if err != nil {
		return
	}
	return processResponse(r)
}

// Put executes a PUT request against the test service.
func (at *AccountsTester) Put(endpoint string, params map[string]string, putParams map[string]string) (r *http.Response, body []byte, err error) {
	return at.executeRequest(http.MethodPut, endpoint, params, putParams)
}

// Shutdown performs a graceful shutdown of the AccountsTester service.
func (at *AccountsTester) Shutdown() {
	at.cancel()
}

// executeRequest is a helper method that puts together and executes an HTTP
// request. It attaches the current cookie, if one exists.
func (at *AccountsTester) executeRequest(method string, endpoint string, queryParams map[string]string, bodyParams map[string]string) (*http.Response, []byte, error) {
	b, err := json.Marshal(bodyParams)
	if err != nil {
		return nil, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	serviceUrl := testPortalAddr + ":" + testPortalPort + endpoint + "?" + encodeValues(queryParams)
	req, err := http.NewRequest(method, serviceUrl, bytes.NewBuffer(b))
	if err != nil {
		return nil, nil, err
	}
	if at.Cookie != nil {
		req.Header.Set("Cookie", at.Cookie.String())
	}
	client := http.Client{}
	r, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	return processResponse(r)
}

// encodeValues URL-encodes a values map.
func encodeValues(values map[string]string) string {
	v := url.Values{}
	for _, key := range values {
		v[key] = append(v[key], values[key])
	}
	return v.Encode()
}

// processResponse is a helper method which extracts the body from the response
// and handles non-OK status codes.
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
