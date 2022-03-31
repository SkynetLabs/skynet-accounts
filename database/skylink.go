package database

import (
	"context"
	"regexp"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// extractSkylinkRE extracts a skylink from the given string. It matches
	// both base32 and base64 skylinks.
	//
	// Note: It's important that we match the base32 first because base32 is a
	// subset of base64, so the base64 regex will match part of the base32 and
	// return partial data which will be useless.
	extractSkylinkRE      = regexp.MustCompile("^.*([a-z0-9]{55})|([a-zA-Z0-9-_]{46}).*$")
	validateSkylinkHashRE = regexp.MustCompile("(^[a-z0-9]{55}$)|(^[a-zA-Z0-9-_]{46}$)")
)

type (
	// FindSkylinksOptions instructs a function that is looking for skylinks
	// what to look for (search terms) and how to order the results. If ordering
	// is not configured, the most relevant (in terms of full text search score)
	// results will be shown first. If there are also no search terms, the
	// results will be ordered by creation timestamp.
	FindSkylinksOptions struct {
		SearchTerms  string
		OrderByField string
		OrderAsc     bool
		Offset       int
		PageSize     int
	}

	// Skylink represents a skylink object in the DB.
	Skylink struct {
		ID      primitive.ObjectID `bson:"_id,omitempty" json:"-"`
		Skylink string             `bson:"skylink" json:"skylink"`
		Size    int64              `bson:"size" json:"size"`
	}
)

// Skylink gets the DB object for the given skylink.
// If it doesn't exist it creates it.
func (db *DB) Skylink(ctx context.Context, skylink string) (*Skylink, error) {
	skylinkHash, err := ExtractSkylinkHash(skylink)
	if err != nil {
		return nil, ErrInvalidSkylink
	}
	// Normalise the skylink. We want skylinks to appear in the same format in
	// the DB, regardless of them being passed as base32 or base64.
	var sl skymodules.Skylink
	err = sl.LoadString(skylinkHash)
	if err != nil {
		return nil, ErrInvalidSkylink
	}
	skylinkHash = sl.String()
	// Provisional skylink object.
	skylinkRec := Skylink{
		Skylink: skylinkHash,
	}
	// Try to find the skylink in the database.
	filter := bson.D{{"skylink", skylinkHash}}
	upsert := bson.M{"$setOnInsert": bson.M{"skylink": skylinkHash}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	sr := db.staticSkylinks.FindOneAndUpdate(ctx, filter, upsert, opts)
	err = sr.Decode(&skylinkRec)
	if err != nil {
		return nil, err
	}
	return &skylinkRec, nil
}

// SkylinkByID finds a skylink by its ID.
func (db *DB) SkylinkByID(ctx context.Context, id primitive.ObjectID) (*Skylink, error) {
	filter := bson.D{{"_id", id}}
	sr := db.staticSkylinks.FindOne(ctx, filter)
	var sl Skylink
	err := sr.Decode(&sl)
	if err != nil {
		return nil, err
	}
	return &sl, nil
}

// SkylinkUpdate updates the metadata about the given skylink. If any of the
// parameters is empty they won't be used in the update operation.
func (db *DB) SkylinkUpdate(ctx context.Context, id primitive.ObjectID, name string, size int64) error {
	filter := bson.M{"_id": id}
	updates := bson.M{}
	if name != "" {
		updates["name"] = name
	}
	if size > 0 {
		updates["size"] = size
	}
	_, err := db.staticSkylinks.UpdateOne(ctx, filter, bson.M{"$set": updates})
	if err != nil {
		return errors.AddContext(err, "failed to update")
	}
	return nil
}

// SkylinkDownloadsUpdate changes the size of the full downloads of this
// skylink. Those should have zero `bytes` in the DB. This method should be
// called from the fetcher.
func (db *DB) SkylinkDownloadsUpdate(ctx context.Context, id primitive.ObjectID, bytes int64) error {
	filter := bson.M{"_id": id}
	updates := bson.M{}
	updates["bytes"] = bytes
	_, err := db.staticDownloads.UpdateMany(ctx, filter, bson.M{"$set": updates})
	if err != nil {
		return errors.AddContext(err, "failed to update")
	}
	return nil
}

// ExtractSkylinkHash extracts the skylink hash from the given skylink that might
// have protocol, path, etc. within it.
func ExtractSkylinkHash(skylink string) (string, error) {
	m := extractSkylinkRE.FindStringSubmatch(skylink)
	if len(m) < 3 || (m[1] == "" && m[2] == "") {
		return "", errors.New("no valid skylink found in string " + skylink)
	}
	if m[1] != "" {
		return m[1], nil
	}
	return m[2], nil
}

// ValidSkylinkHash returns true if the given string is a valid skylink hash.
func ValidSkylinkHash(skylink string) bool {
	return validateSkylinkHashRE.Match([]byte(skylink))
}
