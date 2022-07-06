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
		"start_date": 1657102857,
		"status": "active"
	}
}`
	// StripeCheckoutSessionWithoutSub represents a Stripe Checkout Session
	// without a subscription.
	StripeCheckoutSessionWithoutSub = `{
	"customer": "cus_M0WOqhLQj6siQL"
}`
)
