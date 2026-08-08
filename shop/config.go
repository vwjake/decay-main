// Package shop connects the site's catalogue to Stripe: it syncs items down
// from there, sends buyers to a hosted Checkout Session, and records what
// they bought when the payment webhook confirms it.
//
// Stripe owns an item's name, price, and description. The photo does not
// come from Stripe — it's uploaded here and served from uploads/products/,
// so a sync never touches it.
package shop

import (
	"os"

	stripe "github.com/stripe/stripe-go/v79"
)

// Configure reads the Stripe credentials from the environment and returns
// whether the integration is live. Everything Stripe-facing — the checkout
// routes, the webhook, the sync button — stays dormant when it isn't, so an
// unconfigured site's items just have no checkout yet instead of
// half-working.
func Configure() bool {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	return Configured()
}

// Configured reports whether a Stripe secret key is set.
func Configured() bool { return stripe.Key != "" }

// WebhookSecret is the signing secret incoming webhooks are verified
// against. Empty means webhooks can't be trusted and shouldn't be served.
func WebhookSecret() string { return os.Getenv("STRIPE_WEBHOOK_SECRET") }
