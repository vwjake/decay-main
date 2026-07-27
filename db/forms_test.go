package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBookingRequests(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := CreateBookingRequest(conn, BookingRequest{Name: "Ada", Email: "ada@x.com", EventName: "Synth Night"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateBookingRequest(conn, BookingRequest{Name: "Bo", EventName: "Zine Fair"}); err != nil {
		t.Fatal(err)
	}

	n, err := CountNewBookings(conn)
	if err != nil || n != 2 {
		t.Fatalf("CountNewBookings = %d (%v), want 2", n, err)
	}

	list, err := ListBookingRequests(conn, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || !list[0].IsNew() {
		t.Fatalf("list = %+v", list)
	}

	// Archiving drops it from the default list and the new count.
	if err := SetBookingStatus(conn, list[0].ID, BookingArchived); err != nil {
		t.Fatal(err)
	}
	list, _ = ListBookingRequests(conn, false)
	if len(list) != 1 {
		t.Errorf("after archive, active list = %d, want 1", len(list))
	}
	withArchived, _ := ListBookingRequests(conn, true)
	if len(withArchived) != 2 {
		t.Errorf("with archived = %d, want 2", len(withArchived))
	}
	if n, _ := CountNewBookings(conn); n != 1 {
		t.Errorf("new count after archive = %d, want 1", n)
	}
}

func TestContactMessages(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := CreateContactMessage(conn, ContactMessage{Name: "Ada", Email: "ada@x.com", Subject: "Hi", Message: "hello there"}); err != nil {
		t.Fatal(err)
	}
	// A blank subject falls back to a stand-in rather than showing empty.
	if err := CreateContactMessage(conn, ContactMessage{Name: "Bo", Email: "bo@x.com", Message: "no subject here"}); err != nil {
		t.Fatal(err)
	}

	n, err := CountNewMessages(conn)
	if err != nil || n != 2 {
		t.Fatalf("CountNewMessages = %d (%v), want 2", n, err)
	}

	list, err := ListContactMessages(conn, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || !list[0].IsNew() {
		t.Fatalf("list = %+v", list)
	}
	// Newest first: Bo was created last.
	if list[0].Name != "Bo" || list[0].SubjectOr() != "General inquiry" {
		t.Errorf("first message = %+v, want Bo with fallback subject", list[0])
	}

	// Archiving drops it from the default list and the new count.
	if err := SetContactStatus(conn, list[0].ID, ContactArchived); err != nil {
		t.Fatal(err)
	}
	active, _ := ListContactMessages(conn, false)
	if len(active) != 1 {
		t.Errorf("after archive, active list = %d, want 1", len(active))
	}
	withArchived, _ := ListContactMessages(conn, true)
	if len(withArchived) != 2 {
		t.Errorf("with archived = %d, want 2", len(withArchived))
	}
	if n, _ := CountNewMessages(conn); n != 1 {
		t.Errorf("new count after archive = %d, want 1", n)
	}

	// Delete removes it for good.
	if err := DeleteContactMessage(conn, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if all, _ := ListContactMessages(conn, true); len(all) != 1 {
		t.Errorf("after delete, %d remain, want 1", len(all))
	}
}

func TestVolunteerSignups(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	future := time.Now().Add(48 * time.Hour)
	past := time.Now().Add(-48 * time.Hour)
	upcomingID, err := CreateEvent(conn, Event{Title: "Show", EventType: "Music", StartsAt: future})
	if err != nil {
		t.Fatal(err)
	}
	pastID, err := CreateEvent(conn, Event{Title: "Old Show", EventType: "Music", StartsAt: past})
	if err != nil {
		t.Fatal(err)
	}

	if err := CreateVolunteerSignup(conn, VolunteerSignup{EventID: upcomingID, Role: "door", Name: "Ada", Contact: "ada@x.com"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateVolunteerSignup(conn, VolunteerSignup{EventID: upcomingID, Name: "Bo", Contact: "555-1212"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateVolunteerSignup(conn, VolunteerSignup{EventID: pastID, Name: "Cy", Contact: "cy@x.com"}); err != nil {
		t.Fatal(err)
	}

	got, err := SignupsForEvent(conn, upcomingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("SignupsForEvent = %d, want 2", len(got))
	}
	// A signup with no role reads as the general label.
	var general VolunteerSignup
	for _, s := range got {
		if s.Name == "Bo" {
			general = s
		}
	}
	if general.RoleLabel() != "Wherever needed" {
		t.Errorf("no-role label = %q", general.RoleLabel())
	}

	// The dashboard count only sees offers on upcoming events.
	if n, _ := CountSignupsForUpcoming(conn); n != 2 {
		t.Errorf("CountSignupsForUpcoming = %d, want 2", n)
	}

	// Clearing a single signup (the admin "Clear" action) removes just it.
	if err := DeleteVolunteerSignup(conn, got[0].ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := SignupsForEvent(conn, upcomingID); len(after) != 1 {
		t.Errorf("after clearing one, %d remain, want 1", len(after))
	}

	// Deleting the event cascades its remaining signups away.
	if err := DeleteEvent(conn, upcomingID); err != nil {
		t.Fatal(err)
	}
	if got, _ := SignupsForEvent(conn, upcomingID); len(got) != 0 {
		t.Errorf("signups survived event delete: %d", len(got))
	}
}
