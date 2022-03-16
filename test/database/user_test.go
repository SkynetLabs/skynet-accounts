package database

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/lib"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.sia.tech/siad/crypto"
)

// TestUserByEmail ensures UserByEmail works as expected.
// This method also tests UserCreate.
func TestUserByEmail(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	email := t.Name() + "@siasky.net"
	pass := t.Name() + "password"
	sub := t.Name() + "sub"
	// Ensure we don't have a user with this email and the method handles that
	// correctly.
	_, err = db.UserByEmail(ctx, email)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error %v, got %v.\n", database.ErrUserNotFound, err)
	}
	// Create a user to fetch.
	u, err := db.UserCreate(ctx, email, pass, sub, database.TierFree)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	if u == nil || u.Email != email {
		t.Fatalf("Unexpected result %+v\n", u)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Ensure that once the user exists, we'll fetch it correctly.
	u2, err := db.UserByEmail(ctx, email)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	// Ensuring the same ID is enough because the ID is unique.
	if u2 == nil || u2.ID != u.ID {
		t.Fatalf("Expected %+v, got %+v\n", u, u2)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u2)
}

// TestUserByID ensures UserByID works as expected.
func TestUserByID(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := primitive.ObjectIDFromHex("5fac383fdafc482e510627c3")
	if err != nil {
		t.Fatalf("Expected to be able to parse id hex string, got %v", err)
	}
	// Test finding a non-existent user. This should fail.
	_, err = db.UserByID(ctx, id)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error ErrUserNotFound, got %v", err)
	}

	// Add a user to find.
	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	u, err := db.UserCreate(ctx, "email@example.com", "", sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)

	// Test finding an existent user. This should pass.
	u1, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, u1) {
		t.Fatalf("User not equal to original:\n %+v\n vs\n %+v", u, u1)
	}
}

