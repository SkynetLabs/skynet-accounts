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
		tier                  int
		quotaExceeded         bool
		expectedTier          int
		expectedStorage       int64
		expectedUploadBW      int
		expectedDownloadBW    int
		expectedRegistryDelay int
	}{
		{
			name:                  "anon",
			tier:                  database.TierAnonymous,
			quotaExceeded:         false,
			expectedTier:          database.TierAnonymous,
			expectedStorage:       database.UserLimits[database.TierAnonymous].Storage,
			expectedUploadBW:      database.UserLimits[database.TierAnonymous].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierAnonymous].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierAnonymous].RegistryDelay,
		},
		{
			name:                  "plus, quota not exceeded",
			tier:                  database.TierPremium5,
			quotaExceeded:         false,
			expectedTier:          database.TierPremium5,
			expectedStorage:       database.UserLimits[database.TierPremium5].Storage,
			expectedUploadBW:      database.UserLimits[database.TierPremium5].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierPremium5].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierPremium5].RegistryDelay,
		},
		{
			name:                  "plus, quota exceeded",
			tier:                  database.TierPremium5,
			quotaExceeded:         true,
			expectedTier:          database.TierPremium5,
			expectedStorage:       database.UserLimits[database.TierPremium5].Storage,
			expectedUploadBW:      database.UserLimits[database.TierAnonymous].UploadBandwidth,
			expectedDownloadBW:    database.UserLimits[database.TierAnonymous].DownloadBandwidth,
			expectedRegistryDelay: database.UserLimits[database.TierAnonymous].RegistryDelay,
		},
	}

	for _, tt := range tests {
		ul := userLimitsGetFromTier(tt.tier, tt.quotaExceeded, true)
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
		_ = userLimitsGetFromTier(math.MaxInt, false, true)
		return
	}()
	if err != nil {
		t.Fatal(err)
	}
}

// TestValidateIP ensures that validateIP works as expected for both IPv4 and
// IPv6 IP addresses.
func TestValidateIP(t *testing.T) {
	tests := []struct {
		in       string
		expected string
	}{
		{in: "", expected: ""},
		{in: "12.12.12", expected: ""},
		{in: "1.2.3.256", expected: ""},
		{in: "1.2.3.4", expected: "1.2.3.4"},
		{in: "0.0.0.0", expected: "0.0.0.0"},
		{in: "2001:0db8:85a3:0000:0000:8a2e:0370:7334", expected: "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{in: "FE80:0000:0000:0000:0202:B3FF:FE1E:8329", expected: strings.ToLower("FE80:0000:0000:0000:0202:B3FF:FE1E:8329")},
		{in: "2001:db8:0:0:0:ff00:42:8329", expected: "2001:db8:0:0:0:ff00:42:8329"},
		{in: "2001:db8::ff00:42:8329", expected: "2001:db8::ff00:42:8329"},
		{in: "::1", expected: "::1"},
	}

	for _, tt := range tests {
		out := validateIP(tt.in)
		if out != tt.expected {
			t.Errorf("Expected '%s' to get me '%s' but got '%s'", tt.in, tt.expected, out)
		}
	}
}
