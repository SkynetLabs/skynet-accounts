package test

import (
	"context"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestUserBandwidth ensures that uploads and downloads result are correctly
// billed in terms of bandwidth.
func TestUserBandwidth(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add a test user.
	sub := string(fastrand.Bytes(userSubLen))
	u, err := db.UserCreate(nil, sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(nil, user)
	}(u)

	testUploadSizeSmall := int64(1 + fastrand.Intn(4*database.MiB))
	expectedUploadBandwidthSmall := database.BandwidthUploadCost(testUploadSizeSmall)
	skylinkSmall, err := createTestUpload(ctx, db, u, testUploadSizeSmall)
	if err != nil {
		t.Fatal(err)
	}
	details, err := db.UserDetails(ctx, u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	if details.BandwidthUsed != expectedUploadBandwidthSmall {
		t.Fatalf("Expected used bandwidth of %d (%d MiB), got %d (%d MiB).", expectedUploadBandwidthSmall, expectedUploadBandwidthSmall/database.MiB, details.BandwidthUsed, details.BandwidthUsed/database.MiB)
	}

	testUploadSizeBig := int64(4*database.MiB + 1 + fastrand.Intn(4*database.MiB))
	expectedUploadBandwidthBig := database.BandwidthUploadCost(testUploadSizeBig)
	skylinkBig, err := createTestUpload(ctx, db, u, testUploadSizeBig)
	if err != nil {
		t.Fatal(err)
	}
	details, err = db.UserDetails(ctx, u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	expectedBandwidth := expectedUploadBandwidthSmall + expectedUploadBandwidthBig
	if details.BandwidthUsed != expectedBandwidth {
		t.Fatalf("Expected used bandwidth of %d (%d MiB), got %d (%d MiB).", expectedBandwidth, expectedBandwidth/database.MiB, details.BandwidthUsed, details.BandwidthUsed/database.MiB)
	}

	// Register a download.
	smallDownload := int64(1 + fastrand.Intn(4*database.MiB))
	_, err = db.DownloadCreate(ctx, *u, *skylinkSmall, smallDownload)
	if err != nil {
		t.Fatal("Failed to download.", err)
	}
	// Check bandwidth.
	details, err = db.UserDetails(ctx, u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	expectedBandwidth += database.BandwidthDownloadCost(smallDownload)
	if details.BandwidthUsed != expectedBandwidth {
		t.Fatalf("Expected used bandwidth of %d (%d MiB), got %d (%d MiB).", expectedBandwidth, expectedBandwidth/database.MiB, details.BandwidthUsed, details.BandwidthUsed/database.MiB)
	}
	// Register another download.
	bigDownload := int64(100*database.MiB + fastrand.Intn(4*database.MiB))
	_, err = db.DownloadCreate(ctx, *u, *skylinkBig, bigDownload)
	if err != nil {
		t.Fatal("Failed to download.", err)
	}
	// Check bandwidth.
	details, err = db.UserDetails(ctx, u)
	if err != nil {
		t.Fatal("Failed to fetch user details.", err)
	}
	expectedBandwidth += database.BandwidthDownloadCost(bigDownload)
	if details.BandwidthUsed != expectedBandwidth {
		t.Fatalf("Expected used bandwidth of %d (%d MiB), got %d (%d MiB).", expectedBandwidth, expectedBandwidth/database.MiB, details.BandwidthUsed, details.BandwidthUsed/database.MiB)
	}
}
