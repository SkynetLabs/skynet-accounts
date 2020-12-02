package database

import (
	"bytes"
	"context"
	"fmt"
	"net/url"

	"github.com/NebulousLabs/skynet-accounts/build"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

/*
TODO
 - We should use a tool/library that allows us to catch common ways to go around the unique email requirement, such as adding suffixes to gmail addresses, e.g. ivo@gmail.com and ivo+fake@gmail.com are the same email.
*/

var (
	// mongoCompressors defines the compressors we are going to use for the
	// connection to MongoDB
	mongoCompressors = "zstd,zlib,snappy"
	// mongoReadPreference defines the DB's read preference. The options are:
	// primary, primaryPreferred, secondary, secondaryPreferred, nearest.
	// See https://docs.mongodb.com/manual/core/read-preference/
	mongoReadPreference = "nearest"
	// mongoWriteConcern describes the level of acknowledgment requested from
	// MongoDB.
	mongoWriteConcern = "majority"
	// mongoWriteConcernTimeout specifies a time limit, in milliseconds, for
	// the write concern to be satisfied.
	mongoWriteConcernTimeout = "1000"

	// dbName defines the name of Skynet's database.
	dbName = "skynet"
	// dbUsersCollection defines the name of the "users" collection within
	// skynet's database.
	dbUsersCollection = "users"

	// ErrUserNotFound is returned when we can't find the user in question.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailAlreadyUsed is returned when we try to use an email to either
	// create or update a user and another user already uses this email.
	ErrEmailAlreadyUsed = errors.New("email already in use by another user")
	// ErrGeneralInternalFailure is returned when we do not want to disclose
	// what kind of error occurred. This should always be coupled with another
	// error output for internal use.
	ErrGeneralInternalFailure = errors.New("general internal failure")
)

type (
	// DB represents a MongoDB database connection.
	DB struct {
		staticDB    *mongo.Database
		staticUsers *mongo.Collection
	}

	// DBCredentials is a helper struct that binds together all values needed for
	// establishing a DB connection.
	DBCredentials struct {
		User     string
		Password string
		Host     string
		Port     string
	}
)

// New returns a new DB connection based on the passed parameters.
func New(ctx context.Context, creds DBCredentials) (*DB, error) {
	connStr := connectionString(creds)
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	database := c.Database(dbName)
	users := database.Collection(dbUsersCollection)
	db := &DB{
		staticDB:    database,
		staticUsers: users,
	}
	return db, nil
}

// Disconnect closes the connection to the database in an orderly fashion.
func (db *DB) Disconnect(ctx context.Context) error {
	return db.staticDB.Client().Disconnect(ctx)
}

// UserByEmail returns the user with the given email or nil.
func (db *DB) UserByEmail(ctx context.Context, email Email) (*User, error) {
	if !email.Validate() {
		return nil, ErrInvalidEmail
	}
	users, err := db.managedUsersByField(ctx, "email", string(email))
	if err != nil {
		return nil, err
	}
	// Emails must be unique. If we hit this then we have a serious
	// programmer error which endangers customer's data and finances.
	// We should error out in order to prevent exposing the wrong user data.
	if len(users) > 1 {
		build.Critical(fmt.Sprintf("More than one user found for email '%s'!", email))
		// The error message is intentionally cryptic.
		return nil, ErrGeneralInternalFailure
	}
	return users[0], nil
}

// UserByID finds a user by their ID.
func (db *DB) UserByID(ctx context.Context, id string) (*User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse user ID")
	}
	filter := bson.D{{"_id", oid}}
	c, err := db.staticUsers.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Find")
	}
	// Get the first result.
	if ok := c.Next(ctx); !ok {
		return nil, ErrUserNotFound
	}
	// Ensure there are no more results.
	if ok := c.Next(ctx); ok {
		build.Critical("more than one user found for id", id)
	}
	var u User
	err = c.Decode(&u)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse value from DB")
	}
	return &u, nil
}

// UserCreate creates a new user in the DB. We need the user object to be passed
// by reference because we need to be able to update the ID of new user.
func (db *DB) UserCreate(ctx context.Context, u *User) error {
	if !u.Email.Validate() {
		return ErrInvalidEmail
	}
	// Check for an existing user with this email.
	users, err := db.managedUsersByField(ctx, "email", string(u.Email))
	if err != nil && !errors.Contains(err, ErrUserNotFound) {
		return errors.AddContext(err, "failed to query DB")
	}
	if len(users) > 0 {
		return ErrEmailAlreadyUsed
	}
	// Insert the user.
	fields, err := bson.Marshal(u)
	if err != nil {
		return err
	}
	ir, err := db.staticUsers.InsertOne(ctx, fields)
	if err != nil {
		return errors.AddContext(err, "failed to Insert")
	}
	u.ID = ir.InsertedID.(primitive.ObjectID)
	// Sanity check because races exist.
	users, err = db.managedUsersByField(ctx, "email", string(u.Email))
	if len(users) > 1 {
		// Race detected! Email no longer unique in DB. Delete new user.
		err := db.UserDelete(ctx, u)
		if err != nil {
			build.Critical("Failed to delete new duplicate user! Needs to be cleaned out manually. Offending user id:", u.ID.Hex())
		}
		return ErrEmailAlreadyUsed
	}
	return nil
}

