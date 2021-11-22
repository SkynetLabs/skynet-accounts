package lib

import (
	"encoding/hex"
	"net/mail"

	"github.com/SkynetLabs/skynet-accounts/build"
	"gitlab.com/NebulousLabs/errors"
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

// NormalizeEmail returns the email address and strips all other text from the
// input, e.g. "Barry Gibbs <bg@example.com>" becomes "bg@example.com".
func NormalizeEmail(emailAddr string) (string, error) {
	parsedEmail, err := mail.ParseAddress(emailAddr)
	if err != nil {
		return "", errors.AddContext(err, "failed to parse email")
	}
	return parsedEmail.Address, nil
}
