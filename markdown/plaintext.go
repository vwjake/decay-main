package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// PlainText renders Markdown source down to its readable text, dropping all
// formatting. It's for link-preview descriptions, where markup would
// otherwise leak through as literal characters (#, *, [text](url)). Block and
// line breaks collapse to single spaces so the result reads as one line.
func PlainText(src string) string {
	source := []byte(src)
	doc := md.Parser().Parse(text.NewReader(source))

	var b strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*ast.Text); ok {
			if entering {
				b.Write(t.Segment.Value(source))
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
			}
			return ast.WalkContinue, nil
		}
		// Leaving a block (paragraph, heading, list item) separates it from
		// the next so their words don't run together.
		if !entering && n.Type() == ast.TypeBlock {
			b.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})

	// Collapse any runs of whitespace the joins introduced.
	return strings.Join(strings.Fields(b.String()), " ")
}
