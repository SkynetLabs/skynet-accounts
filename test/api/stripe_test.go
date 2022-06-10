package api

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v72"
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

	tests := map[string]func(t *testing.T, at *test.AccountsTester){
		"get billing":   testStripeBillingGET,
		"post billing":  testStripeBillingPOST,
		"get prices":    testStripePricesGET,
		"post checkout": testStripeCheckoutPOST,
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

// testStripeBillingGET ensures that we can create a new billing session.
func testStripeBillingGET(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, _, err := at.UserPOST(name+"@siasky.net", name+"pass")
	if err != nil {
		t.Fatal(err)
	}
	c := test.ExtractCookie(r)

	at.SetFollowRedirects(false)

	// Try to start a billing session without valid user auth.
	at.ClearCredentials()
	_, s, err := at.StripeBillingGET()
	if err == nil || s != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got %d %s", s, err)
	}
	// Try with a valid user. Expect a temporary redirect error. This is not a
	// fail case, we expect that to happen. In production we'll follow that
	// redirect.
	at.SetCookie(c)
	h, s, err := at.StripeBillingGET()
	if err != nil || s != http.StatusTemporaryRedirect {
		t.Fatalf("Expected %d and no error, got %d '%s'", http.StatusTemporaryRedirect, s, err)
	}
	expectedRedirectPrefix := "https://billing.stripe.com/session/"
	if !strings.HasPrefix(h.Get("Location"), expectedRedirectPrefix) {
		t.Fatalf("Expected a redirect link with prefix '%s', got '%s'", expectedRedirectPrefix, h.Get("Location"))
	}
}

// testStripeBillingPOST ensures that we can create a new billing session.
func testStripeBillingPOST(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, _, err := at.UserPOST(name+"@siasky.net", name+"pass")
	if err != nil {
		t.Fatal(err)
	}
	c := test.ExtractCookie(r)

	at.SetFollowRedirects(false)

	// Try to start a billing session without valid user auth.
	at.ClearCredentials()
	_, s, err := at.StripeBillingPOST()
	if err == nil || s != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got %d %s", s, err)
	}
	// Try with a valid user. Expect a temporary redirect error. This is not a
	// fail case, we expect that to happen. In production we'll follow that
	// redirect.
	at.SetCookie(c)
	h, s, err := at.StripeBillingPOST()
	if err != nil || s != http.StatusTemporaryRedirect {
		t.Fatalf("Expected %d and no error, got %d '%s'", http.StatusTemporaryRedirect, s, err)
	}
	expectedRedirectPrefix := "https://billing.stripe.com/session/"
	if !strings.HasPrefix(h.Get("Location"), expectedRedirectPrefix) {
		t.Fatalf("Expected a redirect link with prefix '%s', got '%s'", expectedRedirectPrefix, h.Get("Location"))
	}
}

// testStripeCheckoutPOST ensures that we can create a new checkout session.
func testStripeCheckoutPOST(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	r, _, err := at.UserPOST(name+"@siasky.net", name+"pass")
	if err != nil {
		t.Fatal(err)
	}
	c := test.ExtractCookie(r)

	at.ClearCredentials()
	_, s, err := at.StripeCheckoutPOST("")
	if err == nil || s != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got %d %s", s, err)
	}
	at.SetCookie(c)
	// Get a valid test price id.
	var price string
	for pid := range api.StripePrices() {
		price = pid
		break
	}
	sessID, s, err := at.StripeCheckoutPOST(price)
	if err != nil {
		t.Fatal(err)
	}
	if sessID == "" {
		t.Fatal("Empty session ID.")
	}
}

// testStripePricesGET ensures that we have the expected test prices set on Stripe.
func testStripePricesGET(t *testing.T, at *test.AccountsTester) {
	ps, _, err := at.StripePricesGET()
	if err != nil {
		t.Fatal(err)
	}
	// Check if all expected test prices are there.
	testPrices := api.StripePrices()
	left := len(testPrices)
	for _, p := range ps {
		if p.Description == "" {
			t.Errorf("Empty description for price %s", p.ID)
		}
		if _, exist := testPrices[p.ID]; exist {
			left--
		}
	}
	if left > 0 {
		t.Fatalf("Expected test prices %+v, got %+v", testPrices, ps)
	}
}
