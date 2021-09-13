package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// maxAttemptsToSend defines the maximum number of attempts we will make to
	// send a given email message before giving up.
	maxAttemptsToSend = 3
)

type (
	// EmailMessage represents an email message waiting to be sent
	EmailMessage struct {
		ID                   primitive.ObjectID `bson:"_id,omitempty"`
		From                 string             `bson:"from"`
		To                   string             `bson:"to"`
		Subject              string             `bson:"subject"`
		Body                 string             `bson:"body"`
		BodyMime             string             `bson:"body_mime"`
		LockedBy             string             `bson:"locked_by"`
		SentAt               time.Time          `bson:"sent_at,omitempty"`
		FailedAttemptsToSend int                `bson:"failed_attempts_to_send"`
		Failed               bool               `bson:"failed"`
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

// EmailLockAndFetch locks up to batchSize records with the given lockId and
// returns up to batchSize locked entries. Some of the returned entries might
// not have been locked during the current execution.
func (db *DB) EmailLockAndFetch(ctx context.Context, lockID string, batchSize int64) (msgs []EmailMessage, err error) {
	// Find out how many entries are already locked by this id. Maybe we don't
	// need to lock any additional ones.
	filter := bson.M{
		"locked_by": lockID,
		"failed":    bson.M{"$ne": true},
		"sent_at":   nil,
	}
	count, err := db.staticEmails.CountDocuments(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to count locked email messages")
	}
	// Lock some more entries in order to fill the batch.
	if count < batchSize {
		// As MongoDB doesn't have an "update up to N entries" operation, what
		// we do here is fetch the ids of the desired number of entries to
		// lock and then lock them by ids.
		var ids []primitive.ObjectID
		ids, err = db.fetchUnlockedMessageIDs(ctx, batchSize-count)
		if err != nil {
			return nil, errors.AddContext(err, "failed to fetch message ids to lock")
		}
		err = db.lockMessages(ctx, lockID, ids)
		if err != nil {
			return nil, errors.AddContext(err, "failed to lock messages")
		}
	}
	// Fetch up to batchSize messages already locked with lockID.
	opts := options.Find()
	opts.SetLimit(batchSize)
	c, err := db.staticEmails.Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch emails")
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			err = errors.Compose(err, errors.AddContext(errDef, "error on closing DB cursor"))
		}
	}()
	for c.Next(ctx) {
		var m EmailMessage
		if err = c.Decode(&m); err != nil {
			return nil, errors.AddContext(err, "failed to parse value from DB")
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// fetchUnlockedMessageIDs is a helper method that fetches the ids of up to num
// unlocked email messages waiting to be sent.
func (db *DB) fetchUnlockedMessageIDs(ctx context.Context, num int64) (ids []primitive.ObjectID, err error) {
	filter := bson.M{
		"locked_by": "",
		"failed":    bson.M{"$ne": true},
		"sent_at":   nil,
	}
	opts := options.Find()
	opts.SetLimit(num)
	c, err := db.staticEmails.Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch ids")
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			err = errors.Compose(err, errors.AddContext(errDef, "error on closing DB cursor"))
		}
	}()
	for c.Next(ctx) {
		var m EmailMessage
		if err = c.Decode(&m); err != nil {
			return nil, errors.AddContext(err, "failed to parse value from DB")
		}
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// lockMessages is a helper method that locks the messages with the given ids.
func (db *DB) lockMessages(ctx context.Context, lockID string, ids []primitive.ObjectID) error {
	if len(ids) == 0 {
		return nil
	}
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{"$set": bson.M{"locked_by": lockID}}
	_, err := db.staticEmails.UpdateMany(ctx, filter, update)
	if err != nil {
		return errors.AddContext(err, "failed to update")
	}
	return nil
}

// MarkAsSent unlocks all given messages and marks them as sent.
func (db *DB) MarkAsSent(ctx context.Context, ids []primitive.ObjectID) error {
	if len(ids) == 0 {
		return nil
	}
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"locked_by": "",
			"sent_at":   time.Now().UTC(),
		},
	}
	_, err := db.staticEmails.UpdateMany(ctx, filter, update)
	if err != nil {
		return errors.AddContext(err, "failed to mark emails as sent")
	}
	return nil
}

// MarkAsFailed increments the FailedAttemptsToSend counter on each message and
// marks the message as Failed if that counter exceeds the maxAttemptsToSend.
// It also unlocks all given messages.
func (db *DB) MarkAsFailed(ctx context.Context, msgs []*EmailMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]primitive.ObjectID, len(msgs))
	var failed []primitive.ObjectID
	for i, m := range msgs {
		ids[i] = m.ID
		// the messages that are about to reach or exceed the limit will be
		// marked as failed and won't be retries again.
		if m.FailedAttemptsToSend >= maxAttemptsToSend-1 {
			failed = append(failed, m.ID)
		}
	}

	// Increment the counter on all listed messages.
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$inc": bson.M{"failed_attempts_to_send": 1},
		"$set": bson.M{"locked_by": ""},
	}
	_, errInc := db.staticEmails.UpdateMany(ctx, filter, update)

	// Mark all messages that exceeded the limit on failures as permanently
	// failed.
	var errFailed error
	if len(failed) > 0 {
		db.staticLogger.Warningf("%d email messages failed to be sent more than %d times and won't be tried anymore.", len(failed), maxAttemptsToSend)

		filter = bson.M{"_id": bson.M{"$in": failed}}
		update = bson.M{"$set": bson.M{"failed": True}}
		_, errFailed = db.staticEmails.UpdateMany(ctx, filter, update)
	}

	return errors.Compose(errInc, errFailed)
}
