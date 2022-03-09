package database

import "testing"

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
