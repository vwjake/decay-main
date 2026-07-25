package views

import (
	"context"
	"io"
	"testing"
	"time"

	"decay-main/db"
)

func TestEventDetailSignupStates(t *testing.T) {
	ev := db.Event{ID: 1, Title: "Synth Night", EventType: "Music", StartsAt: time.Now().Add(24 * time.Hour), Slug: "synth-night"}
	openRoles := []db.EventVolunteer{{EventID: 1, Role: "door"}, {EventID: 1, Role: "sound"}}

	states := []SignupBox{
		{Show: true}, // form shown
		{Show: true, Error: "Please leave contact."}, // form with error
		{Show: true, Done: true},                     // thank-you
		{Show: false},                                // past event, no form
	}
	for i, s := range states {
		if err := EventDetail(ev, openRoles, s).Render(context.Background(), io.Discard); err != nil {
			t.Fatalf("EventDetail state %d: %v", i, err)
		}
	}
	// Past event with open roles still shows the informational list.
	if err := EventDetail(ev, openRoles, SignupBox{Show: false}).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("EventDetail past: %v", err)
	}
}

func TestAdminEventEditSignups(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	ev := db.Event{ID: 1, Title: "Synth Night", EventType: "Music", StartsAt: time.Now(), Slug: "synth-night"}
	signups := []db.VolunteerSignup{
		{ID: 1, EventID: 1, Role: "door", Name: "Ada", Contact: "ada@x.com", CreatedAt: time.Now()},
		{ID: 2, EventID: 1, Name: "Bo", Contact: "555-1212", Note: "Can lift heavy things", CreatedAt: time.Now()},
	}
	if err := AdminEventEdit(ev, nil, signups, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminEventEdit with signups: %v", err)
	}
	if err := AdminEventEdit(ev, nil, nil, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminEventEdit no signups: %v", err)
	}
}
