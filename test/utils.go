package test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/SkynetLabs/skynet-accounts/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.sia.tech/siad/crypto"
)

const (
	// FauxEmailURI is a valid URI for sending emails that points to a local
	// mailslurper instance. That instance is most probably not running, so
	// trying to send mails with it will fail, but it's useful for testing with
	// the DependencySkipSendingEmails.
	FauxEmailURI = "smtps://test:test1@mailslurper:1025/?skip_ssl_verify=true"

	// UserSubLen is string length of a user's `sub` field
	UserSubLen = 36
)

type (
	// User is a helper struct that allows for easy cleanup from the DB.
	// It's called User and not TestUser because lint won't allow that.
	User struct {
		*database.User
		staticDB *database.DB
	}
)

// Delete removes the test user from the DB.
func (tu *User) Delete(ctx context.Context) error {
	return tu.staticDB.UserDelete(ctx, tu.User)
}

// DBNameForTest sanitizes the input string, so it can be used as an email or
// sub.
func DBNameForTest(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "17017",
	}
}

// CreateUser is a helper method which simplifies the creation of test users
func CreateUser(at *AccountsTester, email, password string) (*User, error) {
	// Create a user.
	_, _, err := at.CreateUserPost(email, password)
	if err != nil {
		return nil, errors.AddContext(err, "user creation failed")
	}
	// Fetch the user from the DB, so we can delete it later.
	u, err := at.DB.UserByEmail(at.Ctx, email)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch user from the DB")
	}
	database.SetNonDBFields(u)
	return &User{u, at.DB}, nil
}

// CreateUserAndLogin is a helper method that creates a new test user and
// immediately logs in with it, returning the user, the login cookie, a cleanup
// function that deletes the user.
func CreateUserAndLogin(at *AccountsTester, name string) (*User, *http.Cookie, error) {
	// Use the test's name as an email-compatible identifier.
	params := url.Values{}
	params.Add("email", DBNameForTest(name)+"@siasky.net")
	params.Add("password", hex.EncodeToString(fastrand.Bytes(16)))
	// Create a user.
	u, err := CreateUser(at, params.Get("email"), params.Get("password"))
	if err != nil {
		return nil, nil, err
	}
	// Log in with that user in order to make sure it exists.
	r, _, err := at.Post("/login", nil, params)
	if err != nil {
		return nil, nil, err
	}
	// Grab the Skynet cookie, so we can make authenticated calls.
	c := ExtractCookie(r)
	if c == nil {
		return nil, nil, err
	}
	return u, c, nil
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
	var h crypto.Hash
	fastrand.Read(h[:])
	sl, _ := skymodules.NewSkylinkV1(h, 0, 0)
	return sl.String()
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
