package api

import (
	"net/http"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// testUploadInfo ensures uploadInfoGET works as expected.
func testUploadInfo(t *testing.T, at *test.AccountsTester) {
	// Create two test users.
	name := test.DBNameForTest(t.Name())
	name2 := name + "2"
	email := name + "@siasky.net"
	email2 := name2 + "@siasky.net"
	r, _, err := at.CreateUserPost(email, name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	c1 := test.ExtractCookie(r)
	r, _, err = at.CreateUserPost(email2, name2+"_pass")
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
		t.Fatal("Unexpected skylink.")
	}
	if !ups[0].UserID.IsZero() {
		t.Fatal("Unexpected uploader.")
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
