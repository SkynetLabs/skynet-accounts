package database

import (
	"testing"

	"gitlab.com/NebulousLabs/errors"
)

// TestFromString ensures that FromString correctly extracts a Referrer struct
// from any referrer link.
func TestFromString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		result Referrer
		error  error
	}{
		{
			name:   "empty referrer",
			input:  "",
			result: Referrer{},
			error:  errors.New("empty referrer"),
		},
		{
			name:   "invalid referrer",
			input:  "this is not a valid referrer",
			result: Referrer{},
			error:  errors.New("failed to detect referrer type"),
		},
		{
			name:   "hns domain",
			input:  "https://skygallery.hns.siasky.net/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
		{
			name:   "hns path",
			input:  "https://siasky.net/hns/skygallery/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
		{
			name:   "hns path on specific server",
			input:  "http://eu-ger-1.siasky.net/hns/skygallery/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
		{
			name:   "skylink base64",
			input:  "http://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			result: Referrer{CanonicalName: "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng", Type: "skylink"},
			error:  nil,
		},
		{
			name:   "skylink base64 with path",
			input:  "http://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng/some_path",
			result: Referrer{CanonicalName: "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng", Type: "skylink"},
			error:  nil,
		},
		{
			name:   "skylink base32",
			input:  "http://vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g.siasky.net",
			result: Referrer{CanonicalName: "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng", Type: "skylink"},
			error:  nil,
		},
		{
			name:   "skylink base32 with path",
			input:  "http://vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g.siasky.net/some_path",
			result: Referrer{CanonicalName: "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng", Type: "skylink"},
			error:  nil,
		},
		{
			name:   "web",
			input:  "http://example.com",
			result: Referrer{CanonicalName: "example.com", Type: "web"},
			error:  nil,
		},
		{
			name:   "web with subdomain",
			input:  "http://one.two.three-lala.example.com",
			result: Referrer{CanonicalName: "one.two.three-lala.example.com", Type: "web"},
			error:  nil,
		},
		{
			name:   "web with path",
			input:  "http://example.com/one/two/three?four=five",
			result: Referrer{CanonicalName: "example.com", Type: "web"},
			error:  nil,
		},
		{
			name:   "web with path and subdomain",
			input:  "http://one.two.example.com/three/four=five",
			result: Referrer{CanonicalName: "one.two.example.com", Type: "web"},
			error:  nil,
		},
	}

	for _, tt := range tests {
		r, e := FromString(tt.input)
		if tt.error == nil && e != nil {
			t.Logf("Failing test: %s", tt.name)
			t.Fatalf("Unexpected error '%s' for input '%s'", e.Error(), tt.input)
		}
		if tt.error != nil && (e == nil || e.Error() != tt.error.Error()) {
			t.Logf("Failing test: %s", tt.name)
			t.Fatalf("Expected error '%s', got '%v' for input '%s'.", tt.error.Error(), e, tt.input)
		}
		if tt.error != nil {
			continue
		}
		if r.CanonicalName != tt.result.CanonicalName || r.Type != tt.result.Type {
			t.Logf("Failing test: %s", tt.name)
			t.Fatalf("Expected referrer %v, got %v for input '%s'.", tt.result, r, tt.input)
		}
	}
}
