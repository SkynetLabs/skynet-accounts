package email

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/test"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
	rawDB, err := rawDBConnection(ctx)
	if err != nil {
		t.Fatal(err, "Failed to get a raw DB connection.")
	}
	emailsCol := rawDB.Collection("emails")
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
	emails, err := fetchEmails(ctx, emailsCol, filterTo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = emailsCol.DeleteMany(ctx, filterTo)
		if err != nil {
			t.Fatal(err, "Failed to delete emails.")
		}
	}()
	if len(emails) != 1 {
		t.Fatalf("Expected 1 email in the DB, got %d\n", len(emails))
	}
	if emails[0].Failed || emails[0].FailedAttemptsToSend > 0 || emails[0].LockedBy != "" || !emails[0].SentAt.IsZero() {
		t.Fatal("The email has been picked up already.")
	}
	// Start the sender and wait for a second.
	sender.Start()
	time.Sleep(time.Second)
	// Check that the email has been sent.
	emails, err = fetchEmails(ctx, emailsCol, filterTo)
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

// fetchEmails returns all emails that match the given filter.
func fetchEmails(ctx context.Context, col *mongo.Collection, filter bson.M) ([]database.EmailMessage, error) {
	c, err := col.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch emails from DB")
	}
	defer func() { _ = c.Close(ctx) }()
	var msgs []database.EmailMessage
	for c.Next(ctx) {
		var m database.EmailMessage
		if err = c.Decode(&m); err != nil {
			return nil, errors.AddContext(err, "failed to parse value from DB")
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// rawDBConnection is a helper method that gives us full direct access to the
// skynet database. It doesn't ensure the DB schema, so it needs to be called
// after a regular database.New() call.
func rawDBConnection(ctx context.Context) (*mongo.Database, error) {
	// Get test credentials.
	creds := test.DBTestCredentials()
	// Construct a connection string.
	connStr := fmt.Sprintf(
		"mongodb://%s:%s@%s:%s/?compressors=%s&readPreference=%s&w=%s&wtimeoutMS=%s",
		url.QueryEscape(creds.User),
		url.QueryEscape(creds.Password),
		creds.Host,
		creds.Port,
		"zstd,zlib,snappy",
		"nearest",
		"majority",
		"1000",
	)
	// Create a DB client.
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	// Connect to the database.
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	return c.Database("skynet"), nil
}
