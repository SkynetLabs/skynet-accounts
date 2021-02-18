package test

import (
	"context"
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"

	"github.com/NebulousLabs/skynet-accounts/database"
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
	// skylinkLen is the upload's file size, not the used storage.
	testUploadSize := int64(1 + fastrand.Intn(1e10))
	// Add a test user.
	sub := string(fastrand.Bytes(userSubLen))
	u, err := db.UserCreate(nil, sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(nil, user)
	}(u)
	// Create a skylink record for which to register an upload
	sl := randomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		t.Fatal("Failed to create a test skylink.", err)
	}
	err = db.SkylinkUpdate(ctx, skylink.ID, "test skylink", testUploadSize)
	if err != nil {
		t.Fatal("Failed to update skylink.", err)
	}
	// Get the updated skylink.
	skylink, err = db.Skylink(ctx, sl)
	if err != nil {
		t.Fatal("Failed to fetch skylink from DB.", err)
	}
	if skylink.Size != testUploadSize {
		t.Fatalf("Expected skylink size to be %d, got %d.", testUploadSize, skylink.Size)
	}
	// Register an upload.
	up, err := db.UploadCreate(ctx, *u, *skylink)
	if err != nil {
		t.Fatal("Failed to register an upload.", err)
	}
	if up.UserID != u.ID {
		t.Fatal("Expected upload's userId to match the uploader's id.")
	}
	if up.SkylinkID != skylink.ID {
		t.Fatal("Expected upload's skylinkIs to match the given skylink's id.")
	}
	// Fetch the user's uploads.
	ups, n, err := db.UploadsByUser(ctx, *u, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user.", err)
	}
	if n != 1 {
		t.Fatalf("Expected to have exactly %d upload(s), got %d.", 1, n)
	}
	storageUsed := database.StorageUsed(uint64(testUploadSize))
	if ups[0].Size != storageUsed {
		t.Fatalf("Expected the reported size of an upload with file size of %d (%d MiB) to be its used storage of %d (%d MiB), got %d (%d MiB).", testUploadSize, testUploadSize/database.MiB, storageUsed, storageUsed/database.MiB, ups[0].Size, ups[0].Size/database.MiB)
	}
	// Fetch the user's details and make sure we report storage used accurately.
	details, err := db.UserDetails(ctx, u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if uint64(details.StorageUsed) != storageUsed {
		t.Fatalf("Expected storage used of %d (%d MiB), got %d (%d MiB).", storageUsed, storageUsed/database.MiB, details.StorageUsed, details.StorageUsed/database.MiB)
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
