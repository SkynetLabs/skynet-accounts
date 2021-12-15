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
	// ErrInvalidHash is returned when the given hash does not conform to the
	// expected format.
	ErrInvalidHash = errors.New("the encoded hash is not in the correct format")
	// ErrIncompatibleVersion is returned when the hash was created with a
	// different version of argon2.
	ErrIncompatibleVersion = errors.New("incompatible version of argon2")
	// ErrMismatchedHashAndPassword is returned when the given password doesn't
	// match the given hash.
	ErrMismatchedHashAndPassword = errors.New("passwords do not match")

	// config is the configuration of the argon2id hasher.
	config = argon2Config{
		SaltLength:  16,
		Iterations:  1,
		Memory:      65536,
		Parallelism: 4,
		KeyLength:   16,
	}
)

type (
	// Argon2HashRecord represents a password hashed with argon2id and combined
	// with the settings used for the hash creation using the standard argon2id
	// format.
	Argon2HashRecord []byte

	// argon2Config wraps together all configuration values we need in order to
	// create a hash with argon2.
	argon2Config struct {
		SaltLength  uint32
		Iterations  uint32
		Memory      uint32
		Parallelism uint8
		KeyLength   uint32
	}
)

// Generate returns an argon2 hash record of the given data. That hash record
// will be produced using the configuration settings in the `config` variable.
// The hash record contains not only the hash itself but also the configuration
// setting used for its creation, as well as the auto-generated salt used.
//
// Example hash record value:
// "$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"
func Generate(password string) (Argon2HashRecord, error) {
	// Generate a random salt with the length given in config.SaltLength.
	salt := fastrand.Bytes(int(config.SaltLength))
	// Generate an argon2id hash of the given password using the salt we just
	// generated, as well as the settings from `config`.
	hash := argon2.IDKey([]byte(password), salt, config.Iterations, config.Memory, config.Parallelism, config.KeyLength)

	// Generate a hash record value, consisting of the produced hash and the
	// parameters for its production - salt and settings.
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
func Compare(password string, hash Argon2HashRecord) error {
	// Extract the parameters, salt and derived key from the encoded password
	// hash.
	cf, salt, hash, err := decodeHash(hash)
	if err != nil {
		return errors.AddContext(err, "failed to decode hash record")
	}

	// Derive the key from the other password using the same parameters.
	otherHash := argon2.IDKey([]byte(password), salt, cf.Iterations, cf.Memory, cf.Parallelism, cf.KeyLength)

	// Check that the contents of the hashed passwords are identical. Note
	// that we are using the subtle.ConstantTimeCompare() function for this
	// to help prevent timing attacks.
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return nil
	}
	return ErrMismatchedHashAndPassword
}

// decodeHash is a helper method which extracts the configuration from the
// encoded hash record and returns its parts.
//
// Example of a password hash record:
// "$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"
func decodeHash(encodedHash Argon2HashRecord) (ac *argon2Config, salt, hash []byte, err error) {
	// The encodedHash contains six groups of data which provide all
	// configuration parameters needed to decode the hash:
	// - the sub-type of the hashing function used, e.g. argon2id
	// - the version of the hashing function, e.g. 19
	// - memory, iterations, and parallelism settings, e.g. m=131072,t=2,p=1
	// - the salt used for hashing, e.g. dwr95pEjaa7emZOu9bDAWw
	// - the hash itself, e.g. eDQwOMoSyRmzyvpD/wwGBg
	parts := strings.Split(string(encodedHash), "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	// Make sure the hash is created with the same version of the hashing
	// algorithm that we're using.
	var version int
	_, err = fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	// Fetch the settings used for creating the password hash. Those settings
	// affect the complexity of calculating the hash and also change the
	// resulting hash, so it's important to use the same ones when verifying
	// that a password matches the hash.
	ac = &argon2Config{}
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &ac.Memory, &ac.Iterations, &ac.Parallelism)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get the auto-generated salt used when creating the hash.
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, err
	}
	ac.SaltLength = uint32(len(salt))

	// Get the hash itself.
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, err
	}
	ac.KeyLength = uint32(len(hash))

	return ac, salt, hash, nil
}
