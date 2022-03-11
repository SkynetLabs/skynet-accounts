package database

import (
	"context"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestConfiguration ensures we can correctly read and write from/to the
// configuration DB table.
func TestConfiguration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	key := "this is a key"
	val := "this is a value"
	val2 := "this is another value"

	// Read a non-existent configuration option.
	_, err = db.ReadConfigValue(ctx, key)
	if err == nil || !errors.Contains(err, mongo.ErrNoDocuments) {
		t.Fatalf("Expected error '%s', got '%s'", mongo.ErrNoDocuments, err)
	}
	// Write to a new configuration option.
	err = db.WriteConfigValue(ctx, key, val)
	if err != nil {
		t.Fatal(err)
	}
	// Read an existing configuration option.
	value, err := db.ReadConfigValue(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if value != val {
		t.Fatalf("Expected value '%s', got '%s'", val, value)
	}
	// Write to an exiting configuration option.
	err = db.WriteConfigValue(ctx, key, val2)
	if err != nil {
		t.Fatal(err)
	}
	// Read the updated configuration option.
	value, err = db.ReadConfigValue(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if value != val2 {
		t.Fatalf("Expected value '%s', got '%s'", val, value)
	}
}
