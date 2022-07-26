package fixtures

var (
	// StripeCheckoutSessionWithSubTier5 represents a Stripe Checkout Session
	// with a valid $5 subscription.
	StripeCheckoutSessionWithSubTier5 = `{
    "customer": "cus_M0WOqhLQj6siQL",
    "subscription": {
        "id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
        "created": 1657102857,
        "customer": "cus_M0WOqhLQj6siQL",
        "discount": {
            "coupon": {
                "duration": "repeating",
                "duration_in_months": 3,
                "name": "test50%",
                "percent_off": 50.0
            }
        },
        "items": {
            "object": "list",
            "data": [
                {
                    "id": "si_M0WOWYbfSszXgy",
                    "price": {
                        "id": "price_1IReXpIzjULiPWN66PvsxHL4"
                    },
                    "subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX"
                }
            ]
        },
        "plan": {
            "id": "price_1IReXpIzjULiPWN66PvsxHL4",
            "amount": 500,
            "currency": "usd",
            "interval": "month",
            "interval_count": 1,
            "product": {
                "description": "Pin up to 1TB",
                "name": "Skynet Plus"
            }
        },
        "start_date": 1657102857,
        "status": "active"
    }
}`
	// StripeCheckoutSessionWithSubTier20 represents a Stripe Checkout Session
	// with a valid $20 subscription.
	StripeCheckoutSessionWithSubTier20 = `{
    "customer": "cus_M0WOqhLQj6siQL",
    "subscription": {
        "id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
        "created": 1657102857,
        "customer": "cus_M0WOqhLQj6siQL",
        "discount": {
            "coupon": {
                "duration": "repeating",
                "duration_in_months": 3,
                "name": "test50%",
                "percent_off": 50.0
            }
        },
        "items": {
            "object": "list",
            "data": [
                {
                    "id": "si_M0WOWYbfSszXgy",
                    "price": {
                        "id": "price_1IReY5IzjULiPWN6AxPytHEG"
                    },
                    "subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX"
                }
            ]
        },
        "plan": {
            "id": "price_1IReY5IzjULiPWN6AxPytHEG",
            "amount": 2000,
            "currency": "usd",
            "interval": "month",
            "interval_count": 1,
            "product": {
                "description": "Pin up to 4TB",
                "name": "Skynet Pro"
            }
        },
        "start_date": 1657102857,
        "status": "active"
    }
}`
	// StripeCheckoutSessionWithoutSub represents a Stripe Checkout Session
	// without a subscription.
	StripeCheckoutSessionWithoutSub = `{
	"customer": "cus_M0WOqhLQj6siQL"
}`
	// StripeCheckoutSessionWithInactiveSub represents a Stripe Checkout Session
	// with an inactive subscription.
	StripeCheckoutSessionWithInactiveSub = `{
    "customer": "cus_M0WOqhLQj6siQL",
    "subscription": {
        "id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
        "created": 1657102857,
        "customer": "cus_M0WOqhLQj6siQL",
        "items": {
            "object": "list",
            "data": [
                {
                    "id": "si_M0WOWYbfSszXgy",
                    "price": {
                        "id": "price_1IReXpIzjULiPWN66PvsxHL4"
                    },
                    "subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX"
                }
            ]
        },
        "start_date": 1657102857,
        "status": "unpaid"
    }
}`
	// StripeCheckoutSessionWithPricelessSub represents a Stripe Checkout Session
	// with a subscription without a price field.
	StripeCheckoutSessionWithPricelessSub = `{
    "customer": "cus_M0WOqhLQj6siQL",
    "subscription": {
        "id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
        "created": 1657102857,
        "customer": "cus_M0WOqhLQj6siQL",
        "items": {
            "object": "list",
            "data": [
                {
                    "id": "si_M0WOWYbfSszXgy",
                    "subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX"
                }
            ]
        },
        "start_date": 1657102857,
        "status": "active"
    }
}`
)
