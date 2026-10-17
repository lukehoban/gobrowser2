// Package dom defines the document tree used by the rest of the browser and
// builds it from HTML using golang.org/x/net/html.
package dom

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// NodeType distinguishes document, element and text nodes.
type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
)

// Attr is a single element attribute.
type Attr struct {
	Name, Value string
}

// Node is a node in the document tree.
type Node struct {
	Type     NodeType
	Tag      string // lowercase tag name for elements
	Attrs    []Attr
	Text     string // text content for text nodes
	Parent   *Node
	Children []*Node

	// Quirks is set on the document node when the page has no standards-mode
	// doctype, as with Hacker News. A few rendering rules differ in that mode.
	Quirks bool
}

// Attr returns the value of the named attribute and whether it is present.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// AttrOr returns the value of the named attribute or def if it is absent.
func (n *Node) AttrOr(name, def string) string {
	if v, ok := n.Attr(name); ok {
		return v
	}
	return def
}

// HasClass reports whether the element's class attribute contains c.
func (n *Node) HasClass(c string) bool {
	for _, f := range strings.Fields(n.AttrOr("class", "")) {
		if f == c {
			return true
		}
	}
	return false
}

// TextContent returns the concatenated text of all descendant text nodes.
func (n *Node) TextContent() string {
	var sb strings.Builder
	n.Walk(func(c *Node) {
		if c.Type == TextNode {
			sb.WriteString(c.Text)
		}
	})
	return sb.String()
}

// Walk calls fn for n and every descendant in document order.
func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Find returns the first element (in document order) with the given tag.
func (n *Node) Find(tag string) *Node {
	var found *Node
	n.Walk(func(c *Node) {
		if found == nil && c.Type == ElementNode && c.Tag == tag {
			found = c
		}
	})
	return found
}

// FindAll returns all elements with the given tag in document order.
func (n *Node) FindAll(tag string) []*Node {
	var out []*Node
	n.Walk(func(c *Node) {
		if c.Type == ElementNode && c.Tag == tag {
			out = append(out, c)
		}
	})
	return out
}

// Parse parses an HTML document. Comments, doctypes, <script>, <noscript> and
// <template> contents are dropped since they never render.
func Parse(r io.Reader) (*Node, error) {
	root, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	doc := &Node{Type: DocumentNode, Quirks: true}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.DoctypeNode && strings.EqualFold(c.Data, "html") {
			doc.Quirks = false
			for _, a := range c.Attr {
				if a.Key == "public" && a.Val != "" && !strings.Contains(strings.ToLower(a.Val), "strict") && !strings.Contains(strings.ToLower(a.Val), "xhtml") {
					doc.Quirks = true
				}
			}
		}
	}
	convertChildren(root, doc)
	return doc, nil
}

var skipped = map[string]bool{"script": true, "noscript": true, "template": true}

func convertChildren(src *html.Node, dst *Node) {
	for c := src.FirstChild; c != nil; c = c.NextSibling {
		var n *Node
		switch c.Type {
		case html.ElementNode:
			if skipped[c.Data] {
				continue
			}
			n = &Node{Type: ElementNode, Tag: strings.ToLower(c.Data)}
			for _, a := range c.Attr {
				if a.Namespace != "" {
					continue
				}
				n.Attrs = append(n.Attrs, Attr{Name: a.Key, Value: a.Val})
			}
			convertChildren(c, n)
		case html.TextNode:
			// Merge adjacent text nodes.
			if k := len(dst.Children); k > 0 && dst.Children[k-1].Type == TextNode {
				dst.Children[k-1].Text += c.Data
				continue
			}
			n = &Node{Type: TextNode, Text: c.Data}
		default:
			continue
		}
		n.Parent = dst
		dst.Children = append(dst.Children, n)
	}
}

// Dump writes an indented outline of the tree, for debugging.
func (n *Node) Dump(w io.Writer) {
	n.dump(w, 0)
}

func (n *Node) dump(w io.Writer, depth int) {
	indent := strings.Repeat("  ", depth)
	switch n.Type {
	case DocumentNode:
		fmt.Fprintf(w, "%s#document\n", indent)
	case ElementNode:
		var sb strings.Builder
		for _, a := range n.Attrs {
			fmt.Fprintf(&sb, " %s=%q", a.Name, a.Value)
		}
		fmt.Fprintf(w, "%s<%s%s>\n", indent, n.Tag, sb.String())
	case TextNode:
		t := strings.Join(strings.Fields(n.Text), " ")
		if t == "" {
			return
		}
		fmt.Fprintf(w, "%s%q\n", indent, t)
	}
	for _, c := range n.Children {
		c.dump(w, depth+1)
	}
}
