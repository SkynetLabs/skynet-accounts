package api

import (
	"encoding/json"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/test"
)

type subtest struct {
	Name string
	Test func(t *testing.T, at *test.AccountsTester)
}

// TestHandlers is a meta test that sets up a test instance of accounts and runs
// a suite of tests that ensure all handlers behave as expected.
func TestHandlers(t *testing.T) {
	at, err := test.NewAccountsTester()
	if err != nil {
		t.Fatal(err)
	}
	defer at.Shutdown()

	// Specify subtests to run
	tests := []subtest{
		{Name: "SingleFileRegular", Test: testHandlerHealth},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tt.Test(t, at)
		})
	}
}

func testHandlerHealth(t *testing.T, at *test.AccountsTester) {
	_, _, b, err := at.Get("/health", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	status := struct {
		DBAlive bool `json:"dbAlive"`
	}{}
	err = json.Unmarshal(b, &status)
	if err != nil {
		t.Fatal(err)
	}
	if !status.DBAlive {
		t.Fatal("DB down.")
	}
}
