package shop

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"decay-main/db"

	stripe "github.com/stripe/stripe-go/v79"
)

// fakeMailer stands in for mail.Mailer so a confirmation can be asserted
// without an SMTP server.
type fakeMailer struct {
	enabled bool
	err     error
	sent    []sentMail
}

type sentMail struct{ to, subject, body string }

func (f *fakeMailer) Enabled() bool { return f.enabled }

func (f *fakeMailer) Send(to, subject, body string) error {
	f.sent = append(f.sent, sentMail{to, subject, body})
	return f.err
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// seedPendingOrder creates a product and a pending order for two of it,
// returning the order's secure token.
func seedPendingOrder(t *testing.T, conn *sql.DB, email string) string {
	t.Helper()
	if _, err := db.UpsertStripeProduct(conn, db.StripeProduct{
		ProductID: "prod_tee", PriceID: "price_tee", Name: "Logo Tee", PriceCents: 3000,
	}); err != nil {
		t.Fatal(err)
	}
	products, err := db.ListProducts(conn)
	if err != nil || len(products) == 0 {
		t.Fatalf("seeding the product failed: %v (%d rows)", err, len(products))
	}

	const token = "tok_abc123"
	orderID, err := db.CreateOrder(conn, db.Order{
		SecureToken: token, CustomerEmail: email, Status: "pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddOrderItem(conn, db.OrderItem{
		OrderID: orderID, ProductID: products[0].ID, Quantity: 2, PriceAtPurchase: 3000,
	}); err != nil {
		t.Fatal(err)
	}
	return token
}

func completedSession(token string) *stripe.CheckoutSession {
	return &stripe.CheckoutSession{Metadata: map[string]string{"order_token": token}}
}

// A completed checkout marks the order paid, assigns the code a customer
// quotes, and emails them a receipt carrying it.
func TestCheckoutCompletedMarksPaidAndEmails(t *testing.T) {
	conn := testDB(t)
	token := seedPendingOrder(t, conn, "buyer@example.com")
	mailer := &fakeMailer{enabled: true}

	cfg := WebhookConfig{Mailer: mailer, SiteURL: "https://decay.events"}
	if err := handleCheckoutSessionCompleted(conn, completedSession(token), cfg); err != nil {
		t.Fatalf("handling the completed checkout: %v", err)
	}

	order, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}
	if !order.Paid() {
		t.Errorf("order status = %q, want paid", order.Status)
	}
	if order.Code() == "" {
		t.Error("no order code was assigned")
	}

	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(mailer.sent))
	}
	got := mailer.sent[0]
	if got.to != "buyer@example.com" {
		t.Errorf("sent to %q, want buyer@example.com", got.to)
	}
	if !strings.Contains(got.subject, order.Code()) {
		t.Errorf("subject %q doesn't carry the order code %q", got.subject, order.Code())
	}
	// Two tees at $30 — the receipt has to state what was actually charged.
	if !strings.Contains(got.body, "$60") {
		t.Errorf("body doesn't show the $60 total:\n%s", got.body)
	}
	if !strings.Contains(got.body, token) {
		t.Errorf("body doesn't link back to the order:\n%s", got.body)
	}
}

// Stripe redelivers an event it didn't get a clean response to. A second
// delivery must not mint a new code — the customer already has the first
// one in their inbox — nor email them again.
func TestCheckoutCompletedIgnoresRepeatWebhook(t *testing.T) {
	conn := testDB(t)
	token := seedPendingOrder(t, conn, "buyer@example.com")
	mailer := &fakeMailer{enabled: true}
	cfg := WebhookConfig{Mailer: mailer, SiteURL: "https://decay.events"}

	if err := handleCheckoutSessionCompleted(conn, completedSession(token), cfg); err != nil {
		t.Fatal(err)
	}
	first, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}

	if err := handleCheckoutSessionCompleted(conn, completedSession(token), cfg); err != nil {
		t.Fatalf("the repeat delivery errored: %v", err)
	}
	second, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}

	if second.Code() != first.Code() {
		t.Errorf("order code changed on redelivery: %q then %q", first.Code(), second.Code())
	}
	if len(mailer.sent) != 1 {
		t.Errorf("sent %d emails across two deliveries, want 1", len(mailer.sent))
	}
}

