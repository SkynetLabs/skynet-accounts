package api

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/SkynetLabs/skynet-accounts/api"
	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/SkynetLabs/skynet-accounts/test"
	"github.com/SkynetLabs/skynet-accounts/test/fixtures"
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v72"
	"gopkg.in/h2non/gock.v1"
)

const (
	// Fixture values.
	fixtureSessionIDWithSub5     = "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ3"
	fixtureSessionIDWithSub20    = "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ4"
	fixtureSessionIDWithoutSub   = "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ5"
	fixtureSessionIDInactiveSub  = "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ6"
	fixtureSessionIDPricelessSub = "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ7"
	fixturePriceID5              = "price_1IReXpIzjULiPWN66PvsxHL4"
	fixturePriceID20             = "price_1IReY5IzjULiPWN6AxPytHEG"
	fixtureStripeID              = "cus_M0WOqhLQj6siQL"
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
	if !ok {
		t.Skipf("Skipping %s. If you want to run this test, update STRIPE_API_KEY to hold a test API key.\n", t.Name())
	}
	if !strings.HasPrefix(key, "sk_test_") {
		t.Skipf("Skipping %s. If you want to run this test, update STRIPE_API_KEY to hold a test API key.\n"+
			"Expected STRIPE_API_KEY that starts with '%s', got '%s'", t.Name(), "sk_test_", key[:8])
	}
	stripe.Key = key

	tests := map[string]func(t *testing.T, at *test.AccountsTester){
		"get billing":   testStripeBillingGET,
		"post billing":  testStripeBillingPOST,
		"get prices":    testStripePricesGET,
		"post checkout": testStripeCheckoutPOST,
		"get checkout":  testStripeCheckoutIDGET,
	}

	at, err := test.NewAccountsTester(t.Name(), nil)
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

// testStripeCheckoutIDGET ensures that we can get the info for a checkout
// session and act on it, i.e. promote the user, if needed.
func testStripeCheckoutIDGET(t *testing.T, at *test.AccountsTester) {
	name := test.DBNameForTest(t.Name())
	// Create a test user.
	r, _, err := at.UserPOST(name+"@siasky.net", name+"pass")
	if err != nil {
		t.Fatal(err)
	}
	c := test.ExtractCookie(r)
	at.SetCookie(c)
	defer func(c *http.Cookie) {
		at.SetCookie(c)
		_, _ = at.UserDELETE()
	}(c)
	u, _, err := at.UserGET()
	if err != nil {
		t.Fatal(err)
	}

	// Set the user's Stripe ID to the one from the fixture.
	u.StripeID = fixtureStripeID
	// Make sure the StripeID is also updated in the server DB. We can't run a
	// simple at.DB.UserSave() because the tester and the server might be
	// running off different databases.
	// See https://linear.app/skynetlabs/issue/SKY-1239/accounts-tester-parallel-testers
	_, _, err = at.UserPUT("", "", fixtureStripeID)
	if err != nil {
		t.Fatal(err)
	}

	defer gock.Off()
	// We need to enable networking in order to allow the Tester to call our
	// own API.
	gock.EnableNetworking()

	// mockSessionResponse is a helper that sets up a mocked response.
	mockSessionResponse := func(sessID string, status int, body io.Reader) {
		gock.New("https://api.stripe.com").
			Get("/v1/checkout/sessions/" + sessID).
			Reply(status).
			Body(body)
	}

	// Set up a response that will upgrade the user to tier 20.
	mockSessionResponse(fixtureSessionIDWithSub20, http.StatusOK, strings.NewReader(fixtures.StripeCheckoutSessionWithSubTier20))
	// Get the info on a $20 checkout session.
	info, status, err := at.StripeCheckoutIDGET(fixtureSessionIDWithSub20)
	if err != nil || status != http.StatusOK {
		t.Fatal(err, status)
	}
	// Ensure the price is correct.
	if info.Plan.Price != fixturePriceID20 {
		t.Fatalf("Expected price '%s', got '%s'", fixturePriceID20, info.Plan.Price)
	}
	// Ensure that the discount is correct.
	if info.Discount.PercentOff != 50.0 {
		t.Fatalf("Expected discount of %.2f, got %.2f", 50.0, info.Discount.PercentOff)
	}
	// Ensure that the user has been promoted.
	u, _, err = at.UserGET()
	if err != nil {
		t.Fatal(err)
	}
	if u.Tier != database.TierPremium20 {
		t.Fatalf("Expected tier %d, got %d", database.TierPremium20, u.Tier)
	}
	// Set up a response that won't upgrade a tier 20 user because it's tier 5.
	mockSessionResponse(fixtureSessionIDWithSub5, http.StatusOK, strings.NewReader(fixtures.StripeCheckoutSessionWithSubTier5))
	// Get the info on a $5 checkout session.
	info, status, err = at.StripeCheckoutIDGET(fixtureSessionIDWithSub5)
	if err != nil || status != http.StatusOK {
		t.Fatal(err, status)
	}
	// Ensure the price is correct.
	if info.Plan.Price != fixturePriceID5 {
		t.Fatalf("Expected price '%s', got '%s'", fixturePriceID5, info.Plan.Price)
	}
	// Ensure that the user has NOT been demoted.
	u, _, err = at.UserGET()
	if err != nil {
		t.Fatal(err)
	}
	if u.Tier != database.TierPremium20 {
		t.Fatalf("Expected tier %d, got %d", database.TierPremium20, u.Tier)
	}
	// Set up a response without a subscription.
	mockSessionResponse(fixtureSessionIDWithoutSub, http.StatusOK, strings.NewReader(fixtures.StripeCheckoutSessionWithoutSub))
	// Get the info on a checkout session that hasn't been completed and
	// doesn't have a subscription assigned to it, yet.
	info, status, err = at.StripeCheckoutIDGET(fixtureSessionIDWithoutSub)
	if err == nil || !strings.Contains(err.Error(), api.ErrCheckoutWithoutSub.Error()) || status != http.StatusBadRequest {
		t.Fatalf("Expected %d '%s', got %d '%s'", http.StatusBadRequest, api.ErrCheckoutWithoutSub.Error(), status, err)
	}
	// Set up a response with an inactive subscription.
	mockSessionResponse(fixtureSessionIDInactiveSub, http.StatusOK, strings.NewReader(fixtures.StripeCheckoutSessionWithInactiveSub))
	// Get the info on a checkout session without an active sub.
	info, status, err = at.StripeCheckoutIDGET(fixtureSessionIDInactiveSub)
	if err == nil || !strings.Contains(err.Error(), api.ErrSubNotActive.Error()) || status != http.StatusBadRequest {
		t.Fatalf("Expected %d '%s', got %d '%v'", http.StatusBadRequest, api.ErrSubNotActive.Error(), status, err)
	}
	// Set up a response without a subscription.
	mockSessionResponse(fixtureSessionIDPricelessSub, http.StatusOK, strings.NewReader(fixtures.StripeCheckoutSessionWithPricelessSub))
	// Get the info on a checkout session with a sub without a price.
	info, status, err = at.StripeCheckoutIDGET(fixtureSessionIDPricelessSub)
	if err == nil || !strings.Contains(err.Error(), api.ErrSubWithoutPrice.Error()) || status != http.StatusBadRequest {
		t.Fatalf("Expected %d '%s', got %d '%v'", http.StatusBadRequest, api.ErrSubWithoutPrice.Error(), status, err)
	}

	if gock.HasUnmatchedRequest() {
		t.Fatalf("Gock has %d unmatched requests.", len(gock.GetUnmatchedRequests()))
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
