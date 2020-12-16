package database

import (
	"context"
	"regexp"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	skylinkRE *regexp.Regexp = nil
)

// Skylink represents a skylink object in the DB.
type Skylink struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Skylink string             `bson:"skylink" json:"skylink"`
	Size    uint64             `bson:"size" json:"size"`
}

// Skylink gets the DB object for the given skylink.
// If it doesn't exist it creates it.
func (db *DB) Skylink(ctx context.Context, skylink string) (*Skylink, error) {
	skylinkHash, err := validateSkylink(skylink)
	if err != nil {
		return nil, ErrInvalidSkylink
	}
	// Provisional skylink object.
	skylinkRec := Skylink{
		Skylink: skylinkHash,
	}
	// Try to fins the skylink in the database.
	filter := bson.D{{"skylink", skylinkHash}}
	sr := db.staticSkylinks.FindOne(ctx, filter)
	err = sr.Decode(&skylinkRec)
	if err == mongo.ErrNoDocuments {
		// It's not there, insert it.
		var ior *mongo.InsertOneResult
		ior, err = db.staticSkylinks.InsertOne(ctx, skylinkRec)
		if err == nil {
			skylinkRec.ID = ior.InsertedID.(primitive.ObjectID)
		}
	}
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

// validateSkylink extracts the skylink hash from the given skylink that might
// have protocol, path, etc. within it.
func validateSkylink(skylink string) (string, error) {
	if skylinkRE == nil {
		skylinkRE = regexp.MustCompile("^.*([a-zA-Z0-9-_]{46}).*$")
	}
	m := skylinkRE.FindStringSubmatch(skylink)
	if len(m) < 2 {
		return "", errors.New("no valid skylink found in string " + skylink)
	}
	return m[1], nil
}
