package views

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"decay-main/db"
	"decay-main/youtube"
)

func mediaFixture() MediaPage {
	return MediaPage{
		Featured: []db.Video{{ID: 1, YouTubeID: "aaaaaaaaaaa", Title: "No Tape 001"}},
		Recent: []youtube.Video{{
			ID: "bbbbbbbbbbb", Title: "Undine Noise at ISM Festival",
			Published: time.Date(2026, 8, 6, 8, 20, 0, 0, time.UTC),
		}},
		Photos: []db.Photo{
			{ID: 1, Filename: "1.jpg", Caption: "Open Draw"},
			{ID: 2, Filename: "2.jpg", Caption: "No Tape"},
		},
		ChannelURL: "https://www.youtube.com/@no_tape/videos",
	}
}

// TestMediaViewRenders exercises /media in each combination of what it can
// have, catching runtime template failures a type-check wouldn't.
func TestMediaViewRenders(t *testing.T) {
	full := mediaFixture()

	var buf strings.Builder
	if err := Media(full).Render(context.Background(), &buf); err != nil {
		t.Fatalf("full render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"No Tape 001",                                 // featured video
		"youtube-nocookie.com/embed/aaaaaaaaaaa",      // embedded, privacy-preserving
		"Undine Noise at ISM Festival",                // recent upload
		"https://www.youtube.com/watch?v=bbbbbbbbbbb", // links out
		"i.ytimg.com/vi/bbbbbbbbbbb/mqdefault.jpg",    // recent upload thumbnail
		"Aug 6, 2026",                                 // upload date
		"Open Draw",                                   // photo caption
		"https://www.youtube.com/@no_tape/videos",     // channel link
		"Recent video uploads",                        // renamed section label
		"Check out the gallery",                       // multi-photo teaser button
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
	if strings.Contains(out, "instagram") {
		t.Error("the Instagram button should be gone from /media")
	}

	// Recent uploads must not carry the gallery classes: the lightbox binds
	// to the first .gallery on the page and these links leave the site, so
	// sharing the class would hijack the photo viewer.
	grid := out[strings.Index(out, "Recent video uploads"):]
	if strings.Contains(grid, `class="gallery"`) || strings.Contains(grid, "gallery-item") {
		t.Error("the video grid uses the photo gallery's classes — that breaks the lightbox")
	}
	// Photos still do.
	if !strings.Contains(out, `class="gallery"`) {
		t.Error("the photo gallery lost its class")
	}
	// Photos now lead the page, with the video sections following.
	if strings.Index(out, `class="gallery"`) > strings.Index(out, "video-grid") {
		t.Error("the photo gallery should come before the video grid now that photos lead the page")
	}

	// Only the first photo is shown; the rest stay in the DOM (for the
	// lightbox's prev/next) but hidden until "Check out the gallery" opens it.
	figures := strings.Split(out[strings.Index(out, `class="gallery"`):], "<figure")
	if len(figures) < 3 {
		t.Fatalf("expected 2 gallery figures, got %d", len(figures)-1)
	}
	if strings.Contains(figures[1], "hidden") {
		t.Error("the first photo should be visible, not hidden")
	}
	if !strings.Contains(figures[2], "hidden") {
		t.Error("the second photo should be hidden until the gallery is opened")
	}

	// Each section is optional, and an empty page still says something.
	cases := map[string]MediaPage{
		"no videos at all": {Photos: full.Photos},
		"no photos":        {Featured: full.Featured, Recent: full.Recent},
		"feed unavailable": {Featured: full.Featured, Photos: full.Photos},
		"nothing":          {},
	}
	for name, page := range cases {
		if err := Media(page).Render(context.Background(), io.Discard); err != nil {
			t.Errorf("%s render: %v", name, err)
		}
	}

	buf.Reset()
	if err := Media(MediaPage{}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No photos yet") {
		t.Error("an empty page should still explain itself")
	}
}
