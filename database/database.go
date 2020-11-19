package database

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/user"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// envDBHost holds the name of the environment variable for DB host.
	envDBHost = "SKYNET_DB_HOST"
	// envDBPort holds the name of the environment variable for DB port.
	envDBPort = "SKYNET_DB_PORT"
	// envDBUser holds the name of the environment variable for DB username.
	envDBUser = "SKYNET_DB_USER" // #nosec
	// envDBPass holds the name of the environment variable for DB password.
	envDBPass = "SKYNET_DB_PASS" // #nosec

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

// DB represents a MongoDB database connection.
type DB struct {
	staticDB    *mongo.Database
	staticUsers *mongo.Collection
}

// New returns a new DB connection based on the environment variables.
func New(ctx context.Context) (*DB, error) {
	opts, err := connectionOptionsFromEnv()
	if err != nil {
		return nil, errors.AddContext(err, "failed to get all necessary connection parameters")
	}
	return NewCustom(ctx, opts[envDBUser], opts[envDBPass], opts[envDBHost], opts[envDBPort], dbName)
}

// NewCustom returns a new DB connection based on the passed parameters.
func NewCustom(ctx context.Context, user, pass, host, port, dbname string) (*DB, error) {
	connStr := connectionString(user, pass, host, port)
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	database := c.Database(dbname)
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
func (db *DB) UserByEmail(ctx context.Context, email user.Email) (*user.User, error) {
	if !email.Validate() {
		return nil, user.ErrInvalidEmail
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
func (db *DB) UserByID(ctx context.Context, id string) (*user.User, error) {
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
	var u user.User
	err = c.Decode(&u)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse value from DB")
	}
	return &u, nil
}

// UserCreate creates a new user in the DB. We need the user object to be passed
// by reference because we need to be able to update the ID of new user.
func (db *DB) UserCreate(ctx context.Context, u *user.User) error {
	u.Lock()
	defer u.Unlock()
	if !u.Email.Validate() {
		return user.ErrInvalidEmail
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
	fields := bson.M{
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"email":     u.Email,
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
func (db *DB) UserDelete(ctx context.Context, u *user.User) error {
	u.Lock()
	defer u.Unlock()
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
func (db *DB) UserUpdate(ctx context.Context, u *user.User) error {
	u.Lock()
	defer u.Unlock()
	if !u.Email.Validate() {
		return user.ErrInvalidEmail
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

// managedUsersByField finds all users that have a given field value.
// The calling method is responsible for the validation of the value.
func (db *DB) managedUsersByField(ctx context.Context, fieldName, fieldValue string) ([]*user.User, error) {
	filter := bson.D{{fieldName, fieldValue}}
	c, err := db.staticUsers.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Find")
	}
	var users []*user.User
	for c.Next(ctx) {
		var u user.User
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

// connectionOptionsFromEnv retrieves the DB connection credentials from the
// environment and returns them in a map.
func connectionOptionsFromEnv() (map[string]string, error) {
	opts := make(map[string]string)
	for _, varName := range []string{envDBHost, envDBPort, envDBUser, envDBPass} {
		val, ok := os.LookupEnv(varName)
		if !ok {
			return nil, errors.New("missing env var " + varName)
		}
		opts[varName] = val
	}
	return opts, nil
}

// connectionString is a helper that returns a valid MongoDB connection string
// based on the passed credentials and a set of constants. The connection string
// is using the standalone approach because the service is supposed to talk to
// the replica set only via the local node.
// See https://docs.mongodb.com/manual/reference/connection-string/
func connectionString(user, pass, host, port string) string {
	// There are some symbols in usernames and passwords that need to be escaped.
	// See https://docs.mongodb.com/manual/reference/connection-string/#components
	return fmt.Sprintf(
		"mongodb://%s:%s@%s:%s/?compressors=%s&readPreference=%s&w=%s&wtimeoutMS=%s",
		url.QueryEscape(user),
		url.QueryEscape(pass),
		host,
		port,
		mongoCompressors,
		mongoReadPreference,
		mongoWriteConcern,
		mongoWriteConcernTimeout,
	)
}
