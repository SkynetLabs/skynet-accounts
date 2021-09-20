package email

import (
	"context"
	"testing"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/test"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// FauxEmailURI is a valid URI for sending emails that points to a local
	// mailslurper instance. That instance is most probably not running, so
	// trying to send mails with it will fail, but it's useful for testing with
	// the DependencySkipSendingEmails.
	FauxEmailURI = "smtps://test:test1@mailslurper:1025/?skip_ssl_verify=true"
)

// TestSender goes through the standard Sender workflow and ensures that it
// works correctly.
func TestSender(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := database.New(ctx, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := &logrus.Logger{}
	sender, err := email.NewSender(ctx, db, logger, &test.DependencySkipSendingEmails{}, FauxEmailURI)
	if err != nil {
		t.Fatal(err)
	}
	mailer := email.New(db)

	// Send an email.
	to := t.Name() + "@siasky.net"
	err = mailer.SendAddressConfirmationEmail(ctx, to, t.Name())
	if err != nil {
		t.Fatal(err, "Failed to queue message for sending.")
	}
	// Ensure the email is in the DB and a send has not been attempted.
	filterTo := bson.M{"to": to}
	_, emails, err := db.FindEmails(ctx, filterTo, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 1 {
		t.Fatalf("Expected 1 email in the DB, got %d\n", len(emails))
	}
	if emails[0].FailedAttempts > 0 || !emails[0].SentAt.IsZero() {
		t.Fatal("The email has been picked up already.")
	}
	// Start the sender and wait for a second.
	sender.Start()
	time.Sleep(time.Second)
	// Check that the email has been sent.
	_, emails, err = db.FindEmails(ctx, filterTo, &options.FindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 1 {
		t.Fatalf("Expected 1 email in the DB, got %d\n", len(emails))
	}
	if emails[0].SentAt.IsZero() {
		t.Fatalf("Email not sent. Email: %+v\n", emails[0])
	}
}
