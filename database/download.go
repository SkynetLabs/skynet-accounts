package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Download describes a single download of a skylink by a user.
type Download struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"user_id"`
	SkylinkID primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylink_id"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

// DownloadResponseDTO is the DTO we send as response to the caller.
type DownloadResponseDTO struct {
	ID        string    `bson:"string_id" json:"id"`
	Skylink   string    `bson:"skylink" json:"skylink"`
	Name      string    `bson:"name" json:"name"`
	Size      uint64    `bson:"size" json:"size"`
	Timestamp time.Time `bson:"timestamp" json:"downloaded_on"`
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

// DownloadsBySkylink fetches all downloads of this skylink
func (db *DB) DownloadsBySkylink(ctx context.Context, skylink Skylink, offset, limit int) ([]DownloadResponseDTO, error) {
	if skylink.ID.IsZero() {
		return nil, errors.New("invalid skylink")
	}
	matchStage := bson.D{{"$match", bson.D{{"skylink_id", skylink.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, limit)
}

// DownloadsByUser fetches all downloads by this user
func (db *DB) DownloadsByUser(ctx context.Context, user User, offset, limit int) ([]DownloadResponseDTO, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	matchStage := bson.D{{"$match", bson.D{{"user_id", user.ID}}}}
	return db.downloadsBy(ctx, matchStage, offset, limit)
}

// downloadsBy is a helper function that allows us to fetch a list of downloads,
// filtered by an arbitrary match criteria.
//
// The Mongo query this method executes is
//	db.downloads.aggregate([
//		{ $match: { "user_id": ObjectId("5fda32ef6e0aba5d16c0d550") }},
//		{ $skip: 1 },
//		{ $limit: 5 },
//		{ $lookup: {
//				from: "skylinks",
//				localField: "skylink_id",  // field in the downloads collection
//				foreignField: "_id",	   // field in the skylinks collection
//				as: "fromSkylinks"
//		  }
//		},
//		{ $replaceRoot: { newRoot: { $mergeObjects: [ { $arrayElemAt: [ "$fromSkylinks", 0 ] }, "$$ROOT" ] } } },
//		{ $project: { fromSkylinks: 0 } },
//		{ $addFields: { string_id: { $toString: "$_id" } } }
//	])
//
// This query will get all downloads by the current user, skip $skip of them
// and then fetch $limit of them, allowing us to paginate. It will then
// join with the `skylinks` collection in order to fetch some additional
// data about each download. The last line converts the [12]byte `_id` to hex,
// so we can easily handle it in JSON.
func (db *DB) downloadsBy(ctx context.Context, matchStage bson.D, offset, limit int) ([]DownloadResponseDTO, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	// Specify a pipeline that will join the downloads to the skylinks and will
	// return combined data.
	skipStage := bson.D{{"$skip", offset}}
	limitStage := bson.D{{"$limit", limit}}
	lookupStage := bson.D{
		{"$lookup", bson.D{
			{"from", "skylinks"},
			{"localField", "skylink_id"}, // field in the downloads collection
			{"foreignField", "_id"},      // field in the skylinks collection
			{"as", "fromSkylinks"},
		}},
	}
	replaceStage := bson.D{
		{"$replaceRoot", bson.D{
			{"newRoot", bson.D{
				{"$mergeObjects", bson.A{
					bson.D{{"$arrayElemAt", bson.A{"$fromSkylinks", 0}}}, "$$ROOT"},
				},
			}},
		}},
	}
	projectStage := bson.D{{"$project", bson.D{{"fromSkylinks", 0}}}}
	transformStage := bson.D{{"$addFields", bson.D{{"string_id", bson.D{{"$toString", "$_id"}}}}}}
	pipeline := mongo.Pipeline{matchStage, skipStage, limitStage, lookupStage, replaceStage, projectStage, transformStage}
	c, err := db.staticDownloads.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	downloads := make([]DownloadResponseDTO, 0)
	err = c.All(ctx, &downloads)
	if err != nil {
		return nil, err
	}
	return downloads, nil
}
