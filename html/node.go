// Package html implements an HTML tokenizer and a simplified tree builder
// that produces a DOM tree suitable for styling and layout.
package html

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// NodeType identifies the kind of a DOM node.
type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
	CommentNode
	DoctypeNode
)

// Attr is a single element attribute. Names are lowercased.
type Attr struct {
	Name  string
	Value string
}

// Node is a node in the DOM tree.
//
// For elements, Data is the lowercased tag name. For text and comment
// nodes, Data is the (decoded) content. For doctype nodes, Data is the
// doctype name and Attrs may hold "public" and "system" identifiers.
type Node struct {
	Type     NodeType
	Data     string
	Attrs    []Attr
	Parent   *Node
	Children []*Node

	// Quirks reports whether the document is in quirks mode. Only set on
	// the DocumentNode.
	Quirks bool
}

// GetAttr returns the value of the named attribute and whether it is present.
func (n *Node) GetAttr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// AppendChild appends c as the last child of n.
func (n *Node) AppendChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

// InsertBefore inserts c as a child of n immediately before ref. If ref is
// not a child of n, c is appended.
func (n *Node) InsertBefore(c, ref *Node) {
	for i, ch := range n.Children {
		if ch == ref {
			c.Parent = n
			n.Children = append(n.Children, nil)
			copy(n.Children[i+1:], n.Children[i:])
			n.Children[i] = c
			return
		}
	}
	n.AppendChild(c)
}

// Dump writes an indented, human-readable representation of the tree
// rooted at n to w.
func (n *Node) Dump(w io.Writer) error {
	var sb strings.Builder
	n.dump(&sb, 0)
	_, err := io.WriteString(w, sb.String())
	return err
}

func (n *Node) dump(sb *strings.Builder, depth int) {
	sb.WriteString(strings.Repeat("  ", depth))
	switch n.Type {
	case DocumentNode:
		sb.WriteString("#document")
		if n.Quirks {
			sb.WriteString(" (quirks)")
		}
	case DoctypeNode:
		fmt.Fprintf(sb, "<!DOCTYPE %s>", n.Data)
	case ElementNode:
		sb.WriteString("<" + n.Data)
		for _, a := range n.Attrs {
			fmt.Fprintf(sb, " %s=%s", a.Name, strconv.Quote(a.Value))
		}
		sb.WriteString(">")
	case TextNode:
		sb.WriteString(strconv.Quote(n.Data))
	case CommentNode:
		fmt.Fprintf(sb, "<!-- %s -->", n.Data)
	}
	sb.WriteString("\n")
	for _, c := range n.Children {
		c.dump(sb, depth+1)
	}
}
