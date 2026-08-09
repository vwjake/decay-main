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

	// Admin queue: a plain request and one with notes on file.
	reqs := []db.BookingRequest{
		{ID: 1, Name: "Ada", Email: "ada@x.com", EventName: "Synth Night", Description: "A show.", Status: db.BookingNew, CreatedAt: time.Now()},
		{ID: 2, Name: "Bo", Phone: "555-1212", Status: db.BookingNew, Notes: "Called back, waiting to hear on a date.", CreatedAt: time.Now()},
	}
	if err := AdminBookings(reqs, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminBookings: %v", err)
	}
	if err := AdminBookings(nil, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminBookings empty: %v", err)
	}
}
