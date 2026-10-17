// Package layout builds a box tree from a styled DOM and computes the
// geometry of every box: block layout, inline layout with line breaking,
// and table layout.
package layout

import (
	"image"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
	"github.com/lukehoban/gobrowser2/internal/fonts"
	"github.com/lukehoban/gobrowser2/internal/style"
)

// BoxType identifies the kind of a Box.
type BoxType int

const (
	BlockBox    BoxType = iota // block container (incl. anonymous blocks, list items)
	InlineBlock                // atomic inline block container
	ReplacedBox                // image or other replaced content
	TableBox                   // table wrapper (block-level or inline-level)
	RowGroupBox                // tbody/thead/tfoot or anonymous
	RowBox                     // table row
	CellBox                    // table cell (block container)
)

// Edges holds per-side lengths in px.
type Edges struct{ Top, Right, Bottom, Left float64 }

// Horizontal returns Left+Right.
func (e Edges) Horizontal() float64 { return e.Left + e.Right }

// Vertical returns Top+Bottom.
func (e Edges) Vertical() float64 { return e.Top + e.Bottom }

// Box is a node in the layout tree.
type Box struct {
	Type  BoxType
	Node  *dom.Node // nil for anonymous boxes
	Style *style.Style

	Children []*Box        // block-level children or table structure
	Inline   []*InlineItem // inline content, for block containers in an inline formatting context

	// Geometry, filled in by layout. X, Y, W, H is the content box in
	// absolute page coordinates.
	X, Y, W, H              float64
	Margin, Border, Padding Edges
	Lines                   []*Line

	Image   image.Image // decoded image for replaced boxes
	Marker  string      // list item marker text
	ColSpan int         // for cells

	intrinsic *[2]float64 // cached min/max-content border-box widths
}

// BorderBox returns the border box rectangle.
func (b *Box) BorderBox() (x, y, w, h float64) {
	return b.X - b.Padding.Left - b.Border.Left, b.Y - b.Padding.Top - b.Border.Top,
		b.W + b.Padding.Horizontal() + b.Border.Horizontal(), b.H + b.Padding.Vertical() + b.Border.Vertical()
}

// MarginBoxHeight returns the height of the margin box.
func (b *Box) MarginBoxHeight() float64 {
	return b.H + b.Padding.Vertical() + b.Border.Vertical() + b.Margin.Vertical()
}

// InlineKind identifies the kind of an InlineItem.
type InlineKind int

const (
	TextItem   InlineKind = iota
	AtomicItem            // inline-block, image, inline-table
	BreakItem             // <br>
	OpenItem              // start of an inline element
	CloseItem             // end of an inline element
)

// InlineItem is an element of inline content.
type InlineItem struct {
	Kind  InlineKind
	Text  string
	Style *style.Style
	Node  *dom.Node
	Box   *Box // for AtomicItem
}

// Line is a laid-out line box.
type Line struct {
	Y, Height, Baseline float64 // absolute
	Frags               []*Frag
}

// Frag is a positioned piece of inline content within a line.
type Frag struct {
	X, W     float64
	Baseline float64 // absolute y of the baseline
	Text     string  // text frags only
	Style    *style.Style
	Face     *fonts.Face
	Box      *Box // atomic frags only
	Node     *dom.Node
	space    bool // collapsible whitespace
	spacing  bool // inline start/end spacing
}

// Resources supplies externally loaded content (images) to layout.
type Resources struct {
	Images map[*dom.Node]image.Image
}

// builder constructs the box tree.
type builder struct {
	styles style.Styles
	res    *Resources
}

// entry is either an inline item or a block-level box, produced while
// collecting the children of a block container.
type entry struct {
	item *InlineItem
	box  *Box
}

func isBlockLevel(d style.Display) bool {
	switch d {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayTable, style.DisplayTableRowGroup,
		style.DisplayTableRow, style.DisplayTableCell, style.DisplayTableCaption:
		return true
	}
	return false
}

// Build creates the box tree for the document's root element.
func Build(doc *dom.Node, styles style.Styles, res *Resources) *Box {
	if res == nil {
		res = &Resources{}
	}
	b := &builder{styles: styles, res: res}
	root := doc.Find("html")
	if root == nil {
		return nil
	}
	st := styles[root]
	if st.Display == style.DisplayNone {
		return nil
	}
	box := &Box{Type: BlockBox, Node: root, Style: st}
	b.blockContainer(box, root)
	return box
}

