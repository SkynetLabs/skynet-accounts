package api

import (
	"encoding/json"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
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
	r, body, err := at.Get("/user/apikey", nil)
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
	r, body, err = at.Post("/user/apikey", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ak1 database.APIKey
	err = json.Unmarshal(body, &ak1)
	if err != nil {
		t.Fatal(err)
	}

	// Create another API key.
	r, body, err = at.Post("/user/apikey", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ak2 database.APIKey
	err = json.Unmarshal(body, &ak2)
	if err != nil {
		t.Fatal(err)
	}

	// List all API keys this user has. Expect to find both keys we created.
	r, body, err = at.Get("/user/apikey", nil)
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
	r, body, err = at.Delete("/user/apikey/"+ak1.Key, nil)
	if err != nil {
		t.Fatal(err)
	}
	// List all API keys this user has. Expect to find only the second one.
	r, body, err = at.Get("/user/apikey", nil)
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
	_, _, err := at.CreateUserPost(name+"@siasky.net", name+"_pass")
	if err != nil {
		t.Fatal(err)
	}
	// Create a new API key.
	_, body, err := at.Post("/user/apikey", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ak database.APIKey
	err = json.Unmarshal(body, &ak)
	if err != nil {
		t.Fatal(err)
	}
	// Get user stats without a cookie or headers - pass the API key via a query
	// variable.
	_, body, err = at.Get("/user/stats?api_key="+ak.Key, nil)
	if err != nil {
		t.Fatal(err)
	}
	var us database.UserStats
	err = json.Unmarshal(body, &us)
	if err != nil {
		t.Fatal(err)
	}
}
