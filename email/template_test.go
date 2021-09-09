package email

import (
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/lib"
)

// TestConfirmEmailEmail ensures that the email we send to the user contains
// the correct confirmation link.
func TestConfirmEmailEmail(t *testing.T) {
	to := "user@siasky.net"
	token, err := lib.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	em, err := confirmEmailEmail(to, token)
	if err != nil {
		t.Fatal(err)
	}
	if em.To != to {
		t.Fatalf("Expected the email to go to %s, got %s", to, em.To)
	}
	if em.From != From {
		t.Fatalf("Expected the email to go from %s, got %s", From, em.From)
	}
	if !strings.Contains(em.Body, "https://siasky.net/user/confirm?token="+token) {
		t.Fatal("Invalid confirmation link.")
	}
}

// TestRecoverAccountEmail ensures that the email we send to the user contains
// the correct recovery link.
func TestRecoverAccountEmail(t *testing.T) {
	to := "user@siasky.net"
	token, err := lib.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	em, err := recoverAccountEmail(to, token)
	if err != nil {
		t.Fatal(err)
	}
	if em.To != to {
		t.Fatalf("Expected the email to go to %s, got %s", to, em.To)
	}
	if em.From != From {
		t.Fatalf("Expected the email to go from %s, got %s", From, em.From)
	}
	if !strings.Contains(em.Body, "https://siasky.net/user/recover?token="+token) {
		t.Fatal("Invalid confirmation link.")
	}
}

// TestAccountAccessAttemptedEmail ensures that the email we send to the user
// is going to the correct email.
func TestAccountAccessAttemptedEmail(t *testing.T) {
	to := "user@siasky.net"
	em, err := accountAccessAttemptedEmail(to)
	if err != nil {
		t.Fatal(err)
	}
	if em.To != to {
		t.Fatalf("Expected the email to go to %s, got %s", to, em.To)
	}
	if em.From != From {
		t.Fatalf("Expected the email to go from %s, got %s", From, em.From)
	}
}
