package embed

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchYouTube(t *testing.T) {
	for _, raw := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"dQw4w9WgXcQ",
	} {
		kind, src, ok := Match(raw)
		if !ok || kind != YouTube {
			t.Errorf("Match(%q) = (%v, %q, %v), want a YouTube match", raw, kind, src, ok)
		}
		if !strings.Contains(src, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
			t.Errorf("Match(%q) src = %q, want nocookie embed", raw, src)
		}
	}
}

func TestMatchBandcampOnlyEmbedURL(t *testing.T) {
	// A player URL embeds; a normal album page does not (it needs Resolve).
	if _, src, ok := Match("https://bandcamp.com/EmbeddedPlayer/album=123/size=large/"); !ok || !strings.HasPrefix(src, "https://bandcamp.com/EmbeddedPlayer/") {
		t.Errorf("player URL should match, got src=%q ok=%v", src, ok)
	}
	if _, _, ok := Match("https://artist.bandcamp.com/album/foo"); ok {
		t.Error("a normal Bandcamp album page should not Match on its own")
	}
}

func TestMatchRejectsOther(t *testing.T) {
	for _, raw := range []string{
		"https://example.com/watch?v=dQw4w9WgXcQ",
		"https://vimeo.com/12345",
		"not a url",
		"",
	} {
		if _, _, ok := Match(raw); ok {
			t.Errorf("Match(%q) should not match", raw)
		}
	}
}

func TestResolveYouTubeOffline(t *testing.T) {
	got, err := Resolve("https://youtu.be/dQw4w9WgXcQ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Errorf("Resolve = %q", got)
	}
	// And that resolved URL round-trips through Match.
	if _, _, ok := Match(got); !ok {
		t.Error("resolved YouTube URL doesn't Match")
	}
}

func TestResolveRejectsNonBandcampHost(t *testing.T) {
	// Resolve must not fetch arbitrary hosts — only bandcamp.com pages.
	if _, err := Resolve("https://evil.example.com/album/foo"); err == nil {
		t.Error("Resolve should reject a non-YouTube, non-Bandcamp URL without fetching")
	}
}

func TestResolveBandcampPage(t *testing.T) {
	// A stand-in Bandcamp page whose og:video points at a player URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head>
			<meta content="https://bandcamp.com/EmbeddedPlayer/album=42/size=large/" property="og:video">
			</head><body>album</body></html>`))
	}))
	defer srv.Close()

	// resolveBandcampPage is what Resolve calls once the host check passes;
	// call it directly since the test server isn't on a bandcamp.com host.
	got, err := resolveBandcampPage(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://bandcamp.com/EmbeddedPlayer/album=42/size=large/" {
		t.Errorf("resolved player = %q", got)
	}
	if _, _, ok := Match(got); !ok {
		t.Error("resolved Bandcamp player doesn't Match")
	}
}

func TestOgVideoContentAttrOrder(t *testing.T) {
	// content-before-property must still be found.
	body := []byte(`<meta content="https://bandcamp.com/EmbeddedPlayer/track=9/" property="og:video:secure_url"/>`)
	got, ok := ogVideoContent(body)
	if !ok || got != "https://bandcamp.com/EmbeddedPlayer/track=9/" {
		t.Errorf("ogVideoContent = %q, %v", got, ok)
	}
}
