package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/SkynetLabs/skynet-accounts/types"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// testUploadInfo ensures uploadInfoGET works as expected.
func testUploadInfo(t *testing.T, at *test.AccountsTester) {
	// Create two test users.
	name := test.DBNameForTest(t.Name())
	name2 := name + "2"
	email := types.NewEmail(name + "@siasky.net")
	email2 := types.NewEmail(name2 + "@siasky.net")
	r, _, err := at.UserPOST(email.String(), name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	c1 := test.ExtractCookie(r)
	r, _, err = at.UserPOST(email2.String(), name2+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	c2 := test.ExtractCookie(r)
	u, err := at.DB.UserByEmail(at.Ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := at.DB.UserByEmail(at.Ctx, email2)
	if err != nil {
		t.Fatal(err)
	}
	at.ClearCredentials()

	// Create a skylink.
	sl, err := at.DB.Skylink(at.Ctx, test.RandomSkylink())
	if err != nil {
		t.Fatal(err)
	}

	// Try an invalid skylink.
	_, sc, err := at.UploadInfo("this is not a skylink")
	if err == nil || sc != http.StatusBadRequest {
		t.Fatalf("Expected an error and status 400, got '%s' and %d", err, sc)
	}
	// Try a valid skylink that's not in the DB.
	ups, sc, err := at.UploadInfo(test.RandomSkylink())
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) > 0 {
		t.Fatalf("Expected 200OK and zero uploads, got %d and %d uploads.", sc, len(ups))
	}
	// Try a valid skylink that's already in the DB.
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) > 0 {
		t.Fatalf("Expected 200OK and zero uploads, got %d and %d uploads.", sc, len(ups))
	}

	// Create an anonymous upload for sl.
	ip := "1.2.3.4"
	_, err = at.TrackUpload(sl.Skylink, ip)
	if err != nil {
		t.Fatal(err)
	}
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) != 1 {
		t.Fatalf("Expected 200OK and one upload, got %d and %d uploads.", sc, len(ups))
	}
	if ups[0].Skylink != sl.Skylink {
		t.Fatalf("Expected skylink '%s', got '%s'.", sl.Skylink, ups[0].Skylink)
	}
	if !ups[0].UserID.IsZero() {
		t.Fatalf("Unexpected uploader: %+v", ups)
	}
	if ups[0].UploaderIP != ip {
		t.Fatalf("Expected uploader IP '%s', got '%s'", ip, ups[0].UploaderIP)
	}

	// Create a non-anon upload for sl.
	at.SetCookie(c1)
	_, err = at.TrackUpload(sl.Skylink, "")
	if err != nil {
		t.Fatal(err)
	}
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) != 2 {
		t.Fatalf("Expected 200OK and %d uploads, got %d and %d uploads.", 2, sc, len(ups))
	}
	// Verify that the non-anon upload is by the right user.
	if !(ups[0].UserID.IsZero() && ups[1].UserID.Hex() == u.ID.Hex()) && !(ups[1].UserID.IsZero() && ups[0].UserID.Hex() == u.ID.Hex()) {
		t.Fatalf("Expected one anonymous upload and one upload by user with hex id %s, got %+v", u.ID.Hex(), ups)
	}

	// Add more uploads by the same user.
	_, err = at.TrackUpload(sl.Skylink, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = at.TrackUpload(sl.Skylink, "")
	if err != nil {
		t.Fatal(err)
	}
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) != 4 {
		t.Fatalf("Expected 200OK and %d uploads, got %d and %d uploads.", 4, sc, len(ups))
	}

	// Add an upload from another user.
	at.SetCookie(c2)
	_, err = at.TrackUpload(sl.Skylink, "")
	if err != nil {
		t.Fatal(err)
	}
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) != 5 {
		t.Fatalf("Expected 200OK and %d uploads, got %d and %d uploads.", 5, sc, len(ups))
	}
	// Ensure we have all three uploaders.
	uploaders := make(map[primitive.ObjectID]interface{})
	for _, up := range ups {
		uploaders[up.UserID] = struct{}{}
	}
	if uploaders[u.ID] == nil || uploaders[u2.ID] == nil || uploaders[database.AnonUser.ID] == nil {
		t.Fatalf("Expected to have all three uploaders, got %+v", uploaders)
	}

	// Ensure unpinned uploads are also reported.
	_, err = at.UploadsDELETE(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	ups, sc, err = at.UploadInfo(sl.Skylink)
	if err != nil {
		t.Fatal(err)
	}
	if sc != http.StatusOK || len(ups) != 5 {
		t.Fatalf("Expected 200OK and %d uploads, got %d and %d uploads.", 5, sc, len(ups))
	}
}

