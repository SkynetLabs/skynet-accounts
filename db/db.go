package db

import (
	"context"
	"fmt"
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
	// SkynetDBHostEV holds the name of the environment variable for DB host.
	SkynetDBHostEV = "SKYNET_DB_HOST"
	// SkynetDBPortEV holds the name of the environment variable for DB port.
	SkynetDBPortEV = "SKYNET_DB_PORT"
	// SkynetDBUserEV holds the name of the environment variable for DB username.
	SkynetDBUserEV = "SKYNET_DB_USER"
	// SkynetDBPassEV holds the name of the environment variable for DB password.
	SkynetDBPassEV = "SKYNET_DB_PASS"

	// SkynetDBName defines the name of Skynet's database.
	SkynetDBName = "skynet"
	// SkynetDBUsersCollection defines the name of the "users" collection within
	// skynet's database.
	SkynetDBUsersCollection = "users"

	// TODO Set these to sane defaults and use them in Collection before the user's passed values, so the user can override them.
	SkynetDBWriteConcern = ""
	SkynetDBReadConcern  = ""

	// ErrUserNotFound is returned when we can't find the user in question.
	ErrUserNotFound = errors.New("user not found")
)

type DB struct {
	db    *mongo.Database
	users *mongo.Collection
}

// New returns a new DB connection based on the environment variables.
func New(ctx context.Context) (*DB, error) {
	connStr, err := connectionStrFromEnv()
	if err != nil {
		return nil, errors.AddContext(err, "failed to get all necessary connection parameters")
	}
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	database := c.Database(SkynetDBName)
	users := database.Collection(SkynetDBUsersCollection)
	db := &DB{
		db:    database,
		users: users,
	}
	return db, nil
}

// NewCustom returns a new DB connection based on the passed parameters.
func NewCustom(ctx context.Context, user, pass, host, port, dbname string) (*DB, error) {
	connStr := fmt.Sprintf("mongodb://%s:%s@%s:%s/?compressors=zlib&gssapiServiceName=mongodb&readPreference=nearest", user, pass, host, port)
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	database := c.Database(dbname)
	users := database.Collection(SkynetDBUsersCollection)
	db := &DB{
		db:    database,
		users: users,
	}
	return db, nil
}

// Collection returns a collection object.
// If no collection with that name exists, it creates one.
// TODO Do we need this one or we'd rather not expose the underlying DB?
func (db *DB) Collection(cn string, opts ...*options.CollectionOptions) *mongo.Collection {
	return db.db.Collection(cn, opts...)
}

// UserDeleteByID deletes a user by their ID and returns `false` in case there
// was an error or the user was not found.
func (db *DB) UserDeleteByID(ctx context.Context, id string) (bool, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return false, errors.AddContext(err, "failed to parse user ID")
	}
	filter := bson.D{{"_id", oid}}
	dr, err := db.users.DeleteOne(ctx, filter)
	if err != nil {
		return false, errors.AddContext(err, "failed to Delete")
	}
	return dr.DeletedCount == 1, nil
}

// UserFindByID finds a user by their ID.
func (db *DB) UserFindByID(ctx context.Context, id string) (*user.User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse user ID")
	}
	filter := bson.D{{"_id", oid}}
	c, err := db.users.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Find")
	}
	// TODO check what Next does.
	if ok := c.Next(ctx); !ok {
		return nil, ErrUserNotFound
	}
	if ok := c.Next(ctx); ok {
		build.Critical("more than one user found for id", id)
	}
	var u user.User
	err = c.Decode(&u)
	if err != nil {
		fmt.Printf("UserFindByID: %+v\n", c.Current)
		return nil, errors.AddContext(err, "failed to parse value from DB")
	}
	return &u, nil
}

// UserSave saves the user in the DB and returns `true` if the user was newly
// inserted and `false` if it already existed before.
// We need the user object to be passed by reference because we need to be able
// to update the ID of new users.
func (db *DB) UserSave(ctx context.Context, u *user.User) (bool, error) {
	// TODO when we don't have an id find the user by their email and compare
	// 	the fields. If there is a discrepancy return an error.
	if !u.ID.IsZero() {
		filter := bson.D{{"_id", u.ID}}
		ur, err := db.users.UpdateOne(ctx, filter, u)
		if err != nil {
			return false, errors.AddContext(err, "failed to Update")
		}
		if ur.MatchedCount == 1 {
			return false, nil
		}
	}
	ir, err := db.users.InsertOne(ctx, u)
	if err != nil {
		// This `true` is leaking information but that information is already
		// available via the specific error returned.
		return true, errors.AddContext(err, "failed to Insert")
	}
	u.ID = ir.InsertedID.(primitive.ObjectID)
	return true, nil
}

// connectionStrFromEnv retrieves the DB connection credentials from the
// environment and builds a connection string.
func connectionStrFromEnv() (string, error) {
	cs := make(map[string]string)
	for _, varName := range []string{SkynetDBHostEV, SkynetDBPortEV, SkynetDBUserEV, SkynetDBPassEV} {
		val, ok := os.LookupEnv(varName)
		if !ok {
			return "", errors.New("missing env var " + varName)
		}
		cs[varName] = val
	}
	return fmt.Sprintf("mongodb://%s:%s@%s:%s/?compressors=zlib&gssapiServiceName=mongodb&readPreference=nearest", cs[SkynetDBUserEV], cs[SkynetDBPassEV], cs[SkynetDBHostEV], cs[SkynetDBPortEV]), nil
}
