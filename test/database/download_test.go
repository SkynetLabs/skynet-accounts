package database

import (
	"context"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
)

// TestDownloadCreateAnon ensures that UploadCreate can create anonymous downloads.
func TestDownloadCreateAnon(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sl := test.RandomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Register an anonymous download.
	err = db.DownloadCreate(ctx, nil, *skylink, 123)
	if err != nil {
		t.Fatal(err)
	}
}
