package db

import (
	"path/filepath"
	"testing"
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
	if len(all) != 5 {
		t.Fatalf("ListGroups = %d, want 5", len(all))
	}
	if all[0].Slug != "open-draw" {
		t.Errorf("first group = %q, want open-draw", all[0].Slug)
	}

	// Mutual Aid is seeded disabled, so it's absent from the public list and
	// unreachable by slug.
	enabled, err := EnabledGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 4 {
		t.Errorf("EnabledGroups = %d, want 4", len(enabled))
	}
	if _, err := GroupBySlug(conn, "mutual-aid"); err == nil {
		t.Error("disabled group reachable by slug, want ErrNoRows")
	}
	if _, err := GroupBySlug(conn, "open-draw"); err != nil {
		t.Errorf("enabled group not found: %v", err)
	}
}

func plainText(segs []Segment) string {
	var s string
	for _, seg := range segs {
		s += seg.Text
	}
	return s
}