// TestUserByPubKey makes sure UserByPubKey functions correctly, both with a
// single and multiple pubkeys attached to a user.
func TestUserByPubKey(t *testing.T) {
	ctx := context.Background()
	name := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, name, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Generate a pubkey.
	_, pkk := crypto.GenerateKeyPair()
	pk := database.PubKey(pkk[:])

	// Make sure the method behaves correctly when it doesn't find a user.
	_, err = db.UserByPubKey(ctx, pk)
	if err != database.ErrUserNotFound {
		t.Fatalf("Expected error '%s', got '%s'", database.ErrUserNotFound, err)
	}

	// Create a user with this pubkey.
	u, err := db.UserCreatePK(ctx, name+"@siasky.net", name+"pass", name+"sub", pk, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch it from the DB and make sure it's the same user.
	u2, err := db.UserByPubKey(ctx, pk)
	if err != nil {
		t.Fatal(err)
	}
	if u2.Sub != u.Sub {
		t.Fatalf("Wrong user! Expected '%s', got '%s'.", u.Sub, u2.Sub)
	}
	// Generate another pubkey and attach it to the same user.
	_, pkk2 := crypto.GenerateKeyPair()
	pk2 := database.PubKey(pkk2[:])
	u2.PubKeys = append(u2.PubKeys, pk2)
	err = db.UserSave(ctx, u2)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch the user by both the new and the old pubkeys.
	u3, err3 := db.UserByPubKey(ctx, pk)
	u4, err4 := db.UserByPubKey(ctx, pk2)
	if err3 != nil || err4 != nil {
		t.Fatal(errors.Compose(err3, err4))
	}
	if u3.Sub != u.Sub || u4.Sub != u.Sub {
		t.Fatalf("Expected all fetched users to have sub '%s', got '%s' and '%s'", u.Sub, u3.Sub, u4.Sub)
	}
}

// TestUserByStripeID ensures UserByStripeID works as expected.
// This method also tests UserCreate and UserSetStripeID.
func TestUserByStripeID(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	stripeID := t.Name() + "stripeID"

	// Ensure we don't have a user with this StripeID and the method handles
	// that correctly.
	_, err = db.UserByStripeID(ctx, stripeID)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error %v, got %v.\n", database.ErrUserNotFound, err)
	}
	// Create a test user with the respective StripeID.
	u, err := db.UserCreate(ctx, t.Name()+"@siasky.net", t.Name()+"pass", t.Name()+"sub", database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	err = db.UserSetStripeID(ctx, u, stripeID)
	if err != nil {
		t.Fatal(err)
	}
	// Ensure that once the user exists, we'll fetch it correctly.
	u2, err := db.UserByStripeID(ctx, stripeID)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	// Ensuring the same ID is enough because the ID is unique.
	if u2 == nil || u2.ID != u.ID {
		t.Fatalf("Expected %+v, got %+v\n", u, u2)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u2)
}

// TestUserBySub ensures UserBySub works as expected.
// This method also tests UserCreate.
func TestUserBySub(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sub := t.Name()
	// Ensure we don't have a user with this sub and the method handles that
	// correctly.
	_, err = db.UserBySub(ctx, sub)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatalf("Expected error %v, got %v.\n", database.ErrUserNotFound, err)
	}
	// Create a test user.
	u, err := db.UserCreate(ctx, "", "", sub, database.TierFree)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	if u == nil || u.Sub != sub {
		t.Fatalf("Unexpected result %+v\n", u)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Ensure that once the user exists, we'll fetch it correctly.
	u2, err := db.UserBySub(ctx, sub)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	// Ensuring the same ID is enough because the ID is unique.
	if u2 == nil || u2.ID != u.ID {
		t.Fatalf("Expected %+v, got %+v\n", u, u2)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u2)
}

// TestUserConfirmEmail ensures that email confirmation works as expected,
// including resecting the expiration of tokens.
func TestUserConfirmEmail(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal("Failed to connect to the DB:", err)
	}
	emailAddr := t.Name() + "@siasky.net"
	// Create a user with this email.
	u, err := db.UserCreate(ctx, emailAddr, "password", "sub", database.TierFree)
	if err != nil {
		t.Fatal("Failed to create a test user:", err)
	}
	// Confirm the email.
	_, err = db.UserConfirmEmail(ctx, u.EmailConfirmationToken)
	if err != nil {
		t.Fatal("Failed to confirm email:", err)
	}
	// Generate a new confirmation token.
	u.EmailConfirmationToken, err = lib.GenerateUUID()
	if err != nil {
		t.Fatal("Failed to generate a token.")
	}
	// Set the expiration of the token in the past.
	u.EmailConfirmationTokenExpiration = time.Now().UTC().Add(-time.Minute)
	err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal("Failed to save the user:", err)
	}
	// Try to confirm the email, expecting to get an error because the token has
	// expired.
	_, err = db.UserConfirmEmail(ctx, u.EmailConfirmationToken)
	if !errors.Contains(err, database.ErrInvalidToken) {
		t.Fatalf("Expected error '%s', got '%s'\n", database.ErrInvalidToken.Error(), err.Error())
	}
}

