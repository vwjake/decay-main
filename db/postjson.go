package db

import "time"

// PostResponse is the public JSON shape of a blog post, served from the same
// endpoints as the HTML pages via content negotiation. Body is the Markdown
// source (as stored) rather than rendered HTML, so a JSON consumer keeps the
// freedom to render it however it likes; the URL is absolute for off-site use.
type PostResponse struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Body        string     `json:"body_markdown"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	URL         string     `json:"url"`
}

// Response builds the JSON view of a post. baseURL is the site's absolute
// origin, prefixed onto the canonical path so the URL is usable off-site.
func (p Post) Response(baseURL string) PostResponse {
	return PostResponse{
		ID:          p.ID,
		Slug:        p.Slug,
		Title:       p.Title,
		Body:        p.Body,
		PublishedAt: p.PublishedAt,
		URL:         baseURL + p.Path(),
	}
}

// PostsResponse builds the JSON view of a list of posts.
func PostsResponse(baseURL string, posts []Post) []PostResponse {
	out := make([]PostResponse, 0, len(posts))
	for _, p := range posts {
		out = append(out, p.Response(baseURL))
	}
	return out
}
