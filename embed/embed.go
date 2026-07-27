// Package embed recognises the media links DECAY's blog can turn into
// players — YouTube videos and Bandcamp releases — and nothing else.
//
// Rendering is split from resolving on purpose. Match is offline and total:
// it says whether a URL is already an embeddable player URL and, if so, the
// iframe src to use. Resolve is the author-facing step that may reach the
// network — a normal Bandcamp album page can't be embedded without reading
// the player URL out of its page — and produces a URL that Match then
// accepts. Keeping the network in Resolve means page rendering never makes
// an outbound request.
package embed

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Kind is which provider a URL embeds to.
type Kind int

const (
	None Kind = iota
	YouTube
	Bandcamp
)

var (
	// A YouTube id is exactly 11 URL-safe characters.
	youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	// The id-bearing path forms on youtube.com: /embed/ID, /shorts/ID, etc.
	youtubePathID = regexp.MustCompile(`^/(?:embed|shorts|v|live)/([A-Za-z0-9_-]{11})`)
)

// Match reports whether raw is a URL that can be embedded with no network
// access, returning the iframe src to use. YouTube is recognised in any of
// its URL forms (and as a bare id); Bandcamp only as an EmbeddedPlayer URL,
// which is what Resolve produces, so a plain album page stays a link until
// it's been resolved.
func Match(raw string) (Kind, string, bool) {
	raw = strings.TrimSpace(raw)
	if id, ok := youtubeIDFrom(raw); ok {
		return YouTube, "https://www.youtube-nocookie.com/embed/" + id, true
	}
	if src, ok := bandcampEmbedFrom(raw); ok {
		return Bandcamp, src, true
	}
	return None, "", false
}

// Resolve turns a user-pasted URL into the URL to store, on its own line, in
// a post body — the form Match later renders. A YouTube link normalises to
// a watch URL offline; a Bandcamp album or track page is fetched once to
// read its player URL from the og:video meta tag. A URL that already embeds
// is returned unchanged.
func Resolve(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("paste a link first")
	}
	if id, ok := youtubeIDFrom(raw); ok {
		return "https://www.youtube.com/watch?v=" + id, nil
	}
	if src, ok := bandcampEmbedFrom(raw); ok {
		return src, nil
	}
	u, err := url.Parse(raw)
	if err != nil || !isBandcampHost(u.Host) {
		return "", fmt.Errorf("that's not a YouTube or Bandcamp link")
	}
	return resolveBandcampPage(raw)
}

// youtubeIDFrom pulls the video id out of a YouTube URL (or accepts a bare
// id). It's host-aware on purpose: matching "v=<id>" anywhere would treat an
// unrelated link that happens to carry such a query as a YouTube video.
func youtubeIDFrom(raw string) (string, bool) {
	if youtubeID.MatchString(raw) {
		return raw, true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	switch host {
	case "youtu.be":
		id := strings.Trim(u.Path, "/")
		if i := strings.IndexByte(id, '/'); i >= 0 {
			id = id[:i]
		}
		if youtubeID.MatchString(id) {
			return id, true
		}
	case "youtube.com", "youtube-nocookie.com":
		if v := u.Query().Get("v"); youtubeID.MatchString(v) {
			return v, true
		}
		if m := youtubePathID.FindStringSubmatch(u.Path); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// bandcampEmbedFrom accepts a Bandcamp EmbeddedPlayer URL and returns it
// normalised to https. Only bandcamp.com hosts are accepted, which also
// bounds what Resolve will fetch.
func bandcampEmbedFrom(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !isBandcampHost(u.Host) {
		return "", false
	}
	if !strings.HasPrefix(u.Path, "/EmbeddedPlayer") {
		return "", false
	}
	u.Scheme = "https"
	return u.String(), true
}

func isBandcampHost(host string) bool {
	host = strings.ToLower(host)
	return host == "bandcamp.com" || strings.HasSuffix(host, ".bandcamp.com")
}

// resolveBandcampPage fetches a Bandcamp page and reads the player URL out
// of its Open Graph video tag. The caller has already checked the host is
// bandcamp.com, so this never fetches an arbitrary URL.
func resolveBandcampPage(pageURL string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(pageURL)
	if err != nil {
		return "", fmt.Errorf("couldn't reach Bandcamp")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Bandcamp responded %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	src, ok := ogVideoContent(body)
	if !ok {
		return "", fmt.Errorf("couldn't find a player on that Bandcamp page")
	}
	if _, ok := bandcampEmbedFrom(src); !ok {
		return "", fmt.Errorf("that Bandcamp page didn't give a usable player")
	}
	return src, nil
}

var (
	metaTag     = regexp.MustCompile(`(?i)<meta\b[^>]*>`)
	attrProp    = regexp.MustCompile(`(?i)property\s*=\s*["']([^"']+)["']`)
	attrContent = regexp.MustCompile(`(?i)content\s*=\s*["']([^"']+)["']`)
)

// ogVideoContent scans HTML for the og:video (or its secure_url variant)
// meta tag and returns its content, tolerant of attribute order.
func ogVideoContent(body []byte) (string, bool) {
	for _, tag := range metaTag.FindAll(body, -1) {
		p := attrProp.FindSubmatch(tag)
		if p == nil {
			continue
		}
		switch strings.ToLower(string(p[1])) {
		case "og:video", "og:video:secure_url":
			if c := attrContent.FindSubmatch(tag); c != nil {
				return html.UnescapeString(string(c[1])), true
			}
		}
	}
	return "", false
}
