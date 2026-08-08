// Package youtube reads a channel's recent uploads from YouTube's public
// Atom feed. It's the same arrangement as the staff calendar: a read-only
// feed, no API key, no credentials, cached for a while and degrading to
// the last good copy when the fetch fails — a video list is not worth
// failing a page over.
//
// The Data API would give more (durations, view counts, playlists) but
// needs a key to be issued, stored, and rotated. The feed gives the last
// 15 uploads for nothing, which is what "recent videos" needs.
package youtube

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// cacheTTL is how long a fetched feed is reused. Uploads are occasional,
// so an hour of staleness costs nothing and keeps the site off YouTube's
// doorstep on every page view.
const cacheTTL = time.Hour

// fetchTimeout caps how long a page load waits on YouTube before giving up
// and showing the last good snapshot (or nothing).
const fetchTimeout = 5 * time.Second

// userAgent identifies the site to YouTube. Not required — the feed serves
// a request without one — but a named agent is the courtesy owed to a free
// endpoint being polled, and it's what shows up in their logs if this ever
// misbehaves.
const userAgent = "Mozilla/5.0 (compatible; DECAY-site/1.0; +https://decay.events)"

// retries is one extra attempt on a failed fetch. The endpoint answers 404
// or 500 to a perfectly good request every so often — observed across
// several user agents, including none — and a single retry turns most of
// those into the page rendering normally rather than dropping the section
// for an hour until the cache expires.
const retries = 1

// retryDelay spaces the second attempt. Short: a visitor is waiting.
const retryDelay = 300 * time.Millisecond

// DefaultChannel is DECAY's own channel. The site already links to it by
// hand from the home page, so it's the sensible default rather than
// something every deployment has to be told.
const DefaultChannel = "@no_tape"

// ChannelURL is the public page for a configured channel — the videos tab,
// since that's what someone following the link off /media wants. Works for
// either form the config takes.
func ChannelURL(channel string) string {
	channel = strings.TrimSpace(channel)
	switch {
	case channel == "":
		return ""
	case strings.HasPrefix(channel, "UC"):
		return "https://www.youtube.com/channel/" + channel + "/videos"
	case strings.HasPrefix(channel, "@"):
		return "https://www.youtube.com/" + channel + "/videos"
	}
	return "https://www.youtube.com/@" + channel + "/videos"
}

// Video is one upload.
type Video struct {
	ID          string
	Title       string
	Description string
	Published   time.Time
}

// WatchURL links out to the video on YouTube.
func (v Video) WatchURL() string { return "https://www.youtube.com/watch?v=" + v.ID }

// EmbedURL is the privacy-preserving embed URL — youtube-nocookie doesn't
// set tracking cookies until the viewer actually plays the video.
func (v Video) EmbedURL() string { return "https://www.youtube-nocookie.com/embed/" + v.ID }

// ThumbURL is the still used in the grid, so a page of videos costs one
// image each rather than one player each.
func (v Video) ThumbURL() string { return "https://i.ytimg.com/vi/" + v.ID + "/mqdefault.jpg" }

// PublishedLabel renders the upload date for display.
func (v Video) PublishedLabel() string {
	if v.Published.IsZero() {
		return ""
	}
	return v.Published.Format("Jan 2, 2006")
}

// Client fetches and caches one channel's uploads. It's safe for
// concurrent use. An empty channel means the feature is switched off,
// which Configured reports so a page can leave the section out entirely
// rather than showing an empty shelf.
type Client struct {
	channel string
	http    *http.Client
	// feedURL and channelHost override the endpoints. Empty in production,
	// where they're the real YouTube URLs; the tests point them at a local
	// server.
	feedURL     string
	channelHost string

	mu        sync.Mutex
	channelID string
	cached    []Video
	fetched   time.Time
}

// NewClient builds a client for a channel, given either a bare channel id
// ("UC…") or a handle ("@no_tape"). A handle is resolved to an id on first
// use and remembered. An empty channel leaves the feature unconfigured.
func NewClient(channel string) *Client {
	return &Client{
		channel: strings.TrimSpace(channel),
		http:    &http.Client{Timeout: fetchTimeout},
	}
}

