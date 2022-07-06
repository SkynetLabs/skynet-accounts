package fixtures

var (
	// StripeCheckoutSessionWithSubTier5 represents a Stripe Checkout Session
	// with a valid $5 subscription.
	StripeCheckoutSessionWithSubTier5 = `{
	"id": "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ3",
	"object": "checkout.session",
	"after_expiration": null,
	"allow_promotion_codes": null,
	"amount_subtotal": 2000,
	"amount_total": 2000,
	"automatic_tax": {
		"enabled": false,
		"status": null
	},
	"billing_address_collection": null,
	"cancel_url": "https://example.com/cancel",
	"client_reference_id": "00000000-bd52-4e90-a685-3572137c8989",
	"consent": null,
	"consent_collection": null,
	"currency": "usd",
	"customer": "cus_M0WOqhLQj6siQL",
	"customer_creation": "always",
	"customer_details": {
		"address": {
			"city": null,
			"country": "BG",
			"line1": null,
			"line2": null,
			"postal_code": null,
			"state": null
		},
		"email": "tester@siasky.net",
		"name": "tester",
		"phone": null,
		"tax_exempt": "none",
		"tax_ids": [
		]
	},
	"customer_email": null,
	"expires_at": 1657188169,
	"livemode": false,
	"locale": null,
	"metadata": {
	},
	"mode": "subscription",
	"payment_intent": null,
	"payment_link": null,
	"payment_method_options": null,
	"payment_method_types": [
		"card"
	],
	"payment_status": "paid",
	"phone_number_collection": {
		"enabled": false
	},
	"recovered_from": null,
	"setup_intent": null,
	"shipping": null,
	"shipping_address_collection": null,
	"shipping_options": [
	],
	"shipping_rate": null,
	"status": "complete",
	"submit_type": null,
	"subscription": {
		"id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
		"object": "subscription",
		"application": null,
		"application_fee_percent": null,
		"automatic_tax": {
			"enabled": false
		},
		"billing_cycle_anchor": 1657102857,
		"billing_thresholds": null,
		"cancel_at": null,
		"cancel_at_period_end": false,
		"canceled_at": null,
		"collection_method": "charge_automatically",
		"created": 1657102857,
		"current_period_end": 1659781257,
		"current_period_start": 1657102857,
		"customer": "cus_M0WOqhLQj6siQL",
		"days_until_due": null,
		"default_payment_method": "pm_1LIVLoIzjULiPWN6ylp8aN4O",
		"default_source": null,
		"default_tax_rates": [
		],
		"description": null,
		"discount": null,
		"ended_at": null,
		"items": {
			"object": "list",
			"data": [
				{
					"id": "si_M0WOWYbfSszXgy",
					"object": "subscription_item",
					"billing_thresholds": null,
					"created": 1657102858,
					"metadata": {
					},
					"plan": {
						"id": "price_1IReXpIzjULiPWN66PvsxHL4",
						"object": "plan",
						"active": true,
						"aggregate_usage": null,
						"amount": 2000,
						"amount_decimal": "2000",
						"billing_scheme": "per_unit",
						"created": 1614954157,
						"currency": "usd",
						"interval": "month",
						"interval_count": 1,
						"livemode": false,
						"metadata": {
						},
						"nickname": null,
						"product": "prod_J3m6ioQg90kZj5",
						"tiers_mode": null,
						"transform_usage": null,
						"trial_period_days": null,
						"usage_type": "licensed"
					},
					"price": {
						"id": "price_1IReXpIzjULiPWN66PvsxHL4",
						"object": "price",
						"active": true,
						"billing_scheme": "per_unit",
						"created": 1614954157,
						"currency": "usd",
						"custom_unit_amount": null,
						"livemode": false,
						"lookup_key": null,
						"metadata": {
						},
						"nickname": null,
						"product": "prod_J3m6ioQg90kZj5",
						"recurring": {
							"aggregate_usage": null,
							"interval": "month",
							"interval_count": 1,
							"trial_period_days": null,
							"usage_type": "licensed"
						},
						"tax_behavior": "unspecified",
						"tiers_mode": null,
						"transform_quantity": null,
						"type": "recurring",
						"unit_amount": 2000,
						"unit_amount_decimal": "2000"
					},
					"quantity": 1,
					"subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
					"tax_rates": [
					]
				}
			],
			"has_more": false,
			"total_count": 1,
			"url": "/v1/subscription_items?subscription=sub_1LIVLpIzjULiPWN6DeHJ2pIX"
		},
		"latest_invoice": "in_1LIVLpIzjULiPWN6ZyVHUI8m",
		"livemode": false,
		"metadata": {
		},
		"next_pending_invoice_item_invoice": null,
		"pause_collection": null,
		"payment_settings": {
			"payment_method_options": null,
			"payment_method_types": null,
			"save_default_payment_method": "off"
		},
		"pending_invoice_item_interval": null,
		"pending_setup_intent": null,
		"pending_update": null,
		"plan": {
			"id": "price_1IReY5IzjULiPWN6AxPytHEG",
			"object": "plan",
			"active": true,
			"aggregate_usage": null,
			"amount": 2000,
			"amount_decimal": "2000",
			"billing_scheme": "per_unit",
			"created": 1614954157,
			"currency": "usd",
			"interval": "month",
			"interval_count": 1,
			"livemode": false,
			"metadata": {
			},
			"nickname": null,
			"product": "prod_J3m6ioQg90kZj5",
			"tiers_mode": null,
			"transform_usage": null,
			"trial_period_days": null,
			"usage_type": "licensed"
		},
		"quantity": 1,
		"schedule": null,
		"start_date": 1657102857,
		"status": "active",
		"test_clock": null,
		"transfer_data": null,
		"trial_end": null,
		"trial_start": null
	},
	"success_url": "https://example.com/success",
	"total_details": {
		"amount_discount": 0,
		"amount_shipping": 0,
		"amount_tax": 0
	},
	"url": null
}`
	// StripeCheckoutSessionWithSubTier20 represents a Stripe Checkout Session
	// with a valid $20 subscription.
	StripeCheckoutSessionWithSubTier20 = `{
	"id": "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ4",
	"object": "checkout.session",
	"after_expiration": null,
	"allow_promotion_codes": null,
	"amount_subtotal": 2000,
	"amount_total": 2000,
	"automatic_tax": {
		"enabled": false,
		"status": null
	},
	"billing_address_collection": null,
	"cancel_url": "https://example.com/cancel",
	"client_reference_id": "00000000-bd52-4e90-a685-3572137c8989",
	"consent": null,
	"consent_collection": null,
	"currency": "usd",
	"customer": "cus_M0WOqhLQj6siQL",
	"customer_creation": "always",
	"customer_details": {
		"address": {
			"city": null,
			"country": "BG",
			"line1": null,
			"line2": null,
			"postal_code": null,
			"state": null
		},
		"email": "tester@siasky.net",
		"name": "tester",
		"phone": null,
		"tax_exempt": "none",
		"tax_ids": [
		]
	},
	"customer_email": null,
	"expires_at": 1657188169,
	"livemode": false,
	"locale": null,
	"metadata": {
	},
	"mode": "subscription",
	"payment_intent": null,
	"payment_link": null,
	"payment_method_options": null,
	"payment_method_types": [
		"card"
	],
	"payment_status": "paid",
	"phone_number_collection": {
		"enabled": false
	},
	"recovered_from": null,
	"setup_intent": null,
	"shipping": null,
	"shipping_address_collection": null,
	"shipping_options": [
	],
	"shipping_rate": null,
	"status": "complete",
	"submit_type": null,
	"subscription": {
		"id": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
		"object": "subscription",
		"application": null,
		"application_fee_percent": null,
		"automatic_tax": {
			"enabled": false
		},
		"billing_cycle_anchor": 1657102857,
		"billing_thresholds": null,
		"cancel_at": null,
		"cancel_at_period_end": false,
		"canceled_at": null,
		"collection_method": "charge_automatically",
		"created": 1657102857,
		"current_period_end": 1659781257,
		"current_period_start": 1657102857,
		"customer": "cus_M0WOqhLQj6siQL",
		"days_until_due": null,
		"default_payment_method": "pm_1LIVLoIzjULiPWN6ylp8aN4O",
		"default_source": null,
		"default_tax_rates": [
		],
		"description": null,
		"discount": null,
		"ended_at": null,
		"items": {
			"object": "list",
			"data": [
				{
					"id": "si_M0WOWYbfSszXgy",
					"object": "subscription_item",
					"billing_thresholds": null,
					"created": 1657102858,
					"metadata": {
					},
					"plan": {
						"id": "price_1IReY5IzjULiPWN6AxPytHEG",
						"object": "plan",
						"active": true,
						"aggregate_usage": null,
						"amount": 2000,
						"amount_decimal": "2000",
						"billing_scheme": "per_unit",
						"created": 1614954157,
						"currency": "usd",
						"interval": "month",
						"interval_count": 1,
						"livemode": false,
						"metadata": {
						},
						"nickname": null,
						"product": "prod_J3m6ioQg90kZj5",
						"tiers_mode": null,
						"transform_usage": null,
						"trial_period_days": null,
						"usage_type": "licensed"
					},
					"price": {
						"id": "price_1IReY5IzjULiPWN6AxPytHEG",
						"object": "price",
						"active": true,
						"billing_scheme": "per_unit",
						"created": 1614954157,
						"currency": "usd",
						"custom_unit_amount": null,
						"livemode": false,
						"lookup_key": null,
						"metadata": {
						},
						"nickname": null,
						"product": "prod_J3m6ioQg90kZj5",
						"recurring": {
							"aggregate_usage": null,
							"interval": "month",
							"interval_count": 1,
							"trial_period_days": null,
							"usage_type": "licensed"
						},
						"tax_behavior": "unspecified",
						"tiers_mode": null,
						"transform_quantity": null,
						"type": "recurring",
						"unit_amount": 2000,
						"unit_amount_decimal": "2000"
					},
					"quantity": 1,
					"subscription": "sub_1LIVLpIzjULiPWN6DeHJ2pIX",
					"tax_rates": [
					]
				}
			],
			"has_more": false,
			"total_count": 1,
			"url": "/v1/subscription_items?subscription=sub_1LIVLpIzjULiPWN6DeHJ2pIX"
		},
		"latest_invoice": "in_1LIVLpIzjULiPWN6ZyVHUI8m",
		"livemode": false,
		"metadata": {
		},
		"next_pending_invoice_item_invoice": null,
		"pause_collection": null,
		"payment_settings": {
			"payment_method_options": null,
			"payment_method_types": null,
			"save_default_payment_method": "off"
		},
		"pending_invoice_item_interval": null,
		"pending_setup_intent": null,
		"pending_update": null,
		"plan": {
			"id": "price_1IReY5IzjULiPWN6AxPytHEG",
			"object": "plan",
			"active": true,
			"aggregate_usage": null,
			"amount": 2000,
			"amount_decimal": "2000",
			"billing_scheme": "per_unit",
			"created": 1614954157,
			"currency": "usd",
			"interval": "month",
			"interval_count": 1,
			"livemode": false,
			"metadata": {
			},
			"nickname": null,
			"product": "prod_J3m6ioQg90kZj5",
			"tiers_mode": null,
			"transform_usage": null,
			"trial_period_days": null,
			"usage_type": "licensed"
		},
		"quantity": 1,
		"schedule": null,
		"start_date": 1657102857,
		"status": "active",
		"test_clock": null,
		"transfer_data": null,
		"trial_end": null,
		"trial_start": null
	},
	"success_url": "https://example.com/success",
	"total_details": {
		"amount_discount": 0,
		"amount_shipping": 0,
		"amount_tax": 0
	},
	"url": null
}`
	// StripeCheckoutSessionWithoutSub represents a Stripe Checkout Session
	// without a subscription.
	StripeCheckoutSessionWithoutSub = `{
	"id": "cs_test_a1fQmmAWGp1woxtWil1Xvx1wtv04fXErpaB7d5avGKvxoZiM86tJeATPZ5",
	"object": "checkout.session",
	"after_expiration": null,
	"allow_promotion_codes": null,
	"amount_subtotal": 2000,
	"amount_total": 2000,
	"automatic_tax": {
		"enabled": false,
		"status": null
	},
	"billing_address_collection": null,
	"cancel_url": "https://example.com/cancel",
	"client_reference_id": "00000000-bd52-4e90-a685-3572137c8989",
	"consent": null,
	"consent_collection": null,
	"currency": "usd",
	"customer": "cus_M0WOqhLQj6siQL",
	"customer_creation": "always",
	"customer_details": {
		"address": {
			"city": null,
			"country": "BG",
			"line1": null,
			"line2": null,
			"postal_code": null,
			"state": null
		},
		"email": "tester@siasky.net",
		"name": "tester",
		"phone": null,
		"tax_exempt": "none",
		"tax_ids": [
		]
	},
	"customer_email": null,
	"expires_at": 1657188169,
	"livemode": false,
	"locale": null,
	"metadata": {
	},
	"mode": "subscription",
	"payment_intent": null,
	"payment_link": null,
	"payment_method_options": null,
	"payment_method_types": [
		"card"
	],
	"payment_status": "paid",
	"phone_number_collection": {
		"enabled": false
	},
	"recovered_from": null,
	"setup_intent": null,
	"shipping": null,
	"shipping_address_collection": null,
	"shipping_options": [
	],
	"shipping_rate": null,
	"status": "complete",
	"submit_type": null,
	"subscription": null,
	"success_url": "https://example.com/success",
	"total_details": {
		"amount_discount": 0,
		"amount_shipping": 0,
		"amount_tax": 0
	},
	"url": null
}`
)
