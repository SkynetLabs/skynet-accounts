package database

import (
	"testing"

	"gitlab.com/NebulousLabs/errors"
)

func TestFromString(t *testing.T) {
	tests := []struct {
		input  string
		result Referrer
		error  error
	}{
		{
			input:  "",
			result: Referrer{},
			error:  errors.New("empty referrer"),
		},
		{
			input:  "this is not a valid referrer",
			result: Referrer{},
			error:  errors.New("failed to detect referrer type"),
		},
		{
			input:  "https://skygallery.hns.siasky.net/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
		{
			input:  "https://siasky.net/hns/skygallery/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
		{
			input:  "http://eu-ger-1.siasky.net/hns/skygallery/something/something?dark=side",
			result: Referrer{CanonicalName: "skygallery", Type: "hns"},
			error:  nil,
		},
	}

	for _, tt := range tests {
		r, e := FromString(tt.input)
		if tt.error == nil && e != nil {
			t.Fatalf("Unexpected error '%s' for input '%s'", e.Error(), tt.input)
		}
		if tt.error != nil && (e == nil || e.Error() != tt.error.Error()) {
			t.Fatalf("Expected error '%s', got '%v' for input '%s'.", tt.error.Error(), e, tt.input)
		}
		if tt.error != nil {
			continue
		}
		if r.CanonicalName != tt.result.CanonicalName || r.Type != tt.result.Type {
			t.Fatalf("Expected referrer %v, got %v for input '%s'.", tt.result, r, tt.input)
		}
	}
}
