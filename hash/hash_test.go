package hash

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestDecodeHash is a short sanity check that ensures decodeHash works as
// expected.
func TestDecodeHash(t *testing.T) {
	// Happy case.
	ac, salt, hash, err := decodeHash(Argon2HashRecord("$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"))
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	expectedHash, _ := base64.RawStdEncoding.DecodeString("eDQwOMoSyRmzyvpD/wwGBg")
	expectedSalt, _ := base64.RawStdEncoding.DecodeString("dwr95pEjaa7emZOu9bDAWw")
	if !bytes.Equal(hash, expectedHash) || !bytes.Equal(salt, expectedSalt) ||
		ac.Memory != 131072 || ac.Iterations != 2 || ac.Parallelism != 1 {
		t.Fatal("Unexpected result")
	}
	// Handle <nil>.
	_, _, _, err = decodeHash(nil)
	if err == nil || !errors.Contains(err, ErrInvalidHash) {
		t.Fatalf("Expected error '%v', got %v\n", ErrInvalidHash, err)
	}
	// Handle empty input.
	_, _, _, err = decodeHash(Argon2HashRecord{})
	if err == nil || !errors.Contains(err, ErrInvalidHash) {
		t.Fatalf("Expected error '%v', got %v\n", ErrInvalidHash, err)
	}
	// Corrupted record (missing "t=2").
	_, _, _, err = decodeHash(Argon2HashRecord("$argon2id$v=19$m=131072,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"))
	if err == nil || !strings.Contains(err.Error(), "input does not match format") {
		t.Fatalf("Expected error '%v', got %v\n", "input does not match format", err)
	}
	// Bad algorithm version.
	_, _, _, err = decodeHash(Argon2HashRecord("$argon2id$v=55$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg"))
	if err == nil || !errors.Contains(ErrIncompatibleVersion, err) {
		t.Fatalf("Expected error '%v', got %v\n", ErrIncompatibleVersion, err)
	}
	// Bad hash - not a base64 string.
	_, _, _, err = decodeHash(Argon2HashRecord("$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$кирилица"))
	if err == nil || !strings.Contains(err.Error(), "illegal base64 data") {
		t.Fatalf("Expected error '%v', got %v\n", "illegal base64 data", err)
	}
}

// TestGenerateCompare ensures that Generate and Compare work as expected.
func TestGenerateCompare(t *testing.T) {
	// Test Compare with a known pair of password and hash.
	pw, _ := base64.RawStdEncoding.DecodeString("rcyAlCjsKjg+abXKHT4GKSWZYF4fnkexxU0H2zLGgVM")
	hash, _ := base64.RawStdEncoding.DecodeString("JGFyZ29uMmlkJHY9MTkkbT0xMzEwNzIsdD0yLHA9MSRZNTJNbGtHV1NXWFE0SFBmWGUrbzlRJGcxY21yMHl2YTRsVEI4Z1V5KzdkQlE")
	err := Compare(string(pw), hash)
	if err != nil {
		t.Fatal("Password and hash don't match")
	}

	// Test Generate and Compare with a random password.
	pw = fastrand.Bytes(32)
	hash, err = Generate(string(pw))
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	err = Compare(string(pw), hash)
	if err != nil {
		t.Fatal("Password and hash don't match")
	}
}