// TestUploadedSkylinks ensures UploadsByPeriod returns the correct uploads.
// This one relies on a clean DB, so it can't be run with the same tester as
// other tests which create uploads.
func TestUploadedSkylinks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dbName := test.DBNameForTest(t.Name())
	at, err := test.NewAccountsTester(dbName, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()
	// Add a test user.
	email := types.Email(t.Name() + "@example.com")
	pass := "pass"
	sub := string(fastrand.Bytes(test.UserSubLen))
	u, err := at.DB.UserCreate(at.Ctx, email, pass, sub, database.TierPremium5)
	if err != nil {
		t.Fatal(err)
	}
	defer func(user *database.User) {
		err = at.DB.UserDelete(at.Ctx, user)
		if err != nil {
			t.Fatal(err)
		}
	}(u)
	r, _, err := at.LoginCredentialsPOST(email.String(), pass)
	if err != nil {
		t.Fatal(err)
	}
	at.SetCookie(test.ExtractCookie(r))
	// Fetch uploads from an empty DB. Expect empty result.
	resp, _, err := at.UploadedSkylinks(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skylinks) > 0 {
		t.Fatalf("Expected zero records, got %d", len(resp.Skylinks))
	}
	// Create test uploads.
	// Uploaded 33 days ago.
	sl1, id1, err := test.CreateTestUpload(at.Ctx, at.DB, *u, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Uploaded 4 days ago.
	sl2, id2, err := test.CreateTestUpload(at.Ctx, at.DB, *u, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Uploaded <3 days ago.
	sl3, id3, err := test.CreateTestUpload(at.Ctx, at.DB, *u, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Uploaded now.
	sl4, _, err := test.CreateTestUpload(at.Ctx, at.DB, *u, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Modify uploads' times, so they are a bit more spread in time.
	// One upload more than 3 days ago, another more than a month ago.
	_, err1 := changeUploadTime(at.Ctx, at.DB, id1, daysAgo(33))
	_, err2 := changeUploadTime(at.Ctx, at.DB, id2, daysAgo(4))
	_, err3 := changeUploadTime(at.Ctx, at.DB, id3, daysAgo(3).Add(10*time.Second))
	if err = errors.Compose(err1, err2, err3); err != nil {
		t.Fatal(err)
	}
	// Fetch uploads for a period with no uploads. Expect empty result.
	resp, _, err = at.UploadedSkylinks(daysAgo(30).Unix(), daysAgo(20).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skylinks) > 0 {
		t.Fatalf("Expected zero records, got %d", len(resp.Skylinks))
	}
	// Fetch uploads with from > to. Expect error.
	_, _, err = at.UploadedSkylinks(daysAgo(20).Unix(), daysAgo(30).Unix())
	if err == nil || !strings.Contains(err.Error(), database.ErrInvalidTimePeriod.Error()) {
		t.Fatalf("Expected '%v', got '%v'", database.ErrInvalidTimePeriod, err)
	}
	// Fetch uploads with period more than 30 days. Expect error.
	resp, _, err = at.UploadedSkylinks(daysAgo(33).Unix(), time.Now().UTC().Unix())
	if err == nil || !strings.Contains(err.Error(), api.ErrTimePeriodTooLong.Error()) {
		t.Fatalf("Expected '%v', got '%v'", api.ErrTimePeriodTooLong, err)
	}
	// Fetch uploads with no period. Expect last 3 days.
	resp, _, err = at.UploadedSkylinks(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Expect sl3 and sl4.
	if len(resp.Skylinks) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(resp.Skylinks))
	}
	if !test.Contains(resp.Skylinks, sl3.Skylink) || !test.Contains(resp.Skylinks, sl4.Skylink) {
		t.Fatalf("Expected to have both '%s' and '%s', got '%v'", sl3.Skylink, sl4.Skylink, resp.Skylinks)
	}
	// Fetch uploads without from. Expect to get the 3 days before the 'to'.
	resp, _, err = at.UploadedSkylinks(0, daysAgo(2).Unix())
	if err != nil {
		t.Fatal(err)
	}
	// Expect sl2 and sl3.
	if len(resp.Skylinks) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(resp.Skylinks))
	}
	// Fetch uploads without to. Expect to get the 3 days after the 'from'.
	resp, _, err = at.UploadedSkylinks(daysAgo(5).Unix(), 0)
	if err != nil {
		t.Fatal(err)
	}
	// Expect sl2 and sl3.
	if len(resp.Skylinks) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(resp.Skylinks))
	}
	if !test.Contains(resp.Skylinks, sl2.Skylink) || !test.Contains(resp.Skylinks, sl3.Skylink) {
		t.Fatalf("Expected to have both '%s' and '%s', got '%v'", sl2.Skylink, sl3.Skylink, resp.Skylinks)
	}
	// Fetch uploads with from and to.
	resp, _, err = at.UploadedSkylinks(daysAgo(33).Add(-1*time.Minute).Unix(), daysAgo(3).Add(-1*time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	// Expect sl1 and sl2. We narrowly miss sl3.
	if len(resp.Skylinks) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(resp.Skylinks))
	}
	if !test.Contains(resp.Skylinks, sl1.Skylink) || !test.Contains(resp.Skylinks, sl2.Skylink) {
		t.Fatalf("Expected to have both '%s' and '%s', got '%v'", sl1.Skylink, sl2.Skylink, resp.Skylinks)
	}
}

// changeUploadTime allows us to change the timestamp of an upload.
func changeUploadTime(ctx context.Context, db *database.DB, uploadID primitive.ObjectID, newTime time.Time) (int64, error) {
	update := bson.M{"$set": bson.M{"timestamp": newTime}}
	return db.UpdateUpload(ctx, uploadID, update)
}

// daysAgo is a helper that returns a time N days ago.
func daysAgo(n int) time.Time {
	return time.Now().UTC().Add(time.Duration(n) * -24 * time.Hour)
}
