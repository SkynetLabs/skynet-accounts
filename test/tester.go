package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

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
)

type (
	// AccountsTester is a simple testing kit for accounts. It starts a testing
	// instance of the service and provides simplified ways to call the handlers.
	AccountsTester struct {
		cancel context.CancelFunc
		ctx    context.Context // TODO Do I need this?
		db     *database.DB    // TODO This will probably come handy for inspecting the DB.
		logger *logrus.Logger  // TODO Do I need this?
	}
)

// NewAccountsTester creates and starts a new AccountsTester service.
// Use the Shutdown method for a graceful shutdown.
func NewAccountsTester() (*AccountsTester, error) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logrus.New()

	// Initialise the environment.
	email.ServerLockID = "siasky.test"
	email.PortalAddress = testPortalAddr
	jwt.JWTPortalName = testPortalAddr
	jwt.AccountsJWKSFile = "../../jwt/fixtures/jwks.json" // TODO Const, better file, etc.
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
		println("*** Test server listening on port " + testPortalPort) // TODO DEBUG
		_ = srv.ListenAndServe()
	}()
	go func() {
		select {
		case <-ctx.Done():
			println("*** Shutting down test server") // TODO DEBUG
			_ = srv.Shutdown(context.TODO())         // TODO I can't pass ctx here, as it's closed now.
		}
	}()

	return &AccountsTester{
		cancel: cancel,
		ctx:    ctx,
		db:     db,
		logger: logger,
	}, nil
}

// Delete executes a DELETE request against the test service.
func (at *AccountsTester) Delete(endpoint string, params map[string]string) (status int, headers http.Header, body []byte, err error) {
	req, err := http.NewRequest(http.MethodDelete, testPortalAddr+":"+testPortalPort+endpoint+"?"+encodeValues(params), bytes.NewBuffer([]byte{}))
	if err != nil {
		return
	}
	client := &http.Client{}
	r, err := client.Do(req)
	if err != nil {
		return
	}
	body, err = ioutil.ReadAll(r.Body)
	return r.StatusCode, r.Header, body, err
}

// Get executes a GET request against the test service.
func (at *AccountsTester) Get(endpoint string, params map[string]string) (status int, headers http.Header, body []byte, err error) {
	r, err := http.Get(testPortalAddr + ":" + testPortalPort + endpoint + "?" + encodeValues(params))
	if err != nil {
		return
	}
	body, err = ioutil.ReadAll(r.Body)
	return r.StatusCode, r.Header, body, err
}

// Post executes a POST request against the test service.
func (at *AccountsTester) Post(endpoint string, params map[string]string, postParams map[string]string) (status int, headers http.Header, body []byte, err error) {
	reqBody, err := json.Marshal(postParams)
	if err != nil {
		return
	}
	// TODO Should we use PostForm instead?
	r, err := http.Post(testPortalAddr+":"+testPortalPort+endpoint+"?"+encodeValues(params), "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return
	}
	body, err = ioutil.ReadAll(r.Body)
	return r.StatusCode, r.Header, body, err
}

// Shutdown performs a graceful shutdown of the AccountsTester service.
func (at *AccountsTester) Shutdown() {
	at.cancel()
}

// encodeValues URL-encodes a values map.
func encodeValues(values map[string]string) string {
	v := url.Values{}
	for _, key := range values {
		v[key] = append(v[key], values[key])
	}
	return v.Encode()
}
