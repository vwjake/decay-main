package views

import (
	"context"
	"io"
	"testing"
	"time"

	"decay-main/db"
)

func TestBookingViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}

	// Public form: blank, with an error, and the sent thank-you state.
	if err := BookingForm(db.BookingRequest{}, false, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("BookingForm blank: %v", err)
	}
	if err := BookingForm(db.BookingRequest{Name: "Ada"}, false, "Please leave contact.").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("BookingForm error: %v", err)
	}
	if err := BookingForm(db.BookingRequest{}, true, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("BookingForm sent: %v", err)
	}

	// Admin queue: a new, a reviewed, and an archived request.
	reqs := []db.BookingRequest{
		{ID: 1, Name: "Ada", Email: "ada@x.com", EventName: "Synth Night", Description: "A show.", Status: db.BookingNew, CreatedAt: time.Now()},
		{ID: 2, Name: "Bo", Phone: "555-1212", Status: db.BookingReviewed, CreatedAt: time.Now()},
		{ID: 3, Name: "Cy", Status: db.BookingArchived, CreatedAt: time.Now()},
	}
	if err := AdminBookings(reqs, false, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminBookings: %v", err)
	}
	if err := AdminBookings(nil, true, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminBookings empty: %v", err)
	}
}
