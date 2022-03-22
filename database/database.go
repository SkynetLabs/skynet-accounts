package database

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/SkynetLabs/skynet-accounts/lib"
	lock "github.com/square/mongo-lock"
	"go.sia.tech/siad/build"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// dbName defines the name of Skynet's database.
	dbName = "skynet"
	// collUsers defines the name of the "users" collection within
	// skynet's database.
	collUsers = "users"
	// collSkylinks defines the name of the "skylinks" collection within
	// skynet's database.
	collSkylinks = "skylinks"
	// collUploads defines the name of the "uploads" collection within
	// skynet's database.
	collUploads = "uploads"
	// collDownloads defines the name of the "downloads" collection within
	// skynet's database.
	collDownloads = "downloads"
	// collRegistryReads defines the name of the "registry_reads"
	// collection within skynet's database.
	collRegistryReads = "registry_reads"
	// collRegistryWrites defines the name of the "registry_writes"
	// collection within skynet's database.
	collRegistryWrites = "registry_writes"
	// collEmails defines the name of the "emails" collection within skynet's
	// database.
	collEmails = "emails"
	// collChallenges defines the name of the "challenges" collection within
	// skynet's database.
	collChallenges = "challenges"
	// collUnconfirmedUserUpdates defines the name of the collection which holds
	// all user pubKey updates until their respective challenge has been
	// responded to and they are applied.
	collUnconfirmedUserUpdates = "unconfirmed_user_updates"
	// collConfiguration defines the name of the db table with configuration
	// settings.
	collConfiguration = "configuration"
	// collAPIKeys defines the name of the collection with API keys for users.
	collAPIKeys = "api_keys"
	// collLocks defines the name of the collection used for distributed locks.
	collLocks = "locks"

	// mongoCompressors defines the compressors we are going to use for the
	// connection to MongoDB
	mongoCompressors = "zstd,zlib,snappy"
	// mongoReadPreference defines the DB's read preference. The options are:
	// primary, primaryPreferred, secondary, secondaryPreferred, nearest.
	// See https://docs.mongodb.com/manual/core/read-preference/
	mongoReadPreference = "primary"
	// mongoWriteConcern describes the level of acknowledgment requested from
	// MongoDB.
	mongoWriteConcern = "majority"
	// mongoWriteConcernTimeout specifies a time limit, in milliseconds, for
	// the write concern to be satisfied.
	mongoWriteConcernTimeout = "30000"

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

	// DefaultPageSize defines the default number of records to return.
	DefaultPageSize = 10

	// LockRetrySleep defines how long we'll sleep between attempts to lock a
	// record that's already locked.
	LockRetrySleep = 10. * time.Millisecond

	// ServerLockID holds the name of the name of this particular server. Its
	// value is controlled by the SERVER_DOMAIN entry in the .env file. If the
	// SERVER_DOMAIN entry is empty or missing, the PORTAL_DOMAIN (preceded by
	// schema) will be used instead. The only exception is testing where there's
	// nothing to set it, so we want to always have it set.
	ServerLockID = build.Select(
		build.Var{
			Dev:      "",
			Testing:  "siasky.test",
			Standard: "",
		},
	).(string)
)