// Configured reports whether a channel was set.
func (c *Client) Configured() bool { return c.channel != "" }

// Recent returns up to limit of the channel's latest uploads, fetching
// when the cache is empty or stale. A failed refresh returns the last good
// snapshot alongside the error, so a YouTube outage shows slightly old
// videos rather than an empty page.
func (c *Client) Recent(limit int) ([]Video, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.Configured() {
		return nil, nil
	}
	if c.cached != nil && time.Since(c.fetched) < cacheTTL {
		return clamp(c.cached, limit), nil
	}

	fresh, err := c.fetch()
	if err != nil {
		return clamp(c.cached, limit), err // cached may be nil on a first-load failure
	}
	c.cached = fresh
	c.fetched = time.Now()
	return clamp(fresh, limit), nil
}

// Warm fills the cache off the request path, so the first visitor after a
// restart doesn't wait on YouTube.
func (c *Client) Warm() error {
	_, err := c.Recent(0)
	return err
}

func clamp(videos []Video, limit int) []Video {
	if limit > 0 && len(videos) > limit {
		return videos[:limit]
	}
	return videos
}

func (c *Client) fetch() ([]Video, error) {
	url := c.feedURL
	if url == "" {
		id, err := c.resolveID()
		if err != nil {
			return nil, err
		}
		url = "https://www.youtube.com/feeds/videos.xml?channel_id=" + id
	}
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	return parseFeed(body)
}

// channelIDPattern matches a bare channel id, for validating what's been
// configured.
var channelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)

// canonicalPattern pulls the channel id out of a channel page. It has to
// anchor on the /channel/ URL: a channel page is full of other "UC…"
// strings — recommended channels, the sidebar, embedded thumbnails — and
// taking the first one that merely looks like an id yields somebody else's
// channel, which then 404s as a feed.
var canonicalPattern = regexp.MustCompile(`youtube\.com/channel/(UC[A-Za-z0-9_-]{22})`)

// resolveID turns whatever was configured into a channel id. A configured
// id is used as-is; a handle costs one extra fetch of the channel page,
// whose canonical link carries the id. The result is remembered, so that
// fetch happens once per process, not once per refresh.
func (c *Client) resolveID() (string, error) {
	if c.channelID != "" {
		return c.channelID, nil
	}
	if channelIDPattern.MatchString(c.channel) {
		c.channelID = c.channel
		return c.channelID, nil
	}

	handle := c.channel
	if !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	body, err := c.get(c.handleURL(handle))
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", handle, err)
	}
	m := canonicalPattern.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("no channel id on the page for %s", handle)
	}
	c.channelID = string(m[1])
	return c.channelID, nil
}

// get fetches a URL, retrying once on failure — see retries.
func (c *Client) handleURL(handle string) string {
	if c.channelHost != "" {
		return c.channelHost + "/" + handle
	}
	return "https://www.youtube.com/" + handle
}

func (c *Client) get(url string) ([]byte, error) {
	var err error
	for attempt := 0; ; attempt++ {
		var body []byte
		body, err = c.getOnce(url)
		if err == nil {
			return body, nil
		}
		if attempt >= retries {
			return nil, err
		}
		time.Sleep(retryDelay)
	}
}

func (c *Client) getOnce(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube responded %s", resp.Status)
	}
	// Read a bounded amount: a channel page is far larger than a feed, and
	// neither should be able to exhaust memory.
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// atomFeed is the shape of the response, cut down to what's used.
type atomFeed struct {
	Entries []struct {
		VideoID   string `xml:"videoId"`
		Title     string `xml:"title"`
		Published string `xml:"published"`
		Group     struct {
			Description string `xml:"description"`
		} `xml:"group"`
	} `xml:"entry"`
}

// parseFeed reads the Atom feed into videos, newest first (the order
// YouTube already returns them in).
func parseFeed(body []byte) ([]Video, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}

	videos := make([]Video, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}
		v := Video{
			ID:          e.VideoID,
			Title:       strings.TrimSpace(e.Title),
			Description: strings.TrimSpace(e.Group.Description),
		}
		if t, err := time.Parse(time.RFC3339, e.Published); err == nil {
			v.Published = t
		}
		videos = append(videos, v)
	}
	return videos, nil
}
