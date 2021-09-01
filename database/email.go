package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type (
	// EmailMessage represents an email message waiting to be sent
	EmailMessage struct {
		ID                   primitive.ObjectID `bson:"id,omitempty"` // TODO Do I need this?
		From                 string             `bson:"from"`
		To                   string             `bson:"to"`
		Subject              string             `bson:"subject"`
		Body                 string             `bson:"body"`
		BodyMime             string             `bson:"body_mime"`
		LockedBy             string             `bson:"locked_by"`
		SentOn               time.Time          `bson:"sent_on,omitempty"`
		FailedAttemptsToSend int                `bson:"failed_attempts_to_send"`
		Failed               bool               `bson:"failed,omitempty"`
	}
)

// EmailCreate creates an email message in the DB which is waiting to be sent.
func (db *DB) EmailCreate(ctx context.Context, m EmailMessage) error {
	fields, err := bson.Marshal(m)
	if err != nil {
		return err
	}
	_, err = db.staticEmails.InsertOne(ctx, fields)
	if err != nil {
		return errors.AddContext(err, "failed to Insert")
	}
	return nil
}

// TODO LockBatch - lock up to N messages and return them

// TODO UnlockMessages - unlock a batch of messages identified by their ids. We do this if we fail to send them.

// TODO EmailFetchByLocker(locker string) - fetch all email messages locked by a certain locker

// TODO EmailMarkSent(ms ...EmailMessage) - Mark one or more messages as sent
