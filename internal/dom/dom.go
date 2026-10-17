// Package dom defines the document tree produced by the HTML parser.
package dom

import (
	"fmt"
	"strings"
)

// NodeType identifies the kind of a Node.
type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
	CommentNode
)

// Attr is an element attribute. Names are lower-cased.
type Attr struct {
	Name, Value string
}

// Node is a node in the document tree.
type Node struct {
	Type     NodeType
	Tag      string // lower-cased tag name for elements
	Attrs    []Attr
	Text     string // text for text and comment nodes
	Parent   *Node
	Children []*Node

	// Quirks is set on document nodes parsed without an HTML doctype.
	Quirks bool
}

// NewElement creates an element node.
func NewElement(tag string, attrs ...Attr) *Node {
	return &Node{Type: ElementNode, Tag: tag, Attrs: attrs}
}

// NewText creates a text node.
func NewText(text string) *Node {
	return &Node{Type: TextNode, Text: text}
}

// AppendChild appends c as the last child of n.
func (n *Node) AppendChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

// Attr returns the value of the named attribute.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// HasClass reports whether the element's class attribute contains cls.
func (n *Node) HasClass(cls string) bool {
	v, ok := n.Attr("class")
	if !ok {
		return false
	}
	for _, f := range strings.Fields(v) {
		if f == cls {
			return true
		}
	}
	return false
}

// ElementChildren returns the element children of n.
func (n *Node) ElementChildren() []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Type == ElementNode {
			out = append(out, c)
		}
	}
	return out
}

// Walk calls fn for n and every descendant in document order. If fn returns
// false the node's children are skipped.
func (n *Node) Walk(fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Find returns the first element (in document order) with the given tag.
func (n *Node) Find(tag string) *Node {
	var found *Node
	n.Walk(func(x *Node) bool {
		if found != nil {
			return false
		}
		if x.Type == ElementNode && x.Tag == tag {
			found = x
			return false
		}
		return true
	})
	return found
}

// FindAll returns all elements with the given tag in document order.
func (n *Node) FindAll(tag string) []*Node {
	var out []*Node
	n.Walk(func(x *Node) bool {
		if x.Type == ElementNode && x.Tag == tag {
			out = append(out, x)
		}
		return true
	})
	return out
}

// TextContent returns the concatenated text of n and its descendants.
func (n *Node) TextContent() string {
	var sb strings.Builder
	n.Walk(func(x *Node) bool {
		if x.Type == TextNode {
			sb.WriteString(x.Text)
		}
		return true
	})
	return sb.String()
}

// String renders the tree in a compact indented debug format.
func (n *Node) String() string {
	var sb strings.Builder
	n.dump(&sb, 0)
	return sb.String()
}

func (n *Node) dump(sb *strings.Builder, depth int) {
	indent := strings.Repeat("  ", depth)
	switch n.Type {
	case DocumentNode:
		sb.WriteString(indent + "#document\n")
	case ElementNode:
		sb.WriteString(indent + "<" + n.Tag)
		for _, a := range n.Attrs {
			fmt.Fprintf(sb, " %s=%q", a.Name, a.Value)
		}
		sb.WriteString(">\n")
	case TextNode:
		fmt.Fprintf(sb, "%s%q\n", indent, n.Text)
	case CommentNode:
		fmt.Fprintf(sb, "%s<!--%s-->\n", indent, n.Text)
	}
	for _, c := range n.Children {
		c.dump(sb, depth+1)
	}
}
