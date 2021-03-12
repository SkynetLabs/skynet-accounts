package test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/skynet"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

const (
	// userSubLen is string length of a user's `sub` field
	userSubLen = 36
	// skylinkLen is the byte length of a skylink
	skylinkLen = 46
)

var (
	// skylinkCharset lists all character allowed in a skylink
	skylinkCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
)

// TestUpload_UploadsByUser ensures UploadsByUser returns the correct uploads,
// in the correct order, with the correct sized and so on.
func TestUpload_UploadsByUser(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	testUploadSize := int64(1 + fastrand.Intn(1e10))
	// Add a test user.
	sub := string(fastrand.Bytes(userSubLen))
	u, err := db.UserCreate(ctx, sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Create a skylink record for which to register an upload
	sl, err := createTestUpload(ctx, db, u, testUploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch the user's uploads.
	ups, n, err := db.UploadsByUser(ctx, *u, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user.", err)
	}
	if n != 1 {
		t.Fatalf("Expected to have exactly %d upload(s), got %d.", 1, n)
	}
	storageUsed := skynet.StorageUsed(testUploadSize)
	if ups[0].Size != storageUsed {
		t.Fatalf("Expected the reported size of an upload with file size of %d (%d MiB) to be its used storage of %d (%d MiB), got %d (%d MiB).",
			testUploadSize, testUploadSize/skynet.MiB, storageUsed, storageUsed/skynet.MiB, ups[0].Size, ups[0].Size/skynet.MiB)
	}
	// Refresh the user's record and make sure we report storage used accurately.
	stats, err := db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user.", err)
	}
	if stats.StorageUsed != storageUsed {
		t.Fatalf("Expected storage used of %d (%d MiB), got %d (%d MiB).",
			storageUsed, storageUsed/skynet.MiB, stats.StorageUsed, stats.StorageUsed/skynet.MiB)
	}
	// Delete the last upload. Expect zero uploads and zero storage used after.
	unpinned, err := db.UnpinUploads(ctx, *sl, *u)
	if err != nil {
		t.Fatal("Failed to unpin.", err)
	}
	if unpinned != 1 {
		t.Fatalf("Expected to unpin 1 file, unpinned %d.", unpinned)
	}
	// Fetch the user's uploads.
	ups, n, err = db.UploadsByUser(ctx, *u, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user.", err)
	}
	if n != 0 {
		t.Fatalf("Expected to have exactly %d upload(s), got %d.", 0, n)
	}
	// Refresh the user's record and make sure we report storage used accurately.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user.", err)
	}
	if stats.StorageUsed != 0 {
		t.Fatalf("Expected storage used of %d (%d MiB), got %d (%d MiB).",
			0, 0, stats.StorageUsed, stats.StorageUsed/skynet.MiB)
	}
}

// randomSkylink generates a random skylink
func randomSkylink() string {
	sb := strings.Builder{}
	for i := 0; i < skylinkLen; i++ {
		_ = sb.WriteByte(skylinkCharset[fastrand.Intn(len(skylinkCharset))])
	}
	return sb.String()
}

// createTestUpload creates a new skyfile and uploads it under the given user's
// account.
func createTestUpload(ctx context.Context, db *database.DB, user *database.User, size int64) (*database.Skylink, error) {
	// Create a skylink record for which to register an upload
	sl := randomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a test skylink")
	}
	err = db.SkylinkUpdate(ctx, skylink.ID, "test skylink "+sl, size)
	if err != nil {
		return nil, errors.AddContext(err, "failed to update skylink")
	}
	// Get the updated skylink.
	skylink, err = db.Skylink(ctx, sl)
	if err != nil {
		return nil, errors.AddContext(err, "failed to fetch skylink from DB")
	}
	if skylink.Size != size {
		return nil, errors.AddContext(err, fmt.Sprintf("expected skylink size to be %d, got %d.", size, skylink.Size))
	}
	// Register an upload.
	up, err := db.UploadCreate(ctx, *user, *skylink)
	if err != nil {
		return nil, errors.AddContext(err, "failed to register an upload")

	}
	if up.UserID != user.ID {
		return nil, errors.AddContext(err, "expected upload's userId to match the uploader's id")
	}
	if up.SkylinkID != skylink.ID {
		return nil, errors.AddContext(err, "expected upload's skylinkId to match the given skylink's id")
	}
	return skylink, nil
}
