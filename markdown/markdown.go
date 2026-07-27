// Package markdown renders admin-authored Markdown to HTML for the site.
// Blog posts are stored as Markdown (the posts.body_markdown column), and
// this is the one place that turns that source into the markup a page shows.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// md is configured once and reused. GFM adds the conveniences writers
// expect — tables, strikethrough, task lists and bare-URL autolinking —
// on top of CommonMark.
//
// Raw HTML in the source is deliberately left escaped (goldmark's default,
// i.e. no WithUnsafe), so a post renders as text even though only trusted
// admins write posts. That keeps the blog on the same "never hand raw HTML
// to the browser" footing as the rest of the site.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithASTTransformers(
			util.Prioritized(embedTransformer{}, 500),
		),
	),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(embedRenderer{}, 500),
		),
	),
)

// ToHTML renders Markdown source to an HTML fragment. goldmark only fails
// when a writer errors, which a bytes.Buffer never does, so any error just
// yields whatever was written so far rather than a caller-facing failure.
func ToHTML(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return buf.String()
	}
	return buf.String()
}
