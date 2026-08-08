package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/webhook"
)

// HandleStripeWebhook processes Stripe webhook events.
func HandleStripeWebhook(conn *sql.DB, payload []byte, signature string, webhookSecret string) error {
	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return fmt.Errorf("webhook signature verification failed: %w", err)
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return fmt.Errorf("error parsing checkout.session.completed event: %w", err)
		}
		return handleCheckoutSessionCompleted(conn, &session)
	default:
		log.Printf("Received unknown webhook event type: %s", event.Type)
		return nil
	}
}

func handleCheckoutSessionCompleted(conn *sql.DB, session *stripe.CheckoutSession) error {
	// Extract order token from metadata
	orderToken, ok := session.Metadata["order_token"]
	if !ok {
		return fmt.Errorf("checkout session missing order_token metadata")
	}

	// Fetch the order
	order, err := db.OrderByToken(conn, orderToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("order with token %s not found", orderToken)
		}
		return fmt.Errorf("error fetching order: %w", err)
	}

	// Update order status to paid
	if err := db.UpdateOrderStatus(conn, order.ID, "paid"); err != nil {
		return fmt.Errorf("error updating order status: %w", err)
	}

	// The order code is the short reference a customer quotes when they
	// get in touch about an order — nothing is redeemed with it, unlike
	// ISM's box-office codes.
	code, err := assignOrderCode(conn, order.ID)
	if err != nil {
		return err
	}

	log.Printf("Order %s marked as paid, order code %s", orderToken, code)

	// TODO: Send confirmation email (db.OrderItemDetail would be fetched by caller if needed)

	return nil
}

// assignOrderCode stores a fresh order code, retrying on the (unlikely)
// chance a generated one is already taken — the column is UNIQUE, so a
// collision would otherwise leave a paid order without a reference.
func assignOrderCode(conn *sql.DB, orderID int64) (string, error) {
	const attempts = 5
	var err error
	for i := 0; i < attempts; i++ {
		code := generateOrderCode()
		if err = db.SetOrderRedeemCode(conn, orderID, code); err == nil {
			return code, nil
		}
	}
	return "", fmt.Errorf("could not assign an order code after %d attempts: %w", attempts, err)
}

// generateOrderCode returns a short code customers can read back over the
// phone. I and O are left out so they can't be misheard as 1 and 0.
func generateOrderCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[getRandomInt(len(charset))]
	}
	return string(b)
}

// getRandomInt returns a random integer in [0, max). len(charset) is 32,
// which divides 256 evenly, so the modulo introduces no bias.
func getRandomInt(max int) int {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return int(b[0]) % max
}
