package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/webhook"
)

// OrderMailer is the part of mail.Mailer the shop needs, kept as an
// interface so a webhook can be exercised without an SMTP server.
type OrderMailer interface {
	Send(to, subject, body string) error
	Enabled() bool
}

// WebhookConfig carries what handling an event needs beyond the event
// itself: the signing secret, somewhere to send the buyer's confirmation,
// and the site's own address for the link inside it.
type WebhookConfig struct {
	Secret  string
	Mailer  OrderMailer
	SiteURL string
}

// HandleStripeWebhook processes Stripe webhook events.
func HandleStripeWebhook(conn *sql.DB, payload []byte, signature string, cfg WebhookConfig) error {
	event, err := webhook.ConstructEvent(payload, signature, cfg.Secret)
	if err != nil {
		return fmt.Errorf("webhook signature verification failed: %w", err)
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return fmt.Errorf("error parsing checkout.session.completed event: %w", err)
		}
		return handleCheckoutSessionCompleted(conn, &session, cfg)
	default:
		log.Printf("Received unknown webhook event type: %s", event.Type)
		return nil
	}
}

func handleCheckoutSessionCompleted(conn *sql.DB, session *stripe.CheckoutSession, cfg WebhookConfig) error {
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

	// Stripe redelivers an event it didn't get a clean response to, so the
	// same completed checkout can arrive more than once. Minting a second
	// code would change the reference under a customer already holding the
	// first one — and email them again — so a paid order that has its code
	// is left exactly as it is.
	if order.Status == "paid" && order.RedeemCode != nil && *order.RedeemCode != "" {
		log.Printf("Order %s is already paid with code %s — ignoring the repeat webhook", orderToken, *order.RedeemCode)
		return nil
	}

	// A one-click "buy" leaves the site's own order with no email — the
	// buyer entered it on Stripe's hosted page instead. Save it now so the
	// confirmation page and receipt have somewhere to go.
	if order.CustomerEmail == "" && session.CustomerDetails != nil && session.CustomerDetails.Email != "" {
		if err := db.SetOrderCustomerEmail(conn, order.ID, session.CustomerDetails.Email); err != nil {
			log.Printf("order %s: could not save the email Stripe collected: %v", orderToken, err)
		} else {
			order.CustomerEmail = session.CustomerDetails.Email
		}
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

	sendOrderConfirmation(conn, order, code, cfg)

	return nil
}

// sendOrderConfirmation emails the buyer their order code and what they
// bought. It is best effort on purpose: the payment has already gone
// through and the order is recorded, so a mail server that's down must not
// fail the webhook and have Stripe retry a sale that already completed.
// The confirmation page carries the same details either way.
func sendOrderConfirmation(conn *sql.DB, order db.Order, code string, cfg WebhookConfig) {
	if cfg.Mailer == nil || !cfg.Mailer.Enabled() {
		return
	}
	if order.CustomerEmail == "" {
		log.Printf("order %s has no email address on it — no confirmation sent", order.SecureToken)
		return
	}
	items, err := db.ItemsForOrder(conn, order.ID)
	if err != nil {
		log.Printf("order %s: could not read items for the confirmation email: %v", order.SecureToken, err)
		return
	}
	subject := fmt.Sprintf("Your DECAY order — %s", code)
	body := OrderConfirmationBody(order, items, code, cfg.SiteURL)
	if err := cfg.Mailer.Send(order.CustomerEmail, subject, body); err != nil {
		log.Printf("order %s: confirmation email to %s failed: %v", order.SecureToken, order.CustomerEmail, err)
	}
}

// OrderConfirmationBody is the plain-text receipt sent to a buyer. It is
// exported so its wording can be asserted in a test rather than only ever
// being seen in production.
func OrderConfirmationBody(order db.Order, items []db.OrderItemDetail, code, siteURL string) string {
	var b strings.Builder
	b.WriteString("Thanks — your order is confirmed and paid.\n\n")
	fmt.Fprintf(&b, "Order code: %s\n\n", code)
	for _, it := range items {
		fmt.Fprintf(&b, "  %d x %s   %s\n", it.Quantity, it.ProductName, db.Dollars(it.LineTotal()))
	}
	fmt.Fprintf(&b, "\nTotal: %s\n", db.Dollars(db.OrderTotal(items)))
	if siteURL != "" {
		fmt.Fprintf(&b, "\nYour order: %s/order/confirm?token=%s\n",
			strings.TrimRight(siteURL, "/"), order.SecureToken)
	}
	b.WriteString("\nQuote the order code if you get in touch about this order.\n\nDECAY\n")
	return b.String()
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
