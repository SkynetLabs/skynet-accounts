package database

import (
	"testing"
)

// TestNewAPIKeyFromString validates that NewAPIKeyFromString properly handles
// valid API keys, upper case or lower case.
func TestNewAPIKeyFromString(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		valid bool
	}{
		{name: "empty", in: "", valid: false},
		{name: "valid upper case", in: "6TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL0", valid: true},
		{name: "valid lower case", in: "6taok0rvvkkk25pia33fhdbd1g04dlo015daad6om2j33kcd5cl0", valid: true},
		{name: "too short upper case", in: "6TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL", valid: false},
		{name: "too short lower case", in: "6taok0rvvkkk25pia33fhdbd1g04dlo015daad6om2j33kcd5cl", valid: false},
		{name: "too long upper case", in: "6TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL01", valid: false},
		{name: "too long lower case", in: "6taok0rvvkkk25pia33fhdbd1g04dlo015daad6om2j33kcd5cl01", valid: false},
		{name: "invalid alphabet", in: "!TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL0", valid: false},
	}

	for _, tt := range tests {
		_, err := NewAPIKeyFromString(tt.in)
		if (tt.valid && err != nil) || (!tt.valid && err == nil) {
			t.Errorf("Test '%s' failed.", tt.name)
		}
	}
}

// TestCoversSkylink ensures that CoversSkylink works as expected.
func TestCoversSkylink(t *testing.T) {
	sl1 := "6TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL0"
	sl2 := "7TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL0"
	sl3 := "8TAOK0RVVKKK25PIA33FHDBD1G04DLO015DAAD6OM2J33KCD5CL0"
	akr1 := APIKeyRecord{
		Public: false,
		Key:    NewAPIKey(),
	}
	akr2 := APIKeyRecord{
		Public:   true,
		Key:      NewAPIKey(),
		Skylinks: []string{sl1, sl2},
	}

	tests := []struct {
		name            string
		key             APIKeyRecord
		skylinkToCheck  string
		expectedCovered bool
	}{
		{
			name:            "general API key",
			key:             akr1,
			skylinkToCheck:  sl3,
			expectedCovered: true,
		},
		{
			name:            "public API key 1",
			key:             akr2,
			skylinkToCheck:  sl1,
			expectedCovered: true,
		},
		{
			name:            "public API key 2",
			key:             akr2,
			skylinkToCheck:  sl2,
			expectedCovered: true,
		},
		{
			name:            "public API key 3",
			key:             akr2,
			skylinkToCheck:  sl3,
			expectedCovered: false,
		},
	}

	for _, tt := range tests {
		covered := tt.key.CoversSkylink(tt.skylinkToCheck)
		if covered != tt.expectedCovered {
			t.Errorf("Unexpected result for test %s", tt.name)
		}
	}
}
