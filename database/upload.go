package database

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Upload ...
type Upload struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"user_id"`
	SkylinkID primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylink_id"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

// UploadByID fetches a single upload from the DB.
func (db *DB) UploadByID(ctx context.Context, id primitive.ObjectID) (*Upload, error) {
	var d Upload
	filter := bson.D{{"_id", id}}
	sr := db.staticUploads.FindOne(ctx, filter)
	err := sr.Decode(&d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UploadCreate registers a new upload.
func (db *DB) UploadCreate(ctx context.Context, user User, skylink Skylink) (*Upload, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	if skylink.ID.IsZero() {
		return nil, errors.New("invalid skylink")
	}
	up := Upload{
		UserID:    user.ID,
		SkylinkID: skylink.ID,
		Timestamp: time.Now().UTC(),
	}
	ior, err := db.staticUploads.InsertOne(ctx, up)
	if err != nil {
		return nil, err
	}
	up.ID = ior.InsertedID.(primitive.ObjectID)
	return &up, nil
}

// UploadsBySkylink fetches all uploads of this skylink
func (db *DB) UploadsBySkylink(ctx context.Context, skylink Skylink, offset, limit int) ([]Upload, error) {
	if skylink.ID.IsZero() {
		return nil, errors.New("invalid skylink")
	}
	filter := bson.D{{"skylink_id", skylink.ID}}
	opts := options.FindOptions{}
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	c, err := db.staticUploads.Find(ctx, filter, &opts)
	if err != nil {
		return nil, err
	}
	uploads := make([]Upload, 0)
	err = c.All(ctx, &uploads)
	if err != nil {
		return nil, err
	}
	return uploads, nil
}

// UploadsByUser fetches all uploads by this user
func (db *DB) UploadsByUser(ctx context.Context, user User, offset, limit int) ([]Upload, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	filter := bson.D{{"user_id", user.ID}}
	opts := options.FindOptions{}
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	c, err := db.staticUploads.Find(ctx, filter, &opts)
	if err != nil {
		return nil, err
	}
	uploads := make([]Upload, 0)
	err = c.All(ctx, &uploads)
	if err != nil {
		return nil, err
	}
	return uploads, nil
}
