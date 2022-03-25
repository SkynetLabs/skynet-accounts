package api

import (
	"net/http"
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
	r, body, err := at.UserPOST(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err, string(body))
	}
	at.SetCookie(test.ExtractCookie(r))

	// List all API keys this user has. Expect the list to be empty.
	aks, _, err := at.UserAPIKeysLIST()
	if err != nil {
		t.Fatal(err, string(body))
	}
	if len(aks) > 0 {
		t.Fatalf("Expected an empty list of API keys, got %+v.", aks)
	}

	// Create a new API key.
	ak1, _, err := at.UserAPIKeysPOST(api.APIKeyPOST{Name: "one"})
	if err != nil {
		t.Fatal(err, string(body))
	}
	// Make sure the API key is private.
	if ak1.Public {
		t.Fatal("Expected the API key to be private.")
	}
	if ak1.Name != "one" {
		t.Fatal("Unexpected name.")
	}

	// Create another API key.
	ak2, _, err := at.UserAPIKeysPOST(api.APIKeyPOST{})
	if err != nil {
		t.Fatal(err, string(body))
	}

	// List all API keys this user has. Expect to find both keys we created.
	aks, _, err = at.UserAPIKeysLIST()
	if err != nil {
		t.Fatal(err, string(body))
	}
	if len(aks) != 2 {
		t.Fatalf("Expected two API keys, got %+v.", aks)
	}
	if ak1.ID.Hex() != aks[0].ID.Hex() && ak1.ID.Hex() != aks[1].ID.Hex() {
		t.Fatalf("Missing key '%s'! Set: %+v", ak1.ID.Hex(), aks)
	}
	if ak2.ID.Hex() != aks[0].ID.Hex() && ak2.ID.Hex() != aks[1].ID.Hex() {
		t.Fatalf("Missing key '%s'! Set: %+v", ak2.ID.Hex(), aks)
	}
	if aks[0].Name != "one" && aks[1].Name != "one" {
		t.Fatalf("Expected one of the two keys to be named 'one', got %+v", aks)
	}

	// Delete an API key.
	status, err := at.UserAPIKeysDELETE(ak1.ID)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(err, status)
	}
	// List all API keys this user has. Expect to find only the second one.
	aks, _, err = at.UserAPIKeysLIST()
	if err != nil {
		t.Fatal(err, string(body))
	}
	if len(aks) != 1 {
		t.Fatalf("Expected one API key, got %+v.", aks)
	}
	if ak2.ID.Hex() != aks[0].ID.Hex() {
		t.Fatalf("Missing key '%s'! Set: %+v", ak2.ID.Hex(), aks)
	}

	// Try to delete the same key again. Expect a 404.
	status, _ = at.UserAPIKeysDELETE(ak1.ID)
	if status != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d.", status)
	}
}

// testPrivateAPIKeysUsage makes sure that we can use API keys to make API calls.
func testPrivateAPIKeysUsage(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	// Create a test user.
	email := name + "@siasky.net"
	r, _, err := at.UserPOST(email, name+"_pass")
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
	_, _, err = test.CreateTestUpload(at.Ctx, at.DB, *u, uploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Create a new API key.
	akWithKey, _, err := at.UserAPIKeysPOST(api.APIKeyPOST{})
	if err != nil {
		t.Fatal(err)
	}
	// Stop using the cookie, so we can test the API key.
	at.ClearCredentials()
	// Get user stats with an API key. The main thing we want to see here is
	// whether we get an `Unauthorized` error or not but we'll validate the
	// stats as well.
	at.SetAPIKey(akWithKey.Key.String())
	us, _, err := at.UserStats("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if us.TotalUploadsSize != uploadSize || us.NumUploads != 1 || us.BandwidthUploads != skynet.BandwidthUploadCost(uploadSize) {
		t.Fatalf("Unexpected user stats. Expected TotalUploadSize %d (got %d), NumUploads 1 (got %d), BandwidthUploads %d (got %d).",
			uploadSize, us.TotalDownloadsSize, us.NumUploads, skynet.BandwidthUploadCost(uploadSize), us.BandwidthUploads)
	}
}

// testPublicAPIKeysFlow validates the creation, listing, and deletion of public
// API keys.
func testPublicAPIKeysFlow(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, body, err := at.UserPOST(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err, string(body))
	}
	at.SetCookie(test.ExtractCookie(r))

	sl1 := "AQAh2vxStoSJ_M9tWcTgqebUWerCAbpMfn9xxa9E29UOuw"
	sl2 := "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"

	// List all API keys this user has. Expect the list to be empty.
	aks, _, err := at.UserAPIKeysLIST()
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
	aks, _, err = at.UserAPIKeysLIST()
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
	akr1, _, err := at.UserAPIKeysGET(akr.ID)
	if err != nil {
		t.Fatal(err, string(body))
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
	aks, _, err = at.UserAPIKeysLIST()
	if err != nil {
		t.Fatal(err, string(body))
	}
	if len(aks) != 1 {
		t.Fatalf("Expected one API key, got %d.", len(aks))
	}
	if aks[0].Skylinks[0] != sl1 {
		t.Fatal("Unexpected skylinks list", aks[0].Skylinks)
	}
	// Delete a public API key.
	status, err := at.UserAPIKeysDELETE(akr.ID)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(err, status)
	}
	// List and verify the change.
	aks, _, err = at.UserAPIKeysLIST()
	if err != nil {
		t.Fatal(err, string(body))
	}
	if len(aks) != 0 {
		t.Fatalf("Expected no API keys, got %d.", len(aks))
	}
	// Delete the same key again. Expect a 404.
	status, err = at.UserAPIKeysDELETE(akr.ID)
	if status != http.StatusNotFound {
		t.Fatal("Expected status 404, got", status)
	}
}

