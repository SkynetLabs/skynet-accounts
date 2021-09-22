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
	// EmailConfirmationTokenTTL defines the lifetime of an email confirmation
	// token. After the token expires it can no longer be used and the user
	// needs to request an email re-confirmation.
	EmailConfirmationTokenTTL = 24 * time.Hour
)

type (
	// EmailMessage represents an email message waiting to be sent
	EmailMessage struct {
		ID             primitive.ObjectID `bson:"_id,omitempty"`
		From           string             `bson:"from"`
		To             string             `bson:"to"`
		Subject        string             `bson:"subject"`
		Body           string             `bson:"body"`
		BodyMime       string             `bson:"body_mime"`
		LockedBy       string             `bson:"locked_by"`
		SentAt         time.Time          `bson:"sent_at,omitempty"`
		FailedAttempts int                `bson:"failed_attempts"`
	}
)

// EmailCreate creates an email message in the DB which is waiting to be sent.
func (db *DB) EmailCreate(ctx context.Context, m EmailMessage) error {
	_, err := db.staticEmails.InsertOne(ctx, m)
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
	_, msgs, err = db.FindEmails(ctx, filter, opts)
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
	ids, _, err = db.FindEmails(ctx, filter, opts)
	return ids, err
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

// FindEmails is a helper method that fetches emails and their ids from the
// database.
func (db *DB) FindEmails(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]primitive.ObjectID, []EmailMessage, error) {
	c, err := db.staticEmails.Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, errors.AddContext(err, "failed to fetch ids")
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			err = errors.Compose(err, errors.AddContext(errDef, "error on closing DB cursor"))
		}
	}()
	var ids []primitive.ObjectID
	var msgs []EmailMessage
	for c.Next(ctx) {
		var m EmailMessage
		if err = c.Decode(&m); err != nil {
			return nil, nil, errors.AddContext(err, "failed to parse value from DB")
		}
		msgs = append(msgs, m)
		ids = append(ids, m.ID)
	}
	return ids, msgs, nil
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

// MarkAsFailed increments the FailedAttempts counter on each message and
// marks the message as Failed if that counter exceeds the maxAttemptsToSend.
// It also unlocks all given messages.
func (db *DB) MarkAsFailed(ctx context.Context, msgs []*EmailMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	var ids []primitive.ObjectID
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$inc": bson.M{"failed_attempts": 1},
		"$set": bson.M{"locked_by": ""},
	}
	_, err := db.staticEmails.UpdateMany(ctx, filter, update)
	return err
}
