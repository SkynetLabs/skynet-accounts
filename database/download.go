package database

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// DownloadUpdateWindow defines a time window during which instead of
	// creating a new download record for the given skylink, we'll update the
	// previous one, as long as it has been updated within the window.
	DownloadUpdateWindow = 10 * time.Minute
)

// Download describes a single download of a skylink by a user.
type Download struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	SkylinkID primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylinkId"`
	Bytes     int64              `bson:"bytes" json:"bytes"`
	Created   time.Time          `bson:"timestamp" json:"timestamp"`
	Updated   time.Time          `bson:"updated" json:"-"`
}

// DownloadResponseDTO  is the representation of a download we send as response
// to the caller.
type DownloadResponseDTO struct {
	ID        string    `bson:"_id" json:"id"`
	Skylink   string    `bson:"skylink" json:"skylink"`
	Name      string    `bson:"name" json:"name"`
	Size      uint64    `bson:"size" json:"size"`
	Timestamp time.Time `bson:"timestamp" json:"downloadedOn"`
}

// DownloadsResponseDTO defines the final format of our response to the caller.
type DownloadsResponseDTO struct {
	Items    []DownloadResponseDTO `json:"items"`
	Offset   int                   `json:"offset"`
	PageSize int                   `json:"pageSize"`
	Count    int                   `json:"count"`
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

// DownloadCreate registers a new download. Marks partial downloads by supplying
// the `bytes` param. If `bytes` is 0 we assume a full download.
func (db *DB) DownloadCreate(ctx context.Context, user User, skylink Skylink, bytes int64) error {
	if user.ID.IsZero() {
		return errors.New("invalid user")
	}
	if skylink.ID.IsZero() {
		return errors.New("invalid skylink")
	}

	// Check if there exists a download of this skylink by this user, updated
	// within the DownloadUpdateWindow and keep updating that, if so.
	down, err := db.DownloadRecent(ctx, skylink.ID)
	if err == nil && down.Updated.Add(DownloadUpdateWindow).After(time.Now().UTC()) {
		// We found a recent download of this skylink. Let's update it.
		return db.DownloadIncrement(ctx, down, bytes)
	}

	// We couldn't find a recent download of this skylink, updated within
	// the DownloadUpdateWindow. We will create a new one.
	down = &Download{
		UserID:    user.ID,
		SkylinkID: skylink.ID,
		Bytes:     bytes,
		Created:   time.Now().UTC(),
		Updated:   time.Now().UTC(),
	}
	_, err = db.staticDownloads.InsertOne(ctx, down)
	return err
}

// DownloadsBySkylink fetches a page of downloads of this skylink and the total
// number of such downloads.
func (db *DB) DownloadsBySkylink(ctx context.Context, skylink Skylink, offset, pageSize int) ([]DownloadResponseDTO, int, error) {
	if skylink.ID.IsZero() {
		return nil, 0, errors.New("invalid skylink")
	}
	if err := validateOffsetPageSize(offset, pageSize); err != nil {
		return nil, 0, err
	}
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylink.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, pageSize)
}

// DownloadsByUser fetches a page of downloads by this user and the total number
// of such downloads.
func (db *DB) DownloadsByUser(ctx context.Context, user User, offset, pageSize int) ([]DownloadResponseDTO, int, error) {
	if user.ID.IsZero() {
		return nil, 0, errors.New("invalid user")
	}
	if err := validateOffsetPageSize(offset, pageSize); err != nil {
		return nil, 0, err
	}
	matchStage := bson.D{{"$match", bson.D{{"user_id", user.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, pageSize)
}

// downloadsBy fetches a page of downloads, filtered by an arbitrary match
// criteria. It also reports the total number of records in the list.
func (db *DB) downloadsBy(ctx context.Context, matchStage bson.D, offset, pageSize int) ([]DownloadResponseDTO, int, error) {
	cnt, err := db.count(ctx, db.staticDownloads, matchStage)
	if err != nil || cnt == 0 {
		return []DownloadResponseDTO{}, 0, err
	}
	c, err := db.staticDownloads.Aggregate(ctx, generateDownloadsPipeline(matchStage, offset, pageSize))
	if err != nil {
		return nil, 0, err
	}
	downloads := make([]DownloadResponseDTO, pageSize)
	err = c.All(ctx, &downloads)
	if err != nil {
		return nil, 0, err
	}
	return downloads, int(cnt), nil
}

// DownloadRecent returns the most recent download of the given skylink.
func (db *DB) DownloadRecent(ctx context.Context, skylinkId primitive.ObjectID) (*Download, error) {
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylinkId}}}}
	sortStage := bson.D{{"$sort", bson.D{
		{"updated", -1},
		{"timestamp", -1},
	}}}
	limitStage := bson.D{{"$limit", 1}}
	pipeline := mongo.Pipeline{matchStage, sortStage, limitStage}

	c, err := db.staticDownloads.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	if ok := c.Next(ctx); !ok {
		// No results found. This is expected.
		return nil, errors.New(fmt.Sprintf("no downloads found for skylink with id %v", skylinkId))
	}
	var d Download
	err = c.Decode(&d)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse value from DB")
	}
	return &d, nil
}

// DownloadIncrement increments the size of the download by additionalBytes.
func (db *DB) DownloadIncrement(ctx context.Context, d *Download, additionalBytes int64) error {
	filter := bson.M{"_id": d.ID}
	update := bson.M{"$inc": bson.M{
		"bytes": additionalBytes,
	}}
	_, err := db.staticDownloads.UpdateOne(ctx, filter, update)
	if err != nil {
		return errors.AddContext(err, "failed to update download record")
	}
	return nil
}
