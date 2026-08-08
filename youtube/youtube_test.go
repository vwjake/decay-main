package youtube

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sampleFeed is trimmed from a real response for @no_tape, keeping the
// namespaces and nesting that tripped a naive struct up.
const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
 <id>yt:channel:Souk7745aL7zGIrkdkW9Uw</id>
 <yt:channelId>Souk7745aL7zGIrkdkW9Uw</yt:channelId>
 <title>no_tape</title>
 <entry>
  <id>yt:video:i1Ms6uwZezk</id>
  <yt:videoId>i1Ms6uwZezk</yt:videoId>
  <title>Undine Noise at ISM Festival</title>
  <published>2026-08-06T08:20:42+00:00</published>
  <media:group>
   <media:title>Undine Noise at ISM Festival</media:title>
   <media:description>Recorded at DECAY.</media:description>
  </media:group>
 </entry>
 <entry>
  <id>yt:video:LaUGbsoolhU</id>
  <yt:videoId>LaUGbsoolhU</yt:videoId>
  <title>Transparency Band at ISM Festival 2025</title>
  <published>2026-07-30T02:11:00+00:00</published>
  <media:group>
   <media:description></media:description>
  </media:group>
 </entry>
</feed>`

func TestParseFeed(t *testing.T) {
	videos, err := parseFeed([]byte(sampleFeed))
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 {
		t.Fatalf("parsed %d videos, want 2", len(videos))
	}

	first := videos[0]
	if first.ID != "i1Ms6uwZezk" {
		t.Errorf("id = %q", first.ID)
	}
	if first.Title != "Undine Noise at ISM Festival" {
		t.Errorf("title = %q", first.Title)
	}
	if first.Description != "Recorded at DECAY." {
		t.Errorf("description = %q", first.Description)
	}
	if got := first.Published.UTC().Format("2006-01-02"); got != "2026-08-06" {
		t.Errorf("published = %q", got)
	}
	if first.PublishedLabel() != "Aug 6, 2026" {
		t.Errorf("PublishedLabel() = %q", first.PublishedLabel())
	}

	// The order YouTube returns is newest first, and it's kept.
	if !videos[0].Published.After(videos[1].Published) {
		t.Error("videos should stay newest first")
	}

	// Links are the nocookie embed and the ordinary watch page.
	if first.EmbedURL() != "https://www.youtube-nocookie.com/embed/i1Ms6uwZezk" {
		t.Errorf("EmbedURL() = %q", first.EmbedURL())
	}
	if first.WatchURL() != "https://www.youtube.com/watch?v=i1Ms6uwZezk" {
		t.Errorf("WatchURL() = %q", first.WatchURL())
	}
	if !strings.Contains(first.ThumbURL(), "i1Ms6uwZezk") {
		t.Errorf("ThumbURL() = %q", first.ThumbURL())
	}
}

// TestParseFeedSkipsEntriesWithoutID guards against a malformed entry
// becoming a tile that links nowhere.
func TestParseFeedSkipsEntriesWithoutID(t *testing.T) {
	feed := `<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
	 <entry><title>No id here</title></entry>
	 <entry><yt:videoId>abcdefghijk</yt:videoId><title>Fine</title></entry>
	</feed>`
	videos, err := parseFeed([]byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].ID != "abcdefghijk" {
		t.Errorf("got %+v, want just the entry with an id", videos)
	}
}

// TestResolveHandleIgnoresDecoys is the regression test for a real bug: a
// channel page carries plenty of other "UC…" strings — recommended
// channels, sidebar entries — and taking the first one that merely looks
// like an id resolved @no_tape to a stranger's channel, whose feed then
// 404s. Only the canonical /channel/ URL identifies the page's own channel.
func TestResolveHandleIgnoresDecoys(t *testing.T) {
	const page = `<!DOCTYPE html><html><head>
	<script>var junk = {"clientId":"UCzzzzzzzzzzzzzzzzzzzzzz","x":"UCaaaaaaaaaaaaaaaaaaaaaa"};</script>
	<link rel="canonical" href="https://www.youtube.com/channel/UCSouk7745aL7zGIrkdkW9Uw">
	</head><body><a href="/channel/UCbbbbbbbbbbbbbbbbbbbbbb">Some other channel</a></body></html>`

	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		w.Write([]byte(page))
	}))
	defer server.Close()

	c := NewClient("@no_tape")
	c.http = server.Client()
	c.channelHost = server.URL

	id, err := c.resolveID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "UCSouk7745aL7zGIrkdkW9Uw" {
		t.Errorf("resolved %q, want the canonical id UCSouk7745aL7zGIrkdkW9Uw", id)
	}
	if len(asked) != 1 || asked[0] != "/@no_tape" {
		t.Errorf("fetched %v, want just /@no_tape", asked)
	}

	// Resolution is remembered, so it costs one fetch per process.
	if _, err := c.resolveID(); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 {
		t.Errorf("resolved %d times, want 1 — the id should be cached", len(asked))
	}
}

// TestResolveAcceptsBareID: a configured "UC…" needs no lookup at all.
func TestResolveAcceptsBareID(t *testing.T) {
	c := NewClient("UCSouk7745aL7zGIrkdkW9Uw")
	c.channelHost = "http://127.0.0.1:0" // any fetch here would fail
	id, err := c.resolveID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "UCSouk7745aL7zGIrkdkW9Uw" {
		t.Errorf("resolved %q", id)
	}

	// Something that isn't an id must not be mistaken for one.
	for _, bad := range []string{"UCtooshort", "no_tape", "@no_tape", "UCSouk7745aL7zGIrkdkW9Uw-extra"} {
		if channelIDPattern.MatchString(bad) {
			t.Errorf("%q was accepted as a bare channel id", bad)
		}
	}
}

func TestUnconfiguredClient(t *testing.T) {
	c := NewClient("")
	if c.Configured() {
		t.Error("an empty channel should leave the client unconfigured")
	}
	videos, err := c.Recent(5)
	if err != nil || videos != nil {
		t.Errorf("Recent() = %v, %v; want nil, nil", videos, err)
	}
	if ChannelURL("") != "" {
		t.Error("an empty channel should have no URL")
	}
}

func TestChannelURL(t *testing.T) {
	cases := map[string]string{
		"@no_tape":                 "https://www.youtube.com/@no_tape/videos",
		"no_tape":                  "https://www.youtube.com/@no_tape/videos",
		"UCSouk7745aL7zGIrkdkW9Uw": "https://www.youtube.com/channel/UCSouk7745aL7zGIrkdkW9Uw/videos",
	}
	for in, want := range cases {
		if got := ChannelURL(in); got != want {
			t.Errorf("ChannelURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRecentCachesAndClamps checks the client fetches once, reuses the
// result, and honours the limit — a page view shouldn't cost a round trip.
func TestRecentCachesAndClamps(t *testing.T) {
	var hits int
	var agent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		agent = r.Header.Get("User-Agent")
		w.Write([]byte(sampleFeed))
	}))
	defer server.Close()
	defer func() {
		// The site names itself rather than fetching anonymously.
		if !strings.Contains(agent, "DECAY") {
			t.Errorf("User-Agent = %q, want the site named in it", agent)
		}
	}()

	c := &Client{channel: "UCSouk7745aL7zGIrkdkW9Uw", http: server.Client(), channelID: "x"}
	c.feedURL = server.URL

	videos, err := c.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 {
		t.Errorf("Recent(1) returned %d videos", len(videos))
	}
	if _, err := c.Recent(0); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("fetched %d times, want 1 — the second call should come off the cache", hits)
	}

	// A stale cache refetches.
	c.fetched = time.Now().Add(-2 * cacheTTL)
	if _, err := c.Recent(0); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("fetched %d times after the cache went stale, want 2", hits)
	}
}

// TestRetriesOnce covers the flakiness the real endpoint shows: it answers
// 404 or 500 to a good request now and then, and a single retry is what
// keeps that from costing the page its video section for an hour.
func TestRetriesOnce(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		var hits int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits == 1 {
				http.Error(w, "flaky", status)
				return
			}
			w.Write([]byte(sampleFeed))
		}))

		c := &Client{channel: "UCSouk7745aL7zGIrkdkW9Uw", http: server.Client(), channelID: "x", feedURL: server.URL}
		videos, err := c.Recent(0)
		if err != nil {
			t.Errorf("a single %d should have been retried: %v", status, err)
		}
		if len(videos) != 2 {
			t.Errorf("after a retried %d got %d videos, want 2", status, len(videos))
		}
		server.Close()
	}

	// It gives up rather than hammering: a persistently failing endpoint is
	// two attempts, not a loop.
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Client{channel: "UCSouk7745aL7zGIrkdkW9Uw", http: server.Client(), channelID: "x", feedURL: server.URL}
	if _, err := c.Recent(0); err == nil {
		t.Error("a persistently failing endpoint should report an error")
	}
	if hits != retries+1 {
		t.Errorf("made %d attempts, want %d", hits, retries+1)
	}
}

// TestRecentServesStaleOnFailure is the degradation rule: a YouTube
// outage leaves the last good list on the page rather than emptying it.
func TestRecentServesStaleOnFailure(t *testing.T) {
	var fail bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(sampleFeed))
	}))
	defer server.Close()

	c := &Client{channel: "UCSouk7745aL7zGIrkdkW9Uw", http: server.Client(), channelID: "x"}
	c.feedURL = server.URL
	if _, err := c.Recent(0); err != nil {
		t.Fatal(err)
	}

	fail = true
	c.fetched = time.Now().Add(-2 * cacheTTL)
	videos, err := c.Recent(0)
	if err == nil {
		t.Error("a failed refresh should report the error")
	}
	if len(videos) != 2 {
		t.Errorf("got %d videos on a failed refresh, want the 2 cached", len(videos))
	}
}
