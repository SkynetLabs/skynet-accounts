package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestEmail_String ensures that stringifying an Email will result in a
// lowercase string.
func TestEmail_String(t *testing.T) {
	s := "mIxEdCaSeStRiNg"
	e := Email(s)
	if !strings.EqualFold(e.String(), s) {
		t.Fatalf("Expected '%s', got '%s'", strings.ToLower(s), e)
	}
}

// TestEmail_MarshalJSON ensures that marshalling an Email will result in a
// lower case representation.
func TestEmail_MarshalJSON(t *testing.T) {
	s := "EmAiL@eXaMpLe.CoM"
	e := Email(s)
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	// We expect these bytes to match the source.
	expectedJSON := fmt.Sprintf("\"%s\"", strings.ToLower(s))
	if string(b) != expectedJSON {
		t.Fatalf("Expected '%s', got '%s'", expectedJSON, string(b))
	}
}

// TestEmail_UnmarshalJSON ensures that unmarshalling an Email will result in a
// lower case string, even if the marshalled data was mixed-case.
func TestEmail_UnmarshalJSON(t *testing.T) {
	// Manually craft a mixed-case JSON representation of an Email.
	b := []byte(`"EmAiL@eXaMpLe.CoM"`)
	var e Email
	err := json.Unmarshal(b, &e)
	if err != nil {
		t.Fatal(err)
	}
	// We expect the unmarshalled email to be lowercase only.
	if !strings.EqualFold(string(e), string(b[1:len(b)-1])) {
		t.Fatalf("Expected to get a lowercase version of '%s', i.e. '%s' but got '%s'", e, strings.ToLower(string(e)), e)
	}
}
