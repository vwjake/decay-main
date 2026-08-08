package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
)

// CreateCheckoutSessionParams holds the data needed to create a Stripe Checkout Session.
type CreateCheckoutSessionParams struct {
	ProductIDs []int64
	Quantities []int
	Email      string
	SuccessURL string
	CancelURL  string
}

// CreateCheckoutSession creates a Stripe Checkout Session and returns its ID.
// It also creates a pending order record in the database.
func CreateCheckoutSession(conn *sql.DB, params CreateCheckoutSessionParams) (sessionID string, orderToken string, err error) {
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
		CustomerEmail:      stripe.String(params.Email),
		SuccessURL:         stripe.String(params.SuccessURL),
		CancelURL:          stripe.String(params.CancelURL),
		Metadata: map[string]string{
			"order_token": orderToken,
		},
	}

	sess, err := session.New(params_stripe)
	if err != nil {
		return "", "", fmt.Errorf("failed to create Stripe session: %w", err)
	}

	return sess.ID, orderToken, nil
}

// generateSecureToken creates a random hex token for secure order identification.
func generateSecureToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
