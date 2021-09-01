package email

import (
	"context"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
)

const (
	// sleepBetweenScans defines how long the sender should sleep between its
	// sweeps of the DB.
	sleepBetweenScans = 30 * time.Second
)

type (
	// Sender is a daemon that periodically checks the DB for emails waiting to
	// be sent and sends them.
	Sender struct {
		staticCtx context.Context
		staticDB  *database.DB
	}
)

// NewSender returns a new Sender instance.
func NewSender(ctx context.Context, db *database.DB) Sender {
	return Sender{
		staticCtx: ctx,
		staticDB:  db,
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
func (s Sender) scanAndSend() {
	// TODO Implement
}