// UserDelete deletes a user by their ID.
func (db *DB) UserDelete(ctx context.Context, u *User) error {
	if u.ID.IsZero() {
		return errors.AddContext(ErrUserNotFound, "user struct not fully initialised")
	}
	filter := bson.D{{"_id", u.ID}}
	dr, err := db.staticUsers.DeleteOne(ctx, filter)
	if err != nil {
		return errors.AddContext(err, "failed to Delete")
	}
	if dr.DeletedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UserUpdate saves the user in the DB.
func (db *DB) UserUpdate(ctx context.Context, u *User) error {
	if !u.Email.Validate() {
		return ErrInvalidEmail
	}
	// Check for an existing user with this email.
	users, err := db.managedUsersByField(ctx, "email", string(u.Email))
	if err != nil && !errors.Contains(err, ErrUserNotFound) {
		return errors.AddContext(err, "failed to query DB")
	}
	// Sanity check.
	if len(users) > 1 {
		build.Critical("More than one user found with email", u.Email)
		return ErrEmailAlreadyUsed
	}
	if len(users) > 0 && !bytes.Equal(u.ID[:], users[0].ID[:]) {
		return ErrEmailAlreadyUsed
	}
	// TODO What if we have a race a user gets this email right at this point?
	// Update the user.
	filter := bson.M{"_id": u.ID}
	update := bson.M{"$set": bson.M{
		"_id":       u.ID,
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"email":     u.Email,
	}}
	ur, err := db.staticUsers.UpdateOne(ctx, filter, update)
	if err != nil {
		return errors.AddContext(err, "failed to Update")
	}
	if ur.UpsertedCount > 1 || ur.ModifiedCount > 1 {
		build.Critical(fmt.Sprintf("updated more than one user! filter: %v, update: %v, update result:%v\n", filter, update, ur))
	}
	return nil
}

// UserUpdatePassword implements the entire password changing process - it
// verifies that the user exist, that old password is correct, sets the new
// password and saves the changes to the DB.
// TODO Wrap this into a transaction. https://docs.mongodb.com/manual/core/transactions/
func (db *DB) UserUpdatePassword(ctx context.Context, uid, oldPass, newPass string) error {
	// Update the user.
	u, err := db.UserByID(ctx, uid)
	if err != nil {
		return errors.AddContext(err, "can't fetch user")
	}
	err = u.VerifyPassword(oldPass)
	if err != nil {
		return errors.AddContext(err, "invalid password")
	}
	err = u.SetPassword(newPass)
	if err != nil {
		return errors.AddContext(err, "failed to set new password")
	}

	// Persist the changes.
	filter := bson.M{"_id": u.ID}
	update := bson.M{"$set": bson.M{
		"_id":      u.ID,
		"password": u.Password,
		"salt":     u.Salt,
	}}
	ur, err := db.staticUsers.UpdateOne(ctx, filter, update)
	if err != nil {
		return errors.AddContext(err, "failed to Update")
	}
	if ur.UpsertedCount > 1 || ur.ModifiedCount > 1 {
		build.Critical(fmt.Sprintf("updated more than one user! user_id used: %v\n", u.ID.Hex()))
	}
	return nil
}

// managedUsersByField finds all users that have a given field value.
// The calling method is responsible for the validation of the value.
func (db *DB) managedUsersByField(ctx context.Context, fieldName, fieldValue string) ([]*User, error) {
	filter := bson.D{{fieldName, fieldValue}}
	c, err := db.staticUsers.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Find")
	}
	var users []*User
	for c.Next(ctx) {
		var u User
		if err = c.Decode(&u); err != nil {
			return nil, errors.AddContext(err, "failed to parse value from DB")
		}
		users = append(users, &u)
	}
	if len(users) == 0 {
		return users, ErrUserNotFound
	}
	return users, nil
}

// connectionString is a helper that returns a valid MongoDB connection string
// based on the passed credentials and a set of constants. The connection string
// is using the standalone approach because the service is supposed to talk to
// the replica set only via the local node.
// See https://docs.mongodb.com/manual/reference/connection-string/
func connectionString(creds DBCredentials) string {
	// There are some symbols in usernames and passwords that need to be escaped.
	// See https://docs.mongodb.com/manual/reference/connection-string/#components
	return fmt.Sprintf(
		"mongodb://%s:%s@%s:%s/?compressors=%s&readPreference=%s&w=%s&wtimeoutMS=%s",
		url.QueryEscape(creds.User),
		url.QueryEscape(creds.Password),
		creds.Host,
		creds.Port,
		mongoCompressors,
		mongoReadPreference,
		mongoWriteConcern,
		mongoWriteConcernTimeout,
	)
}
