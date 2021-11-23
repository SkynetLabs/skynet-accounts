package lib

import (
	"strings"
	"testing"
)

// TestNormalizeEmail ensures that NormalizeEmail works as expected.
func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
		err  string
	}{
		{name: "empty", in: "", out: "", err: ""},
		{name: "standard", in: "user@siasky.net", out: "user@siasky.net", err: ""},
		{name: "with names", in: "Firstname Lastname <user@siasky.net>", out: "user@siasky.net", err: ""},
		{name: "invalid", in: "not an email", out: "", err: "failed to parse email"},
	}

	for _, tt := range tests {
		out, err := NormalizeEmail(tt.in)
		if err == nil && tt.err != "" {
			t.Fatalf("Test %s failed. Expected error '%s', got 'nil'", tt.name, tt.err)
		}
		if err != nil && (tt.err == "" || !strings.Contains(err.Error(), tt.err)) {
			t.Fatalf("Test %s failed. Expected error '%s', got '%s'", tt.name, tt.err, err)
		}
		if out != tt.out {
			t.Fatalf("Test %s failed. Expected output '%s', got '%s'", tt.name, tt.out, out)
		}
	}
}
