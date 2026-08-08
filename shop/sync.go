package shop

import (
	"database/sql"
	"fmt"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/price"
	"github.com/stripe/stripe-go/v79/product"
)

// SyncResult is what a sync did, for the message shown back on the admin
// page. Skipped counts active Stripe products with no usable one-time
// price — a subscription, or a product priced only in another currency.
type SyncResult struct {
	Created int
	Updated int
	Retired int
	Skipped int
}

func (r SyncResult) Summary() string {
	return fmt.Sprintf("Synced from Stripe: %d added, %d updated, %d retired, %d skipped.",
		r.Created, r.Updated, r.Retired, r.Skipped)
}

// SyncProducts pulls the active catalogue from Stripe into the products
// table. Stripe owns the name, price, and description; the photo, variants
// text, and ordering stay here, so the sync only writes the columns it owns
// (see db.UpsertStripeProduct).
//
// Anything Stripe no longer lists is marked sold out rather than deleted —
// the row carries a photo someone uploaded, and relisting should not mean
// re-uploading it.
func SyncProducts(conn *sql.DB) (SyncResult, error) {
	var result SyncResult

	// Stripe splits an item across two objects: the Product carries the
	// name and description, its Prices carry the amounts. Walk products,
	// then find each one's current one-time price.
	seen := make([]string, 0)
	iter := product.List(&stripe.ProductListParams{
		Active: stripe.Bool(true),
	})
	for iter.Next() {
		p := iter.Product()

		amount, priceID, ok, err := activeOneTimePrice(p.ID)
		if err != nil {
			return result, err
		}
		if !ok {
			result.Skipped++
			continue
		}

		created, err := db.UpsertStripeProduct(conn, db.StripeProduct{
			ProductID:   p.ID,
			PriceID:     priceID,
			Name:        p.Name,
			Description: p.Description,
			PriceCents:  amount,
		})
		if err != nil {
			return result, fmt.Errorf("saving %q: %w", p.Name, err)
		}
		if created {
			result.Created++
		} else {
			result.Updated++
		}
		seen = append(seen, p.ID)
	}
	if err := iter.Err(); err != nil {
		return result, fmt.Errorf("listing Stripe products: %w", err)
	}

	retired, err := db.RetireMissingStripeProducts(conn, seen)
	if err != nil {
		return result, err
	}
	result.Retired = retired

	return result, nil
}

// activeOneTimePrice returns the amount and id of a product's current
// one-time price. Recurring prices are skipped — the shop sells goods, not
// subscriptions — as is anything not in USD, which the site has no way to
// display. ok is false when the product has no price the shop can use.
func activeOneTimePrice(productID string) (amountCents int, priceID string, ok bool, err error) {
	iter := price.List(&stripe.PriceListParams{
		Product: stripe.String(productID),
		Active:  stripe.Bool(true),
	})
	for iter.Next() {
		pr := iter.Price()
		if pr.Recurring != nil {
			continue
		}
		if pr.Currency != stripe.CurrencyUSD {
			continue
		}
		return int(pr.UnitAmount), pr.ID, true, nil
	}
	if err := iter.Err(); err != nil {
		return 0, "", false, fmt.Errorf("listing prices for %s: %w", productID, err)
	}
	return 0, "", false, nil
}