// boxFor creates the box for an element whose display is not inline.
func (b *builder) boxFor(n *dom.Node, st *style.Style) *Box {
	if replaced := b.replaced(n, st); replaced != nil {
		return replaced
	}
	switch st.Display {
	case style.DisplayTable, style.DisplayInlineTable:
		box := &Box{Type: TableBox, Node: n, Style: st}
		b.tableChildren(box, n)
		return box
	case style.DisplayTableRowGroup:
		box := &Box{Type: RowGroupBox, Node: n, Style: st}
		b.rowGroupChildren(box, n)
		return box
	case style.DisplayTableRow:
		box := &Box{Type: RowBox, Node: n, Style: st}
		b.rowChildren(box, n)
		return box
	case style.DisplayTableCell:
		box := &Box{Type: CellBox, Node: n, Style: st, ColSpan: colspan(n)}
		b.blockContainer(box, n)
		return box
	case style.DisplayInlineBlock:
		box := &Box{Type: InlineBlock, Node: n, Style: st}
		b.formControl(box, n)
		return box
	}
	box := &Box{Type: BlockBox, Node: n, Style: st}
	if st.Display == style.DisplayListItem {
		box.Marker = listMarker(n, st)
	}
	b.blockContainer(box, n)
	return box
}

func colspan(n *dom.Node) int {
	v, _ := n.Attr("colspan")
	c := 0
	for _, ch := range strings.TrimSpace(v) {
		if ch < '0' || ch > '9' {
			break
		}
		c = c*10 + int(ch-'0')
		if c > 1000 {
			break
		}
	}
	if c < 1 {
		c = 1
	}
	return c
}

func listMarker(n *dom.Node, st *style.Style) string {
	switch st.ListStyleType {
	case "none":
		return ""
	case "decimal":
		idx := 1
		if n.Parent != nil {
			if v, ok := n.Parent.Attr("start"); ok {
				var s int
				for _, ch := range strings.TrimSpace(v) {
					if ch < '0' || ch > '9' {
						break
					}
					s = s*10 + int(ch-'0')
				}
				if s > 0 {
					idx = s
				}
			}
			for _, c := range n.Parent.ElementChildren() {
				if c == n {
					break
				}
				if c.Tag == "li" {
					idx++
				}
			}
		}
		return itoa(idx) + ". "
	case "circle":
		return "◦ "
	case "square":
		return "▪ "
	}
	return "• "
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	p := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		p--
		buf[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		buf[p] = '-'
	}
	return string(buf[p:])
}

// replaced returns a replaced box for images, or nil.
func (b *builder) replaced(n *dom.Node, st *style.Style) *Box {
	if n.Tag != "img" {
		return nil
	}
	return &Box{Type: ReplacedBox, Node: n, Style: st, Image: b.res.Images[n]}
}

// formControl fills an inline-block box for form controls with their
// visible text; other inline-blocks get their normal children.
func (b *builder) formControl(box *Box, n *dom.Node) {
	text := ""
	switch n.Tag {
	case "input":
		typ, _ := n.Attr("type")
		v, hasValue := n.Attr("value")
		switch strings.ToLower(typ) {
		case "submit":
			text = "Submit"
		case "reset":
			text = "Reset"
		case "checkbox", "radio":
			text = ""
		}
		if hasValue {
			text = v
		}
	case "textarea":
		text = n.TextContent()
	case "select":
		if opt := n.Find("option"); opt != nil {
			text = opt.TextContent()
		}
	default:
		b.blockContainer(box, n)
		return
	}
	textStyle := *box.Style
	if text == "" {
		text = "\u00a0"
	}
	box.Inline = []*InlineItem{{Kind: TextItem, Text: text, Style: &textStyle, Node: n}}
}

// blockContainer fills box with block-level children or inline content.
func (b *builder) blockContainer(box *Box, n *dom.Node) {
	var entries []entry
	for _, c := range n.Children {
		entries = b.collect(entries, c)
	}
	hasBlock := false
	for _, e := range entries {
		if e.box != nil {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		box.Inline = items(entries)
		return
	}
	var run []*InlineItem
	flush := func() {
		if len(run) > 0 && !whitespaceOnly(run) {
			anon := &Box{Type: BlockBox, Style: anonStyle(box.Style), Inline: run}
			box.Children = append(box.Children, anon)
		}
		run = nil
	}
	for _, e := range entries {
		if e.box != nil {
			flush()
			box.Children = append(box.Children, e.box)
		} else {
			run = append(run, e.item)
		}
	}
	flush()
}

func items(entries []entry) []*InlineItem {
	out := make([]*InlineItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.item)
	}
	return out
}

func whitespaceOnly(items []*InlineItem) bool {
	for _, it := range items {
		switch it.Kind {
		case TextItem:
			if strings.Trim(it.Text, " \t\n\r\f") != "" {
				return false
			}
		case AtomicItem, BreakItem:
			return false
		}
	}
	return true
}

