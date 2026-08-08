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
		Photos:     []db.Photo{{ID: 1, Filename: "1.jpg", Caption: "Open Draw"}},
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
		"Aug 6, 2026",                                 // upload date
		"Open Draw",                                   // photo caption
		"https://www.youtube.com/@no_tape/videos",     // channel link
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// Recent uploads must not carry the gallery classes: the lightbox binds
	// to the first .gallery on the page and these links leave the site, so
	// sharing the class would hijack the photo viewer.
	grid := out[strings.Index(out, "Recent uploads"):strings.Index(out, "Photos</span>")]
	if strings.Contains(grid, `class="gallery"`) || strings.Contains(grid, "gallery-item") {
		t.Error("the video grid uses the photo gallery's classes — that breaks the lightbox")
	}
	// Photos still do.
	if !strings.Contains(out, `class="gallery"`) {
		t.Error("the photo gallery lost its class")
	}
	// The gallery has to come after the videos for the lightbox to bind to
	// photos rather than to video thumbnails.
	if strings.Index(out, "video-grid") > strings.Index(out, `class="gallery"`) {
		t.Error("the photo gallery should come after the video grid")
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
