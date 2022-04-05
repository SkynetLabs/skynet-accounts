package test

import "testing"

// TestSanitizeName ensures that SanitizeName works as expected.
func TestSanitizeName(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"valid name": {input: "valid_name", expected: "valid_name"},
		"with space": {input: "with space", expected: "with_space"},
		// The list of bad characters is: /\. "$*<>:|?
		// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions
		"various bad characters": {input: "/\\. \"$*<>:|?", expected: "____________"},
		"long name": {
			input:    "this_is_a_very_long_name_and_it_will_be_trimmed_to_sixty_four_characters",
			expected: "this_is_a_very_long_name_and_it_will_be_trimmed_to_sixty_four_characters"[:64],
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := SanitizeName(tt.input)
			if out != tt.expected {
				t.Errorf("Test '%s' failed. Expected '%s', got '%s'", name, tt.expected, out)
			}
		})
	}
}
