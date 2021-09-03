package lib

import (
	"encoding/hex"
	"fmt"

	"go.mongodb.org/mongo-driver/x/mongo/driver/uuid"
)

// GenerateUUID is a helper method that generates a UUID and encodes it in hex.
func GenerateUUID() string {
	uid, err := uuid.New()
	// the only way to get an error here is for uuid to be unable to read from
	// the RNG reader, which is highly unlikely. We'll log and retry.
	for err != nil {
		fmt.Println("Error during UUID creation:", err)
		uid, err = uuid.New()
	}
	return hex.EncodeToString(uid[:])
}
