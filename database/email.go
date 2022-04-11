package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.sia.tech/siad/build"
)

const (
	// EmailConfirmationTokenTTL defines the lifetime of an email confirmation
	// token. After the token expires it can no longer be used and the user
	// needs to request an email re-confirmation.
	EmailConfirmationTokenTTL = 24 * time.Hour

	// EmailMaxSendAttempts defines the maximum number of attempts we are going
	// to make at sending a given email before giving up on it. This const is
	// defined here and not in the email package because the database package
	// cannot import the email package (loop).
	EmailMaxSendAttempts = 3

	// emailLockTTL defines how long an email can stay locked for sending. Once
	// the lock expires the record will be unlocked and free for other servers
	// to lock and send.
	emailLockTTL = 5 * time.Minute
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
		LockedAt       time.Time          `bson:"locked_at,omitempty"`
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
		"locked_by":       lockID,
		"failed_attempts": bson.M{"$lt": EmailMaxSendAttempts},
		"sent_at":         nil,
	}
	count, err := db.staticEmails.CountDocuments(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to count locked email messages")
	}
	// Lock some more entries in order to fill the batch.
	// We select entries which:
	//  - haven't failed more times than the limit
	//  - aren't sent, yet
	//  - are either unlocked or their lock has expired
	filterLock := bson.M{
		"failed_attempts": bson.M{"$lt": EmailMaxSendAttempts},
		"sent_at":         nil,
		"$or": bson.A{
			bson.M{"locked_by": ""},
			bson.M{"locked_at": bson.M{"$lt": time.Now().UTC().Add(-emailLockTTL)}},
		},
	}
	updateLock := bson.M{"$set": bson.M{
		"locked_by": lockID,
		"locked_at": time.Now().UTC(),
	}}
	for i := int64(0); i < batchSize-count; i++ {
		sr := db.staticEmails.FindOneAndUpdate(ctx, filterLock, updateLock)
		if sr.Err() == mongo.ErrNoDocuments {
			// No more records to lock. We can't fill the batch but we can send
			// what we have.
			break
		}
		if sr.Err() != nil {
			db.staticLogger.Debugln("Error while trying to lock a message:", err)
			continue
		}
	}
	// Fetch up to batchSize messages already locked with lockID.
	opts := options.Find()
	opts.SetLimit(batchSize)
	_, msgs, err = db.FindEmails(ctx, filter, opts)
	return msgs, nil
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
			"locked_at": time.Time{},
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
		"$set": bson.M{
			"locked_by": "",
			"locked_at": time.Time{},
		},
	}
	_, err := db.staticEmails.UpdateMany(ctx, filter, update)
	return err
}

// PurgeEmailCollection is a helper method for testing purposes. It removes all
// records from the email database collection.
func (db *DB) PurgeEmailCollection(ctx context.Context) (int64, error) {
	if build.Release != "testing" {
		return 0, nil
	}
	dr, err := db.staticEmails.DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return dr.DeletedCount, nil
}
