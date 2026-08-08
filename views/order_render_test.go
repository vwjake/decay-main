package views

import (
	"context"
	"strings"
	"testing"

	"decay-main/db"
)

func orderItems() []db.OrderItemDetail {
	return []db.OrderItemDetail{
		{OrderItem: db.OrderItem{Quantity: 2, PriceAtPurchase: 3000}, ProductName: "Logo Tee"},
		{OrderItem: db.OrderItem{Quantity: 1, PriceAtPurchase: 1250}, ProductName: "Bandana"},
	}
}

func renderOrder(t *testing.T, p OrderConfirmPage) string {
	t.Helper()
	var b strings.Builder
	if err := OrderConfirm(p).Render(context.Background(), &b); err != nil {
		t.Fatalf("OrderConfirm render: %v", err)
	}
	return b.String()
}

// A confirmed order shows the code to quote and what was paid.
func TestOrderConfirmPaid(t *testing.T) {
	code := "ABC234"
	html := renderOrder(t, OrderConfirmPage{
		Order: db.Order{
			SecureToken: "tok_abc", CustomerEmail: "buyer@example.com",
			Status: "paid", RedeemCode: &code,
		},
		Items: orderItems(),
	})

	for _, want := range []string{"Order confirmed", "ABC234", "Logo Tee", "Bandana", "$60", "$12.50", "$72.50"} {
		if !strings.Contains(html, want) {
			t.Errorf("paid confirmation is missing %q", want)
		}
	}
	// Nothing to poll for once it's paid, or the page would reload itself
	// in a loop.
	if strings.Contains(html, "data-order-poll") {
		t.Error("a paid order still carries the polling hook")
	}
}

// Stripe redirects the buyer back the instant they pay, which can be
// before the webhook lands. That order is pending, not broken: the page
// shows what they bought and marks itself for polling.
func TestOrderConfirmPendingPolls(t *testing.T) {
	html := renderOrder(t, OrderConfirmPage{
		Order: db.Order{SecureToken: "tok_abc", CustomerEmail: "buyer@example.com", Status: "pending"},
		Items: orderItems(),
	})

	if !strings.Contains(html, `data-order-poll="tok_abc"`) {
		t.Error("pending order isn't marked for polling with its token")
	}
	if !strings.Contains(html, "$72.50") {
		t.Error("pending order doesn't show what was charged")
	}
	// The code doesn't exist yet, so the block that presents one mustn't
	// render empty.
	if strings.Contains(html, "Order code") {
		t.Error("pending order shows an order code block before there's a code")
	}
}

// An order whose items couldn't be listed, or one with none, still has to
// render — the buyer has paid either way.
func TestOrderConfirmWithoutItems(t *testing.T) {
	code := "ABC234"
	html := renderOrder(t, OrderConfirmPage{
		Order: db.Order{SecureToken: "tok_abc", Status: "paid", RedeemCode: &code},
	})
	if !strings.Contains(html, "ABC234") {
		t.Error("order code missing when there are no line items")
	}
}
