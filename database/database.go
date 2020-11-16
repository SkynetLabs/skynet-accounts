package database

import (
	"bytes"
	"context"
	"fmt"
	"log"
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

	// ErrUserNotFound is returned when we can't find the user in question.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailAlreadyUsed is returned when we try to use an email to either
	// either create or update a user and another user already uses this email.
	ErrEmailAlreadyUsed = errors.New("email already in use by another user")
	// ErrNoDBConnection is returned when we can't connect to the database.
	ErrNoDBConnection = errors.New("no connection to the database")

	// True is a helper, so we can easily provide a *bool for UpdateOptions.
	True = true
	// False is a helper, so we can easily provide a *bool for UpdateOptions.
	False = false
)

// DB represents a MongoDB database connection.
type DB struct {
	ctx   context.Context
	db    *mongo.Database
	users *mongo.Collection
}

// New returns a new DB connection based on the environment variables.
func New(ctx context.Context) (*DB, error) {
	opts, err := connectionOptionsFromEnv()
	if err != nil {
		return nil, errors.AddContext(err, "failed to get all necessary connection parameters")
	}
	return NewCustom(ctx, opts[SkynetDBUserEV], opts[SkynetDBPassEV], opts[SkynetDBHostEV], opts[SkynetDBPortEV], SkynetDBName)
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
		ctx:   ctx,
		db:    database,
		users: users,
	}
	return db, nil
}

// Context returns the DB's internal context, so we can build upon it.
func (db *DB) Context() context.Context {
	return db.ctx
}

// Disconnect closes the connection to the database in an orderly fashion.
func (db *DB) Disconnect(ctx context.Context) error {
	if db.db == nil {
		return ErrNoDBConnection
	}
	return db.db.Client().Disconnect(ctx)
}

// UserDeleteByID deletes a user by their ID and returns `false` in case there
// was an error or the user was not found.
func (db *DB) UserDeleteByID(ctx context.Context, id string) (bool, error) {
	if db.db == nil {
		return false, ErrNoDBConnection
	}
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

// UserFindAllByField finds a user by their ID.
func (db *DB) UserFindAllByField(ctx context.Context, fieldName, fieldValue string) ([]*user.User, error) {
	if db.db == nil {
		return nil, ErrNoDBConnection
	}

	// TODO SANITIZE THE INPUT!!!
	// 	https://stackoverflow.com/questions/30585213/do-i-need-to-sanitize-user-input-before-inserting-in-mongodb-mongodbnode-js-co
	// 	https://dev.to/katerakeren/data-sanitization-against-nosql-query-injection-in-mongodb-and-node-js-application-1eab
	// 	https://severalnines.com/database-blog/securing-mongodb-external-injection-attacks

	filter := bson.D{{fieldName, fieldValue}}
	c, err := db.users.Find(ctx, filter)
	if err != nil {
		return nil, errors.AddContext(err, "failed to Find")
	}
	var users []*user.User
	for c.Next(ctx) {
		var u user.User
		err = c.Decode(&u)
		if err != nil {
			fmt.Printf("UserFindByID: %+v\n", c.Current)
			return nil, errors.AddContext(err, "failed to parse value from DB")
		}
		users = append(users, &u)
	}
	if len(users) == 0 {
		return users, ErrUserNotFound
	}
	return users, nil
}

// UserFindByID finds a user by their ID.
func (db *DB) UserFindByID(ctx context.Context, id string) (*user.User, error) {
	if db.db == nil {
		return nil, ErrNoDBConnection
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.AddContext(err, "failed to parse user ID")
	}
	filter := bson.D{{"_id", oid}}
	c, err := db.users.Find(ctx, filter)
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
	if db.db == nil {
		return false, ErrNoDBConnection
	}
	if !u.Email.Validate() {
		return false, user.ErrInvalidEmail
	}
	// Check for an existing user with this email.
	filter := bson.M{"email": u.Email}
	sr := db.users.FindOne(ctx, filter)
	// ErrNoDocuments would mean that no user with this email is found.
	// A nil error would mean that another user with this email is found.
	// Any other error would be a failure to execute the query.
	if sr.Err() != mongo.ErrNoDocuments {
		if sr.Err() != nil {
			return false, errors.AddContext(sr.Err(), "failed to query DB")
		}
		var u2 user.User
		err := sr.Decode(&u2)
		if err != nil {
			baseErr := errors.AddContext(err, "failed to parse value from DB")
			return false, errors.AddContext(baseErr, "cannot ensure email uniqueness")
		}
		if u.ID.IsZero() || !bytes.Equal(u.ID[:], u2.ID[:]) {
			return false, ErrEmailAlreadyUsed
		}
	}
	// Upsert the user.
	fields := bson.M{
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"email":     u.Email,
	}
	if !u.ID.IsZero() {
		fields["_id"] = u.ID
		// If the user is new (u.ID is zero) then we can filer by email because
		// we won't find anything and we'll insert a new user.
		// If we're updating an existing user, though, we need to filter by
		// their ID.
		filter = bson.M{"_id": u.ID}
	}
	update := bson.M{"$set": fields}
	ur, err := db.users.UpdateOne(ctx, filter, update, &options.UpdateOptions{Upsert: &True})
	if err != nil {
		return false, errors.AddContext(err, "failed to Update")
	}
	if u.ID.IsZero() {
		if ur.UpsertedID == nil {
			log.Printf("Failed to Upsert a new user. UpsertResult: %+v\n", ur)
			return false, errors.New("failed to upsert a new user")
		}
		u.ID = ur.UpsertedID.(primitive.ObjectID)
	}
	insertedNewUser := ur.UpsertedCount == 1
	return insertedNewUser, nil
}

// connectionOptionsFromEnv retrieves the DB connection credentials from the
// environment and returns them in a map.
func connectionOptionsFromEnv() (map[string]string, error) {
	opts := make(map[string]string)
	for _, varName := range []string{SkynetDBHostEV, SkynetDBPortEV, SkynetDBUserEV, SkynetDBPassEV} {
		val, ok := os.LookupEnv(varName)
		if !ok {
			return nil, errors.New("missing env var " + varName)
		}
		opts[varName] = val
	}
	return opts, nil
}
