/**
Package types provides various service-wide types.

These types are used by more than one subsystem and should not provide
subsystem-specific behaviours, e.g. database-specific serialization.
The exception to this rule should be test methods which should allow testing
with all kinds of input.
*/

package types

import (
	"encoding/json"
	"strings"
)

type (
	// Email is a string type with some extra rules about its casing (it always
	// gets converted to lowercase). All subsystems working with emails should
	// use this type in the signatures of their exported methods and functions.
	Email string
)

// NewEmail creates a new Email.
func NewEmail(s string) Email {
	return Email(strings.ToLower(s))
}

// MarshalJSON defines a custom marshaller for this type.
func (e Email) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.String())
}

// UnmarshalJSON defines a custom unmarshaller for this type.
// The only custom part is the fact that we cast the email to lower case.
func (e *Email) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*e = NewEmail(s)
	return nil
}

// String is a fmt.Stringer implementation for Email.
func (e Email) String() string {
	return strings.ToLower(string(e))
}
