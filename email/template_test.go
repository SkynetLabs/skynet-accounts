package email

import "testing"

// TODO this isn't an actual test. maybe write one? does it make sense?
func TestConfirmEmail(t *testing.T) {
	e, err := confirmEmailEmail("inovakov@gmail.com", confirmEmailData{"somecode"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("message: %+v\n", e)
	e, err = recoverAccountEmail("inovakov@gmail.com", recoverAccountData{"somecode"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("message: %+v\n", e)
	e, err = accountAccessAttemptedEmail("inovakov@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("message: %+v\n", e)
}
