package types

import (
	"encoding/json"

	"github.com/SkynetLabs/skynet-accounts/lib"
)

// EmailField is a helper type that has custom unmarshalling code which
// makes sure the email has the correct structure.
type EmailField string

// UnmarshalJSON implements the json.Unmashaller interface and allows us to
// normalize the email right after it's unmarshalled.
func (e *EmailField) UnmarshalJSON(b []byte) error {
	// Missing or empty values will be interpreted as an empty string.
	if b == nil || len(b) == 0 {
		*e = ""
		return nil
	}
	var emailStr string
	if err := json.Unmarshal(b, &emailStr); err != nil {
		return err
	}
	normalizedEmail, err := lib.NormalizeEmail(emailStr)
	if err != nil {
		return err
	}
	*e = EmailField(normalizedEmail)
	return nil
}
