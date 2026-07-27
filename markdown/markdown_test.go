package markdown

import (
	"strings"
	"testing"
)

// TestToHTMLRendersStructure checks that the common Markdown a post uses —
// headings, lists, links and emphasis — becomes real markup rather than the
// flat text the old Paragraphs() split produced.
func TestToHTMLRendersStructure(t *testing.T) {
	out := ToHTML("# Title\n\nSome **bold** and a [link](https://decay.events).\n\n- one\n- two\n")

	for _, want := range []string{
		"<h1>Title</h1>",
		"<strong>bold</strong>",
		`<a href="https://decay.events">link</a>`,
		"<li>one</li>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

// TestToHTMLEscapesRawHTML pins the safe-by-default posture: raw HTML in a
// post's source is escaped, so a post can't inject markup or script even
// though only trusted admins write posts.
func TestToHTMLEscapesRawHTML(t *testing.T) {
	out := ToHTML("Hello <script>alert(1)</script>")
	if strings.Contains(out, "<script>") {
		t.Errorf("raw HTML was not escaped: %s", out)
	}
}

// TestToHTMLAutolinksBareURL confirms GFM's linkify turns a bare URL into a
// link, matching how bare URLs behave elsewhere on the site.
func TestToHTMLAutolinksBareURL(t *testing.T) {
	out := ToHTML("Visit https://decay.events for details.")
	if !strings.Contains(out, `href="https://decay.events"`) {
		t.Errorf("bare URL was not autolinked: %s", out)
	}
}

// TestToHTMLEmbedsMediaOnItsOwnLine covers the embed transformer: a media
// URL alone on a line becomes a player, while the same URL in a sentence
// stays an ordinary link.
func TestToHTMLEmbedsMediaOnItsOwnLine(t *testing.T) {
	yt := ToHTML("Watch this:\n\nhttps://youtu.be/dQw4w9WgXcQ\n")
	if !strings.Contains(yt, `class="embed embed-video"`) ||
		!strings.Contains(yt, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
		t.Errorf("bare YouTube line didn't become a video embed: %s", yt)
	}

	bc := ToHTML("https://bandcamp.com/EmbeddedPlayer/album=42/size=large/\n")
	if !strings.Contains(bc, `class="embed embed-audio"`) {
		t.Errorf("Bandcamp player line didn't become an audio embed: %s", bc)
	}

	// In a sentence, it must remain a plain link, not an embed.
	inline := ToHTML("See https://youtu.be/dQw4w9WgXcQ now.")
	if strings.Contains(inline, "class=\"embed") {
		t.Errorf("a URL mid-sentence should not be embedded: %s", inline)
	}
	if !strings.Contains(inline, `href="https://youtu.be/dQw4w9WgXcQ"`) {
		t.Errorf("mid-sentence URL should stay a link: %s", inline)
	}
}
