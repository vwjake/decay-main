package views

import (
	"strings"

	"decay-main/db"
)

// siteName and defaultDescription are the site-wide fallbacks used for the
// preview tags when a page doesn't supply its own.
const (
	siteName           = "DECAY"
	defaultDescription = "All-ages DIY venue and community space in Olympia, WA."
)

// Meta is the per-page metadata the layout turns into the <title>, a
// description, and Open Graph / Twitter card tags — so a shared link previews
// with a real title, blurb, and image instead of a bare URL. A zero Meta is
// valid and yields the site-wide defaults.
type Meta struct {
	Description string // plain-text summary, roughly <=200 chars
	Image       string // absolute URL of a preview image; optional
	URL         string // absolute canonical URL of the page; optional
	Type        string // Open Graph type: "" (=> "website") or "article"
}

// EventMeta builds the preview metadata for an event's page. baseURL is the
// site's absolute origin, prefixed onto the flyer and canonical paths so the
// tags carry absolute URLs — which is what scrapers require.
func EventMeta(ev db.Event, baseURL string) Meta {
	m := Meta{
		Description: summarizeEvent(ev),
		URL:         baseURL + ev.Path(),
		Type:        "article",
	}
	if ev.HasFlyer() {
		m.Image = baseURL + ev.FlyerWebPath()
	}
	return m
}

// ogDescription returns the description to advertise, falling back to the
// site-wide default when a page doesn't set one.
func ogDescription(meta Meta) string {
	if meta.Description != "" {
		return meta.Description
	}
	return defaultDescription
}

// ogType returns the Open Graph type, defaulting to "website".
func ogType(meta Meta) string {
	if meta.Type != "" {
		return meta.Type
	}
	return "website"
}

// summarizeEvent builds a one-line description for an event preview: the date
// and place, then as much of the blurb as fits.
func summarizeEvent(ev db.Event) string {
	head := ev.LongDate()
	if ev.Location != "" {
		head += " · " + ev.Location
	}
	body := strings.Join(ev.Paragraphs(), " ")
	if body == "" {
		return head
	}
	return truncate(head+" — "+body, 200)
}

// truncate shortens s to at most max runes, adding an ellipsis when it cuts.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max-1])) + "…"
}
