package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Download describes a single download of a skylink by a user.
type Download struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	SkylinkID primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylinkId"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
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
	Items      []DownloadResponseDTO `json:"items"`
	Offset     int                   `json:"offset"`
	PageSize   int                   `json:"pageSize"`
	TotalCount int                   `json:"totalCount"`
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
		Timestamp: time.Now().UTC(),
	}
	ior, err := db.staticDownloads.InsertOne(ctx, up)
	if err != nil {
		return nil, err
	}
	up.ID = ior.InsertedID.(primitive.ObjectID)
	return &up, nil
}

// DownloadsBySkylink fetches a page of downloads of this skylink and the total
// number of such downloads.
func (db *DB) DownloadsBySkylink(ctx context.Context, skylink Skylink, offset, limit int) ([]DownloadResponseDTO, int, error) {
	if skylink.ID.IsZero() {
		return nil, 0, errors.New("invalid skylink")
	}
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylink.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, limit)
}

// DownloadsByUser fetches a page of downloads by this user and the total number
// of such downloads.
func (db *DB) DownloadsByUser(ctx context.Context, user User, offset, limit int) ([]DownloadResponseDTO, int, error) {
	if user.ID.IsZero() {
		return nil, 0, errors.New("invalid user")
	}
	matchStage := bson.D{{"$match", bson.D{{"user_id", user.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, limit)
}

// downloadsBy fetches a page of downloads, filtered by an arbitrary match
// criteria. It also reports the total number of records in the list.
func (db *DB) downloadsBy(ctx context.Context, matchStage bson.D, offset, limit int) ([]DownloadResponseDTO, int, error) {
	cnt, err := count(ctx, db.staticDownloads, matchStage)
	if err != nil || cnt == 0 {
		return []DownloadResponseDTO{}, 0, err
	}
	pipeline := generateUploadsDownloadsPipeline(matchStage, offset, limit)
	c, err := db.staticDownloads.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	downloads := make([]DownloadResponseDTO, 0)
	err = c.All(ctx, &downloads)
	if err != nil {
		return nil, 0, err
	}
	return downloads, cnt, nil
}
