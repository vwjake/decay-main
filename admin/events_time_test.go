package admin

import (
	"testing"
	_ "time/tzdata"
)

// TestParseAdminTimeAcrossDST covers the venue timezone being resolved
// against each event's own date. Pinning a single offset stored winter
// events an hour off, and that error reached the calendar feed.
func TestParseAdminTimeAcrossDST(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // RFC3339 in the venue's zone
	}{
		{"summer is PDT", "2026-07-25T16:00", "2026-07-25T16:00:00-07:00"},
		{"winter is PST", "2026-12-29T20:30", "2026-12-29T20:30:00-08:00"},
		{"day before spring forward", "2026-03-07T12:00", "2026-03-07T12:00:00-08:00"},
		{"day after spring forward", "2026-03-09T12:00", "2026-03-09T12:00:00-07:00"},
		{"day after fall back", "2026-11-02T12:00", "2026-11-02T12:00:00-08:00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAdminTime(tc.input)
			if err != nil {
				t.Fatalf("parseAdminTime(%q): %v", tc.input, err)
			}
			if formatted := got.Format("2006-01-02T15:04:05-07:00"); formatted != tc.want {
				t.Errorf("parseAdminTime(%q) = %s, want %s", tc.input, formatted, tc.want)
			}
		})
	}
}

// TestParseAdminTimeRejectsGarbage makes sure a malformed value surfaces
// as an error the handler can turn into a message, not a zero time.
func TestParseAdminTimeRejectsGarbage(t *testing.T) {
	for _, input := range []string{"", "not-a-time", "2026-13-45T99:99"} {
		if _, err := parseAdminTime(input); err == nil {
			t.Errorf("parseAdminTime(%q) succeeded, want an error", input)
		}
	}
}
