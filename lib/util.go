package lib

import (
	"encoding/hex"

	"github.com/NebulousLabs/skynet-accounts/build"
	"go.mongodb.org/mongo-driver/x/mongo/driver/uuid"
)

// GenerateUUID is a helper method that generates a UUID and encodes it in hex.
func GenerateUUID() (string, error) {
	uid, err := uuid.New()
	if err != nil {
		build.Critical("Error during UUID creation:", err)
		return "", err
	}
	return hex.EncodeToString(uid[:]), nil
}
