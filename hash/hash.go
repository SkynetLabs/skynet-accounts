package hash

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidHash               = errors.New("the encoded hash is not in the correct format")
	ErrIncompatibleVersion       = errors.New("incompatible version of argon2")
	ErrMismatchedHashAndPassword = errors.New("passwords do not match")

	// config is the configuration of the argon2id hasher.
	// For the moment we'll use the setup we had with Kratos. Later on we can
	// move this to a configuration file, if needed.
	config = argon2Config{
		SaltLength:  16,
		Iterations:  2,
		Memory:      131072,
		Parallelism: 1,
		KeyLength:   16,
	}
)

type argon2Config struct {
	SaltLength  uint32
	Iterations  uint32
	Memory      uint32
	Parallelism uint8
	KeyLength   uint32
}

// Generate hashes the given password with the default configuration.
func Generate(password []byte) ([]byte, error) {
	salt := fastrand.Bytes(int(config.SaltLength))
	hash := argon2.IDKey(password, salt, config.Iterations, config.Memory, config.Parallelism, config.KeyLength)

	var b bytes.Buffer
	if _, err := fmt.Fprintf(
		&b,
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, config.Memory, config.Iterations, config.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	); err != nil {
		return nil, errors.AddContext(err, "failed to generate password hash")
	}

	return b.Bytes(), nil
}

// Compare verifies whether the given password matches the given hash.
func Compare(password []byte, hash []byte) error {
	// Extract the parameters, salt and derived key from the encoded password
	// hash.
	cf, salt, hash, err := decodeHash(string(hash))
	if err != nil {
		return err
	}

	// Derive the key from the other password using the same parameters.
	otherHash := argon2.IDKey(password, salt, cf.Iterations, cf.Memory, cf.Parallelism, cf.KeyLength)

	// Check that the contents of the hashed passwords are identical. Note
	// that we are using the subtle.ConstantTimeCompare() function for this
	// to help prevent timing attacks.
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return nil
	}
	return ErrMismatchedHashAndPassword
}

// decodeHash is a helper method which extracts the configuration from the
// encoded hash string and returns its parts.
//
// Example of a password hash record:
// "$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"
func decodeHash(encodedHash string) (ac *argon2Config, salt, hash []byte, err error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	var version int
	_, err = fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	ac = &argon2Config{}
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &ac.Memory, &ac.Iterations, &ac.Parallelism)
	if err != nil {
		return nil, nil, nil, err
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, err
	}
	ac.SaltLength = uint32(len(salt))

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, err
	}
	ac.KeyLength = uint32(len(hash))

	return ac, salt, hash, nil
}
