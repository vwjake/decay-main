package markdown

import (
	"bytes"
	"html"

	"decay-main/embed"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// embedKind is the AST node kind for a recognised media embed.
var embedKind = ast.NewNodeKind("Embed")

// embedNode stands in for a paragraph that was nothing but an embeddable
// link. It carries the provider and the iframe src our renderer writes.
type embedNode struct {
	ast.BaseBlock
	kind embed.Kind
	src  string
}

func (n *embedNode) Kind() ast.NodeKind         { return embedKind }
func (n *embedNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// embedTransformer replaces a paragraph consisting solely of a link to a
// recognised provider with an embed node. Only URLs embed.Match validates
// (a YouTube URL, a Bandcamp player URL) ever become a node, and the node's
// HTML is emitted by our own renderer — so this is a controlled exception
// to goldmark leaving source HTML escaped, not a general raw-HTML opening.
type embedTransformer struct{}

func (embedTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()

	var paragraphs []*ast.Paragraph
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*ast.Paragraph); ok {
			paragraphs = append(paragraphs, p)
		}
	}

	for _, p := range paragraphs {
		url, ok := soleLinkURL(p, source)
		if !ok {
			continue
		}
		kind, src, ok := embed.Match(url)
		if !ok {
			continue
		}
		node := &embedNode{kind: kind, src: src}
		p.Parent().ReplaceChild(p.Parent(), p, node)
	}
}

// soleLinkURL returns the destination of a paragraph whose only meaningful
// content is a single link or autolink; ok is false for anything else, so a
// URL sitting in a sentence is left as an ordinary link.
func soleLinkURL(p *ast.Paragraph, source []byte) (string, bool) {
	var link ast.Node
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Text:
			if len(bytes.TrimSpace(node.Segment.Value(source))) != 0 {
				return "", false
			}
		case *ast.AutoLink, *ast.Link:
			if link != nil {
				return "", false
			}
			link = node
		default:
			return "", false
		}
	}
	switch l := link.(type) {
	case *ast.AutoLink:
		return string(l.URL(source)), true
	case *ast.Link:
		return string(l.Destination), true
	}
	return "", false
}

// embedRenderer writes the iframe for an embed node.
type embedRenderer struct{}

func (embedRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(embedKind, renderEmbed)
}

func renderEmbed(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*embedNode)
	src := html.EscapeString(n.src)
	switch n.kind {
	case embed.YouTube:
		w.WriteString(`<div class="embed embed-video"><iframe src="` + src +
			`" title="YouTube video player" loading="lazy" frameborder="0" ` +
			`allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" ` +
			`referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe></div>`)
	case embed.Bandcamp:
		w.WriteString(`<iframe class="embed embed-audio" src="` + src +
			`" title="Bandcamp player" loading="lazy" seamless frameborder="0"></iframe>`)
	}
	return ast.WalkSkipChildren, nil
}
