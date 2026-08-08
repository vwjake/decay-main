package views

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"decay-main/db"
)

func TestShopViewRenders(t *testing.T) {
	products := []db.Product{
		{ID: 1, Name: "Logo Tee", PriceCents: 3000, Image: "t.png",
			Variants: "S, M, L", Description: "Soft cotton.", Position: 1, StripePriceID: "price_tee"},
		{ID: 2, Name: "Bandana", PriceCents: 1000, Placeholder: "product photo",
			SoldOut: true, Position: 2},
	}
	if err := Shop(products, true, false).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Shop render: %v", err)
	}
	// Empty catalogue still renders.
	if err := Shop(nil, true, false).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Shop(nil) render: %v", err)
	}
	// Home reuses merchCard, so render it with the same mix.
	if err := Home(nil, products, nil, nil, true).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Home render: %v", err)
	}
}

// TestShopCheckoutWiring covers where a buy click actually goes. A
// Stripe-synced item checks out right on this site once Stripe is
// configured; a local-only item (no Stripe price) keeps using its manually
// set buy link either way; and an item with neither renders as
// unpurchasable rather than linking anywhere — shop.decay.events is being
// retired, so there's no external site left to fall back to.
func TestShopCheckoutWiring(t *testing.T) {
	synced := db.Product{ID: 1, Name: "Logo Tee", PriceCents: 3000, StripePriceID: "price_tee"}
	localOnly := db.Product{ID: 2, Name: "Zine", PriceCents: 500, StripeURL: "https://buy.stripe.com/zine"}
	notYetLinked := db.Product{ID: 3, Name: "Sticker Pack", PriceCents: 500}
	products := []db.Product{synced, localOnly, notYetLinked}

	var live strings.Builder
	if err := Shop(products, true, false).Render(context.Background(), &live); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(live.String(), `action="/shop/checkout"`) {
		t.Error("Stripe-synced item doesn't post to /shop/checkout")
	}
	if !strings.Contains(live.String(), `name="product_id" value="1"`) {
		t.Error("checkout form doesn't carry the product id")
	}
	if !strings.Contains(live.String(), "https://buy.stripe.com/zine") {
		t.Error("local-only item lost its manual buy link")
	}
	if !strings.Contains(live.String(), `class="merch-item is-unavailable"`) {
		t.Error("item with no checkout path doesn't render as unavailable")
	}

	var unconfigured strings.Builder
	if err := Shop(products, false, false).Render(context.Background(), &unconfigured); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(unconfigured.String(), `action="/shop/checkout"`) {
		t.Error("checkout form rendered with Stripe unconfigured")
	}
	if !strings.Contains(unconfigured.String(), "https://buy.stripe.com/zine") {
		t.Error("local-only item lost its manual buy link when Stripe is unconfigured")
	}

	// shop.decay.events is retired — nothing should ever point there again,
	// configured or not.
	for _, rendered := range []string{live.String(), unconfigured.String()} {
		if strings.Contains(rendered, "shop.decay.events") {
			t.Error("shop.decay.events still referenced")
		}
	}

	var errored strings.Builder
	if err := Shop(products, true, true).Render(context.Background(), &errored); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(errored.String(), "admin-flash-error") {
		t.Error("checkout error flag didn't render a flash")
	}
}

// TestAdminProductsSyncButton covers the sync control. It's always on the
// page — hiding it when Stripe is unconfigured just reads as a missing
// feature — but it's disabled until there's a key to sync with.
func TestAdminProductsSyncButton(t *testing.T) {
	me := db.User{ID: 1, Username: "admin", Role: db.RoleMaster}
	products := []db.Product{
		{ID: 1, Name: "Logo Tee", PriceCents: 3000, StripeProductID: "prod_abc"},
		{ID: 2, Name: "Zine", PriceCents: 500},
	}

	for _, tc := range []struct {
		name         string
		stripeReady  bool
		wantDisabled bool
	}{
		{"stripe configured", true, false},
		{"stripe absent", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			if err := AdminProducts(products, me, "", "", tc.stripeReady).Render(context.Background(), &b); err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(b.String(), "/admin/products/sync") {
				t.Error("sync form missing")
			}
			if got := strings.Contains(b.String(), "disabled"); got != tc.wantDisabled {
				t.Errorf("sync button disabled = %v, want %v", got, tc.wantDisabled)
			}
		})
	}

	// Items are created in Stripe, so the page must not offer a create form.
	var list strings.Builder
	if err := AdminProducts(products, me, "", "", true).Render(context.Background(), &list); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(list.String(), `action="/admin/products"`) {
		t.Error("add-product form still on the page")
	}
	// Every row needs a way through to its edit page.
	for _, p := range products {
		want := fmt.Sprintf("/admin/products/%d", p.ID)
		if !strings.Contains(list.String(), want) {
			t.Errorf("no edit link for product %d", p.ID)
		}
	}

	// The two flashes are distinct: a sync summary must not be dressed up as
	// an error, and vice versa.
	var b strings.Builder
	if err := AdminProducts(products, me, "", "Synced from Stripe: 1 added.", true).Render(context.Background(), &b); err != nil {
		t.Fatalf("render with notice: %v", err)
	}
	if strings.Contains(b.String(), "admin-flash-error") {
		t.Error("sync summary rendered with the error style")
	}
	// A Stripe-backed row is marked as such so it's clear which items an
	// admin's edits here would be overwritten on the next sync. Match the
	// marker itself — a bare "stripe" also hits the add form's field names.
	if !strings.Contains(b.String(), "· stripe") {
		t.Error("Stripe-synced product not flagged in the list")
	}
	// The local-only row must not be flagged, or the marker says nothing.
	if strings.Count(b.String(), "· stripe") != 1 {
		t.Errorf("want exactly one Stripe marker for two products, got %d",
			strings.Count(b.String(), "· stripe"))
	}
}
