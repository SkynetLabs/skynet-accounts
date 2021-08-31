package hash

import (
	"crypto/subtle"
	"encoding/base64"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestDecodeHash is a short sanity check that ensures decodeHash works as
// expected.
func TestDecodeHash(t *testing.T) {
	ac, salt, hash, err := decodeHash("$argon2id$v=19$m=131072,t=2,p=1$dwr95pEjaa7emZOu9bDAWw$eDQwOMoSyRmzyvpD/wwGBg")
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	expectedHash, _ := base64.RawStdEncoding.DecodeString("eDQwOMoSyRmzyvpD/wwGBg")
	expectedSalt, _ := base64.RawStdEncoding.DecodeString("dwr95pEjaa7emZOu9bDAWw")
	if subtle.ConstantTimeCompare(hash, expectedHash) != 1 ||
		subtle.ConstantTimeCompare(salt, expectedSalt) != 1 ||
		ac.Memory != 131072 || ac.Iterations != 2 || ac.Parallelism != 1 {
		t.Fatal("Unexpected result")
	}
}

// TestGenerateCompare ensures that Generate and Compare work as expected.
func TestGenerateCompare(t *testing.T) {
	// Test Compare with a known pair of password and hash.
	pw, _ := base64.RawStdEncoding.DecodeString("rcyAlCjsKjg+abXKHT4GKSWZYF4fnkexxU0H2zLGgVM")
	hash, _ := base64.RawStdEncoding.DecodeString("JGFyZ29uMmlkJHY9MTkkbT0xMzEwNzIsdD0yLHA9MSRZNTJNbGtHV1NXWFE0SFBmWGUrbzlRJGcxY21yMHl2YTRsVEI4Z1V5KzdkQlE")
	err := Compare(pw, hash)
	if err != nil {
		t.Fatal("Password and hash don't match")
	}

	// Test Generate and Compare with a random password.
	pw = fastrand.Bytes(32)
	hash, err = Generate(pw)
	if err != nil {
		t.Fatal("Unexpected error", err)
	}
	err = Compare(pw, hash)
	if err != nil {
		t.Fatal("Password and hash don't match")
	}
}
