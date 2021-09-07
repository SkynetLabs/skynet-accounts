package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/api"
	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/jwt"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v71"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// defaultPortal is the URL of the default Skynet portal, maintained by
	// Skynet Labs. It can be overridden by an environment variable.
	defaultPortal = "https://siasky.net"
)

var (
	// envAccountsJWKSFile holds the name of the environment variable which
	// holds the path to the JWKS file we need to use. Optional.
	envAccountsJWKSFile = "ACCOUNTS_JWKS_FILE"
	// envDBHost holds the name of the environment variable for DB host.
	envDBHost = "SKYNET_DB_HOST"
	// envDBPort holds the name of the environment variable for DB port.
	envDBPort = "SKYNET_DB_PORT"
	// envDBUser holds the name of the environment variable for DB username.
	envDBUser = "SKYNET_DB_USER"
	// envDBPass holds the name of the environment variable for DB password.
	envDBPass = "SKYNET_DB_PASS" // #nosec G101: Potential hardcoded credentials
	// envEmailURI holds the name of the environment variable for email URI.
	envEmailURI = "ACCOUNTS_EMAIL_URI"
	// envLogLevel holds the name of the environment variable which defines the
	// desired log level.
	envLogLevel = "SKYNET_ACCOUNTS_LOG_LEVEL"
	// envPortal holds the name of the environment variable for the portal to
	// use to fetch skylinks and sign JWT tokens.
	envPortal = "PORTAL_DOMAIN"
	// envServerDomain holds the name of the environment variable for the
	// identity of this server. Example: eu-ger-1.siasky.net
	envServerDomain = "SERVER_DOMAIN"
	// envOathkeeperAddr hold the name of the environment variable for
	// Oathkeeper's address. Defaults to "oathkeeper:4456".
	envOathkeeperAddr = "OATHKEEPER_ADDR"
	// envStripeAPIKey hold the name of the environment variable for Stripe's
	// API key. It's only required when integrating with Stripe.
	envStripeAPIKey = "STRIPE_API_KEY"
)

// loadDBCredentials creates a new DB connection based on credentials found in
// the environment variables.
func loadDBCredentials() (database.DBCredentials, error) {
	var cds database.DBCredentials
	var ok bool
	if cds.User, ok = os.LookupEnv(envDBUser); !ok {
		return database.DBCredentials{}, errors.New("missing env var " + envDBUser)
	}
	if cds.Password, ok = os.LookupEnv(envDBPass); !ok {
		return database.DBCredentials{}, errors.New("missing env var " + envDBPass)
	}
	if cds.Host, ok = os.LookupEnv(envDBHost); !ok {
		return database.DBCredentials{}, errors.New("missing env var " + envDBHost)
	}
	if cds.Port, ok = os.LookupEnv(envDBPort); !ok {
		return database.DBCredentials{}, errors.New("missing env var " + envDBPort)
	}
	return cds, nil
}

func main() {
	// Load the environment variables from the .env file.
	// Existing variables take precedence and won't be overwritten.
	_ = godotenv.Load()

	// Initialise the global context and logger. These will be used throughout
	// the service. Once the context is closed, all background threads will
	// wind themselves down.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logLevel())

	portal, ok := os.LookupEnv(envPortal)
	if !ok {
		portal = defaultPortal
	}
	if !strings.HasPrefix(portal, "https://") && !strings.HasPrefix(portal, "http://") {
		portal = "https://" + portal
	}
	jwt.JWTPortalName = portal
	email.ServerDomain = os.Getenv(envServerDomain)
	if email.ServerDomain == "" {
		email.ServerDomain = jwt.JWTPortalName
		logger.Warningf(`Environment variable %s is missing! This server's identity 
			is set to the default '%s' value. That is OK only if this server is running on its own 
			and it's not sharing its DB with other nodes.\n`, envServerDomain, email.ServerDomain)
	}
	dbCreds, err := loadDBCredentials()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to fetch DB credentials"))
	}
	if oaddr := os.Getenv(envOathkeeperAddr); oaddr != "" {
		jwt.OathkeeperAddr = oaddr
	}
	if sk := os.Getenv(envStripeAPIKey); sk != "" {
		stripe.Key = sk
		api.StripeTestMode = !strings.HasPrefix(stripe.Key, "sk_live_")
	}
	if jwks := os.Getenv(envAccountsJWKSFile); jwks != "" {
		jwt.AccountsJWKSFile = jwks
	}
	if emailStr := os.Getenv(envEmailURI); emailStr != "" {
		email.ConnectionURI = emailStr
	}

	// Set up key components:

	// Load the JWKS that we'll use to sign and validate JWTs.
	err = jwt.LoadAccountsKeySet(logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, fmt.Sprintf("failed to load JWKS file from %s", jwt.AccountsJWKSFile)))
	}
	// Connect to the database.
	db, err := database.New(ctx, dbCreds, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to connect to the DB"))
	}
	mailer := email.New(db)
	// Start the mail sender background thread.
	email.NewSender(ctx, db, logger).Start()
	// The meta fetcher will fetch metadata for all skylinks. This is needed, so
	// we can determine their size.
	mf := metafetcher.New(ctx, db, portal, logger)
	// Start the HTTP server.
	server, err := api.New(db, mf, logger, mailer)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the API"))
	}
	logger.Info("Listening on port 3000")
	logger.Fatal(http.ListenAndServe(":3000", server.Router()))
}

// logLevel returns the desires log level.
func logLevel() logrus.Level {
	switch debugEnv, _ := os.LookupEnv(envLogLevel); debugEnv {
	case "panic":
		return logrus.PanicLevel
	case "fatal":
		return logrus.FatalLevel
	case "error":
		return logrus.ErrorLevel
	case "warn":
		return logrus.WarnLevel
	case "info":
		return logrus.InfoLevel
	case "debug":
		return logrus.DebugLevel
	case "trace":
		return logrus.TraceLevel
	}
	if build.DEBUG {
		return logrus.TraceLevel
	}
	if build.Release == "testing" || build.Release == "dev" {
		return logrus.DebugLevel
	}
	return logrus.InfoLevel
}