// anonStyle derives the style of an anonymous block from its parent:
// inherited properties only.
func anonStyle(parent *style.Style) *style.Style {
	st := *parent
	st.Display = style.DisplayBlock
	st.BackgroundColor.A = 0
	st.BackgroundImage = ""
	st.Margin = [4]style.Length{{Unit: style.Px}, {Unit: style.Px}, {Unit: style.Px}, {Unit: style.Px}}
	st.Padding = st.Margin
	st.BorderWidth = [4]float64{}
	st.Width, st.Height = style.Length{}, style.Length{}
	st.MinWidth, st.MaxWidth, st.MinHeight = style.Length{}, style.Length{}, style.Length{}
	return &st
}

// collect appends the entries generated by node n.
func (b *builder) collect(out []entry, n *dom.Node) []entry {
	switch n.Type {
	case dom.TextNode:
		st := b.styles.Of(n)
		return append(out, entry{item: &InlineItem{Kind: TextItem, Text: n.Text, Style: st, Node: n.Parent}})
	case dom.ElementNode:
	default:
		return out
	}
	st := b.styles[n]
	if st == nil || st.Display == style.DisplayNone {
		return out
	}
	if n.Tag == "br" {
		return append(out, entry{item: &InlineItem{Kind: BreakItem, Style: st, Node: n}})
	}
	if isBlockLevel(st.Display) {
		return append(out, entry{box: b.boxFor(n, st)})
	}
	if st.Display == style.DisplayInlineBlock || st.Display == style.DisplayInlineTable || n.Tag == "img" {
		return append(out, entry{item: &InlineItem{Kind: AtomicItem, Style: st, Node: n, Box: b.boxFor(n, st)}})
	}
	// Inline element.
	out = append(out, entry{item: &InlineItem{Kind: OpenItem, Style: st, Node: n}})
	for _, c := range n.Children {
		out = b.collect(out, c)
	}
	return append(out, entry{item: &InlineItem{Kind: CloseItem, Style: st, Node: n}})
}

// tableChildren collects row groups (wrapping stray rows) for a table.
func (b *builder) tableChildren(table *Box, n *dom.Node) {
	var anon *Box
	var visit func(n *dom.Node)
	visit = func(n *dom.Node) {
		for _, c := range n.Children {
			if c.Type != dom.ElementNode {
				continue
			}
			st := b.styles[c]
			if st == nil || st.Display == style.DisplayNone {
				continue
			}
			switch st.Display {
			case style.DisplayTableRowGroup:
				anon = nil
				table.Children = append(table.Children, b.boxFor(c, st))
			case style.DisplayTableRow, style.DisplayTableCell:
				if anon == nil {
					anon = &Box{Type: RowGroupBox, Style: anonStyle(table.Style)}
					table.Children = append(table.Children, anon)
				}
				if st.Display == style.DisplayTableRow {
					anon.Children = append(anon.Children, b.boxFor(c, st))
				} else {
					row := &Box{Type: RowBox, Style: anonStyle(table.Style)}
					row.Children = append(row.Children, b.boxFor(c, st))
					anon.Children = append(anon.Children, row)
				}
			case style.DisplayTableCaption:
				// Captions are not yet supported; skip.
			default:
				// Transparent wrappers such as <form> inside tables.
				visit(c)
			}
		}
	}
	visit(n)
}

func (b *builder) rowGroupChildren(group *Box, n *dom.Node) {
	var visit func(n *dom.Node)
	visit = func(n *dom.Node) {
		for _, c := range n.Children {
			if c.Type != dom.ElementNode {
				continue
			}
			st := b.styles[c]
			if st == nil || st.Display == style.DisplayNone {
				continue
			}
			switch st.Display {
			case style.DisplayTableRow:
				group.Children = append(group.Children, b.boxFor(c, st))
			case style.DisplayTableCell:
				row := &Box{Type: RowBox, Style: anonStyle(group.Style)}
				row.Children = append(row.Children, b.boxFor(c, st))
				group.Children = append(group.Children, row)
			default:
				visit(c)
			}
		}
	}
	visit(n)
}

func (b *builder) rowChildren(row *Box, n *dom.Node) {
	var visit func(n *dom.Node)
	visit = func(n *dom.Node) {
		for _, c := range n.Children {
			if c.Type != dom.ElementNode {
				continue
			}
			st := b.styles[c]
			if st == nil || st.Display == style.DisplayNone {
				continue
			}
			if st.Display == style.DisplayTableCell {
				row.Children = append(row.Children, b.boxFor(c, st))
			} else {
				visit(c)
			}
		}
	}
	visit(n)
}
