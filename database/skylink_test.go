package database

import "testing"

// TestExtractSkylinkHash ensures ExtractSkylink properly returns the
// skylink hash.
func TestExtractSkylinkHash(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		out   string
		valid bool
	}{
		// base64 postfix
		{
			name:  "base64 postfix",
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "base64 postfix with path",
			in:    "https://siasky.net/_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng/some/path",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "base64 postfix bad",
			in:    "https://siasky.net/0A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			out:   "",
			valid: false,
		},
		// base64 prefix
		{
			name:  "base64 prefix",
			in:    "https://_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng.siasky.net",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "base64 prefix with path",
			in:    "https://_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng.siasky.net/some/path",
			out:   "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "base64 prefix bad",
			in:    "https://0A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng.siasky.net/",
			out:   "",
			valid: false,
		},
		// base32 postfix
		{
			name:  "base32 postfix",
			in:    "https://siasky.net/vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			out:   "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			valid: true,
		},
		{
			name:  "base32 postfix with path",
			in:    "https://siasky.net/vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g/some/path",
			out:   "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			valid: true,
		},
		{
			name:  "base32 postfix bad",
			in:    "https://siasky.net/vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9",
			out:   "",
			valid: false,
		},
		// base32 prefix
		{
			name:  "base32 prefix",
			in:    "https://vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g.siasky.net",
			out:   "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			valid: true,
		},
		{
			name:  "base32 prefix with path",
			in:    "https://vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g.siasky.net/some/path",
			out:   "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			valid: true,
		},
		{
			name:  "base32 prefix bad",
			in:    "https://vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9.siasky.net/",
			out:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		out, err := ExtractSkylink(tt.in)
		if tt.valid && err != nil {
			t.Fatalf("%s: expected %s to be valid, got error %s", tt.name, tt.in, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("%s: expected %s to be invalid", tt.name, tt.in)
		}
		if out != tt.out {
			t.Fatalf("%s: expected %s to have output %s, got %s", tt.name, tt.in, tt.out, out)
		}
	}
}

// TestValidSkylinkHash ensures ValidSkylink works properly.
func TestValidSkylinkHash(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		valid bool
	}{
		{
			name:  "base64",
			in:    "_A70A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: true,
		},
		{
			name:  "base64 bad",
			in:    "0A-ibzv2Woueb2_LutFjMq5nL9bamDtoSxYeq4nYwng",
			valid: false,
		},
		{
			name:  "base32",
			in:    "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9ger89cb1tas9r317g",
			valid: true,
		},
		{
			name:  "base32 bad",
			in:    "vg7f80v8jf7fr5l2sudtnsnemhccpasppfqrd9",
			valid: false,
		},
	}

	for _, tt := range tests {
		valid := ValidSkylink(tt.in)
		if valid != tt.valid {
			t.Fatalf("%s: expected %s to return %t, got %t", tt.name, tt.in, tt.valid, valid)
		}
	}
}
