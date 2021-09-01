package email

import (
	"context"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	// sleepBetweenScans defines how long the sender should sleep between its
	// sweeps of the DB.
	sleepBetweenScans = 30 * time.Second
)

var (
	// ServerDomain holds the name of the name of this particular server.
	ServerDomain = "siasky.net"
)

type (
	// Sender is a daemon that periodically checks the DB for emails waiting to
	// be sent and sends them.
	Sender struct {
		staticCtx    context.Context
		staticDB     *database.DB
		staticLogger *logrus.Logger
	}
)

// NewSender returns a new Sender instance.
func NewSender(ctx context.Context, db *database.DB, logger *logrus.Logger) Sender {
	return Sender{
		staticCtx:    ctx,
		staticDB:     db,
		staticLogger: logger,
	}
}

// Start continually scans the database for email messages waiting to be sent
// and sends them.
func (s Sender) Start() {
	go func() {
		select {
		case <-s.staticCtx.Done():
			return
		default:
		}
		s.scanAndSend()
		time.Sleep(sleepBetweenScans)
	}()
}

// scanAndSend scans the database for email messages waiting to be sent and
// sends them.
//
// We lock the messages before sending them and update their SentAt field after
// sending them. We also don't lock more than emailBatchSize messages.
// TODO test
func (s Sender) scanAndSend() {
	msgs, err := s.staticDB.EmailLockAndFetch(s.staticCtx, ServerDomain, emailBatchSize)
	if err != nil {
		s.staticLogger.Warningln(errors.AddContext(err, "failed to send email batch"))
		return
	}
	var sent []primitive.ObjectID
	var failed []*database.EmailMessage
	var errs []error
	for _, m := range msgs {
		err = send(m.From, m.To, m.Subject, m.Body, m.BodyMime)
		if err != nil {
			errs = append(errs, err)
			failed = append(failed, &m)
			continue
		}
		sent = append(sent, m.ID)
	}
	err = errors.Compose(errs...)
	err = errors.AddContext(err, "failed to send some emails")
	s.staticLogger.Warningln(err)

	err = s.staticDB.MarkAsSent(s.staticCtx, sent)
	if err != nil {
		err = errors.AddContext(err, "failed to mark emails as sent. they might get sent again")
		s.staticLogger.Warningln(err)
	}

	err = s.staticDB.MarkAsFailed(s.staticCtx, failed)
	if err != nil {
		err = errors.AddContext(err, "failed to mark emails as failed. we might attempt to send them one extra time")
		s.staticLogger.Debugln(err)
	}
}
