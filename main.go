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
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v71"
	"gitlab.com/SkynetLabs/skyd/skymodules"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

const (
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
	// envMaxNumAPIKeysPerUser hold the name of the environment variable which
	// sets the limit for number of API keys a single user can create. If a user
	// reaches that limit they can always delete some API keys in order to make
	// space for new ones.
	envMaxNumAPIKeysPerUser = "ACCOUNTS_MAX_NUM_API_KEYS_PER_USER"
)

type (
	// ServiceConfig represents all configuration values we expect to receive
	// via environment variables or config files.
	ServiceConfig struct {
		DBCreds               database.DBCredentials
		PortalName            string
		PortalAddressAccounts string
		ServerLockID          string
		StripeKey             string
		StripeTestMode        bool
		JWKSFile              string
		JWTTTL                int
		EmailURI              string
		EmailFrom             string
		MaxAPIKeys            int
	}
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

// parseConfiguration is responsible for reading and validating all environment
// variables we support - both required and optional ones. It also defers to the
// default values when certain config values are not provided. If in the future
// we add a config file for this service, it should be handled here as well.
func parseConfiguration(logger *logrus.Logger) (ServiceConfig, error) {
	config := ServiceConfig{}
	var err error

	config.DBCreds, err = loadDBCredentials()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to fetch DB credentials"))
	}

	// portal tells us which Skynet portal to use for downloading skylinks.
	portal := os.Getenv(envPortal)
	if portal == "" {
		return ServiceConfig{}, errors.New("missing env var " + envPortal)
	}
	config.PortalName = "https://" + portal
	config.PortalAddressAccounts = "https://account." + portal

	config.ServerLockID = os.Getenv(envServerDomain)
	if config.ServerLockID == "" {
		config.ServerLockID = config.PortalName
		logger.Warningf(`Environment variable %s is missing! This server's identity`+
			` is set to the default '%s' value. That is OK only if this server is running on its own`+
			` and it's not sharing its DB with other nodes.\n`, envServerDomain, config.ServerLockID)
	}

	if sk := os.Getenv(envStripeAPIKey); sk != "" {
		config.StripeKey = sk
		config.StripeTestMode = !strings.HasPrefix(sk, "sk_live_")
	}
	if jwks := os.Getenv(envAccountsJWKSFile); jwks != "" {
		config.JWKSFile = jwks
	}
	// Parse the optional env var that controls the TTL of the JWTs we generate.
	if jwtTTLStr := os.Getenv(envJWTTTL); jwtTTLStr != "" {
		jwtTTL, err := strconv.Atoi(jwtTTLStr)
		if err != nil {
			return ServiceConfig{}, fmt.Errorf("failed to parse env var %s: %s", envJWTTTL, err)
		}
		if jwtTTL == 0 {
			return ServiceConfig{}, fmt.Errorf("the %s env var is set to zero, which is an invalid value (must be positive or unset)", envJWTTTL)
		}
		config.JWTTTL = jwtTTL
	} else {
		// The environment doesn't specify a value, use the default.
		config.JWTTTL = jwt.TTL
	}

	// Fetch configuration data for sending emails.
	config.EmailURI = os.Getenv(envEmailURI)
	{
		if config.EmailURI == "" {
			return ServiceConfig{}, errors.New(envEmailURI + " is empty")
		}
		// Validate the given URI.
		uri, err := url.Parse(config.EmailURI)
		if err != nil || uri.Host == "" || uri.User == nil {
			return ServiceConfig{}, errors.New("invalid email URI given in " + envEmailURI)
		}
		// Set the FROM address to outgoing emails. This can be overridden by
		// the ACCOUNTS_EMAIL_FROM optional environment variable.
		if uri.User != nil {
			config.EmailFrom = uri.User.Username()
		}
		if emailFrom := os.Getenv(envEmailFrom); emailFrom != "" {
			config.EmailFrom = emailFrom
		}
		// No custom value set, use the default.
		if config.EmailFrom == "" {
			config.EmailFrom = email.From
		}
	}
	// Fetch the configuration for maximum number of API keys allowed per user.
	if maxAPIKeysStr, exists := os.LookupEnv(envMaxNumAPIKeysPerUser); exists {
		maxAPIKeys, err := strconv.Atoi(maxAPIKeysStr)
		if err != nil {
			log.Printf("Warning: Failed to parse %s env var. Error: %s", envMaxNumAPIKeysPerUser, err.Error())
		}
		if maxAPIKeys > 0 {
			config.MaxAPIKeys = maxAPIKeys
		} else {
			log.Printf("Warning: Invalid value of %s. The invalid value is ignored and the default value of %d is used.", envMaxNumAPIKeysPerUser, config.MaxAPIKeys)
		}
		config.MaxAPIKeys = maxAPIKeys
	} else {
		// The environment doesn't specify a value, use the default.
		config.MaxAPIKeys = database.MaxNumAPIKeysPerUser
	}

	return config, nil
}

func main() {
	// Initialise the global context and logger. These will be used throughout
	// the service. Once the context is closed, all background threads will
	// wind themselves down.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logLevel())

	// Load the environment variables from the .env file.
	_ = godotenv.Load()
	config, err := parseConfiguration(logger)
	if err != nil {
		log.Fatal(err)
	}
	database.PortalName = config.PortalName
	jwt.PortalName = config.PortalName
	email.PortalAddressAccounts = config.PortalAddressAccounts
	email.ServerLockID = config.ServerLockID
	stripe.Key = config.StripeKey
	api.StripeTestMode = config.StripeTestMode
	jwt.AccountsJWKSFile = config.JWKSFile
	jwt.TTL = config.JWTTTL
	email.From = config.EmailFrom
	database.MaxNumAPIKeysPerUser = config.MaxAPIKeys

	// Set up key components:

	// Load the JWKS that we'll use to sign and validate JWTs.
	err = jwt.LoadAccountsKeySet(logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, fmt.Sprintf("failed to load JWKS file from %s", jwt.AccountsJWKSFile)))
	}
	// Connect to the database.
	db, err := database.New(ctx, config.DBCreds, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to connect to the DB"))
	}
	mailer := email.NewMailer(db)
	// Start the mail sender background thread.
	sender, err := email.NewSender(ctx, db, logger, &skymodules.SkynetDependencies{}, config.EmailURI)
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
