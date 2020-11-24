package database

import "testing"

// TestEmail_Validate ensures email validation functions as expected.
// See https://gist.github.com/cjaoude/fd9910626629b53c4d25
func TestEmail_Validate(t *testing.T) {
	valid := []string{
		"email@example.com",
		"firstname.lastname@example.com",
		"email@subdomain.example.com",
		"first_name+lastname@example.com",
		"1234567890@example.com",
		"email@example-one.com",
		"_______@example.com",
		"email@example.name",
		"email@example.museum",
		"email@example.co.jp",
		"firstname-lastname@example.com",
	}
	invalid := []string{
		"",
		"plainaddress",
		"#@%^%#$@#$@#.com",
		"@example.com",
		"Joe Smith <email@example.com>",
		"email.example.com",
		"email@example@example.com",
		"あいうえお@example.com",
		"email@example.com (Joe Smith)",
		"email@example",
		"email@111.222.333.44444",
		// Strange Invalid Addresses:
		"”(),:;<>[\\]@example.com",
		"just”not”right@example.com",
		"this\\ is\"really\"not\allowed@example.com",
	}
	for _, email := range valid {
		if !(Email)(email).Validate() {
			t.Errorf("Expected '%s' to be valid\n", email)
		}
	}
	for _, email := range invalid {
		if (Email)(email).Validate() {
			t.Errorf("Expected '%s' to be invalid\n", email)
		}
	}
}
