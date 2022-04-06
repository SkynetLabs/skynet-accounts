package api

import (
	"os"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v71"
)

// TestStripe is a complete test suite that covers all Stripe endpoints we
// expose.
func TestStripe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	// Ignore the error here because we don't care if we manage to load a .env
	// file or not. We only care whether we have the right env vars, which might
	// be set in a different way on dev machines and on CI.
	_ = godotenv.Load("../../.env")
	// We only run tests against Stripe's test infrastructure. For that we need
	// a test API key.
	key, ok := os.LookupEnv("STRIPE_API_KEY")
	if !ok || !strings.HasPrefix(key, "sk_test_") {
		t.Skipf("Skipping %s. If you want to run this test, update STRIPE_API_KEY to hold a test API key.\n"+
			"Expected STRIPE_API_KEY that starts with '%s', got '%s'", t.Name(), "sk_test_", key[:8])
	}
	stripe.Key = key
	api.StripeTestMode = true

	tests := map[string]func(t *testing.T, at *test.AccountsTester){
		"get prices": testPricesGET,
	}

	at, err := test.NewAccountsTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt(t, at)
		})
	}
}

// testPricesGET ensures that we have the expected test prices set on Stripe.
func testPricesGET(t *testing.T, at *test.AccountsTester) {
	ps, _, err := at.StripePricesGET()
	if err != nil {
		t.Fatal(err)
	}
	// Check if all expected test prices are there.
	testPrices := api.StripePrices()
	left := len(testPrices)
	for _, p := range ps {
		if _, exist := testPrices[p.ID]; exist {
			left--
		}
	}
	if left > 0 {
		t.Fatalf("Expected test prices %+v, got %+v", testPrices, ps)
	}
}
