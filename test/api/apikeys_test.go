package api

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/modules"
)

// testAPIKeysFlow validates the creation, listing, and deletion of API keys.
func testAPIKeysFlow(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, _, err := at.CreateUserPost(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	at.Cookie = test.ExtractCookie(r)

	aks := make([]database.APIKey, 0)

	// List all API keys this user has. Expect the list to be empty.
	r, body, err := at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) > 0 {
		t.Fatalf("Expected an empty list of API keys, got %+v.", aks)
	}

	// Create a new API key.
	r, body, err = at.Post("/user/apikeys", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ak1 database.APIKey
	err = json.Unmarshal(body, &ak1)
	if err != nil {
		t.Fatal(err)
	}

	// Create another API key.
	r, body, err = at.Post("/user/apikeys", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ak2 database.APIKey
	err = json.Unmarshal(body, &ak2)
	if err != nil {
		t.Fatal(err)
	}

	// List all API keys this user has. Expect to find both keys we created.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) != 2 {
		t.Fatalf("Expected two API keys, got %+v.", aks)
	}
	if ak1.Key != aks[0].Key && ak1.Key != aks[1].Key {
		t.Fatalf("Missing key '%s'! Set: %+v", ak1.Key, aks)
	}
	if ak2.Key != aks[0].Key && ak2.Key != aks[1].Key {
		t.Fatalf("Missing key '%s'! Set: %+v", ak2.Key, aks)
	}

	// Delete an API key.
	r, body, err = at.Delete("/user/apikeys/"+ak1.Key, nil)
	if err != nil {
		t.Fatal(err)
	}
	// List all API keys this user has. Expect to find only the second one.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) != 1 {
		t.Fatalf("Expected one API key, got %+v.", aks)
	}
	if ak2.Key != aks[0].Key {
		t.Fatalf("Missing key '%s'! Set: %+v", ak2.Key, aks)
	}
}

// testAPIKeysUsage makes sure that we can use API keys to make API calls.
func testAPIKeysUsage(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	// Create a test user.
	email := name + "@siasky.net"
	r, _, err := at.CreateUserPost(email, name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	at.Cookie = test.ExtractCookie(r)
	// Get the user and create a test upload, so the stats won't be all zeros.
	u, err := at.DB.UserByEmail(at.Ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	uploadSize := int64(fastrand.Intn(int(modules.SectorSize / 2)))
	_, _, err = test.CreateTestUpload(at.Ctx, at.DB, u, uploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Create a new API key.
	_, body, err := at.Post("/user/apikeys", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Stop using the cookie, so we can test the API key.
	at.Cookie = nil
	var ak database.APIKey
	err = json.Unmarshal(body, &ak)
	if err != nil {
		t.Fatal(err)
	}
	// Get user stats without a cookie or headers - pass the API key via a query
	// variable. The main thing we want to see here is whether we get
	// an `Unauthorized` error or not but we'll validate the stats as well.
	params := url.Values{}
	params.Add("api_key", ak.Key)
	_, body, err = at.Get("/user/stats", params)
	if err != nil {
		t.Fatal(err, string(body))
	}
	var us database.UserStats
	err = json.Unmarshal(body, &us)
	if err != nil {
		t.Fatal(err)
	}
	if us.TotalUploadsSize != uploadSize || us.NumUploads != 1 || us.BandwidthUploads != skynet.BandwidthUploadCost(uploadSize) {
		t.Fatalf("Unexpected user stats. Expected TotalUploadSize %d (got %d), NumUploads 1 (got %d), BandwidthUploads %d (got %d).",
			uploadSize, us.TotalDownloadsSize, us.NumUploads, skynet.BandwidthUploadCost(uploadSize), us.BandwidthUploads)
	}
}
