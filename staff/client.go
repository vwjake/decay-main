package staff

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// cacheTTL is how long a fetched feed is reused before the next admin view
// re-fetches. The internal calendar changes rarely and an admin page load
// shouldn't wait on a network round-trip every time, so a few minutes of
// staleness is a fair trade — "live" here means minutes, not seconds.
const cacheTTL = 5 * time.Minute

// fetchTimeout caps how long a page load will wait on the calendar host
// before giving up and showing the last good snapshot (or an error).
const fetchTimeout = 8 * time.Second

// Client fetches and caches the internal calendar from a shared read-only
// .ics URL. It's safe for concurrent use. A zero-value URL means the
// feature isn't configured, which Configured reports so the page can
// explain how to turn it on instead of erroring.
type Client struct {
	url   string
	venue *time.Location
	http  *http.Client

	mu      sync.Mutex
	cached  []Meeting
	fetched time.Time
	lastErr error
}

// NewClient builds a client for the given .ics URL. An empty url leaves the
// feature unconfigured. venue is the timezone floating times are read in.
func NewClient(url string, venue *time.Location) *Client {
	return &Client{
		url:   url,
		venue: venue,
		http:  &http.Client{Timeout: fetchTimeout},
	}
}

// Configured reports whether a calendar URL was set.
func (c *Client) Configured() bool { return c.url != "" }

// Meetings returns the internal calendar, fetching it if the cache is empty
// or stale. On a failed refresh it returns the last good snapshot together
// with the error, so a transient outage degrades to slightly stale data
// rather than an empty page.
func (c *Client) Meetings() ([]Meeting, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.Configured() {
		return nil, nil
	}
	if c.cached != nil && time.Since(c.fetched) < cacheTTL {
		return c.cached, nil
	}

	fresh, err := c.fetch()
	if err != nil {
		c.lastErr = err
		return c.cached, err // possibly nil on a first-load failure
	}
	c.cached = fresh
	c.fetched = time.Now()
	c.lastErr = nil
	return fresh, nil
}

func (c *Client) fetch() ([]Meeting, error) {
	resp, err := c.http.Get(c.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar responded %s", resp.Status)
	}
	// Read a bounded amount so a misconfigured URL pointing at something
	// huge can't exhaust memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return Parse(body, c.venue), nil
}
