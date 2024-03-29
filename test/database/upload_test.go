package database

import (
	"context"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestUploadsByUser ensures UploadsByUser returns the correct uploads,
// in the correct order, with the correct sized and so on.
func TestUploadsByUser(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}
	testUploadSize := int64(1 + fastrand.Intn(1e10))
	// Add a test user.
	sub := string(fastrand.Bytes(test.UserSubLen))
	u, err := db.UserCreate(ctx, "email@example.com", "", sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		err := db.UserDelete(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
	}(u)
	// Create a skylink record and register an upload for it.
	sl, _, err := test.CreateTestUpload(ctx, db, *u, testUploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Note that we don't initialise the "total" values, as we are not going to
	// use them in this test.
	expected := database.UserStatsUpload{
		Count:          1,
		Size:           testUploadSize,
		RawStorageUsed: skynet.RawStorageUsed(testUploadSize),
		Bandwidth:      skynet.BandwidthUploadCost(testUploadSize),
	}
	// Fetch the user's uploads.
	ups, n, err := db.UploadsByUser(ctx, *u, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user.", err)
	}
	if n != expected.Count {
		t.Fatalf("Expected to have %d upload(s), got %d.", expected.Count, n)
	}
	if ups[0].RawStorage != expected.RawStorageUsed {
		t.Fatalf("Expected the raw storage used of an upload with file size of %d (%d MiB) to be %d (%d MiB), got %d (%d MiB).",
			testUploadSize, testUploadSize/skynet.MiB, expected.RawStorageUsed, expected.RawStorageUsed/skynet.MiB, ups[0].Size, ups[0].Size/skynet.MiB)
	}
	if ups[0].Size != expected.Size {
		t.Fatalf("Expected the uploads size of an upload with file size of %d (%d MiB) to be %d (%d MiB), got %d (%d MiB).",
			testUploadSize, testUploadSize/skynet.MiB, expected.Size, expected.Size/skynet.MiB, ups[0].Size, ups[0].Size/skynet.MiB)
	}
	// Refresh the user's record and make sure we report storage used accurately.
	stats, err := db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user.", err)
	}
	if stats.RawStorageUsed != expected.RawStorageUsed {
		t.Fatalf("Expected raw storage used of %d (%d MiB), got %d (%d MiB).",
			expected.RawStorageUsed, expected.RawStorageUsed/skynet.MiB, stats.RawStorageUsed, stats.RawStorageUsed/skynet.MiB)
	}
	if stats.UploadsSize != expected.Size {
		t.Fatalf("Expected upload size of %d (%d MiB), got %d (%d MiB).",
			expected.Size, expected.Size/skynet.MiB, stats.UploadsSize, stats.UploadsSize/skynet.MiB)
	}
	if stats.BandwidthUploads != expected.Bandwidth {
		t.Fatalf("Expected upload bandwidth used of %d (%d MiB), got %d (%d MiB).",
			expected.Bandwidth, expected.Bandwidth/skynet.MiB, stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}
	if stats.NumUploads != expected.Count {
		t.Fatalf("Expected to have %d upload(s), got %d.", expected.Count, stats.NumUploads)
	}
	// Create a second upload for the same skylink. The user's used storage
	// should stay the same but the upload bandwidth should increase.
	_, _, err = test.RegisterTestUpload(ctx, db, *u, sl)
	if err != nil {
		t.Fatal("Failed to re-upload.", err)
	}
	expected.Size += 0           // raw storage used stays the same
	expected.RawStorageUsed += 0 // storage stays the same
	expected.Bandwidth += skynet.BandwidthUploadCost(testUploadSize)
	expected.Count++
	// Refresh the user's record and make sure we report storage used accurately.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user.", err)
	}
	if stats.RawStorageUsed != expected.RawStorageUsed {
		t.Fatalf("Expected raw storage used of %d (%d MiB), got %d (%d MiB).",
			expected.RawStorageUsed, expected.RawStorageUsed/skynet.MiB, stats.RawStorageUsed, stats.RawStorageUsed/skynet.MiB)
	}
	if stats.UploadsSize != expected.Size {
		t.Fatalf("Expected upload size of %d (%d MiB), got %d (%d MiB).",
			expected.Size, expected.Size/skynet.MiB, stats.UploadsSize, stats.UploadsSize/skynet.MiB)
	}
	if stats.BandwidthUploads != expected.Bandwidth {
		t.Fatalf("Expected upload bandwidth used of %d (%d MiB), got %d (%d MiB).",
			expected.Bandwidth, expected.Bandwidth/skynet.MiB, stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}
	if stats.NumUploads != expected.Count {
		t.Fatalf("Expected to have %d upload(s), got %d.", expected.Count, stats.NumUploads)
	}
	// Upload the same file again. Uploads go up, storage stays the same.
	_, _, err = test.RegisterTestUpload(ctx, db, *u, sl)
	if err != nil {
		t.Fatal("Failed to re-upload after unpinning.", err)
	}
	expected.Size += 0           // upload size stays the same
	expected.RawStorageUsed += 0 // storage stays the same
	expected.Bandwidth += skynet.BandwidthUploadCost(testUploadSize)
	expected.Count++
	// Refresh the user's record and make sure we report storage used accurately.
	stats, err = db.UserStats(ctx, *u)
	if err != nil {
		t.Fatal("Failed to fetch user.", err)
	}
	if stats.RawStorageUsed != expected.RawStorageUsed {
		t.Fatalf("Expected raw storage used of %d (%d MiB), got %d (%d MiB).",
			expected.RawStorageUsed, expected.RawStorageUsed/skynet.MiB, stats.RawStorageUsed, stats.RawStorageUsed/skynet.MiB)
	}
	if stats.UploadsSize != expected.Size {
		t.Fatalf("Expected upload size of %d (%d MiB), got %d (%d MiB).",
			expected.Size, expected.Size/skynet.MiB, stats.UploadsSize, stats.UploadsSize/skynet.MiB)
	}
	if stats.BandwidthUploads != expected.Bandwidth {
		t.Fatalf("Expected upload bandwidth used of %d (%d MiB), got %d (%d MiB).",
			expected.Bandwidth, expected.Bandwidth/skynet.MiB, stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}
	if stats.NumUploads != expected.Count {
		t.Fatalf("Expected to have %d upload(s), got %d.", expected.Count, stats.NumUploads)
	}
}

// TestUnpinUploads ensures UnpinUploads unpins all uploads of this
// skylink by this user without affecting uploads by other users.
func TestUnpinUploads(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}
	testUploadSize := int64(1 + fastrand.Intn(1e10))
	// Add two test users.
	sub1 := string(fastrand.Bytes(test.UserSubLen))
	u1, err := db.UserCreate(ctx, "email1@example.com", "", sub1, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		err := db.UserDelete(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
	}(u1)
	sub2 := string(fastrand.Bytes(test.UserSubLen))
	u2, err := db.UserCreate(ctx, "email2@example.com", "", sub2, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		err := db.UserDelete(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
	}(u2)
	// Create a skylink record and register an upload for it.
	sl, _, err := test.CreateTestUpload(ctx, db, *u1, testUploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Upload it again for the same user.
	_, _, err = test.RegisterTestUpload(ctx, db, *u1, sl)
	if err != nil {
		t.Fatal("Failed to re-upload.", err)
	}
	// Upload it for the second user.
	_, _, err = test.RegisterTestUpload(ctx, db, *u2, sl)
	if err != nil {
		t.Fatal("Failed to re-upload.", err)
	}
	// Delete all uploads by the first user.
	unpinned, err := db.UnpinUploads(ctx, *sl, *u1)
	if err != nil {
		t.Fatal("Failed to unpin.", err)
	}
	if unpinned != 2 {
		t.Fatalf("Expected to unpin 2 files, unpinned %d.", unpinned)
	}
	// Fetch the first user's uploads.
	_, n, err := db.UploadsByUser(ctx, *u1, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user1.", err)
	}
	if n != 0 {
		t.Fatalf("Expected to have exactly %d upload(s), got %d.", 0, n)
	}
	// Refresh the user's stats and make sure we report storage used accurately.
	stats, err := db.UserStats(ctx, *u1)
	if err != nil {
		t.Fatal("Failed to fetch user1.", err)
	}
	if stats.RawStorageUsed != 0 {
		t.Fatalf("Expected raw storage used of %d (%d MiB), got %d (%d MiB).",
			0, 0, stats.RawStorageUsed, stats.RawStorageUsed/skynet.MiB)
	}
	if stats.TotalUploadsSize != 0 {
		t.Fatalf("Expected total upload size of %d (%d MiB), got %d (%d MiB).",
			0, 0, stats.TotalUploadsSize, stats.TotalUploadsSize/skynet.MiB)
	}
	expectedUploadBandwidth := 2 * skynet.BandwidthUploadCost(testUploadSize)
	if stats.BandwidthUploadsTotal != expectedUploadBandwidth {
		t.Fatalf("Expected upload bandwidth used of %d (%d MiB), got %d (%d MiB).",
			expectedUploadBandwidth, expectedUploadBandwidth/skynet.MiB, stats.BandwidthUploadsTotal, stats.BandwidthUploadsTotal/skynet.MiB)
	}
	// Fetch the second user's uploads.
	_, n, err = db.UploadsByUser(ctx, *u2, 0, database.DefaultPageSize)
	if err != nil {
		t.Fatal("Failed to fetch uploads by user2.", err)
	}
	if n != 1 {
		t.Fatalf("Expected to have exactly %d upload(s), got %d.", 1, n)
	}
	// Refresh the user's stats and make sure we report storage used accurately.
	stats, err = db.UserStats(ctx, *u2)
	if err != nil {
		t.Fatal("Failed to fetch user2.", err)
	}
	expectedRawStorage := skynet.RawStorageUsed(testUploadSize)
	if stats.RawStorageUsed != expectedRawStorage {
		t.Fatalf("Expected raw storage used of %d (%d MiB), got %d (%d MiB).",
			expectedRawStorage, expectedRawStorage, stats.RawStorageUsed, stats.RawStorageUsed/skynet.MiB)
	}
	if stats.TotalUploadsSize != testUploadSize {
		t.Fatalf("Expected total upload size of %d (%d MiB), got %d (%d MiB).",
			testUploadSize, testUploadSize, stats.TotalUploadsSize, stats.TotalUploadsSize/skynet.MiB)
	}
	expectedUploadBandwidth = skynet.BandwidthUploadCost(testUploadSize)
	if stats.BandwidthUploads != expectedUploadBandwidth {
		t.Fatalf("Expected upload bandwidth used of %d (%d MiB), got %d (%d MiB).",
			expectedUploadBandwidth, expectedUploadBandwidth/skynet.MiB, stats.BandwidthUploads, stats.BandwidthUploads/skynet.MiB)
	}
}

// TestUploadCreateAnon ensures that UploadCreate can create anonymous uploads.
func TestUploadCreateAnon(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sl := test.RandomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Register an anonymous upload.
	ip := "1.0.2.233"
	up, err := db.UploadCreate(ctx, database.AnonUser, ip, *skylink)
	if err != nil {
		t.Fatal(err)
	}
	if !up.UserID.IsZero() {
		t.Fatal("Expected zero user ID.")
	}
	if up.UploaderIP != ip {
		t.Fatalf("Expected UploaderIP '%s', got '%s'", ip, up.UploaderIP)
	}
	// Register an anonymous upload without an UploaderIP address.
	up, err = db.UploadCreate(ctx, database.AnonUser, "", *skylink)
	if err != nil {
		t.Fatal(err)
	}
	if !up.UserID.IsZero() {
		t.Fatal("Expected zero user ID.")
	}
	if up.UploaderIP != "" {
		t.Fatalf("Expected empty UploaderIP, got '%s'", up.UploaderIP)
	}
}
