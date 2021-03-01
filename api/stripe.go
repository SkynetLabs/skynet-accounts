package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/stripe/stripe-go/v71/sub"

	"github.com/julienschmidt/httprouter"
	"github.com/stripe/stripe-go/v71"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// stripePlans maps Stripe user plans to specific tiers.
	// TODO This should be in the DB.
	stripePlans = map[string]int{
		"prod_J06Q7nJH3HJcYN": database.TierPremium5,
		"prod_J06Qu7zg1unO8R": database.TierPremium20,
		"prod_J06QbGjCvmZQGZ": database.TierPremium80,
	}
)

// stripeWebhookHandler handles various events issued by Stripe.
// See https://stripe.com/docs/api/events/types
func (api *API) stripeWebhookHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	api.staticLogger.Tracef("Processing request: %+v", req)
	event, code, err := readStripeEvent(w, req)
	if err != nil {
		api.WriteError(w, err, code)
		return
	}
	api.staticLogger.Debugf("Received event: %+v", event)
	api.staticLogger.Traceln("WH raw event data >>> ", string(event.Data.Raw)) // TODO DEBUG

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
	u, err := api.staticDB.UserByStripeID(ctx, s.Customer.ID)
	if err != nil {
		return errors.AddContext(err, "failed to fetch user from DB based on subscription info")
	}
	oldTier := u.Tier
	oldSubbedUntil := u.SubscribedUntil
	if s.Status != stripe.SubscriptionStatusActive {
		// The user's subscription is not active, demote them to "free".
		u.Tier = database.TierFree
	} else {
		// Check the subscription plan and set it to the user.
		tier, exists := stripePlans[s.Plan.Product.ID]
		if !exists {
			tier = database.TierFree
		}
		u.Tier = tier
		u.SubscribedUntil = time.Unix(s.CurrentPeriodEnd, 0).UTC()
	}
	// Avoid the tript to the DB if nothing has changed.
	if u.Tier != oldTier || u.SubscribedUntil != oldSubbedUntil {
		return api.staticDB.UserSave(ctx, u)
	}
	return nil
}
