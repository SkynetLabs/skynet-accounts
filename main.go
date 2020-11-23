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

func main() {
	port, ok := os.LookupEnv("SKYNET_ACCOUNTS_PORT")
	if !ok {
		port = "3000"
	}
	ctx := context.Background()
	db, err := database.New(ctx)
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
