package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
			"sent_at": time.Now().UTC(),
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
	update := bson.M{"$inc": bson.M{"failed_attempts": 1}}
	_, err := db.staticEmails.UpdateMany(ctx, filter, update)
	return err
}
