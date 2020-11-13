package user

import "testing"

// TestEmail_Validate ensures email validation functions as expected.
func TestEmail_Validate(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"valid@example.com", true},
		{"valid@nebulous.tech", true},
		{"invalid_nebulous.tech", false},
		{"", false},
	}

	for _, tt := range tests {
		if tt.valid != (Email)(tt.email).Validate() {
			t.Errorf("Expected validation of '%s' to return %t, got %t.\n", tt.email, tt.valid, !tt.valid)
		}
	}
}