// testPublicAPIKeysUsage makes sure that we can use public API keys to make
// GET requests to covered skylinks and that we cannot use them for other
// requests.
func testPublicAPIKeysUsage(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	// Create a test user.
	email := name + "@siasky.net"
	r, _, err := at.UserPOST(email, name+"_pass")
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
	sl, _, err := test.CreateTestUpload(at.Ctx, at.DB, *u, uploadSize)
	if err != nil {
		t.Fatal(err)
	}
	sl2, _, err := test.CreateTestUpload(at.Ctx, at.DB, *u, uploadSize)
	if err != nil {
		t.Fatal(err)
	}
	// Create a new public API key.
	apiKeyPOST := api.APIKeyPOST{
		Public:   true,
		Skylinks: []string{sl.Skylink},
	}
	pakWithKey, _, err := at.UserAPIKeysPOST(apiKeyPOST)
	if err != nil {
		t.Fatal(err)
	}
	// Stop using the cookie, use the public API key instead.
	at.SetAPIKey(pakWithKey.Key.String())
	// Try to fetch the user's stats with the new public API key.
	// Expect this to fail.
	_, _, err = at.UserStats("", nil)
	if err == nil {
		t.Fatal("Managed to get user stats with a public API key.")
	}
	// Get the user's limits for downloading a skylink covered by the public
	// API key. Expect to get TierFree values.
	ul, _, err := at.UserLimitsSkylink(sl.Skylink, "byte", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ul.DownloadBandwidth != database.UserLimits[database.TierFree].DownloadBandwidth {
		t.Fatalf("Expected to get download bandwidth of %d, got %d", database.UserLimits[database.TierFree].DownloadBandwidth, ul.DownloadBandwidth)
	}
	// Get the user's limits for downloading a skylink that is not covered by
	// the public API key. Expect to get TierAnonymous values.
	ul, _, err = at.UserLimitsSkylink(sl2.Skylink, "byte", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ul.DownloadBandwidth != database.UserLimits[database.TierAnonymous].DownloadBandwidth {
		t.Fatalf("Expected to get download bandwidth of %d, got %d", database.UserLimits[database.TierAnonymous].DownloadBandwidth, ul.DownloadBandwidth)
	}
	// Stop using the header, pass the skylink as a query parameter.
	at.ClearCredentials()
	// Get the user's limits for downloading a skylink covered by the public
	// API key. Expect to get TierFree values.
	ul, _, err = at.UserLimitsSkylink(sl.Skylink, "byte", pakWithKey.Key.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ul.DownloadBandwidth != database.UserLimits[database.TierFree].DownloadBandwidth {
		t.Fatalf("Expected to get download bandwidth of %d, got %d", database.UserLimits[database.TierFree].DownloadBandwidth, ul.DownloadBandwidth)
	}
}
