package email

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/email"
	"github.com/NebulousLabs/skynet-accounts/test"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	if _, err = db.PurgeEmailCollection(ctx); err != nil {
		t.Fatal("Failed to purge email collection:", err)
	}
	defer func() {
		if _, err = db.PurgeEmailCollection(ctx); err != nil {
			t.Fatal("Failed to purge email collection:", err)
		}
	}()
	logger := &logrus.Logger{}
	sender, err := email.NewSender(ctx, db, logger, &test.DependencySkipSendingEmails{}, test.FauxEmailURI)
	if err != nil {
		t.Fatal(err)
	}
	mailer := email.NewMailer(db)

	// Send an email.
	to := t.Name() + "@siasky.net"
	token := t.Name()
	err = mailer.SendAddressConfirmationEmail(ctx, to, token)
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
	time.Sleep(2 * time.Second)
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

// TestContendingSenders ensures that each email generated by a cluster of
// servers is sent exactly once. The test has several "servers" continuously
// creating and "sending" emails.
func TestContendingSenders(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	db, err := database.New(ctx, test.DBTestCredentials(), logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.PurgeEmailCollection(ctx); err != nil {
		t.Fatal("Failed to purge email collection:", err)
	}
	defer func() {
		if _, err = db.PurgeEmailCollection(ctx); err != nil {
			t.Fatal("Failed to purge email collection:", err)
		}
	}()
	targetAddr := t.Name() + "@siasky.net"
	numMsgs := 200
	// count will hold the total number of messages sent.
	var count int32
	var wg sync.WaitGroup
	// The generator will run in a thread. It will generate a predetermined
	// number of messages.
	generator := func(n int) {
		m := email.NewMailer(db)
		for i := 0; i < n; i++ {
			err1 := m.SendAddressConfirmationEmail(ctx, targetAddr, targetAddr)
			if err1 != nil {
				t.Error("Failed to send email.", err1)
				return
			}
		}
	}
	// The sender function will run in a thread. It will continuously pull
	// messages from the DB and "send" them. It will stop doing that when it
	// reaches two executions that fail to send any messages.
	sender := func(serverID string) {
		s, err := email.NewSender(ctx, db, logger, &test.DependencySkipSendingEmails{}, test.FauxEmailURI)
		if err != nil {
			t.Fatal(err)
		}
		var noneFetched int
		for {
			success, failure := s.ScanAndSend(serverID)
			sum := success + failure
			atomic.AddInt32(&count, int32(sum))
			if sum == 0 {
				noneFetched++
			} else {
				noneFetched = 0
			}
			if noneFetched > 10 {
				return
			}
		}
	}
	// Start some generators and some senders. Make sire the number of messages
	// to be sent divides without remainder by the number of generators.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			generator(numMsgs / 10)
			wg.Done()
		}()
		serverID := t.Name() + strconv.Itoa(i)
		wg.Add(1)
		go func() {
			sender(serverID)
			wg.Done()
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	if int(count) != numMsgs {
		t.Fatalf("Expected %d messages to be sent, got %d.", numMsgs, count)
	}
}
