package lib

import (
	"encoding/hex"

	"github.com/google/uuid"
)

// GenerateUUID is a helper method that generates a UUID and encodes it in hex.
func GenerateUUID() (string, error) {
	uid := uuid.New()
	return hex.EncodeToString(uid[:]), nil
}