// Mail is a best-effort extra: the sale is already complete and recorded,
// so a mail server that's down must not fail the webhook and have Stripe
// retry a finished payment.
func TestCheckoutCompletedSurvivesMailFailure(t *testing.T) {
	conn := testDB(t)
	token := seedPendingOrder(t, conn, "buyer@example.com")
	mailer := &fakeMailer{enabled: true, err: errors.New("smtp is down")}

	cfg := WebhookConfig{Mailer: mailer, SiteURL: "https://decay.events"}
	if err := handleCheckoutSessionCompleted(conn, completedSession(token), cfg); err != nil {
		t.Fatalf("a failed email failed the webhook: %v", err)
	}

	order, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}
	if !order.Paid() || order.Code() == "" {
		t.Errorf("order not completed despite the sale going through: status %q, code %q", order.Status, order.Code())
	}
}

// An unconfigured mailer is the default for a site with no SMTP set up.
// The sale still has to complete.
func TestCheckoutCompletedWithoutMailer(t *testing.T) {
	conn := testDB(t)
	token := seedPendingOrder(t, conn, "buyer@example.com")

	if err := handleCheckoutSessionCompleted(conn, completedSession(token), WebhookConfig{}); err != nil {
		t.Fatalf("handling with no mailer: %v", err)
	}
	order, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}
	if !order.Paid() {
		t.Errorf("order status = %q, want paid", order.Status)
	}
}

// A one-click buy from the shop grid creates the order with no email —
// Stripe collects it on the hosted checkout page instead. The webhook has
// to pull it back from the completed session so the confirmation page and
// receipt have somewhere to go.
func TestCheckoutCompletedBackfillsEmailFromStripe(t *testing.T) {
	conn := testDB(t)
	token := seedPendingOrder(t, conn, "")
	mailer := &fakeMailer{enabled: true}
	cfg := WebhookConfig{Mailer: mailer, SiteURL: "https://decay.events"}

	session := completedSession(token)
	session.CustomerDetails = &stripe.CheckoutSessionCustomerDetails{Email: "buyer@example.com"}

	if err := handleCheckoutSessionCompleted(conn, session, cfg); err != nil {
		t.Fatalf("handling the completed checkout: %v", err)
	}

	order, err := db.OrderByToken(conn, token)
	if err != nil {
		t.Fatal(err)
	}
	if order.CustomerEmail != "buyer@example.com" {
		t.Errorf("order email = %q, want the address Stripe collected", order.CustomerEmail)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].to != "buyer@example.com" {
		t.Errorf("receipt not sent to the backfilled address: %+v", mailer.sent)
	}
}

func TestOrderConfirmationBodyListsItemsAndTotal(t *testing.T) {
	order := db.Order{SecureToken: "tok_xyz", CustomerEmail: "buyer@example.com"}
	items := []db.OrderItemDetail{
		{OrderItem: db.OrderItem{Quantity: 2, PriceAtPurchase: 3000}, ProductName: "Logo Tee"},
		{OrderItem: db.OrderItem{Quantity: 1, PriceAtPurchase: 1250}, ProductName: "Bandana"},
	}

	body := OrderConfirmationBody(order, items, "ABC234", "https://decay.events/")

	for _, want := range []string{
		"ABC234",        // the code
		"2 x Logo Tee",  // the line
		"$60",           // its total
		"$12.50",        // an amount with cents
		"Total: $72.50", // the order total
		"https://decay.events/order/confirm?token=tok_xyz", // no doubled slash
	} {
		if !strings.Contains(body, want) {
			t.Errorf("confirmation body is missing %q:\n%s", want, body)
		}
	}
}

// The success URL has to come back carrying our own order token: Stripe's
// {CHECKOUT_SESSION_ID} is a session id, which matches no order row.
func TestSuccessURLCarriesTheOrderToken(t *testing.T) {
	got := successURLFor("https://decay.events/order/confirm?token={ORDER_TOKEN}", "tok_abc")
	want := "https://decay.events/order/confirm?token=tok_abc"
	if got != want {
		t.Errorf("successURLFor = %q, want %q", got, want)
	}
}
