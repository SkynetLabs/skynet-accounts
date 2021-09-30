package test

import "github.com/NebulousLabs/skynet-accounts/database"

const (
	// FauxEmailURI is a valid URI for sending emails that points to a local
	// mailslurper instance. That instance is most probably not running, so
	// trying to send mails with it will fail, but it's useful for testing with
	// the DependencySkipSendingEmails.
	FauxEmailURI = "smtps://test:test1@mailslurper:1025/?skip_ssl_verify=true"
)

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "17017",
	}
}
