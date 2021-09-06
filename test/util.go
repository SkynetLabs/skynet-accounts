package test

import (
	"context"
	"fmt"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/database"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	// UserSubLen is string length of a user's `sub` field
	UserSubLen = 36
	// skylinkLen is the byte length of a skylink
	skylinkLen = 46
)

var (
	// skylinkCharset lists all character allowed in a skylink
	skylinkCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
)

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "17017",
	}
}

// CreateTestUpload creates a new skyfile and uploads it under the given user's
// account. Returns the skylink, the upload's id and error.
func CreateTestUpload(ctx context.Context, db *database.DB, user *database.User, size int64) (*database.Skylink, primitive.ObjectID, error) {
	// Create a skylink record for which to register an upload
	sl := RandomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to create a test skylink")
	}
	err = db.SkylinkUpdate(ctx, skylink.ID, "test skylink "+sl, size)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to update skylink")
	}
	// Get the updated skylink.
	skylink, err = db.Skylink(ctx, sl)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to fetch skylink from DB")
	}
	if skylink.Size != size {
		return nil, primitive.ObjectID{}, errors.AddContext(err, fmt.Sprintf("expected skylink size to be %d, got %d.", size, skylink.Size))
	}
	// Register an upload.
	return RegisterTestUpload(ctx, db, user, skylink)
}

// RandomSkylink generates a random skylink
func RandomSkylink() string {
	sb := strings.Builder{}
	for i := 0; i < skylinkLen; i++ {
		_ = sb.WriteByte(skylinkCharset[fastrand.Intn(len(skylinkCharset))])
	}
	return sb.String()
}

// RegisterTestUpload registers an upload of the given skylink by the given user.
// Returns the skylink, the upload's id and error.
func RegisterTestUpload(ctx context.Context, db *database.DB, user *database.User, skylink *database.Skylink) (*database.Skylink, primitive.ObjectID, error) {
	up, err := db.UploadCreate(ctx, *user, *skylink)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to register an upload")
	}
	if up.UserID != user.ID {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "expected upload's userId to match the uploader's id")
	}
	if up.SkylinkID != skylink.ID {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "expected upload's skylinkId to match the given skylink's id")
	}
	return skylink, up.ID, nil
}
