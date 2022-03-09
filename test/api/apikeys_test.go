package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/skynet"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
	"go.sia.tech/siad/modules"
)

// testPrivateAPIKeysFlow validates the creation, listing, and deletion of private
// API keys.
func testPrivateAPIKeysFlow(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, body, err := at.CreateUserPost(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err, string(body))
	}
	at.SetCookie(test.ExtractCookie(r))

	aks := make([]database.APIKeyRecord, 0)

	// List all API keys this user has. Expect the list to be empty.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
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
		t.Fatal(err, string(body))
	}
	var ak1 database.APIKeyRecord
	err = json.Unmarshal(body, &ak1)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the API key is private.
	if ak1.Public {
		t.Fatal("Expected the API key to be private.")
	}

	// Create another API key.
	r, body, err = at.Post("/user/apikeys", nil, nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	var ak2 database.APIKeyRecord
	err = json.Unmarshal(body, &ak2)
	if err != nil {
		t.Fatal(err)
	}

	// List all API keys this user has. Expect to find both keys we created.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
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
	r, body, err = at.Delete("/user/apikeys/"+ak1.ID.Hex(), nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	// List all API keys this user has. Expect to find only the second one.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
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

	// Try to delete the same key again. Expect a Bad Request.
	r, body, err = at.Delete("/user/apikeys/"+ak1.ID.Hex(), nil)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d.", r.StatusCode)
	}
}

// testPrivateAPIKeysUsage makes sure that we can use API keys to make API calls.
func testPrivateAPIKeysUsage(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	// Create a test user.
	email := name + "@siasky.net"
	r, _, err := at.CreateUserPost(email, name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	at.SetCookie(test.ExtractCookie(r))
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
	at.ClearCredentials()
	// We use a custom struct and not the APIKeyRecord one because that one does
	// not render the key in JSON form and therefore it won't unmarshal it,
	// either.
	var ak struct {
		Key database.APIKey
	}
	err = json.Unmarshal(body, &ak)
	if err != nil {
		t.Fatal(err)
	}
	// Get user stats without a cookie or headers - pass the API key via a query
	// variable. The main thing we want to see here is whether we get
	// an `Unauthorized` error or not but we'll validate the stats as well.
	params := url.Values{}
	params.Set("apiKey", string(ak.Key))
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

// TestPublicAPIKeyFlow validates the creation, listing, and deletion of public
// API keys.
func testPublicAPIKeysFlow(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, body, err := at.CreateUserPost(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err, string(body))
	}
	at.Cookie = test.ExtractCookie(r)

	sl1 := "AQAh2vxStoSJ_M9tWcTgqebUWerCAbpMfn9xxa9E29UOuw"
	sl2 := "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"

	aks := make([]database.APIKeyRecord, 0)

	// List all API keys this user has. Expect the list to be empty.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) > 0 {
		t.Fatalf("Expected an empty list of API keys, got %+v.", aks)
	}
	// Create a public API key.
	akPost := api.APIKeyPOST{
		Public:   true,
		Skylinks: []string{sl1},
	}
	akr, s, err := at.UserAPIKeysPOST(akPost)
	if err != nil || s != http.StatusOK {
		t.Fatal(err)
	}
	// List all API keys again. Expect to find a key.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) != 1 {
		t.Fatalf("Expected one API key, got %d.", len(aks))
	}
	if aks[0].Skylinks[0] != sl1 {
		t.Fatal("Unexpected skylinks list", aks[0].Skylinks)
	}
	// Update a public API key. Expect to go from sl1 to sl2.
	s, err = at.UserAPIKeysPUT(akr.ID, api.APIKeyPUT{Skylinks: []string{sl2}})
	if err != nil {
		t.Fatal(err)
	}
	// Get the key and verify the change.
	r, body, err = at.Get("/user/apikeys/"+akr.ID.Hex(), nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	var akr1 database.APIKeyRecord
	err = json.Unmarshal(body, &akr1)
	if err != nil {
		t.Fatal(err)
	}
	if akr1.Skylinks[0] != sl2 {
		t.Fatal("Unexpected skylinks list", aks[0].Skylinks)
	}
	// Patch a public API key. Expect to go from sl2 to sl1.
	akPatch := api.APIKeyPATCH{
		Add:    []string{sl1},
		Remove: []string{sl2},
	}
	s, err = at.UserAPIKeysPATCH(akr.ID, akPatch)
	if err != nil {
		t.Fatal(err)
	}
	// List and verify the change.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) != 1 {
		t.Fatalf("Expected one API key, got %d.", len(aks))
	}
	if aks[0].Skylinks[0] != sl1 {
		t.Fatal("Unexpected skylinks list", aks[0].Skylinks)
	}
	// Delete a public API key.
	r, body, err = at.Delete("/user/apikeys/"+akr.ID.Hex(), nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	// List and verify the change.
	r, body, err = at.Get("/user/apikeys", nil)
	if err != nil {
		t.Fatal(err, string(body))
	}
	err = json.Unmarshal(body, &aks)
	if err != nil {
		t.Fatal(err)
	}
	if len(aks) != 0 {
		t.Fatalf("Expected no API keys, got %d.", len(aks))
	}
}
