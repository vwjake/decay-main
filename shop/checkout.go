package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
)

// CreateCheckoutSessionParams holds the data needed to create a Stripe Checkout Session.
type CreateCheckoutSessionParams struct {
	ProductIDs []int64
	Quantities []int
	// Email pre-fills the buyer's address if the caller already has it.
	// Left empty, Stripe collects it on its own hosted page instead.
	Email string
	// SuccessURL is where Stripe returns the buyer after payment. Any
	// "{ORDER_TOKEN}" in it is replaced with this order's secure token,
	// the same way Stripe substitutes its own {CHECKOUT_SESSION_ID}. The
	// confirmation page looks orders up by that token, so it has to be
	// ours in the URL rather than Stripe's session id.
	SuccessURL string
	CancelURL  string
}

// CreateCheckoutSession creates a Stripe Checkout Session and returns the
// hosted URL to send the buyer to. It also creates a pending order record in
// the database.
func CreateCheckoutSession(conn *sql.DB, params CreateCheckoutSessionParams) (checkoutURL string, orderToken string, err error) {
	// Fetch product details
	var products []db.Product
	for _, id := range params.ProductIDs {
		p, err := db.ProductByID(conn, id)
		if err != nil {
			return "", "", fmt.Errorf("product %d not found: %w", id, err)
		}
		if p.StripePriceID == "" {
			return "", "", fmt.Errorf("product %d does not have a Stripe price ID", id)
		}
		products = append(products, p)
	}

	// Create line items for Checkout Session
	var lineItems []*stripe.CheckoutSessionLineItemParams
	for i, p := range products {
		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(p.StripePriceID),
			Quantity: stripe.Int64(int64(params.Quantities[i])),
		})
	}

	// Generate a secure token for the order
	orderToken, err = generateSecureToken()
	if err != nil {
		return "", "", err
	}

	// Create order record with pending status
	order := db.Order{
		SecureToken:   orderToken,
		CustomerName:  "", // Will be filled from Stripe metadata later
		CustomerEmail: params.Email,
		Status:        "pending",
	}
	orderID, err := db.CreateOrder(conn, order)
	if err != nil {
		return "", "", fmt.Errorf("failed to create order: %w", err)
	}

	// Add order items
	for i, p := range products {
		item := db.OrderItem{
			OrderID:         orderID,
			ProductID:       p.ID,
			Quantity:        params.Quantities[i],
			PriceAtPurchase: p.PriceCents,
		}
		if err := db.AddOrderItem(conn, item); err != nil {
			return "", "", fmt.Errorf("failed to add order item: %w", err)
		}
	}

	// Create Checkout Session on Stripe
	params_stripe := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:          lineItems,
		SuccessURL:         stripe.String(successURLFor(params.SuccessURL, orderToken)),
		CancelURL:          stripe.String(params.CancelURL),
		Metadata: map[string]string{
			"order_token": orderToken,
		},
	}
	if params.Email != "" {
		params_stripe.CustomerEmail = stripe.String(params.Email)
	}

	sess, err := session.New(params_stripe)
	if err != nil {
		return "", "", fmt.Errorf("failed to create Stripe session: %w", err)
	}

	return sess.URL, orderToken, nil
}

// successURLFor puts this order's token into the URL Stripe returns the
// buyer to. Stripe substitutes its own {CHECKOUT_SESSION_ID} in the same
// string, and that id matches nothing in the orders table — the
// confirmation page finds an order by its secure token, so that is what
// has to end up in the link.
func successURLFor(template, orderToken string) string {
	return strings.ReplaceAll(template, "{ORDER_TOKEN}", orderToken)
}

// generateSecureToken creates a random hex token for secure order identification.
func generateSecureToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
