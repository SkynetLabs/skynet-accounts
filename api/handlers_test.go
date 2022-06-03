package api

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/database"
	"gitlab.com/NebulousLabs/errors"
)

// TestUserGETFromUser ensures the UserGETFromUser method correctly converts
// from database.User to UserGET.
func TestUserGETFromUser(t *testing.T) {
	var u *database.User
	var uGET *UserGET

	// Call with a nil value. Expect not to panic.
	uGET = UserGETFromUser(u)
	if uGET != nil {
		t.Fatal("Expected nil.")
	}

	u = &database.User{}
	// Call with a user without a confirmation token.
	u.EmailConfirmationToken = ""
	uGET = UserGETFromUser(u)
	if uGET == nil {
		t.Fatal("Unexpected nil.")
	}
	if !uGET.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be true.")
	}

	// Call with a user with a confirmation token.
	u.EmailConfirmationToken = "token"
	uGET = UserGETFromUser(u)
	if uGET == nil {
		t.Fatal("Unexpected nil.")
	}
	if uGET.EmailConfirmed {
		t.Fatal("Expected EmailConfirmed to be false.")
	}
}

// TestUserLimitsGetFromTier ensures the proper functioning of
// userLimitsGetFromTier.
func TestUserLimitsGetFromTier(t *testing.T) {
	tests := []struct {
		name                  string
		sub                   string
		tier                  int
		quotaExceeded         bool
		expectedSub           string
		expectedTier          int
		expectedStorage       int64
		expectedUploadBW      int
		expectedDownloadBW    int
		expectedRegistryDelay int
	}{
		{
			name:                  "anon",
			sub:                   "",
			tier:                  database.TierAnonymous,
			quotaExceeded:         false,
			expectedSub:           "",
			expectedTier:          database.TierAnonymous,
			expectedStorage:       database.UserLimits[database.TierAnonymous].Storage,
			expectedUploadBW:      database.UserLimits[database.TierAnonymous].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierAnonymous].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierAnonymous].RegistryDelay,
		},
		{
			name:                  "plus, quota not exceeded",
			sub:                   "this is a plus sub",
			tier:                  database.TierPremium5,
			quotaExceeded:         false,
			expectedSub:           "this is a plus sub",
			expectedTier:          database.TierPremium5,
			expectedStorage:       database.UserLimits[database.TierPremium5].Storage,
			expectedUploadBW:      database.UserLimits[database.TierPremium5].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierPremium5].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierPremium5].RegistryDelay,
		},
		{
			name:                  "plus, quota exceeded",
			sub:                   "this is a plus sub",
			tier:                  database.TierPremium5,
			quotaExceeded:         true,
			expectedSub:           "this is a plus sub",
			expectedTier:          database.TierPremium5,
			expectedStorage:       database.UserLimits[database.TierPremium5].Storage,
			expectedUploadBW:      database.UserLimits[database.TierAnonymous].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierAnonymous].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierAnonymous].RegistryDelay,
		},
	}

	for _, tt := range tests {
		ul := userLimitsGetFromTier(tt.sub, tt.tier, tt.quotaExceeded, true)
		if ul.Sub != tt.expectedSub {
			t.Errorf("Test '%s': expected sub '%s', got '%s'", tt.name, tt.expectedSub, ul.Sub)
		}
		if ul.TierID != tt.expectedTier {
			t.Errorf("Test '%s': expected tier %d, got %d", tt.name, tt.expectedTier, ul.TierID)
		}
		if ul.Storage != tt.expectedStorage {
			t.Errorf("Test '%s': expected storage %d, got %d", tt.name, tt.expectedStorage, ul.Storage)
		}
		if ul.UploadBandwidth != tt.expectedUploadBW {
			t.Errorf("Test '%s': expected upload bandwidth %d, got %d", tt.name, tt.expectedUploadBW, ul.UploadBandwidth)
		}
		if ul.DownloadBandwidth != tt.expectedDownloadBW {
			t.Errorf("Test '%s': expected download bandwidth %d, got %d", tt.name, tt.expectedDownloadBW, ul.DownloadBandwidth)
		}
		if ul.RegistryDelay != tt.expectedRegistryDelay {
			t.Errorf("Test '%s': expected registry delay %d, got %d", tt.name, tt.expectedRegistryDelay, ul.RegistryDelay)
		}
	}

	// Additionally, let us ensure that userLimitsGetFromTier logs a critical
	// when called with an invalid tier ID.
	err := func() (err error) {
		defer func() {
			e := recover()
			if e == nil {
				err = errors.New("expected to panic")
			}
			if !strings.Contains(fmt.Sprint(e), "userLimitsGetFromTier was called with non-existent tierID") {
				err = fmt.Errorf("expected to panic with a specific error message, got '%s'", fmt.Sprint(e))
			}
		}()
		// The call that we expect to log a critical.
		_ = userLimitsGetFromTier("", math.MaxInt, false, true)
		return
	}()
	if err != nil {
		t.Fatal(err)
	}
}
