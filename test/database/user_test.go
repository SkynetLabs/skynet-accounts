package test

import (
	"context"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/skynet"
	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/NebulousLabs/fastrand"
)

func TestUserBySub(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sub := t.Name()
	// Ensure we don't have a user with this sub and the method handles that
	// correctly.
	_, err = db.UserBySub(ctx, sub, false)
	if err == nil || !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error %v, got %v.\n", database.ErrUserNotFound, err)
	}
	// Ensure creating a user via this method works as expected.
	u, err := db.UserBySub(ctx, sub, true)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	if u == nil || u.Sub != sub {
		t.Fatalf("Unexpected result %+v\n", u)
	}
	// Ensure that once the user exists, we'll fetch it correctly.
	u2, err := db.UserBySub(ctx, sub, false)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	if u2 == nil || u2.Sub != u.Sub || u2.ID != u.ID {
		t.Fatalf("Expected %+v, got %+v\n", u, u2)
	}
}

// TestUserSave ensures that UserSave works as expected.
func TestUserSave(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	username := t.Name()
	// Case: save a user that doesn't exist in the DB.
	u := &database.User{
		Email: username + "@siasky.net",
		Sub:   t.Name() + "sub",
		Tier:  1,
	}
	err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	u1, err := db.UserBySub(ctx, u.Sub, false)
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID.Hex() != u.ID.Hex() {
		t.Fatalf("Expected user id %s, got %s.", u.ID.Hex(), u1.ID.Hex())
	}
	// Case: save a user that does exist in the DB.
	u.Email = username + "_changed@siasky.net"
	err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	u1, err = db.UserBySub(ctx, u.Sub, false)
	if err != nil {
		t.Fatal(err)
	}
	if u1.Email != u.Email {
		t.Fatalf("Expected first name '%s', got '%s'.", u.Email, u1.Email)
	}
}

