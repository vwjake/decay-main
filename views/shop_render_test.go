package views

import (
	"context"
	"io"
	"testing"

	"decay-main/db"
)

func TestShopViewRenders(t *testing.T) {
	products := []db.Product{
		{ID: 1, Name: "Logo Tee", PriceCents: 3000, Image: "t.png",
			Variants: "S, M, L", Description: "Soft cotton.", Position: 1},
		{ID: 2, Name: "Bandana", PriceCents: 1000, Placeholder: "product photo",
			SoldOut: true, Position: 2},
	}
	if err := Shop(products).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Shop render: %v", err)
	}
	// Empty catalogue still renders.
	if err := Shop(nil).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Shop(nil) render: %v", err)
	}
	// Home reuses merchCard, so render it with the same mix.
	if err := Home(nil, products).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Home render: %v", err)
	}
}
