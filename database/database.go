package database

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/NebulousLabs/skynet-accounts/lib"
	"github.com/sirupsen/logrus"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// dbName defines the name of Skynet's database.
	dbName = "skynet"
	// dbUsersCollection defines the name of the "users" collection within
	// skynet's database.
	dbUsersCollection = "users"
	// dbSkylinksCollection defines the name of the "skylinks" collection within
	// skynet's database.
	dbSkylinksCollection = "skylinks"
	// dbUploadsCollection defines the name of the "uploads" collection within
	// skynet's database.
	dbUploadsCollection = "uploads"
	// dbDownloadsCollection defines the name of the "downloads" collection within
	// skynet's database.
	dbDownloadsCollection = "downloads"

	// defaultPageSize defines the default number of records to return.
	defaultPageSize = 10

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

	// ErrGeneralInternalFailure is returned when we do not want to disclose
	// what kind of error occurred. This should always be coupled with another
	// error output for internal use.
	ErrGeneralInternalFailure = errors.New("general internal failure")
	// ErrUserNotFound is returned when we can't find the user in question.
	ErrUserNotFound = errors.New("user not found")
	// ErrUserAlreadyExists is returned when we try to use a sub to create a
	// user and a user already exists with this identity.
	ErrUserAlreadyExists = errors.New("identity already belongs to an existing user")
	// ErrInvalidSkylink is returned when the given string is not a valid
	// skylink.
	ErrInvalidSkylink = errors.New("invalid skylink")

	// ValidSortingValues defines a set to valid sorting values to be used when fetching uploads and downloads.
	ValidSortingValues = map[string]string{
		"date": "timestamp",
		"name": "name",
		"size": "size",
	}
)

type (
	// DB represents a MongoDB database connection.
	DB struct {
		staticDB        *mongo.Database
		staticUsers     *mongo.Collection
		staticSkylinks  *mongo.Collection
		staticUploads   *mongo.Collection
		staticDownloads *mongo.Collection
		staticDep       lib.Dependencies
		staticLogger    *logrus.Logger
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
func New(ctx context.Context, creds DBCredentials, logger *logrus.Logger) (*DB, error) {
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
	if logger == nil {
		logger = &logrus.Logger{}
	}
	err = ensureDBSchema(ctx, database, logger)
	if err != nil {
		return nil, err
	}
	db := &DB{
		staticDB:        database,
		staticUsers:     database.Collection(dbUsersCollection),
		staticSkylinks:  database.Collection(dbSkylinksCollection),
		staticUploads:   database.Collection(dbUploadsCollection),
		staticDownloads: database.Collection(dbDownloadsCollection),
		staticLogger:    logger,
	}
	return db, nil
}

// Disconnect closes the connection to the database in an orderly fashion.
func (db *DB) Disconnect(ctx context.Context) error {
	return db.staticDB.Client().Disconnect(ctx)
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

// ensureDBSchema checks that we have all collections and indexes we need and
// creates them if needed.
// See https://docs.mongodb.com/manual/indexes/
// See https://docs.mongodb.com/manual/core/index-unique/
func ensureDBSchema(ctx context.Context, db *mongo.Database, log *logrus.Logger) error {
	// schema defines a mapping between a collection name and the indexes that
	// must exist for that collection.
	schema := map[string][]mongo.IndexModel{
		dbUsersCollection: {
			{
				Keys:    bson.D{{"sub", 1}},
				Options: options.Index().SetName("sub_unique").SetUnique(true),
			},
		},
		dbSkylinksCollection: {
			{
				Keys:    bson.D{{"skylink", 1}},
				Options: options.Index().SetName("skylink_unique").SetUnique(true),
			},
		},
		dbUploadsCollection: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.D{{"skylink_id", 1}},
				Options: options.Index().SetName("skylink_id"),
			},
		},
		dbDownloadsCollection: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.D{{"skylink_id", 1}},
				Options: options.Index().SetName("skylink_id"),
			},
		},
	}
	for collName, models := range schema {
		coll, err := ensureCollection(ctx, db, collName)
		if err != nil {
			return err
		}
		iv := coll.Indexes()
		names, err := iv.CreateMany(ctx, models)
		if err != nil {
			return errors.AddContext(err, "failed to create indexes")
		}
		log.Debugf("Created new indexes: %v\n", names)
	}
	return nil
}

// ensureCollection gets the given collection from the
// database and creates it if it doesn't exist.
func ensureCollection(ctx context.Context, db *mongo.Database, collName string) (*mongo.Collection, error) {
	coll := db.Collection(collName)
	if coll == nil {
		err := db.CreateCollection(ctx, dbUsersCollection)
		if err != nil {
			return nil, err
		}
		coll = db.Collection(dbUsersCollection)
		if coll == nil {
			return nil, errors.New("failed to create collection " + dbUsersCollection)
		}
	}
	return coll, nil
}
