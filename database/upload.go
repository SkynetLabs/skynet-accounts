package database

import (
	"context"
	"fmt"
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

// UploadResponseDTO is the representation of an upload we send as response to
// the caller.
type UploadResponseDTO struct {
	ID        string    `bson:"_id" json:"id"`
	Skylink   string    `bson:"skylink" json:"skylink"`
	Name      string    `bson:"name" json:"name"`
	Size      int64     `bson:"size" json:"size"`
	Timestamp time.Time `bson:"timestamp" json:"uploadedOn"`
}

// UploadsResponseDTO defines the final format of our response to the caller.
type UploadsResponseDTO struct {
	Items    []UploadResponseDTO `json:"items"`
	Offset   int                 `json:"offset"`
	PageSize int                 `json:"pageSize"`
	Count    int                 `json:"count"`
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

// UploadCreate registers a new upload and counts it towards the user's used
// storage.
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
	if skylink.Size > 0 {
		err = db.UserUpdateUsedStorage(ctx, user.ID, skylink.Size)
		if err != nil {
			// Do not fail on error, just log a message.
			msg := fmt.Sprintf("Failed to update user's used storage for user %s and skylink %v.", user.ID.Hex(), skylink)
			db.staticLogger.Debugln(msg, err)
		}
	}
	return &up, nil
}

// UploadsBySkylink fetches a page of uploads of this skylink and the total
// number of such uploads.
func (db *DB) UploadsBySkylink(ctx context.Context, skylink Skylink, offset, pageSize int) ([]UploadResponseDTO, int, error) {
	if skylink.ID.IsZero() {
		return nil, 0, errors.New("invalid skylink")
	}
	if err := validateOffsetPageSize(offset, pageSize); err != nil {
		return nil, 0, err
	}
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylink.ID}}}}
	return db.uploadsBy(ctx, matchStage, offset, pageSize)
}

// UploadsByUser fetches a page of uploads by this user and the total number of
// such uploads.
func (db *DB) UploadsByUser(ctx context.Context, user User, offset, pageSize int) ([]UploadResponseDTO, int, error) {
	if user.ID.IsZero() {
		return nil, 0, errors.New("invalid user")
	}
	if err := validateOffsetPageSize(offset, pageSize); err != nil {
		return nil, 0, err
	}
	matchStage := bson.D{{"$match", bson.D{{"user_id", user.ID}}}}
	return db.uploadsBy(ctx, matchStage, offset, pageSize)
}

// uploadsBy fetches a page of uploads, filtered by an arbitrary match criteria.
// It also reports the total number of records in the list.
func (db *DB) uploadsBy(ctx context.Context, matchStage bson.D, offset, pageSize int) ([]UploadResponseDTO, int, error) {
	if err := validateOffsetPageSize(offset, pageSize); err != nil {
		return nil, 0, err
	}
	cnt, err := count(ctx, db.staticUploads, matchStage)
	if err != nil || cnt == 0 {
		return []UploadResponseDTO{}, 0, err
	}
	c, err := db.staticUploads.Aggregate(ctx, generateUploadsPipeline(matchStage, offset, pageSize))
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = c.Close(ctx) }()
	uploads := make([]UploadResponseDTO, pageSize)
	err = c.All(ctx, &uploads)
	if err != nil {
		return nil, 0, err
	}
	for ix := range uploads {
		uploads[ix].Size = StorageUsed(uploads[ix].Size)
	}
	return uploads, int(cnt), nil
}

// validateOffsetPageSize returns an error if offset and/or page size are invalid.
func validateOffsetPageSize(offset, pageSize int) error {
	errs := []error{}
	if offset < 0 {
		errs = append(errs, errors.New("the offset must be non-negative"))
	}
	if pageSize < 1 {
		errs = append(errs, errors.New("the page size needs to be positive"))
	}
	return errors.Compose(errs...)
}
