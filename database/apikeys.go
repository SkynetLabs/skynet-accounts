package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

/**
API keys are authentication tokens generated by users. They do not expire, thus
allowing users to use them for a long time and to embed them in apps and on
machines. API keys can be revoked when they are no longer needed or if they get
compromised. This is done by deleting them from this service.
*/

type (
	// APIKey is a non-expiring authentication token generated on user demand.
	APIKey struct {
		ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
		UserID    primitive.ObjectID `bson:"user_id" json:"-"`
		Key       PubKey             `bson:"key" json:"key"`
		CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
	}
)

// APIKeyCreate creates a new API key.
func (db *DB) APIKeyCreate(ctx context.Context, user User) (*APIKey, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	ak := APIKey{
		UserID:    user.ID,
		Key:       fastrand.Bytes(PubKeySize),
		CreatedAt: time.Now().UTC(),
	}
	ior, err := db.staticAPIKeys.InsertOne(ctx, ak)
	if err != nil {
		return nil, err
	}
	ak.ID = ior.InsertedID.(primitive.ObjectID)
	return &ak, nil
}

// APIKeyFetch fetches an API key from the DB.
func (db *DB) APIKeyFetch(ctx context.Context, user User, id primitive.ObjectID) (*APIKey, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	var ak APIKey
	sr := db.staticAPIKeys.FindOne(ctx, bson.M{"user_id": user.ID, "_id": id})
	if sr.Err() != nil {
		return nil, sr.Err()
	}
	err := sr.Decode(&ak)
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

// APIKeyDelete deletes an API key.
func (db *DB) APIKeyDelete(ctx context.Context, user User, id primitive.ObjectID) error {
	if user.ID.IsZero() {
		return errors.New("invalid user")
	}
	filter := bson.M{
		"_id":     id,
		"user_id": user.ID,
	}
	_, err := db.staticAPIKeys.DeleteOne(ctx, filter)
	return err
}

// APIKeyCreate lists all API keys that belong to the user.
func (db *DB) APIKeyList(ctx context.Context, user User) ([]*APIKey, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	aks := make([]*APIKey, 0)
	c, err := db.staticAPIKeys.Find(ctx, bson.M{"user_id": user.ID})
	if err != nil {
		return nil, err
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			db.staticLogger.Debugln("Error on closing DB cursor.", errDef)
		}
	}()
	err = c.All(ctx, aks)
	if err != nil {
		return nil, err
	}
	return aks, nil
}
