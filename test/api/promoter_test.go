package api

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/api"
	"gitlab.com/NebulousLabs/errors"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestPromoterHandlers covers all handlers which are directly dependent on
// Promoter as payments handler.
func TestPromoterHandlers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dbName := test.DBNameForTest(t.Name())
	at, err := test.NewAccountsTester(dbName, api.PromoterPromoter, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	// Specify subtests to run
	tests := []subtest{
		{name: "PromoterSetTier", test: testHandlerPromoterSetTierPOST},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t, at)
		})
	}
}

// testHandlerUserPOST tests user creation and login.
func testHandlerPromoterSetTierPOST(t *testing.T, at *test.AccountsTester) {
	// Use the test's name as an email-compatible identifier.
	name := test.DBNameForTest(t.Name())
	u, _, err := test.CreateUserAndLogin(at, name)

	// Make sure the user is free tier.
	if u.Tier != database.TierFree {
		t.Fatalf("Expected %d, got %d", database.TierFree, u.Tier)
	}

	// Call the endpoint with a bad tier.
	status, err := at.PromoterSetTierPOST(u.Sub, -1)
	if err == nil || !strings.Contains(err.Error(), "invalid tier") || status != http.StatusBadRequest {
		t.Fatalf("Expected an 'invalid tier' error and %d, got %v and %d", http.StatusBadRequest, err, status)
	}
	status, err = at.PromoterSetTierPOST(u.Sub, database.TierMaxReserved+1)
	if err == nil || !strings.Contains(err.Error(), "invalid tier") || status != http.StatusBadRequest {
		t.Fatalf("Expected an 'invalid tier' error and %d, got %v and %d", http.StatusBadRequest, err, status)
	}
	status, err = at.PromoterSetTierPOST(u.Sub, database.TierAnonymous)
	if err == nil || !strings.Contains(err.Error(), "invalid tier") || status != http.StatusBadRequest {
		t.Fatalf("Expected an 'invalid tier' error and %d, got %v and %d", http.StatusBadRequest, err, status)
	}

	// Call the endpoint with a bad sub.
	badsub := hex.EncodeToString(fastrand.Bytes(16))
	status, err = at.PromoterSetTierPOST(badsub, 1)
	if err == nil || status != http.StatusNotFound {
		t.Fatalf("Expected an error and status %d, got %v, %d", http.StatusNotFound, err, status)
	}

	status, err = at.PromoterSetTierPOST(u.Sub, database.TierPremium20)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	u1, err := at.DB.UserBySub(at.Ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if u1.Tier != database.TierPremium20 {
		t.Fatalf("Expected tier %d, got %d", database.TierPremium20, u1.Tier)
	}
	status, err = at.PromoterSetTierPOST(u.Sub, database.TierFree)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	u1, err = at.DB.UserBySub(at.Ctx, u.Sub)
	if err != nil {
		t.Fatal(err)
	}
	if u1.Tier != database.TierFree {
		t.Fatalf("Expected tier %d, got %d", database.TierFree, u1.Tier)
	}
}
