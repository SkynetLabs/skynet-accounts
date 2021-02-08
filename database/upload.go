package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Upload ...
type Upload struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	SkylinkID primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylinkId"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

// UploadResponseDTO is the DTO we send as response to the caller.
type UploadResponseDTO struct {
	ID        string    `bson:"_id" json:"id"`
	Skylink   string    `bson:"skylink" json:"skylink"`
	Name      string    `bson:"name" json:"name"`
	Size      uint64    `bson:"size" json:"size"`
	Timestamp time.Time `bson:"timestamp" json:"uploadedOn"`
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
func (db *DB) UploadsBySkylink(ctx context.Context, skylink Skylink, offset, limit int) ([]UploadResponseDTO, error) {
	if skylink.ID.IsZero() {
		return nil, errors.New("invalid skylink")
	}
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylink.ID}}}}
	return db.uploadsBy(ctx, matchStage, offset, limit)
}

// UploadsByUser fetches all uploads by this user
func (db *DB) UploadsByUser(ctx context.Context, user User, offset, limit int) ([]UploadResponseDTO, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	matchStage := bson.D{{"$match", bson.D{{"user_id", user.ID}}}}
	return db.uploadsBy(ctx, matchStage, offset, limit)
}

// uploadsBy is a helper function that allows us to fetch a list of downloads,
// filtered by an arbitrary match criteria.
func (db *DB) uploadsBy(ctx context.Context, matchStage bson.D, offset, limit int) ([]UploadResponseDTO, error) {
	pipeline := generateUploadsDownloadsPipeline(matchStage, offset, limit)
	c, err := db.staticUploads.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	uploads := make([]UploadResponseDTO, 0)
	err = c.All(ctx, &uploads)
	if err != nil {
		return nil, err
	}
	return uploads, nil
}