type (
	// DB represents a MongoDB database connection.
	DB struct {
		staticDB                     *mongo.Database
		staticUsers                  *mongo.Collection
		staticSkylinks               *mongo.Collection
		staticUploads                *mongo.Collection
		staticDownloads              *mongo.Collection
		staticRegistryReads          *mongo.Collection
		staticRegistryWrites         *mongo.Collection
		staticEmails                 *mongo.Collection
		staticChallenges             *mongo.Collection
		staticUnconfirmedUserUpdates *mongo.Collection
		staticConfiguration          *mongo.Collection
		staticAPIKeys                *mongo.Collection
		staticLockClient             *lock.Client
		staticDeps                   lib.Dependencies
		staticLogger                 *logrus.Logger
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
	return NewCustomDB(ctx, dbName, creds, logger)
}

// NewCustomDB returns a new DB connection based on the passed parameters.
func NewCustomDB(ctx context.Context, dbName string, creds DBCredentials, logger *logrus.Logger) (*DB, error) {
	connStr := connectionString(creds)
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	db := c.Database(dbName)
	if logger == nil {
		logger = &logrus.Logger{}
	}
	err = ensureDBSchema(ctx, db, Schema, logger)
	if err != nil {
		return nil, err
	}
	lockClient := lock.NewClient(db.Collection(collLocks))
	err = lockClient.CreateIndexes(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to create the indexes needed for locking")
	}
	return &DB{
		staticDB:                     db,
		staticUsers:                  db.Collection(collUsers),
		staticSkylinks:               db.Collection(collSkylinks),
		staticUploads:                db.Collection(collUploads),
		staticDownloads:              db.Collection(collDownloads),
		staticRegistryReads:          db.Collection(collRegistryReads),
		staticRegistryWrites:         db.Collection(collRegistryWrites),
		staticEmails:                 db.Collection(collEmails),
		staticChallenges:             db.Collection(collChallenges),
		staticUnconfirmedUserUpdates: db.Collection(collUnconfirmedUserUpdates),
		staticConfiguration:          db.Collection(collConfiguration),
		staticAPIKeys:                db.Collection(collAPIKeys),
		staticLockClient:             lockClient,
		staticLogger:                 logger,
	}, nil
}

// Disconnect closes the connection to the database in an orderly fashion.
func (db *DB) Disconnect(ctx context.Context) error {
	return db.staticDB.Client().Disconnect(ctx)
}

// NewSession starts a new Mongo session.
func (db *DB) NewSession() (mongo.Session, error) {
	return db.staticDB.Client().StartSession()
}

// Lock creates a new distributed database lock.
func (db *DB) Lock(ctx context.Context, resource, lockID, owner string) (err error) {
	ld := lock.LockDetails{
		Owner: owner,
		Host:  ServerLockID,
	}
	for {
		err = db.staticLockClient.XLock(ctx, resource, lockID, ld)
		// If the resource is already locked we'll sleep for a bit and then
		// retry. We'll do that until the context times out.
		if errors.Contains(err, lock.ErrAlreadyLocked) {
			select {
			// TODO Should this wait for the context to time out or have its own TTL?
			// 	Personally, I would prefer own TTL but I don't know what would make a good value.
			// 	5 seconds? 30? Custom, specified by the caller?
			case <-ctx.Done():
				break
			default:
			}
			time.Sleep(LockRetrySleep)
			continue
		}
		// On success or any other we'll break and return.
		break
	}
	return err
}

// Unlock removes an existing distributed database lock.
func (db *DB) Unlock(ctx context.Context, lockID string) ([]lock.LockStatus, error) {
	return db.staticLockClient.Unlock(ctx, lockID)
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
func ensureDBSchema(ctx context.Context, db *mongo.Database, schema map[string][]mongo.IndexModel, log *logrus.Logger) error {
	for collName, models := range schema {
		coll, err := ensureCollection(ctx, db, collName)
		if err != nil {
			return err
		}
		iv := coll.Indexes()
		var names []string
		names, err = iv.CreateMany(ctx, models)
		if err != nil {
			return errors.AddContext(err, "failed to create indexes")
		}
		log.Debugf("Ensured index exists: %v", names)
	}
	return nil
}

// ensureCollection gets the given collection from the
// database and creates it if it doesn't exist.
func ensureCollection(ctx context.Context, db *mongo.Database, collName string) (*mongo.Collection, error) {
	coll := db.Collection(collName)
	if coll == nil {
		err := db.CreateCollection(ctx, collName)
		if err != nil {
			return nil, err
		}
		coll = db.Collection(collName)
		if coll == nil {
			return nil, errors.New("failed to create collection " + collName)
		}
	}
	return coll, nil
}

// generateUploadsPipeline generates a mongo pipeline for transforming
// an `Upload` or `Download` struct into the respective
// `<Up/Down>loadResponse` struct.
//
// The Mongo query we want to ultimately execute is:
//	db.downloads.aggregate([
//		{ $match: { "user_id": ObjectId("5fda32ef6e0aba5d16c0d550") }},
//		{ $skip: 1 },
//		{ $limit: 5 },
//		{ $lookup: {
//				from: "skylinks",
//				localField: "skylink_id",  // field in the downloads collection
//				foreignField: "_id",	   // field in the skylinks collection
//				as: "fromSkylinks"
//		  }
//		},
//		{ $replaceRoot: { newRoot: { $mergeObjects: [ { $arrayElemAt: [ "$fromSkylinks", 0 ] }, "$$ROOT" ] } } },
//		{ $project: { fromSkylinks: 0 } }
//	])
//
// This query will get all downloads by the current user, skip $skip of them
// and then fetch $limit of them, allowing us to paginate. It will then
// join with the `skylinks` collection in order to fetch some additional
// data about each download.
func generateUploadsPipeline(matchStage bson.D, offset, pageSize int) mongo.Pipeline {
	sortStage := bson.D{{"$sort", bson.D{{"timestamp", -1}}}}
	skipStage := bson.D{{"$skip", offset}}
	limitStage := bson.D{{"$limit", pageSize}}
	lookupStage := bson.D{
		{"$lookup", bson.D{
			{"from", "skylinks"},
			{"localField", "skylink_id"}, // field in the uploads collection
			{"foreignField", "_id"},      // field in the skylinks collection
			{"as", "fromSkylinks"},
		}},
	}
	replaceStage := bson.D{
		{"$replaceRoot", bson.D{
			{"newRoot", bson.D{
				{"$mergeObjects", bson.A{
					bson.D{{"$arrayElemAt", bson.A{"$fromSkylinks", 0}}}, "$$ROOT"},
				},
			}},
		}},
	}
	projectStage := bson.D{{"$project", bson.D{{"fromSkylinks", 0}}}}
	return mongo.Pipeline{matchStage, sortStage, skipStage, limitStage, lookupStage, replaceStage, projectStage}
}

// generateDownloadsPipeline is similar to generateUploadsPipeline. The only
// difference is that it supports partial downloads via the `bytes` field in the
// `downloads` collection.
func generateDownloadsPipeline(matchStage bson.D, offset, pageSize int) mongo.Pipeline {
	sortStage := bson.D{{"$sort", bson.D{{"created_at", -1}}}}
	skipStage := bson.D{{"$skip", offset}}
	limitStage := bson.D{{"$limit", pageSize}}
	lookupStage := bson.D{
		{"$lookup", bson.D{
			{"from", "skylinks"},
			{"localField", "skylink_id"}, // field in the downloads collection
			{"foreignField", "_id"},      // field in the skylinks collection
			{"as", "fromSkylinks"},
		}},
	}
	replaceStage := bson.D{
		{"$replaceRoot", bson.D{
			{"newRoot", bson.D{
				{"$mergeObjects", bson.A{
					bson.D{{"$arrayElemAt", bson.A{"$fromSkylinks", 0}}}, "$$ROOT"},
				},
			}},
		}},
	}
	// This stage checks if the download has a non-zero `bytes` field and if so,
	// it takes it as the download's size, otherwise it reports the full
	// skylink's size as download's size.
	projectStage := bson.D{{"$project", bson.D{
		{"skylink", 1},
		{"name", 1},
		{"user_id", 1},
		{"skylink_id", 1},
		{"created_at", 1},
		{"size", bson.D{
			{"$cond", bson.A{
				bson.D{{"$gt", bson.A{"$bytes", 0}}}, // if
				"$bytes",                             // then
				"$size",                              // else
			}},
		}},
	}}}
	return mongo.Pipeline{matchStage, sortStage, skipStage, limitStage, lookupStage, replaceStage, projectStage}
}

// count returns the number of documents in the given collection that match the
// given matchStage.
func (db *DB) count(ctx context.Context, coll *mongo.Collection, matchStage bson.D) (int64, error) {
	pipeline := mongo.Pipeline{matchStage, bson.D{{"$count", "count"}}}
	c, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, errors.AddContext(err, "DB query failed")
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			db.staticLogger.Debugln("Error on closing DB cursor.", errDef)
		}
	}()

	if ok := c.Next(ctx); !ok {
		// No results found. This is expected.
		return 0, nil
	}
	// We need this struct, so we can safely decode both int32 and int64.
	result := struct {
		Count int64 `bson:"count"`
	}{}
	if err = c.Decode(&result); err != nil {
		return 0, errors.AddContext(err, "failed to decode DB data")
	}
	return result.Count, nil
}
