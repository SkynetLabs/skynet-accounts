package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"gitlab.com/NebulousLabs/errors"

	"github.com/NebulousLabs/skynet-accounts/api"
	"github.com/NebulousLabs/skynet-accounts/database"
)

var (
	// envDBHost holds the name of the environment variable for DB host.
	envDBHost = "SKYNET_DB_HOST"
	// envDBPort holds the name of the environment variable for DB port.
	envDBPort = "SKYNET_DB_PORT"
	// envDBUser holds the name of the environment variable for DB username.
	envDBUser = "SKYNET_DB_USER" // #nosec
	// envDBPass holds the name of the environment variable for DB password.
	envDBPass = "SKYNET_DB_PASS" // #nosec
)

// dbConnFromEnv creates a new DB connection based on credentials found in the
// environment variables.
func dbConnFromEnv(ctx context.Context) (*database.DB, error) {
	var creds database.DBCredentials
	var ok bool
	if creds.User, ok = os.LookupEnv(envDBUser); !ok {
		return nil, errors.New("missing env var " + envDBUser)
	}
	if creds.Password, ok = os.LookupEnv(envDBPass); !ok {
		return nil, errors.New("missing env var " + envDBPass)
	}
	if creds.Host, ok = os.LookupEnv(envDBHost); !ok {
		return nil, errors.New("missing env var " + envDBHost)
	}
	if creds.Port, ok = os.LookupEnv(envDBPort); !ok {
		return nil, errors.New("missing env var " + envDBPort)
	}
	return database.New(ctx, creds)
}

func main() {
	port, ok := os.LookupEnv("SKYNET_ACCOUNTS_PORT")
	if !ok {
		port = "3000"
	}
	ctx := context.Background()
	db, err := dbConnFromEnv(ctx)
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
