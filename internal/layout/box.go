// Package layout turns a styled DOM into a tree of positioned boxes.
//
// It implements a subset of CSS 2.1 visual formatting: block layout with
// sibling margin collapsing, inline layout with line breaking, and the
// automatic table layout algorithm (which Hacker News depends on).
package layout

import (
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
	"github.com/lukehoban/gobrowser2/internal/style"
)

// Kind is the kind of a box.
type Kind int

const (
	KBlock       Kind = iota // block container (including anonymous blocks)
	KInline                  // non-replaced inline box
	KText                    // run of text
	KReplaced                // replaced element: <img>, <input>
	KInlineBlock             // atomic inline-level block container
	KBreak                   // <br>
	KTable
	KRowGroup
	KRow
	KCell
)

var kindNames = [...]string{"block", "inline", "text", "replaced", "inline-block", "br", "table", "row-group", "row", "cell"}

func (k Kind) String() string { return kindNames[k] }

// Image is a decoded raster or vector image.
type Image interface {
	// Size returns the intrinsic size in CSS pixels.
	Size() (w, h float64)
	// Draw draws the image scaled to fill r.
	Draw(dst *image.RGBA, r image.Rectangle)
}

// Box is a node of the box tree. X, Y, W and H describe the content box in
// page coordinates once layout has run.
type Box struct {
	Kind     Kind
	Node     *dom.Node // nil for anonymous boxes
	Style    *style.Style
	Text     string // KText only
	Image    Image  // KReplaced only; may be nil
	Children []*Box
	ColSpan  int // KCell only

	X, Y, W, H              float64
	Padding, Border, Margin [4]float64
	Lines                   []*Line // block containers with inline content

	minMax *[2]float64 // cached intrinsic content widths
}

// Line is a line box.
type Line struct {
	X, Y, W, H float64
	Baseline   float64
	Frags      []*Frag
}

// Frag is a piece of a line: a text run or an atomic inline box.
type Frag struct {
	X, Y  float64 // for text, Y is the baseline
	W     float64
	Text  string
	Style *style.Style
	Box   *Box // atomic inline (already positioned)
}

// BorderBox returns the border box rectangle.
func (b *Box) BorderBox() (x, y, w, h float64) {
	return b.X - b.Padding[style.Left] - b.Border[style.Left],
		b.Y - b.Padding[style.Top] - b.Border[style.Top],
		b.W + b.Padding[style.Left] + b.Padding[style.Right] + b.Border[style.Left] + b.Border[style.Right],
		b.H + b.Padding[style.Top] + b.Padding[style.Bottom] + b.Border[style.Top] + b.Border[style.Bottom]
}

func (b *Box) pbH() float64 {
	return b.Padding[style.Left] + b.Padding[style.Right] + b.Border[style.Left] + b.Border[style.Right]
}

func (b *Box) pbV() float64 {
	return b.Padding[style.Top] + b.Padding[style.Bottom] + b.Border[style.Top] + b.Border[style.Bottom]
}

// Options configures layout.
type Options struct {
	Width  float64               // viewport width in CSS pixels
	Image  func(*dom.Node) Image // resolves the image for an <img>; may return nil
	Quirks bool
}

// Result is a laid-out document.
type Result struct {
	Root   *Box // anonymous box for the initial containing block
	Height float64
}

// Layout builds and lays out the box tree for doc.
func Layout(doc *dom.Node, styles map[*dom.Node]*style.Style, opt Options) *Result {
	bld := &builder{styles: styles, image: opt.Image}
	root := &Box{Kind: KBlock, Style: style.AnonymousBlock(&style.Style{FontSize: 16, TextAlign: "left", WhiteSpace: "normal"})}
	for _, c := range doc.Children {
		if b := bld.build(c, root.Style); b != nil {
			root.Children = append(root.Children, b)
		}
	}
	fixBlockContainer(root)
	c := &ctx{quirks: opt.Quirks}
	root.X, root.W = 0, opt.Width
	h := c.layoutContents(root)
	root.H = h
	return &Result{Root: root, Height: h}
}

// ---------------------------------------------------------------------------
// Box tree construction

type builder struct {
	styles map[*dom.Node]*style.Style
	image  func(*dom.Node) Image
}

func (bl *builder) build(n *dom.Node, parent *style.Style) *Box {
	if n.Type == dom.TextNode {
		return &Box{Kind: KText, Style: parent, Text: n.Text}
	}
	if n.Type != dom.ElementNode {
		return nil
	}
	s := bl.styles[n]
	if s == nil || s.Display == "none" {
		return nil
	}
	b := &Box{Node: n, Style: s}
	switch n.Tag {
	case "br":
		b.Kind = KBreak
		return b
	case "img", "input", "select", "textarea", "button", "iframe", "canvas", "video", "svg":
		b.Kind = KReplaced
		if n.Tag == "img" && bl.image != nil {
			b.Image = bl.image(n)
		}
		return b
	}
	switch s.Display {
	case "block", "list-item", "flex", "grid", "flow-root", "table-caption":
		b.Kind = KBlock
	case "inline-block", "inline-flex", "inline-grid":
		b.Kind = KInlineBlock
	case "table", "inline-table":
		b.Kind = KTable
	case "table-row-group", "table-header-group", "table-footer-group":
		b.Kind = KRowGroup
	case "table-row":
		b.Kind = KRow
	case "table-cell":
		b.Kind = KCell
		b.ColSpan = 1
		if v, ok := n.Attr("colspan"); ok {
			fmt.Sscan(v, &b.ColSpan)
			if b.ColSpan < 1 || b.ColSpan > 1000 {
				b.ColSpan = 1
			}
		}
	case "table-column", "table-column-group":
		return nil
	default:
		b.Kind = KInline
	}
	for _, c := range n.Children {
		if cb := bl.build(c, s); cb != nil {
			b.Children = append(b.Children, cb)
		}
	}
	switch b.Kind {
	case KInline:
		// Block-in-inline: approximate by turning the inline into a block.
		for _, c := range b.Children {
			if isBlockLevel(c) {
				b.Kind = KBlock
				break
			}
		}
		if b.Kind == KBlock {
			fixBlockContainer(b)
		}
	case KBlock, KInlineBlock, KCell:
		fixBlockContainer(b)
	case KTable:
		fixTable(b)
	}
	return b
}

