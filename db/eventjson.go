package db

import "time"

// EventResponse is the public JSON shape of an event, served from the same
// endpoints as the HTML pages via content negotiation. It is deliberately
// kept separate from Event so the API contract does not shift every time the
// storage struct changes, and so media and canonical links can be absolute —
// which is what off-site consumers (and link-preview scrapers) need.
type EventResponse struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Type        string     `json:"type"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Location    string     `json:"location,omitempty"`
	Description string     `json:"description"`
	Link        string     `json:"link,omitempty"`
	// URL is the event's absolute canonical page.
	URL string `json:"url"`
	// Flyer is the absolute URL of the web-sized flyer; FlyerFull is the
	// original. Both are omitted when the event has no flyer.
	Flyer     string `json:"flyer,omitempty"`
	FlyerFull string `json:"flyer_full,omitempty"`
	// OpenRoles are the volunteer jobs still needing someone, by label. It is
	// always present (never null) so clients can iterate it unconditionally.
	OpenRoles []string `json:"open_roles"`
}

// Response builds the JSON view of an event. baseURL is the site's absolute
// origin (e.g. "https://decay.events"); it is prefixed onto the otherwise
// root-relative media and canonical paths so the JSON is usable off-site.
// openRoles may be nil for list endpoints that don't load volunteer data.
func (e Event) Response(baseURL string, openRoles []EventVolunteer) EventResponse {
	r := EventResponse{
		ID:          e.ID,
		Slug:        e.Slug,
		Title:       e.Title,
		Type:        e.EventType,
		StartsAt:    e.StartsAt,
		EndsAt:      e.EndsAt,
		Location:    e.Location,
		Description: e.Description,
		URL:         baseURL + e.Path(),
		OpenRoles:   []string{},
	}
	if e.HasLink() {
		r.Link = e.Link
	}
	if e.HasFlyer() {
		r.Flyer = baseURL + e.FlyerWebPath()
		r.FlyerFull = baseURL + e.FlyerPath()
	}
	for _, role := range openRoles {
		r.OpenRoles = append(r.OpenRoles, role.Label())
	}
	return r
}

// EventsResponse builds the JSON view of a list of events. Open roles are
// omitted (empty) here — list consumers that need them fetch the single
// event, which carries the volunteer data.
func EventsResponse(baseURL string, events []Event) []EventResponse {
	out := make([]EventResponse, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Response(baseURL, nil))
	}
	return out
}
