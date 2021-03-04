package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/customer"
	"github.com/stripe/stripe-go/v71/price"
	"github.com/stripe/stripe-go/v71/sub"
	"github.com/stripe/stripe-go/v71/webhook"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// StripeTestMode tells us whether to use Stripe's test mode or prod mode
	// plan and price ids.
	StripeTestMode = false

	// TODO These should be in the DB.

	// stripePlansTest maps Stripe plans to specific tiers.
	// DO NOT USE THESE DIRECTLY! Use stripePlans() instead.
	stripePlansTest = map[string]int{
		"prod_J2FBsxvEl4VoUK": database.TierFree,
		"prod_J06Q7nJH3HJcYN": database.TierPremium5,
		"prod_J06Qu7zg1unO8R": database.TierPremium20,
		"prod_J06QbGjCvmZQGZ": database.TierPremium80,
	}
	// stripePricesTest maps Stripe plan prices to specific tiers.
	// DO NOT USE THESE DIRECTLY! Use stripePrices() instead.
	stripePricesTest = map[string]int{
		"price_1IQAgvIzjULiPWN60U5buItF": database.TierFree,
		"price_1IO6DLIzjULiPWN6ix1KyCtf": database.TierPremium5,
		"price_1IO6DgIzjULiPWN6NiaSLEKa": database.TierPremium20,
		"price_1IO6DvIzjULiPWN6wHgK35J4": database.TierPremium80,
	}
	// stripePlansProd maps Stripe plans to specific tiers.
	// DO NOT USE THESE DIRECTLY! Use stripePlans() instead.
	stripePlansProd = map[string]int{
		"prod_J2FJE4gMqrOSwn": database.TierFree,
		"prod_J06NWykm9SRvWw": database.TierPremium5,
		"prod_J19xHMxmCmBScY": database.TierPremium20,
		"prod_J19xoBYOMbSlq4": database.TierPremium80,
	}
	// stripePricesProd maps Stripe plan prices to specific tiers.
	// DO NOT USE THESE DIRECTLY! Use stripePrices() instead.
	stripePricesProd = map[string]int{
		"price_1IQApHIzjULiPWN6tGNYEIOi": database.TierFree,
		"price_1IO6AdIzjULiPWN6PtviaWtS": database.TierPremium5,
		"price_1IP7dMIzjULiPWN6YHoHM3hK": database.TierPremium20,
		"price_1IP7ddIzjULiPWN6vBhBe9EG": database.TierPremium80,
	}
)

type (
	// stripePrice ...
	stripePrice struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Tier        int     `json:"tier"`
		Price       float64 `json:"price"`
		Currency    string  `json:"currency"`
		StripeID    string  `json:"stripe"`
		ProductID   string  `json:"productId"`
		LiveMode    bool    `json:"livemode"`
	}
)

// stripeWebhookHandler handles various events issued by Stripe.
// See https://stripe.com/docs/api/events/types
func (api *API) stripeWebhookHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %+v", req)
	event, code, err := readStripeEvent(w, req)
	if err != nil {
		api.WriteError(w, err, code)
		return
	}
	api.staticLogger.Debugf("Received event: %+v", event)

	// Here we handle the entire class of subscription events.
	// https://stripe.com/docs/billing/subscriptions/overview#build-your-own-handling-for-recurring-charge-failures
	// https://stripe.com/docs/api/subscriptions/object
	if strings.Contains(event.Type, "customer.subscription") {
		var s stripe.Subscription
		err = json.Unmarshal(event.Data.Raw, &s)
		if err != nil {
			api.staticLogger.Warningln("Failed to parse event. Error: ", err, "\nEvent: ", string(event.Data.Raw))
			return
		}
		err = api.processStripeSub(req.Context(), &s)
		if err != nil {
			api.staticLogger.Debugln("Failed to process sub:", err)
		}
		api.WriteSuccess(w)
		return
	}

	// Here we handle the entire class of subscription_schedule events.
	// See https://stripe.com/docs/api/subscription_schedules/object
	if strings.Contains(event.Type, "subscription_schedule") {
		var hasSub struct {
			Sub string `json:"subscription"`
		}
		err = json.Unmarshal(event.Data.Raw, &hasSub)
		if err != nil {
			api.staticLogger.Warningln("Failed to parse event. Error: ", err, "\nEvent: ", string(event.Data.Raw))
			return
		}
		if hasSub.Sub == "" {
			api.staticLogger.Debugln("Event doesn't refer to a subscription.")
			return
		}
		// Check the details about this subscription:
		s, err := sub.Get(hasSub.Sub, nil)
		if err != nil {
			api.staticLogger.Debugln("Failed to fetch sub:", err)
			return
		}
		err = api.processStripeSub(req.Context(), s)
		if err != nil {
			api.staticLogger.Debugln("Failed to process sub:", err)
		}
	}

	api.WriteSuccess(w)
}

