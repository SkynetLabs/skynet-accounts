package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/build"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/email"
	"github.com/SkynetLabs/skynet-accounts/jwt"
	"github.com/SkynetLabs/skynet-accounts/metafetcher"
	"gitlab.com/SkynetLabs/skyd/skymodules"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v71"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// envAccountsJWKSFile holds the name of the environment variable which
	// holds the path to the JWKS file we need to use. Optional.
	envAccountsJWKSFile = "ACCOUNTS_JWKS_FILE"
	// envJWTTTL holds the name of the environment variable for JWT TTL.
	envJWTTTL = "ACCOUNTS_JWT_TTL"
	// envDBHost holds the name of the environment variable for DB host.
	envDBHost = "SKYNET_DB_HOST"
	// envDBPort holds the name of the environment variable for DB port.
	envDBPort = "SKYNET_DB_PORT"
	// envDBUser holds the name of the environment variable for DB username.
	envDBUser = "SKYNET_DB_USER"
	// envDBPass holds the name of the environment variable for DB password.
	envDBPass = "SKYNET_DB_PASS" // #nosec G101: Potential hardcoded credentials
	// envEmailFrom holds the name of the environment variable that allows us to
	// override the "from" address of our emails to users.
	envEmailFrom = "ACCOUNTS_EMAIL_FROM"
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

// logLevel returns the desires log level.
func logLevel() logrus.Level {
	lvl, err := logrus.ParseLevel(os.Getenv(envLogLevel))
	if err == nil {
		return lvl
	}
	if build.DEBUG {
		return logrus.TraceLevel
	}
	if build.Release == "testing" || build.Release == "dev" {
		return logrus.DebugLevel
	}
	return logrus.InfoLevel
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

	// portal tells us which Skynet portal to use for downloading skylinks.
	portal := os.Getenv(envPortal)
	if portal == "" {
		log.Fatal("missing env var " + envPortal)
	}
	database.PortalName = "https://" + portal
	jwt.PortalName = database.PortalName
	email.PortalAddressAccounts = "https://account." + portal
	email.ServerLockID = os.Getenv(envServerDomain)
	if email.ServerLockID == "" {
		email.ServerLockID = database.PortalName
		logger.Warningf(`Environment variable %s is missing! This server's identity 
			is set to the default '%s' value. That is OK only if this server is running on its own 
			and it's not sharing its DB with other nodes.\n`, envServerDomain, email.ServerLockID)
	}
	dbCreds, err := loadDBCredentials()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to fetch DB credentials"))
	}
	if sk := os.Getenv(envStripeAPIKey); sk != "" {
		stripe.Key = sk
		api.StripeTestMode = !strings.HasPrefix(stripe.Key, "sk_live_")
	}
	if jwks := os.Getenv(envAccountsJWKSFile); jwks != "" {
		jwt.AccountsJWKSFile = jwks
	}
	// Parse the optional env var that controls the TTL of the JWTs we generate.
	jwtTTL, err := strconv.ParseInt(os.Getenv(envJWTTTL), 10, 32)
	if err == nil && jwtTTL > 0 {
		jwt.TTL = int(jwtTTL)
	}
	// Fetch configuration data for sending emails.
	emailURI := os.Getenv(envEmailURI)
	{
		if emailURI == "" {
			log.Fatal(envEmailURI + " is empty")
		}
		// Validate the given URI.
		uri, err := url.Parse(emailURI)
		if err != nil {
			log.Fatal(errors.AddContext(err, "invalid email URI given in "+envEmailURI))
		}
		// Set the FROM address to outgoing emails. This can be overridden by
		// the ACCOUNTS_EMAIL_FROM optional environment variable.
		if uri.User != nil {
			email.From = uri.User.Username()
		}
		if emailFrom := os.Getenv(envEmailFrom); emailFrom != "" {
			email.From = emailFrom
		}
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
	mailer := email.NewMailer(db)
	// Start the mail sender background thread.
	sender, err := email.NewSender(ctx, db, logger, &skymodules.SkynetDependencies{}, emailURI)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to create an email sender"))
	}
	sender.Start()
	// The meta fetcher will fetch metadata for all skylinks. This is needed, so
	// we can determine their size.
	mf := metafetcher.New(ctx, db, logger)
	// Start the HTTP server.
	server, err := api.New(db, mf, logger, mailer)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the API"))
	}
	logger.Fatal(server.ListenAndServe(3000))
}
