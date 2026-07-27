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