// TestUserStats ensures we report accurate statistics for users.
func TestUserStats(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add a test user.
	sub := string(fastrand.Bytes(userSubLen))
	u, err := db.UserCreate(ctx, "user@example.com", "", sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)

	testUploadSizeSmall := int64(1 + fastrand.Intn(4*skynet.MiB-1))
	testUploadSizeBig := int64(4*skynet.MiB + 1 + fastrand.Intn(4*skynet.MiB))
	expectedUploadBandwidth := int64(0)
	expectedDownloadBandwidth := int64(0)

	// Create a small upload.
	skylinkSmall, _, err := createTestUpload(ctx, db, u, testUploadSizeSmall)
	if err != nil {
		t.Fatal(err)
	}
	expectedUploadBandwidth = skynet.BandwidthUploadCost(testUploadSizeSmall)
	// Check the stats.
	stats, err := db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user stats.", err)
	}
	if stats.NumUploads != 1 {
		t.Fatalf("Expected a total of %d uploads, got %d.", 1, stats.NumUploads)
	}
	if stats.BandwidthUploads != expectedUploadBandwidth {
		t.Fatalf("Expected upload bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedUploadBandwidth, expectedUploadBandwidth/skynet.MiB,
			stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}

	// Create a big upload.
	skylinkBig, _, err := createTestUpload(ctx, db, u, testUploadSizeBig)
	if err != nil {
		t.Fatal(err)
	}
	expectedUploadBandwidth += skynet.BandwidthUploadCost(testUploadSizeBig)
	// Check the stats.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user stats.", err)
	}
	if stats.NumUploads != 2 {
		t.Fatalf("Expected a total of %d uploads, got %d.", 2, stats.NumUploads)
	}
	if stats.BandwidthUploads != expectedUploadBandwidth {
		t.Fatalf("Expected upload bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedUploadBandwidth, expectedUploadBandwidth/skynet.MiB,
			stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}

	// Register a small download.
	smallDownload := int64(1 + fastrand.Intn(4*skynet.MiB))
	err = db.DownloadCreate(ctx, *u, *skylinkSmall, smallDownload)
	if err != nil {
		t.Fatal("Failed to download.", err)
	}
	expectedDownloadBandwidth += skynet.BandwidthDownloadCost(smallDownload)
	// Check the stats.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user stats.", err)
	}
	if stats.NumDownloads != 1 {
		t.Fatalf("Expected a total of %d downloads, got %d.", 1, stats.NumDownloads)
	}
	if stats.BandwidthDownloads != expectedDownloadBandwidth {
		t.Fatalf("Expected download bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedDownloadBandwidth, expectedDownloadBandwidth/skynet.MiB,
			stats.BandwidthDownloads, stats.BandwidthDownloads/skynet.MiB)
	}
	// Register a big download.
	bigDownload := int64(100*skynet.MiB + fastrand.Intn(4*skynet.MiB))
	err = db.DownloadCreate(ctx, *u, *skylinkBig, bigDownload)
	if err != nil {
		t.Fatal("Failed to download.", err)
	}
	expectedDownloadBandwidth += skynet.BandwidthDownloadCost(bigDownload)
	// Check bandwidth.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user stats.", err)
	}
	if stats.NumDownloads != 2 {
		t.Fatalf("Expected a total of %d downloads, got %d.", 2, stats.NumDownloads)
	}
	if stats.BandwidthDownloads != expectedDownloadBandwidth {
		t.Fatalf("Expected download bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedDownloadBandwidth, expectedDownloadBandwidth/skynet.MiB,
			stats.BandwidthDownloads, stats.BandwidthDownloads/skynet.MiB)
	}

	// Register a registry read.
	_, err = db.RegistryReadCreate(ctx, *u)
	if err != nil {
		t.Fatal("Failed to register a registry read.", err)
	}
	expectedRegReadBandwidth := int64(skynet.CostBandwidthRegistryRead)
	// Check bandwidth.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if stats.NumRegReads != 1 {
		t.Fatalf("Expected a total of %d registry reads, got %d.", 1, stats.NumRegReads)
	}
	if stats.BandwidthRegReads != expectedRegReadBandwidth {
		t.Fatalf("Expected registry read bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedRegReadBandwidth, expectedRegReadBandwidth/skynet.MiB,
			stats.BandwidthRegReads, stats.BandwidthRegReads/skynet.MiB)
	}
	// Register a registry read.
	_, err = db.RegistryReadCreate(ctx, *u)
	if err != nil {
		t.Fatal("Failed to register a registry read.", err)
	}
	expectedRegReadBandwidth += int64(skynet.CostBandwidthRegistryRead)
	// Check bandwidth.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if stats.NumRegReads != 2 {
		t.Fatalf("Expected a total of %d registry reads, got %d.", 2, stats.NumRegReads)
	}
	if stats.BandwidthRegReads != expectedRegReadBandwidth {
		t.Fatalf("Expected registry read bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedRegReadBandwidth, expectedRegReadBandwidth/skynet.MiB,
			stats.BandwidthRegReads, stats.BandwidthRegReads/skynet.MiB)
	}

	// Register a registry write.
	_, err = db.RegistryWriteCreate(ctx, *u)
	if err != nil {
		t.Fatal("Failed to register a registry write.", err)
	}
	expectedRegWriteBandwidth := int64(skynet.CostBandwidthRegistryWrite)
	// Check bandwidth.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if stats.NumRegWrites != 1 {
		t.Fatalf("Expected a total of %d registry writes, got %d.", 1, stats.NumRegWrites)
	}
	if stats.BandwidthRegWrites != expectedRegWriteBandwidth {
		t.Fatalf("Expected registry write bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedRegWriteBandwidth, expectedRegWriteBandwidth/skynet.MiB,
			stats.BandwidthRegWrites, stats.BandwidthRegWrites/skynet.MiB)
	}
	// Register a registry write.
	_, err = db.RegistryWriteCreate(ctx, *u)
	if err != nil {
		t.Fatal("Failed to register a registry write.", err)
	}
	expectedRegWriteBandwidth += int64(skynet.CostBandwidthRegistryWrite)
	// Check bandwidth.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if stats.NumRegWrites != 2 {
		t.Fatalf("Expected a total of %d registry writes, got %d.", 2, stats.NumRegWrites)
	}
	if stats.BandwidthRegWrites != expectedRegWriteBandwidth {
		t.Fatalf("Expected registry write bandwidth of %d (%d MiB), got %d (%d MiB).",
			expectedRegWriteBandwidth, expectedRegWriteBandwidth/skynet.MiB,
			stats.BandwidthRegWrites, stats.BandwidthRegWrites/skynet.MiB)
	}
}
