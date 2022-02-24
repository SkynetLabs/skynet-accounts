package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/price"
	"github.com/stripe/stripe-go/v71/sub"
	"github.com/stripe/stripe-go/v71/webhook"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// MaxBodyBytes defines the maximum length of a webhook call's request body.
	MaxBodyBytes = int64(65536)
)

var (
	// StripeTestMode tells us whether to use Stripe's test mode or prod mode
	// plan and price ids. This depends on what kind of key is stored in the
	// STRIPE_API_KEY environment variable.
	StripeTestMode = false

	// True is a helper for when we need to pass a *bool to Stripe.
	True = true

	// TODO These should be in the DB.

	// stripePricesTest maps Stripe plan prices to specific tiers.
	// DO NOT USE THESE DIRECTLY! Use stripePrices() instead.
	stripePricesTest = map[string]int{
		// "price_1IQAgvIzjULiPWN60U5buItF": database.TierFree,
		"price_1IReXpIzjULiPWN66PvsxHL4": database.TierPremium5,
		"price_1IReY5IzjULiPWN6AxPytHEG": database.TierPremium20,
		"price_1IReYFIzjULiPWN6DqN2DwjN": database.TierPremium80,
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

// stripeWebhookPOST handles various events issued by Stripe.
// See https://stripe.com/docs/api/events/types
func (api *API) stripeWebhookPOST(_ *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.staticLogger.Tracef("Webhook request: %+v", req)
	event, code, err := readStripeEvent(w, req)
	if err != nil {
		api.WriteError(w, err, code)
		return
	}
	api.staticLogger.Tracef("Webhook event: %+v", event)

	// Here we handle the entire class of subscription events.
	// https://stripe.com/docs/billing/subscriptions/overview#build-your-own-handling-for-recurring-charge-failures
	// https://stripe.com/docs/api/subscriptions/object
	if strings.Contains(event.Type, "customer.subscription") {
		var s stripe.Subscription
		err = json.Unmarshal(event.Data.Raw, &s)
		if err != nil {
			api.staticLogger.Warningln("Webhook: Failed to parse event. Error: ", err, "\nEvent: ", string(event.Data.Raw))
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		err = api.processStripeSub(req.Context(), &s)
		if err != nil {
			api.staticLogger.Debugln("Webhook: Failed to process sub:", err)
			api.WriteError(w, err, http.StatusInternalServerError)
			return
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
			api.staticLogger.Warningln("Webhook: Failed to parse event. Error: ", err, "\nEvent: ", string(event.Data.Raw))
			api.WriteError(w, err, http.StatusBadRequest)
			return
		}
		if hasSub.Sub == "" {
			api.staticLogger.Debugln("Webhook: Event doesn't refer to a subscription.")
			api.WriteSuccess(w)
			return
		}
		// Check the details about this subscription:
		s, err := sub.Get(hasSub.Sub, nil)
		if err != nil {
			api.staticLogger.Debugln("Webhook: Failed to fetch sub:", err)
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		err = api.processStripeSub(req.Context(), s)
		if err != nil {
			api.staticLogger.Debugln("Webhook: Failed to process sub:", err)
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
	}

	api.WriteSuccess(w)
}

// stripePricesGET returns a list of plans and prices.
func (api *API) stripePricesGET(_ *database.User, w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	var sPrices []stripePrice
	params := &stripe.PriceListParams{Active: &True}
	params.AddExpand("data.product")
	params.Filters.AddFilter("limit", "", "1000")
	i := price.List(params)
	for i.Next() {
		p := i.Price()
		if !p.Active {
			continue
		}
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
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(req.Body)
	if err != nil {
		err = errors.AddContext(err, "error reading request body")
		return nil, http.StatusBadRequest, err
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
	// TODO Allow multiple stripe ids per user?
	u, err := api.staticDB.UserByStripeID(ctx, s.Customer.ID)
	if err != nil {
		return errors.AddContext(err, "failed to fetch user from DB based on subscription info")
	}
	// Pick the latest active plan and set the user's tier based on that.
	subs := it.SubscriptionList().Data
	var mostRecentSub *stripe.Subscription
	for _, subsc := range subs {
		if mostRecentSub == nil || subsc.Created > mostRecentSub.Created {
			mostRecentSub = subsc
		}
	}
	if mostRecentSub == nil {
		// No active sub, set the default values.
		u.Tier = database.TierFree
		u.SubscribedUntil = time.Time{}
		u.SubscriptionStatus = ""
		u.SubscriptionCancelAt = time.Time{}
		u.SubscriptionCancelAtPeriodEnd = false
	} else {
		// It seems weird that the Plan.ID is actually a price id but this
		// is what we get from Stripe.
		u.Tier = stripePrices()[mostRecentSub.Plan.ID]
		u.SubscribedUntil = time.Unix(mostRecentSub.CurrentPeriodEnd, 0).UTC().Truncate(time.Millisecond)
		u.SubscriptionStatus = string(mostRecentSub.Status)
		u.SubscriptionCancelAt = time.Unix(mostRecentSub.CancelAt, 0).UTC().Truncate(time.Millisecond)
		u.SubscriptionCancelAtPeriodEnd = mostRecentSub.CancelAtPeriodEnd
	}
	// Cancel all subs aside from the latest one.
	p := stripe.SubscriptionCancelParams{
		Params: stripe.Params{
			StripeAccount: &s.Customer.ID,
		},
		InvoiceNow: &True,
		Prorate:    &True,
	}
	for _, subsc := range subs {
		if subsc == nil || (mostRecentSub != nil && subsc.ID == mostRecentSub.ID) {
			continue
		}
		subsc, err = sub.Cancel(subsc.ID, &p)
		if err != nil {
			api.staticLogger.Warnf("Failed to cancel sub with id %s for user %s with Stripe customer id %s. Error: %s", subsc.ID, u.ID.Hex(), s.Customer.ID, err.Error())
		} else {
			api.staticLogger.Tracef("Successfully cancelled sub with id %s for user %s with Stripe customer id %s.", subsc.ID, u.ID.Hex(), s.Customer.ID)
		}
	}
	err = api.staticDB.UserSave(ctx, u)
	if err == nil {
		api.staticLogger.Tracef("Subscribed user id %s, tier %d, until %s.", u.ID, u.Tier, u.SubscribedUntil.String())
	}
	// Re-set the tier cache for this user, in case their tier changed.
	api.staticUserTierCache.Set(u)
	return err
}

// stripePrices returns a mapping of Stripe price ids to Skynet tiers.
func stripePrices() map[string]int {
	if StripeTestMode {
		return stripePricesTest
	}
	return stripePricesProd
}