func isBlockLevel(b *Box) bool {
	switch b.Kind {
	case KBlock, KTable, KRowGroup, KRow, KCell:
		return true
	case KReplaced:
		return b.Style.Display == "block"
	}
	return false
}

func isCollapsibleWhitespace(b *Box) bool {
	if b.Kind != KText {
		return false
	}
	ws := b.Style.WhiteSpace
	if ws == "pre" || ws == "pre-wrap" || ws == "break-spaces" {
		return false
	}
	return strings.Trim(b.Text, " \t\n\r\f") == ""
}

// fixBlockContainer wraps runs of inline-level children in anonymous blocks
// when a block container has block-level children.
func fixBlockContainer(b *Box) {
	hasBlock := false
	for _, c := range b.Children {
		if isBlockLevel(c) {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		return
	}
	var out, run []*Box
	flush := func() {
		keep := false
		for _, r := range run {
			if !isCollapsibleWhitespace(r) {
				keep = true
				break
			}
		}
		if keep {
			out = append(out, &Box{Kind: KBlock, Style: style.AnonymousBlock(b.Style), Children: run})
		}
		run = nil
	}
	for _, c := range b.Children {
		if isBlockLevel(c) {
			flush()
			out = append(out, c)
		} else {
			run = append(run, c)
		}
	}
	flush()
	b.Children = out
}

// fixTable flattens row groups and drops anything that isn't a row.
// Cells directly in a table or row group get an anonymous row.
func fixTable(t *Box) {
	var rows []*Box
	var anon *Box
	var add func(c *Box)
	add = func(c *Box) {
		switch c.Kind {
		case KRow:
			anon = nil
			rows = append(rows, fixRow(c))
		case KRowGroup:
			anon = nil
			for _, r := range c.Children {
				add(r)
			}
			anon = nil
		case KCell:
			if anon == nil {
				anon = &Box{Kind: KRow, Style: style.AnonymousBlock(t.Style)}
				rows = append(rows, anon)
			}
			anon.Children = append(anon.Children, c)
		}
	}
	for _, c := range t.Children {
		add(c)
	}
	t.Children = rows
}

func fixRow(r *Box) *Box {
	var cells []*Box
	for _, c := range r.Children {
		if c.Kind == KCell {
			cells = append(cells, c)
		}
	}
	r.Children = cells
	return r
}

// ---------------------------------------------------------------------------
// Utilities

// translate moves b and all its descendants by (dx, dy).
func translate(b *Box, dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	b.X += dx
	b.Y += dy
	for _, l := range b.Lines {
		l.X += dx
		l.Y += dy
		l.Baseline += dy
		for _, f := range l.Frags {
			f.X += dx
			f.Y += dy
		}
	}
	for _, c := range b.Children {
		translate(c, dx, dy)
	}
}

// Dump writes the box tree with geometry, for debugging.
func Dump(w io.Writer, b *Box) {
	dump(w, b, 0)
}

func dump(w io.Writer, b *Box, depth int) {
	ind := strings.Repeat("  ", depth)
	name := "anonymous"
	if b.Node != nil {
		name = "<" + b.Node.Tag + ">"
		if id := b.Node.AttrOr("id", ""); id != "" {
			name += "#" + id
		}
		if cl := b.Node.AttrOr("class", ""); cl != "" {
			name += "." + strings.ReplaceAll(cl, " ", ".")
		}
	}
	switch b.Kind {
	case KText:
		return
	case KInline, KBreak:
		// Inline boxes have no geometry of their own; see the line boxes.
		fmt.Fprintf(w, "%s%s %s\n", ind, b.Kind, name)
	default:
		fmt.Fprintf(w, "%s%s %s x=%.1f y=%.1f w=%.1f h=%.1f\n", ind, b.Kind, name, b.X, b.Y, b.W, b.H)
	}
	for _, l := range b.Lines {
		var sb strings.Builder
		for _, f := range l.Frags {
			if f.Box == nil {
				sb.WriteString(f.Text)
			} else {
				sb.WriteString("[" + f.Box.Kind.String() + "]")
			}
		}
		t := sb.String()
		if len(t) > 60 {
			t = t[:60] + "…"
		}
		fmt.Fprintf(w, "%s  line y=%.1f h=%.1f %q\n", ind, l.Y, l.H, t)
	}
	for _, c := range b.Children {
		dump(w, c, depth+1)
	}
}
