package api

import (
	"reflect"
	"testing"

	"github.com/stripe/stripe-go/v72"
)

// TestStripePrices ensures that we work with the correct set of prices.
func TestStripePrices(t *testing.T) {
	// Set the Stripe key to a live key.
	stripe.Key = "sk_live_FAKE_LIVE_KEY"
	// Make sure we got the prod prices we expect.
	if !reflect.DeepEqual(StripePrices(), stripePricesProd) {
		t.Fatal("Expected prod prices, got something else.")
	}
	// Set the Stripe key to a test key.
	stripe.Key = "sk_test_FAKE_TEST_KEY"
	// Make sure we got the prod prices we expect.
	if !reflect.DeepEqual(StripePrices(), stripePricesTest) {
		t.Fatal("Expected test prices, got something else.")
	}
}

// TestStripeTestMode ensures that we detect test mode accurately.
func TestStripeTestMode(t *testing.T) {
	// Set the Stripe key to a live key.
	stripe.Key = "sk_live_FAKE_LIVE_KEY"
	// Expect test mode to be off.
	if StripeTestMode() {
		t.Fatal("Expected live mode, got test mode.")
	}
	// Set the Stripe key to a test key.
	stripe.Key = "sk_test_FAKE_TEST_KEY"
	// Expect test mode to be on.
	if !StripeTestMode() {
		t.Fatal("Expected test mode, got live mode.")
	}
}
