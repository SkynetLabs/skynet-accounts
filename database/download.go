package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Download describes a single download of a skylink by a user.
type Download struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"_id"`
	UserID    primitive.ObjectID  `bson:"user_id,omitempty" json:"user_id"`
	SkylinkID primitive.ObjectID  `bson:"skylink_id,omitempty" json:"skylink_id"`
	Timestamp primitive.Timestamp `bson:"timestamp" json:"timestamp"`
}

// DownloadByID fetches a single download from the DB.
func (db *DB) DownloadByID(ctx context.Context, id primitive.ObjectID) (*Download, error) {
	var d Download
	filter := bson.D{{"_id", id}}
	sr := db.staticDownloads.FindOne(ctx, filter)
	err := sr.Decode(&d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// DownloadCreate registers a new download.
func (db *DB) DownloadCreate(ctx context.Context, user User, skylink Skylink) (*Download, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	if skylink.ID.IsZero() {
		return nil, errors.New("invalid skylink")
	}
	up := Download{
		UserID:    user.ID,
		SkylinkID: skylink.ID,
		Timestamp: primitive.Timestamp{T: uint32(time.Now().Unix())},
	}
	ior, err := db.staticDownloads.InsertOne(ctx, up)
	if err != nil {
		return nil, err
	}
	up.ID = ior.InsertedID.(primitive.ObjectID)
	return &up, nil
}

// DownloadsBySkylink fetches all downloads of this skylink
func (db *DB) DownloadsBySkylink(ctx context.Context, skylink Skylink, offset, limit int) ([]Download, error) {
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
	c, err := db.staticDownloads.Find(ctx, filter, &opts)
	if err != nil {
		return nil, err
	}
	downloads := make([]Download, 0)
	err = c.All(ctx, &downloads)
	if err != nil {
		return nil, err
	}
	return downloads, nil
}

// DownloadsByUser fetches all downloads by this user
func (db *DB) DownloadsByUser(ctx context.Context, user User, offset, limit int) ([]Download, error) {
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
	c, err := db.staticDownloads.Find(ctx, filter, &opts)
	if err != nil {
		return nil, err
	}
	downloads := make([]Download, 0)
	err = c.All(ctx, &downloads)
	if err != nil {
		return nil, err
	}
	return downloads, nil
}
