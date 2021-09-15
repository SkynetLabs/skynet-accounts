package email

import (
	"testing"
)

// TestConfig ensures that config properly parses email connection URIs.
func TestConfig(t *testing.T) {
	s := "smtps://test:test1@mailslurper:1025/?skip_ssl_verify=true"
	c, err := config(s)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server != "mailslurper" || c.Port != 1025 || c.User != "test" || c.Pass != "test1" || !c.InsecureSkipVerify {
		t.Fatal("Unexpected result.")
	}

	s = "smtps://asdf:fdsa@mail.siasky.net:999"
	c, err = config(s)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server != "mail.siasky.net" || c.Port != 999 || c.User != "asdf" || c.Pass != "fdsa" || c.InsecureSkipVerify {
		t.Fatal("Unexpected result.")
	}
}
