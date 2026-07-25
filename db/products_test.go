package db

import (
	"path/filepath"
	"testing"
)

// TestProductsSoldOutAndOrder covers the shop additions: manual ordering,
// the sold-out flag surviving a round-trip, and AvailableProducts dropping
// sold-out items.
func TestProductsSoldOutAndOrder(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Insert out of position order to prove ListProducts sorts by position.
	if err := CreateProduct(conn, Product{Name: "Second", PriceCents: 2000, Position: 2}); err != nil {
		t.Fatal(err)
	}
	if err := CreateProduct(conn, Product{Name: "First", PriceCents: 1000, Position: 1, Description: "Comes first."}); err != nil {
		t.Fatal(err)
	}

	list, err := ListProducts(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "First" {
		t.Fatalf("order wrong: %+v", list)
	}
	if list[0].Description != "Comes first." {
		t.Errorf("description not stored: %q", list[0].Description)
	}

	// Mark the first sold out and confirm it round-trips and is filtered.
	p := list[0]
	p.SoldOut = true
	if err := UpdateProduct(conn, p); err != nil {
		t.Fatal(err)
	}
	got, err := ProductByID(conn, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SoldOut {
		t.Error("SoldOut did not persist")
	}

	avail, err := AvailableProducts(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(avail) != 1 || avail[0].Name != "Second" {
		t.Errorf("AvailableProducts = %+v, want just Second", avail)
	}
}
