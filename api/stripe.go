package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/julienschmidt/httprouter"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/customer"
	"github.com/stripe/stripe-go/v71/sub"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// stripePlans maps Stripe user plans to specific tiers.
	// TODO This should be in the DB.
	stripePlans = map[string]int{
		"prod_J2FBsxvEl4VoUK": database.TierFree,
		"prod_J06Q7nJH3HJcYN": database.TierPremium5,
		"prod_J06Qu7zg1unO8R": database.TierPremium20,
		"prod_J06QbGjCvmZQGZ": database.TierPremium80,
	}
	// stripePlansPrices maps Stripe user plan prices to specific tiers.
	// TODO This should be in the DB.
	stripePlansPrices = map[string]int{
		"price_1IQAgvIzjULiPWN60U5buItF": database.TierFree,
		"price_1IO6DLIzjULiPWN6ix1KyCtf": database.TierPremium5,
		"price_1IO6DgIzjULiPWN6NiaSLEKa": database.TierPremium20,
		"price_1IO6DvIzjULiPWN6wHgK35J4": database.TierPremium80,
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

	/*
		TODO
			Events that carry the information we want:
			- invoice.payment_succeeded
			- invoice.paid
			- payment_intent.succeeded
			- invoice.updated:	This event is often sent when a payment succeeds or fails. If payment is successful the paid attribute is set to true and the status is paid. If payment fails, paid is set to false and the status remains open. Payment failures also trigger a invoice.payment_failed event.
	*/

	// Here we handle the entire class of subscription events.
	// https://stripe.com/docs/billing/subscriptions/overview#build-your-own-handling-for-recurring-charge-failures
	// https://stripe.com/docs/api/subscriptions/object
	if strings.Contains(event.Type, "customer.subscription") {
		//api.staticLogger.Traceln("WH raw event data >>> ", string(event.Data.Raw)) // TODO DEBUG
		var s stripe.Subscription
		err = json.Unmarshal(event.Data.Raw, &s)
		if err != nil {
			api.staticLogger.Warningln("Failed to parse event. Error: ", err, "\nEvent: ", string(event.Data.Raw))
			return
		}
		err = api.processSub(req.Context(), &s)
		if err != nil {
			api.staticLogger.Debugln("Failed to process sub:", err)
		}
		api.WriteSuccess(w)
		return
	}

	// Here we handle the entire class of stripeSchedule events.
	// See https://stripe.com/docs/api/subscription_schedules/object
	if strings.Contains(event.Type, "subscription_schedule") {
		//api.staticLogger.Traceln("WH raw event data >>> ", string(event.Data.Raw)) // TODO DEBUG
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
		err = api.processSub(req.Context(), s)
		if err != nil {
			api.staticLogger.Debugln("Failed to process sub:", err)
		}
	}

	api.WriteSuccess(w)
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
	//// Read the event and verify its signature.
	//event, err := webhook.ConstructEvent(payload, req.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	//if err != nil {
	//	return nil, http.StatusBadRequest, err
	//}

	// Read the event without any verification. Used for testing and development.
	event := stripe.Event{}
	if err = json.Unmarshal(payload, &event); err != nil {
		err = errors.AddContext(err, "error parsing request body")
		return nil, http.StatusBadRequest, err
	}
	return &event, http.StatusOK, nil
}

// processSub reads the information about the user's subscription and adjusts
// the user's record accordingly.
func (api *API) processSub(ctx context.Context, s *stripe.Subscription) error {
	api.staticLogger.Traceln(" >> Processing subscription:", s.ID)
	u, err := api.staticDB.UserByStripeID(ctx, s.Customer.ID)
	if err != nil {
		return errors.AddContext(err, "failed to fetch user from DB based on subscription info")
	}
	api.staticLogger.Tracef(" >> Subscribed user id %s, tier %d, until %s.\n", u.ID, u.Tier, u.SubscribedUntil.String())
	oldTier := u.Tier
	oldSubbedUntil := u.SubscribedUntil

	api.staticLogger.Tracef(" >>> customer %+v\n", s.Customer)
	// Get all active subscriptions for this customer.
	it := sub.List(&stripe.SubscriptionListParams{
		Customer: s.Customer.ID,
		Status:   string(stripe.SubscriptionStatusActive),
	})
	// Pick the highest active plan and set the user's tier based on that.
	tier := database.TierFree
	var expTime time.Time
	var numSubs int
	for _, subsc := range it.SubscriptionList().Data {
		api.staticLogger.Tracef(" >>> sub: %+v\n%s\n", subsc, subsc.Object)
		api.staticLogger.Tracef(" >>> sub plan: %+v\n", subsc.Plan)
		if tier < stripePlansPrices[subsc.Plan.ID] {
			tier = stripePlansPrices[subsc.Plan.ID]
			expTime = time.Unix(subsc.CurrentPeriodEnd, 0).UTC()
		}
		numSubs++
	}
	// We need the user to have at least one active subscription in order to be
	// able to manage them via the Dashboard. So, if they don't have one we will
	// create a free sub for them and make it active for an year.
	if numSubs == 0 {
		f := false
		params := &stripe.SubscriptionParams{
			Customer:          stripe.String(u.StripeId),
			CancelAtPeriodEnd: &f,
			Items: []*stripe.SubscriptionItemsParams{
				{
					Price: stripe.String(priceForTier(database.TierFree)),
				},
			},
		}
		s, err := sub.New(params)
		if err != nil {
			api.staticLogger.Warnf("Failed to create a subscription for user %+v, error %+v", u, err)
			return err
		}
		api.staticLogger.Traceln(" >>> free subscription auto-created!")
		tier = stripePlans[s.Plan.ID]
		expTime = time.Unix(s.CurrentPeriodEnd, 0).UTC()
	}
	u.Tier = tier
	u.SubscribedUntil = expTime
	api.staticLogger.Tracef(" >> User set to tier %d until %s.\n", u.Tier, u.SubscribedUntil.String())
	// Avoid the trip to the DB if nothing has changed.
	if u.Tier != oldTier || u.SubscribedUntil != oldSubbedUntil {
		return api.staticDB.UserSave(ctx, u)
	}
	return nil
}

// createStripeCustomer creates a new Stripe customer for the given user returns
// the Stripe ID. The customer always starts with the free tier.
// TODO Check if we need a valid payment method in order to set them on a paid tier.
func (api *API) createStripeCustomer(_ context.Context, u *database.User) (*stripe.Customer, error) {
	name := fmt.Sprintf("%s %s", u.FirstName, u.LastName)
	freePlan := planForTier(u.Tier)
	cp := &stripe.CustomerParams{
		Description: &u.Sub,
		Email:       &u.Email,
		Name:        &name,
		Plan:        &freePlan,
	}
	return customer.New(cp)
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
		api.staticLogger.Tracef(" >>>> Failed to update user %s, customer id %s to plan %s. Error: %+v\n", u.ID.Hex(), u.StripeId, plan, err)
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
	for plan, tier := range stripePlans {
		if tier == t {
			return plan
		}
	}
	return ""
}

// priceForTier is a small helper that returns the proper Stripe price id for
// the given Skynet tier.
func priceForTier(t int) string {
	for plan, tier := range stripePlansPrices {
		if tier == t {
			return plan
		}
	}
	return ""
}
