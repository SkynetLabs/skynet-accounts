package database

import (
	"context"
	"fmt"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// ErrUnexpectedNumberOfModifications is returned when a modification
	// query, like Update or Replace, modifies an unexpected number of entries,
	// for example as 0 or more than 1 when we expect a single update.
	// "Modification" in this case also includes updates and inserts.
	ErrUnexpectedNumberOfModifications = errors.New("unexpected number of modifications")

	// ConfValRegistrationsDisabled is the configuration value that disables
	// new registration on the service.
	ConfValRegistrationsDisabled = "registrations_disabled"

	// ConfValTrue represents the truthy value for flag-like configuration
	// options.
	ConfValTrue = "true"
	// ConfValFalse represents the falsy value for flag-like configuration
	// options.
	ConfValFalse = "false"
)

type (
	// ConfVal represents a single configuration value in the database.
	ConfVal struct {
		Key   string `bson:"key" json:"key"`
		Value string `bson:"value" json:"value"`
	}
)

// ReadConfigValue reads the value for the given key from the collConfiguration
// table.
func (db *DB) ReadConfigValue(ctx context.Context, key string) (string, error) {
	sr := db.staticConfiguration.FindOne(ctx, bson.M{"key": key})
	if sr.Err() != nil {
		return "", sr.Err()
	}
	option := &ConfVal{}
	err := sr.Decode(option)
	if err != nil {
		return "", err
	}
	return option.Value, nil
}

// WriteConfigValue writes the value for the given key to the collConfiguration
// table.
func (db *DB) WriteConfigValue(ctx context.Context, key, value string) error {
	opts := &options.ReplaceOptions{
		Upsert: &True,
	}
	ur, err := db.staticConfiguration.ReplaceOne(ctx, bson.M{"key": key}, bson.M{"key": key, "value": value}, opts)
	if err != nil {
		return err
	}
	if ur.ModifiedCount+ur.UpsertedCount != 1 {
		return errors.AddContext(ErrUnexpectedNumberOfModifications, fmt.Sprintf("updated %d entries", ur.ModifiedCount))
	}
	return nil
}
