package bookingmail

import (
	"html"
	"io"
	"regexp"
	"strings"
)

func readAllString(r io.Reader) string {
	if r == nil {
		return ""
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return ""
	}
	return string(b)
}

var (
	// RE2 (Go's regexp package) has no backreferences, so script and style
	// are matched as separate alternatives rather than one group.
	scriptStyleRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)
	brRe          = regexp.MustCompile(`(?i)<br\s*/?>`)
	blockCloseRe  = regexp.MustCompile(`(?i)</(p|div|tr|li|h[1-6]|blockquote)\s*>`)
	tagRe         = regexp.MustCompile(`<[^>]*>`)
	blankRunsRe   = regexp.MustCompile(`\n{3,}`)
	// Non-breaking space (U+00A0), written as an escape so the source file
	// stays plain ASCII rather than carrying an invisible raw character.
	nbsp = " "
)

// htmlToText is a crude HTML-to-text conversion for messages that only ship
// an HTML part, good enough for a readable admin-panel preview.
func htmlToText(h string) string {
	if h == "" {
		return ""
	}
	h = scriptStyleRe.ReplaceAllString(h, "")
	h = brRe.ReplaceAllString(h, "\n")
	h = blockCloseRe.ReplaceAllString(h, "\n")
	text := tagRe.ReplaceAllString(h, "")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, nbsp, " ")
	text = blankRunsRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
