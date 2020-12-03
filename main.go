package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/NebulousLabs/skynet-accounts/api"
	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/joho/godotenv"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// envDBHost holds the name of the environment variable for DB host.
	envDBHost = "SKYNET_DB_HOST"
	// envDBPort holds the name of the environment variable for DB port.
	envDBPort = "SKYNET_DB_PORT"
	// envDBUser holds the name of the environment variable for DB username.
	envDBUser = "SKYNET_DB_USER"
	// envDBPass holds the name of the environment variable for DB password.
	envDBPass = "SKYNET_DB_PASS" // #nosec G101: Potential hardcoded credentials
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
	port, ok := os.LookupEnv("SKYNET_ACCOUNTS_PORT")
	if !ok {
		port = "3000"
	}
	dbCreds, err := loadDBCredentials()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to fetch DB credentials"))
	}
	ctx := context.Background()
	db, err := database.New(ctx, dbCreds)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to connect to the DB"))
	}
	server, err := api.New(db)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the API"))
	}
	fmt.Println("Listening on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, server.Router()))
}