// TestUserCreate ensures UserCreate works as expected.
func TestUserCreate(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	email := t.Name() + "@siasky.net"
	pass := t.Name() + "pass"
	sub := t.Name() + "sub"

	// Try to create a user with an invalid email.
	_, err = db.UserCreate(ctx, "invalid email", pass, sub, database.TierFree)
	if err == nil {
		t.Fatal("Expected a malformed email error, got nil.")
	}
	// Add a user. Happy case.
	u, err := db.UserCreate(ctx, email, pass, sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Make sure the user is there.
	fu, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fu == nil {
		t.Fatal("Expected to find a user but didn't.")
	}
	newEmail := t.Name() + "_new@siasky.net"
	newPass := t.Name() + "pass_new"
	newSub := t.Name() + "sub_new"
	// Try to create a user with an email which is already in use.
	_, err = db.UserCreate(ctx, email, newPass, newSub, database.TierFree)
	if !errors.Contains(err, database.ErrUserAlreadyExists) {
		t.Fatalf("Expected error %+v, got %+v.\n", database.ErrUserAlreadyExists, err)
	}
	// Try to create a user with a sub which is already in use.
	_, err = db.UserCreate(ctx, newEmail, newPass, sub, database.TierFree)
	if !errors.Contains(err, database.ErrUserAlreadyExists) {
		t.Fatalf("Expected error %+v, got %+v.\n", database.ErrUserAlreadyExists, err)
	}
}

// TestUserDelete ensures UserDelete works as expected.
func TestUserDelete(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sub := "695725d4-a345-4e68-919a-7395cb68484c"
	// Add a user to delete.
	u, err := db.UserCreate(ctx, "email@example.com", "", sub, database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	// Make sure the user is there.
	fu, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fu == nil {
		t.Fatal("expected to find a user but didn't")
	}
	// Delete the user.
	err = db.UserDelete(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the user is not there anymore.
	_, err = db.UserByID(ctx, u.ID)
	if !errors.Contains(err, database.ErrUserNotFound) {
		t.Fatal(err)
	}
}

// TestUserSave ensures that UserSave works as expected.
func TestUserSave(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	username := t.Name()
	// Case: save a user that doesn't exist in the DB.
	u := &database.User{
		Email: username + "@siasky.net",
		Sub:   t.Name() + "sub",
		Tier:  database.TierFree,
	}
	err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	u1, err := db.UserBySub(ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID.Hex() != u.ID.Hex() {
		t.Fatalf("Expected user id %s, got %s.", u.ID.Hex(), u1.ID.Hex())
	}
	// Case: save a user that does exist in the DB.
	u.Email = username + "_changed@siasky.net"
	u.Tier = database.TierPremium80
	err = db.UserSave(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	u1, err = db.UserBySub(ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if u1.Email != u.Email {
		t.Fatalf("Expected email '%s', got '%s'.", u.Email, u1.Email)
	}
	if u1.Tier != u.Tier {
		t.Fatalf("Expected tier '%d', got '%d'.", u.Tier, u1.Tier)
	}
}

// TestUserSetStripeID ensures that UserSetStripeID works as expected.
func TestUserSetStripeID(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	stripeID := t.Name() + "stripeid"
	// Create a test user with the respective StripeID.
	u, err := db.UserCreate(ctx, t.Name()+"@siasky.net", t.Name()+"pass", t.Name()+"sub", database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	err = db.UserSetStripeID(ctx, u, stripeID)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u2.StripeID != stripeID {
		t.Fatalf("Expected tier %s got %s.\n", stripeID, u2.StripeID)
	}
}

// TestUserSetTier ensures that UserSetTier works as expected.
func TestUserSetTier(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Create a test user with the respective StripeID.
	u, err := db.UserCreate(ctx, t.Name()+"@siasky.net", t.Name()+"pass", t.Name()+"sub", database.TierFree)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		_ = db.UserDelete(ctx, user)
	}(u)
	err = db.UserSetTier(ctx, u, database.TierPremium80)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u2.Tier != database.TierPremium80 {
		t.Fatalf("Expected tier %d got %d.\n", database.TierPremium80, u2.Tier)
	}
}

// TestUserStats ensures we report accurate statistics for users.
func TestUserStats(t *testing.T) {
	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := database.NewCustomDB(ctx, dbName, test.DBTestCredentials(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add a test user.
	sub := string(fastrand.Bytes(test.UserSubLen))
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
	skylinkSmall, _, err := test.CreateTestUpload(ctx, db, u, testUploadSizeSmall)
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
	skylinkBig, _, err := test.CreateTestUpload(ctx, db, u, testUploadSizeBig)
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
