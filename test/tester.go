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
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/metafetcher"
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
// Use the Close method for a graceful shutdown.
func NewAccountsTester(dbName string) (*AccountsTester, error) {
	ctx := context.Background()
	logger := logrus.New()

	// Initialise the environment.
	email.PortalAddress = testPortalAddr
	jwt.JWTPortalName = testPortalAddr
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
		Handler: server.Router(),
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

	// Sometimes we manage to hit the test endpoints before the goroutine which
	// runs ListenAndServe manages to start the server. So, we'll wait for a
	// moment here.
	time.Sleep(10 * time.Millisecond)

	return &AccountsTester{
		Ctx:    ctxWithCancel,
		DB:     db,
		Logger: logger,
		cancel: cancel,
	}, nil
}

// Get executes a GET request against the test service.
func (at *AccountsTester) Get(endpoint string, params url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodGet, endpoint, params, nil)
}

// Delete executes a DELETE request against the test service.
func (at *AccountsTester) Delete(endpoint string, params url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodDelete, endpoint, params, nil)
}

// Post executes a POST request against the test service.
func (at *AccountsTester) Post(endpoint string, params url.Values, postParams url.Values) (r *http.Response, body []byte, err error) {
	if params == nil {
		params = url.Values{}
	}
	if postParams == nil {
		postParams = url.Values{}
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodPost, serviceURL, strings.NewReader(postParams.Encode()))
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
func (at *AccountsTester) Put(endpoint string, params url.Values, putParams url.Values) (r *http.Response, body []byte, err error) {
	return at.request(http.MethodPut, endpoint, params, putParams)
}

// Close performs a graceful shutdown of the AccountsTester service.
func (at *AccountsTester) Close() error {
	at.cancel()
	return nil
}

// CreateUserPost is a helper method.
func (at *AccountsTester) CreateUserPost(email, password string) (r *http.Response, body []byte, err error) {
	params := url.Values{}
	params.Add("email", email)
	params.Add("password", password)
	return at.Post("/user", nil, params)
}

// UserPUT is a helper.
func (at *AccountsTester) UserPUT(email, stipeID string) (*http.Response, []byte, error) {
	serviceURL := testPortalAddr + ":" + testPortalPort + "/user"
	b, err := json.Marshal(map[string]string{
		"email":            email,
		"stripeCustomerId": stipeID,
	})
	if err != nil {
		return nil, nil, errors.AddContext(err, "failed to marshal the body JSON")
	}
	req, err := http.NewRequest(http.MethodPut, serviceURL, bytes.NewBuffer(b))
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

// request is a helper method that puts together and executes an HTTP
// request. It attaches the current cookie, if one exists.
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
