package database

import "testing"

// TestValidateSkylink ensures validateSkylink properly returns the skylink hash
func TestValidateSkylink(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		out   string
		valid bool
	}{
		{
			name:  "valid0",
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "valid1",
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng/some/path",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "invalid",
			in:    "https://siasky.net/0A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		out, err := validateSkylink(tt.in)
		if tt.valid && err != nil {
			t.Fatalf("expected %s to succeed, got error %s\n", tt.name, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("expected %s to fail\n", tt.name)
		}
		if out != tt.out {
			t.Fatalf("expected %s, got %s\n", tt.out, out)
		}
	}
}
