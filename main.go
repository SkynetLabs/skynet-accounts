package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/NebulousLabs/skynet-accounts/api"
)

func main() {
	port, ok := os.LookupEnv("SKYNET_ACCOUNTS_PORT")
	if !ok {
		port = "3000"
	}
	server, err := api.New()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Listening on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, server.Router()))
}
