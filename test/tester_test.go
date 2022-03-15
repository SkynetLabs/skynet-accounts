package test

import "testing"

// TestCleanName ensures that CleanName works as expected.
func TestCleanName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "valid name", input: "valid_name", expected: "valid_name"},
		{name: "with space", input: "with space", expected: "with_space"},
		// The list of bad characters is: /\. "$*<>:|?
		// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions
		{name: "various bad characters", input: "/\\. \"$*<>:|?", expected: "____________"},
		{
			name:     "long name",
			input:    "this_is_a_very_long_name_and_it_will_be_trimmed_to_sixty_four_characters",
			expected: "this_is_a_very_long_name_and_it_will_be_trimmed_to_sixty_four_characters"[:64],
		},
	}

	for _, tt := range tests {
		out := CleanName(tt.input)
		if out != tt.expected {
			t.Errorf("Test '%s' failed. Expected '%s', got '%s'", tt.name, tt.expected, out)
		}
	}
}
