package database

import "testing"

// TestValidateSkylink ensures validateSkylink properly returns the skylink hash
func TestValidateSkylink(t *testing.T) {
	tests := []struct {
		in    string
		out   string
		valid bool
	}{
		{
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng/some/path",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			in:    "https://siasky.net/0A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		out, err := validateSkylink(tt.in)
		if tt.valid && err != nil {
			t.Fatalf("expected %s to be valid, got error %s\n", tt.in, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("expected %s to be invalid\n", tt.in)
		}
		if out != tt.out {
			t.Fatalf("expected %s to have output %s, got %s\n", tt.in, tt.out, out)
		}
	}
}
