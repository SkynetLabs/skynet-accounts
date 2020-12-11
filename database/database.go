package database

import (
	"context"
	"fmt"
	"net/url"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/lib"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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
	// ErrUserAlreadyExists is returned when we try to use a sub to create a
	// user and a user already exists with this identity.
	ErrUserAlreadyExists = errors.New("identity already belongs to an existing user")
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
		staticDep   lib.Dependencies
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

// UserBySub returns the user with the given sub or nil. The sub is the Kratos
// id of that user.
func (db *DB) UserBySub(ctx context.Context, sub string) (*User, error) {
	users, err := db.managedUsersByField(ctx, "sub", sub)
	if err != nil {
		return nil, err
	}
	// Subs must be unique. If we hit this then we have a serious
	// programmer error which endangers customer's data and finances.
	// We should error out in order to prevent exposing the wrong user data.
	if len(users) > 1 {
		build.Critical(fmt.Sprintf("More than one user found for sub '%s'!", sub))
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

// UserCreate creates a new user in the DB.
func (db *DB) UserCreate(ctx context.Context, sub string, tier int) (*User, error) {
	// Check for an existing user with this sub.
	users, err := db.managedUsersByField(ctx, "sub", sub)
	if err != nil && !errors.Contains(err, ErrUserNotFound) {
		return nil, errors.AddContext(err, "failed to query DB")
	}
	if len(users) > 0 {
		return nil, ErrUserAlreadyExists
	}
	u := &User{
		ID:   primitive.ObjectID{},
		Sub:  sub,
		Tier: tier,
	}
	// Insert the user.
	fields, err := bson.Marshal(u)
	if err != nil {
		return nil, err
	}
	ir, err := db.staticUsers.InsertOne(ctx, fields)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Insert")
	}
	u.ID = ir.InsertedID.(primitive.ObjectID)
	// Sanity check because races exist.
	users, err = db.managedUsersByField(ctx, "sub", sub)
	if len(users) > 1 {
		// Race detected! Sub no longer unique in DB. Delete new user.
		err := db.UserDelete(ctx, u)
		if err != nil {
			build.Critical("Failed to delete new duplicate user! Needs to be cleaned out manually. Offending user id:", u.ID.Hex())
		}
		return nil, ErrUserAlreadyExists
	}
	return u, nil
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

// UserUpdate changes the user's data in the DB.
// It never changes the id or sub of the user.
func (db *DB) UserUpdate(ctx context.Context, u *User) error {
	// Update the user.
	filter := bson.M{"_id": u.ID}
	update := bson.M{"$set": bson.M{
		"tier": u.Tier,
	}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticUsers.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.AddContext(err, "failed to update")
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