// stripePricesHandler returns a list of plans and prices.
func (api *API) stripePricesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %+v", req)
	var sPrices []stripePrice
	t := true
	params := &stripe.PriceListParams{Active: &t}
	params.Filters.AddFilter("limit", "", "100")
	i := price.List(params)
	for i.Next() {
		p := i.Price()
		sp := stripePrice{
			ID:          p.ID,
			Name:        p.Product.Name,
			Description: p.Product.Description,
			Tier:        stripePrices()[p.ID],
			Price:       float64(p.UnitAmount) / 100,
			Currency:    string(p.Currency),
			StripeID:    p.ID,
			ProductID:   p.Product.ID,
			LiveMode:    p.Livemode,
		}
		sPrices = append(sPrices, sp)
	}
	api.WriteJSON(w, sPrices)
}

// readStripeEvent reads the event from the request body and verifies its
// signature.
func readStripeEvent(w http.ResponseWriter, req *http.Request) (*stripe.Event, int, error) {
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(req.Body)
	if err != nil {
		err = errors.AddContext(err, "error reading request body")
		return nil, http.StatusServiceUnavailable, err
	}
	// Read the event and verify its signature.
	event, err := webhook.ConstructEvent(payload, req.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	return &event, http.StatusOK, nil
}

// processStripeSub reads the information about the user's subscription and
// adjusts the user's record accordingly.
func (api *API) processStripeSub(ctx context.Context, s *stripe.Subscription) error {
	api.staticLogger.Traceln("Processing subscription:", s.ID)
	// Get all active subscriptions for this customer. There should be only one
	// (or none) but we'd better check.
	it := sub.List(&stripe.SubscriptionListParams{
		Customer: s.Customer.ID,
		Status:   string(stripe.SubscriptionStatusActive),
	})
	u, err := api.staticDB.UserByStripeID(ctx, s.Customer.ID)
	if err != nil {
		return errors.AddContext(err, "failed to fetch user from DB based on subscription info")
	}
	// Set the default values.
	u.Tier = database.TierFree
	u.SubscribedUntil = time.Time{}
	u.SubscriptionStatus = ""
	u.SubscriptionCancelAt = time.Time{}
	u.SubscriptionCancelAtPeriodEnd = false
	//Pick the highest active plan and set the user's tier based on that.
	for _, subsc := range it.SubscriptionList().Data {
		// It seems weird that the Plan.ID is actually a price id but this is
		// what we get from Stripe.
		t := stripePrices()[subsc.Plan.ID]
		if t > u.Tier {
			u.Tier = t
			u.SubscribedUntil = time.Unix(subsc.CurrentPeriodEnd, 0).UTC()
			u.SubscriptionStatus = string(subsc.Status)
			u.SubscriptionCancelAt = time.Unix(subsc.CancelAt, 0)
			u.SubscriptionCancelAtPeriodEnd = subsc.CancelAtPeriodEnd
		}
	}
	api.staticLogger.Tracef("Subscribed user id %s, tier %d, until %s.", u.ID, u.Tier, u.SubscribedUntil.String())
	return api.staticDB.UserSave(ctx, u)
}

// assignTier sets the user's account to the given tier, both on Stripe's side
// and in the DB.
func (api *API) assignTier(ctx context.Context, tier int, u *database.User) error {
	plan := planForTier(tier)
	oldTier := u.Tier
	cp := &stripe.CustomerParams{
		Plan: &plan,
	}
	_, err := customer.Update(u.StripeId, cp)
	if err != nil {
		return errors.AddContext(err, "failed to update customer on Stripe")
	}
	err = api.staticDB.UserSetTier(ctx, u, tier)
	if err != nil {
		err = errors.AddContext(err, "failed to update user in DB")
		// Try to revert the change on Stripe's side.
		plan = planForTier(oldTier)
		cp = &stripe.CustomerParams{
			Plan: &plan,
		}
		_, err2 := customer.Update(u.StripeId, cp)
		if err2 != nil {
			err2 = errors.AddContext(err2, "failed to revert the change on Stripe")
		}
		return errors.Compose(err, err2)
	}
	return nil
}

// planForTier is a small helper that returns the proper Stripe plan id for the
// given Skynet tier.
func planForTier(t int) string {
	for plan, tier := range stripePlans() {
		if tier == t {
			return plan
		}
	}
	return ""
}

// priceForTier is a small helper that returns the proper Stripe price id for
// the given Skynet tier.
func priceForTier(t int) string {
	for plan, tier := range stripePrices() {
		if tier == t {
			return plan
		}
	}
	return ""
}

// stripePlans returns a mapping of Stripe plan ids to Skynet tiers.
func stripePlans() map[string]int {
	if StripeTestMode {
		return stripePlansTest
	}
	return stripePlansProd
}

// stripePrices returns a mapping of Stripe price ids to Skynet tiers.
func stripePrices() map[string]int {
	if StripeTestMode {
		return stripePricesTest
	}
	return stripePricesProd
}
