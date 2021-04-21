package database

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// RegistryRead describes a single registry read by a user.
type RegistryRead struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	Timestamp    time.Time          `bson:"timestamp" json:"timestamp"`
	Referrer     string             `bson:"referrer" json:"referrer"`
	ReferrerType string             `bson:"referrer_type" json:"referrerType"`
}

// RegistryWrite describes a single registry write by a user.
type RegistryWrite struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       primitive.ObjectID `bson:"user_id,omitempty" json:"userId"`
	Timestamp    time.Time          `bson:"timestamp" json:"timestamp"`
	Referrer     string             `bson:"referrer" json:"referrer"`
	ReferrerType string             `bson:"referrer_type" json:"referrerType"`
}

// RegistryReadCreate registers a new registry read.
func (db *DB) RegistryReadCreate(ctx context.Context, user User, referrer string) (*RegistryRead, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	ref, err := FromString(referrer)
	if err != nil && err != ErrorReferrerEmpty {
		db.staticLogger.Info(fmt.Sprintf("Failed to get referrer. Error: %s", err.Error()))
	}
	rr := RegistryRead{
		UserID:       user.ID,
		Timestamp:    time.Now().UTC(),
		Referrer:     ref.CanonicalName,
		ReferrerType: ref.Type,
	}
	ior, err := db.staticRegistryReads.InsertOne(ctx, rr)
	if err != nil {
		return nil, err
	}
	rr.ID = ior.InsertedID.(primitive.ObjectID)
	return &rr, nil
}

// RegistryWriteCreate registers a new registry write.
func (db *DB) RegistryWriteCreate(ctx context.Context, user User, referrer string) (*RegistryWrite, error) {
	if user.ID.IsZero() {
		return nil, errors.New("invalid user")
	}
	ref, err := FromString(referrer)
	if err != nil && err != ErrorReferrerEmpty {
		db.staticLogger.Info(fmt.Sprintf("Failed to get referrer. Error: %s", err.Error()))
	}
	rw := RegistryWrite{
		UserID:       user.ID,
		Timestamp:    time.Now().UTC(),
		Referrer:     ref.CanonicalName,
		ReferrerType: ref.Type,
	}
	ior, err := db.staticRegistryWrites.InsertOne(ctx, rw)
	if err != nil {
		return nil, err
	}
	rw.ID = ior.InsertedID.(primitive.ObjectID)
	return &rw, nil
}
