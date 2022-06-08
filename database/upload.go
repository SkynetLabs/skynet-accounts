package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SkynetLabs/skynet-accounts/skynet"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Upload ...
type Upload struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	UploaderIP string             `bson:"uploader_ip" json:"uploaderIP"`
	SkylinkID  primitive.ObjectID `bson:"skylink_id,omitempty" json:"skylinkId"`
	Timestamp  time.Time          `bson:"timestamp" json:"timestamp"`
	Unpinned   bool               `bson:"unpinned" json:"-"`
}

// UploadResponse is the representation of an upload we send as response to
// the caller.
type UploadResponse struct {
	ID         string    `bson:"_id" json:"id"`
	Skylink    string    `bson:"skylink" json:"skylink"`
	Name       string    `bson:"name" json:"name"`
	Size       int64     `bson:"size" json:"size"`
	RawStorage int64     `bson:"raw_storage" json:"rawStorage"`
	Timestamp  time.Time `bson:"timestamp" json:"uploadedOn"`
}

// UnpinUploads unpins all uploads of this skylink by this user. Returns
// the number of unpinned uploads.
func (db *DB) UnpinUploads(ctx context.Context, skylink Skylink, user User) (int64, error) {
	if skylink.ID.IsZero() {
		return 0, ErrInvalidSkylink
	}
	if user.ID.IsZero() {
		return 0, errors.New("invalid user")
	}
	filter := bson.M{
		"skylink_id": skylink.ID,
		"user_id":    user.ID,
		"unpinned":   false,
	}
	update := bson.M{"$set": bson.M{"unpinned": true}}
	ur, err := db.staticUploads.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, err
	}
	return ur.ModifiedCount, nil
}

