package bookingmail

import (
	"regexp"
	"strings"
)

var (
	quotedLineRe  = regexp.MustCompile(`^\s*>`)
	onWroteRe     = regexp.MustCompile(`(?i)^\s*On\b.{6,200}\bwrote:\s*$`)
	onWroteEsRe   = regexp.MustCompile(`(?i)^\s*El\b.{6,200}\bescribi[oó]:\s*$`)
	onCommaRe     = regexp.MustCompile(`(?i)^\s*On\b.{6,200},\s*$`)
	wroteTailRe   = regexp.MustCompile(`(?i)\bwrote:\s*$`)
	origMessageRe = regexp.MustCompile(`(?i)^\s*-{2,}\s*(Original|Forwarded) [Mm]essage\s*-{2,}`)
	underscoreRe  = regexp.MustCompile(`^\s*_{10,}\s*$`)
	fromHeaderRe  = regexp.MustCompile(`(?i)^\s*From:\s*.+<[^>]+>\s*$`)
)

// splitQuoted separates what a sender actually wrote from the quoted reply
// chain trailing it. Every reply in a thread carries the whole history, so
// without this a thread view shows the same text over and over — repeated
// once per reply. Recognizes Gmail/Apple's "On ... wrote:" (including the
// form wrapped onto two lines), Outlook's "-----Original Message-----" and
// "From:" blocks, and bare "> " quoting.
func splitQuoted(body string) (newText, quoted string) {
	lines := strings.Split(body, "\n")
	cut := -1

	for i, line := range lines {
		switch {
		case quotedLineRe.MatchString(line):
			cut = i
		case onWroteRe.MatchString(line), onWroteEsRe.MatchString(line):
			cut = i
		case onCommaRe.MatchString(line) && i+1 < len(lines) && wroteTailRe.MatchString(lines[i+1]):
			cut = i
		case origMessageRe.MatchString(line), underscoreRe.MatchString(line), fromHeaderRe.MatchString(line):
			cut = i
		}
		if cut >= 0 {
			break
		}
	}

	if cut < 0 {
		return strings.TrimRight(body, "\r\n \t"), ""
	}

	newPart := strings.TrimRight(strings.Join(lines[:cut], "\n"), "\r\n \t")
	quotedPart := strings.TrimSpace(strings.Join(lines[cut:], "\n"))

	// If trimming would leave nothing readable, showing the whole thing beats
	// hiding a message with no visible body.
	if strings.TrimSpace(newPart) == "" {
		return strings.TrimRight(body, "\r\n \t"), ""
	}
	return newPart, quotedPart
}

var strayQuoteRe = regexp.MustCompile(`(?m)^>.*$`)
var whitespaceRunRe = regexp.MustCompile(`\s+`)

// snippet is a short, quote-stripped preview of a message body.
func snippet(body string, length int) string {
	newText, _ := splitQuoted(body)
	newText = strayQuoteRe.ReplaceAllString(newText, "")
	newText = strings.TrimSpace(whitespaceRunRe.ReplaceAllString(newText, " "))
	if newText == "" {
		return ""
	}
	runes := []rune(newText)
	if len(runes) > length {
		return string(runes[:length]) + "…"
	}
	return newText
}
