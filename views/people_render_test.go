package views

import (
	"context"
	"io"
	"testing"

	"decay-main/db"
)

// TestPeopleViewsRender exercises the About page and the admin people
// templates with a photo-less and a fully-filled profile, catching runtime
// template failures a type-check wouldn't.
func TestPeopleViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	people := []db.Person{
		{ID: 1, Name: "Moon Fery", Role: "Board of Directors"}, // no photo, no bio
		{ID: 2, Name: "Sam Staff", Pronouns: "they/them", Role: "Coordinator",
			Bio: "First paragraph.\n\nSecond paragraph.", Photo: "1.jpg", Position: 10},
	}

	if err := About(people).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("About render: %v", err)
	}
	// About must also render with no people at all.
	if err := About(nil).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("About(nil) render: %v", err)
	}
	bios := []db.CommunityBio{
		{ID: 1, Name: "Riley Regular", Role: "DJ", Bio: "Spins on Fridays.", Public: true},
	}
	if err := AdminPeople(people, bios, me, "", "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminPeople render: %v", err)
	}
	if err := AdminPersonEdit(people[1], me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminPersonEdit render: %v", err)
	}
}

// TestInitials covers the portrait placeholder text.
func TestInitials(t *testing.T) {
	cases := map[string]string{
		"Moon Fery":     "MF",
		"Abe":           "A",
		"Robin A. Diaz": "RA",
		"":              "",
	}
	for in, want := range cases {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}