// UploadByID fetches a single upload from the DB.
func (db *DB) UploadByID(ctx context.Context, id primitive.ObjectID) (*Upload, error) {
	var d Upload
	sr := db.staticUploads.FindOne(ctx, bson.M{"_id": id})
	err := sr.Decode(&d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UploadCreate registers a new upload and counts it towards the user's used
// storage.
func (db *DB) UploadCreate(ctx context.Context, user User, ip string, skylink Skylink) (*Upload, error) {
	if skylink.ID.IsZero() {
		return nil, errors.New("skylink doesn't exist")
	}
	up := Upload{
		UserID:     user.ID,
		UploaderIP: ip,
		SkylinkID:  skylink.ID,
		Timestamp:  time.Now().UTC().Truncate(time.Millisecond),
	}
	ior, err := db.staticUploads.InsertOne(ctx, up)
	if err != nil {
		return nil, err
	}
	up.ID = ior.InsertedID.(primitive.ObjectID)
	return &up, nil
}

// UploadsBySkylink fetches a page of uploads of this skylink and the total
// number of such uploads.
func (db *DB) UploadsBySkylink(ctx context.Context, skylink Skylink, offset, pageSize int) ([]UploadResponse, int, error) {
	if skylink.ID.IsZero() {
		return nil, 0, ErrInvalidSkylink
	}
	matchStage := bson.D{{"$match", bson.D{
		{"skylink_id", skylink.ID},
		{"unpinned", false},
	}}}
	opts := FindSkylinksOptions{
		Offset:   offset,
		PageSize: pageSize,
	}
	return db.uploadsBy(ctx, matchStage, opts)
}

// UploadsBySkylinkID returns all uploads of the given skylink.
func (db *DB) UploadsBySkylinkID(ctx context.Context, slID primitive.ObjectID) ([]Upload, error) {
	if slID.IsZero() {
		return nil, ErrInvalidSkylink
	}
	c, err := db.staticUploads.Find(ctx, bson.M{"skylink_id": slID})
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

// UploadsByUser fetches a page of uploads by this user and the total number of
// such uploads.
func (db *DB) UploadsByUser(ctx context.Context, user User, opts FindSkylinksOptions) ([]UploadResponse, int, error) {
	if user.ID.IsZero() {
		return nil, 0, errors.New("invalid user")
	}
	var matchStage bson.D
	if len(opts.SearchTerms) == 0 {
		matchStage = bson.D{{"$match", bson.D{
			{"user_id", user.ID},
			{"unpinned", false},
		}}}
		// If the client didn't specifically select ordering, we'll order by
		// timestamp in descending order.
		if opts.OrderByField == "" {
			opts.OrderByField = "timestamp"
			opts.OrderAsc = false
		}
	} else {
		matchStage = bson.D{{"$match", bson.D{
			{"user_id", user.ID},
			{"unpinned", false},
			{"$text", bson.D{{"$search", opts.SearchTerms}}},
		}}}
		// If the client didn't specifically select ordering, we'll order by
		// the most relevant result.
		if opts.OrderByField == "" {
			opts.OrderByField = "textScore"
			opts.OrderAsc = false
		}
	}
	// db.getCollection('skylinks').find({ $text: { $search: "lines logo" } }, { score: { $meta: "textScore" } }).sort( { score: { $meta: "textScore" } } )
	return db.uploadsBy(ctx, matchStage, opts)
}

// uploadsBy fetches a page of uploads, filtered by an arbitrary match criteria.
// It also reports the total number of records in the list.
func (db *DB) uploadsBy(ctx context.Context, matchStage bson.D, opts FindSkylinksOptions) ([]UploadResponse, int, error) {
	opts.Offset, opts.PageSize = validOffsetPageSize(opts.Offset, opts.PageSize)
	cnt := 10 // TODO Re-enable the actual count
	// cnt, err := db.count(ctx, db.staticUploads, matchStage)
	// if err != nil || cnt == 0 {
	// 	return []UploadResponse{}, 0, err
	// }

	var c *mongo.Cursor
	var err error
	if opts.OrderByField == "textScore" {
		c, err = db.uploadsAggregateText(ctx, matchStage, opts)
	} else {
		c, err = db.uploadsAggregateGeneral(ctx, matchStage, opts)
	}
	if err != nil {
		return nil, 0, err
	}
	uploads := make([]UploadResponse, opts.PageSize)
	// uploads := make([]interface{}, opts.PageSize)
	err = c.All(ctx, &uploads)
	if err != nil {
		return nil, 0, err
	}
	fmt.Printf(" >  uploads %+v\n", uploads)
	for idx := range uploads {
		uploads[idx].RawStorage = skynet.RawStorageUsed(uploads[idx].Size)
	}
	return uploads, int(cnt), nil
}

func (db *DB) uploadsAggregateGeneral(ctx context.Context, matchStage bson.D, opts FindSkylinksOptions) (*mongo.Cursor, error) {
	pipeline := generateUploadsPipeline(matchStage, opts)
	fmt.Printf(" >  pipeline %+v\n", pipeline)
	return db.staticUploads.Aggregate(ctx, pipeline)
}

func (db *DB) uploadsAggregateText(ctx context.Context, matchStage bson.D, opts FindSkylinksOptions) (*mongo.Cursor, error) {
	pipeline := generateUploadsPipelineText(matchStage, opts)
	c, e := db.staticSkylinks.Indexes().List(ctx)
	if e != nil {
		panic(e)
	}
	idxs := make([]interface{}, 1000)
	e = c.All(ctx, &idxs)
	if e != nil {
		panic(e)
	}
	fmt.Printf("\n\n > Indexes: %+v\n\n", idxs)
	pp, _ := json.Marshal(pipeline)
	fmt.Printf(" >  pipeline %+v\n", string(pp))
	return db.staticSkylinks.Aggregate(ctx, pipeline)
}

// validOffsetPageSize returns valid values for offset and page size. If the
// input values are invalid, it returns defaults.
func validOffsetPageSize(offset, pageSize int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	return offset, pageSize
}
