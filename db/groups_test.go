package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseBody(t *testing.T) {
	body := `# What to bring
- Sketchbook
- Pens and markers

# New here?
Come solo or bring a friend.
Join the Discord: https://discord.gg/abc123 today`

	blocks := ParseBody(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}

	if blocks[0].Heading != "What to bring" || len(blocks[0].Bullets) != 2 {
		t.Errorf("block 0 = %+v", blocks[0])
	}
	if got := plainText(blocks[0].Bullets[0]); got != "Sketchbook" {
		t.Errorf("first bullet = %q", got)
	}

	if blocks[1].Heading != "New here?" || len(blocks[1].Paragraphs) != 2 {
		t.Errorf("block 1 = %+v", blocks[1])
	}

	// The URL in the second paragraph should become a link segment, and the
	// trailing " today" should stay plain text outside it.
	var linked *Segment
	for i := range blocks[1].Paragraphs[1] {
		if blocks[1].Paragraphs[1][i].IsLink() {
			linked = &blocks[1].Paragraphs[1][i]
		}
	}
	if linked == nil || linked.URL != "https://discord.gg/abc123" {
		t.Fatalf("expected a discord link segment, got %+v", blocks[1].Paragraphs[1])
	}
}

func TestParseBodyTrailingPunctuation(t *testing.T) {
	// A URL followed by a period keeps the period out of the link target.
	segs := parseSegments("See https://decay.events. Thanks")
	var link Segment
	for _, s := range segs {
		if s.IsLink() {
			link = s
		}
	}
	if link.URL != "https://decay.events" {
		t.Errorf("link URL = %q, want without the period", link.URL)
	}
}

func TestSeedAndQueryGroups(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := Seed(conn); err != nil {
		t.Fatal(err)
	}

	all, err := ListGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("ListGroups = %d, want 6", len(all))
	}
	if all[0].Slug != "film" {
		t.Errorf("first group = %q, want film", all[0].Slug)
	}

	// All six categories are seeded enabled.
	enabled, err := EnabledGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 6 {
		t.Errorf("EnabledGroups = %d, want 6", len(enabled))
	}
	if _, err := GroupBySlug(conn, "community"); err != nil {
		t.Errorf("enabled group not found: %v", err)
	}
}

func TestMigrateGroupsToCategories(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Simulate a database still on the old five named groups, as if it
	// predates the switch to fixed categories: wipe the seeded categories
	// and insert rows under the old slugs instead.
	if _, err := conn.Exec(`DELETE FROM groups`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(
		`INSERT INTO groups (slug, name, match_terms, position, enabled) VALUES (?, ?, ?, ?, ?)`,
		"open-draw", "Open Draw", "Open Draw", 0, true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(
		`INSERT INTO groups (slug, name, match_terms, position, enabled) VALUES (?, ?, ?, ?, ?)`,
		"mutual-aid", "Mutual Aid", "Mutual Aid", 1, false,
	); err != nil {
		t.Fatal(err)
	}

	if err := migrateGroupsToCategories(conn); err != nil {
		t.Fatal(err)
	}

	// The renamed rows keep the same id (and so any hero image or tagged
	// photos), just under the new slug and content.
	visualArt, err := GroupBySlug(conn, "visual-art")
	if err != nil {
		t.Fatalf("open-draw did not become visual-art: %v", err)
	}
	if visualArt.Name != "Visual Art" {
		t.Errorf("visual-art name = %q", visualArt.Name)
	}
	if _, err := GroupByID(conn, visualArt.ID); err != nil {
		t.Fatal(err)
	}

	// Mutual Aid folds into Community and comes back enabled, since the
	// category is now a top-level page rather than an unlisted group.
	community, err := GroupBySlug(conn, "community")
	if err != nil {
		t.Fatalf("mutual-aid did not become community: %v", err)
	}
	if !community.Enabled {
		t.Error("community should be enabled after migrating from mutual-aid")
	}

	// Live Performances has no predecessor, so it's inserted outright.
	if _, err := GroupBySlug(conn, "live-performances"); err != nil {
		t.Errorf("live-performances not created: %v", err)
	}

	all, err := ListGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ListGroups = %d, want 3 (visual-art, community, live-performances)", len(all))
	}

	// Running the migration again is a no-op — the old slugs are gone, so
	// nothing matches the WHERE clause a second time.
	if err := migrateGroupsToCategories(conn); err != nil {
		t.Fatal(err)
	}
	again, err := ListGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("second migration changed row count: %d", len(again))
	}
}

func TestUpcomingForGroup(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tz data")
	}
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// A group with terms, but no seeded event archive, so the only matches
	// are the events this test creates.
	if _, err := CreateGroup(conn, Group{Slug: "no-tape", Name: "No Tape", MatchTerms: "No Tape", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	future := time.Now().In(loc).Add(48 * time.Hour)
	// An underscore title must still match the "No Tape" term, and a term
	// matching the event type (not the title) must count too.
	mk := func(title, etype string) {
		if _, err := CreateEvent(conn, Event{Title: title, EventType: etype, StartsAt: future}); err != nil {
			t.Fatal(err)
		}
	}
	mk("NO_TAPE", "Music")       // matches no-tape by normalized title
	mk("Some Jam", "no_tape")    // matches no-tape by type
	mk("Open Draw", "Drawing")   // matches open-draw
	mk("Random Lecture", "Talk") // matches nothing
	// A past event that matches should be excluded as not upcoming.
	if _, err := CreateEvent(conn, Event{Title: "Old NO_TAPE", EventType: "Music", StartsAt: time.Now().Add(-72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	noTape, err := GroupBySlug(conn, "no-tape")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UpcomingForGroup(conn, noTape, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("no-tape upcoming = %d, want 2 (%v)", len(got), titles(got))
	}

	// The limit is honoured.
	limited, err := UpcomingForGroup(conn, noTape, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Errorf("limit 1 returned %d", len(limited))
	}
}

func titles(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Title)
	}
	return out
}

func plainText(segs []Segment) string {
	var s string
	for _, seg := range segs {
		s += seg.Text
	}
	return s
}
