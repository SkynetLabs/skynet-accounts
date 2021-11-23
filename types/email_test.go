package types

import (
	"strings"
	"testing"
)

// TestEmailField_UnmarshalJSON test the custom unmarshalling of EmailField
// values and more specifically the proper email normalization.
func TestEmailField_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		out  EmailField
		err  string
	}{
		{name: "nil", in: nil, out: "", err: ""},
		{name: "empty", in: []byte{}, out: "", err: ""},
		{name: "standard", in: []byte("\"user@siasky.net\""), out: "user@siasky.net", err: ""},
		{name: "with names", in: []byte("\"Firstname Lastname \u003cuser@siasky.net\u003e\""), out: "user@siasky.net", err: ""},
		{name: "invalid", in: []byte("\"not an email\""), out: "", err: "failed to parse email"},
	}

	for _, tt := range tests {
		var e EmailField
		err := e.UnmarshalJSON(tt.in)
		if err == nil && tt.err != "" {
			t.Fatalf("Test %s failed. Expected error '%s', got 'nil'", tt.name, tt.err)
		}
		if err != nil && (tt.err == "" || !strings.Contains(err.Error(), tt.err)) {
			t.Fatalf("Test %s failed. Expected error '%s', got '%s'", tt.name, tt.err, err)
		}
		if e != tt.out {
			t.Fatalf("Test %s failed. Expected output '%s', got '%s'", tt.name, tt.out, e)
		}
	}
}
