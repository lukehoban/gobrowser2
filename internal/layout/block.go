package layout

import (
	"math"

	"github.com/lukehoban/gobrowser2/internal/style"
)

// Layout lays out the box tree for a viewport of the given width and
// returns the document height.
func Layout(root *Box, viewportWidth float64) float64 {
	if root == nil {
		return 0
	}
	resolveEdges(root, viewportWidth)
	layoutBlockLevel(root, 0, 0, viewportWidth, false)
	_, y, _, h := root.BorderBox()
	return y + h + root.Margin.Bottom
}

// resolveEdges computes margins (auto as 0), borders and padding.
func resolveEdges(b *Box, cbW float64) {
	st := b.Style
	b.Padding = Edges{
		st.Padding[0].Resolve(cbW, 0), st.Padding[1].Resolve(cbW, 0),
		st.Padding[2].Resolve(cbW, 0), st.Padding[3].Resolve(cbW, 0),
	}
	b.Border = Edges{st.BorderWidth[0], st.BorderWidth[1], st.BorderWidth[2], st.BorderWidth[3]}
	b.Margin = Edges{
		st.Margin[0].Resolve(cbW, 0), st.Margin[1].Resolve(cbW, 0),
		st.Margin[2].Resolve(cbW, 0), st.Margin[3].Resolve(cbW, 0),
	}
	if b.Type == RowGroupBox || b.Type == RowBox {
		b.Padding, b.Border, b.Margin = Edges{}, Edges{}, Edges{}
	}
	if b.Type == CellBox {
		b.Margin = Edges{}
	}
}

// layoutBlockLevel lays out a block-level box whose top-left margin edge is
// at (x, y) inside a containing block of width cbW. center requests
// -webkit-center alignment of narrower boxes.
func layoutBlockLevel(b *Box, x, y, cbW float64, center bool) {
	resolveEdges(b, cbW)
	var w float64
	switch b.Type {
	case TableBox:
		w = tableWidth(b, cbW)
	case ReplacedBox:
		w, b.H = replacedSize(b, cbW)
	default:
		w = blockWidth(b, cbW)
	}
	b.W = w
	placeHorizontally(b, x, cbW, center)
	b.Y = y + b.Margin.Top + b.Border.Top + b.Padding.Top
	switch b.Type {
	case TableBox:
		layoutTable(b)
	case ReplacedBox:
	default:
		layoutBlockContents(b)
	}
}

// blockWidth computes the content width of a block box in normal flow.
func blockWidth(b *Box, cbW float64) float64 {
	st := b.Style
	pb := b.Padding.Horizontal() + b.Border.Horizontal()
	var w float64
	if st.Width.IsAuto() {
		w = cbW - b.Margin.Horizontal() - pb
	} else {
		w = st.Width.Resolve(cbW, 0)
		if st.BoxSizingBorder {
			w -= pb
		}
	}
	return clampWidth(b, w, cbW)
}

// clampWidth applies min-width and max-width to a content width.
func clampWidth(b *Box, w, cbW float64) float64 {
	st := b.Style
	pb := 0.0
	if st.BoxSizingBorder {
		pb = b.Padding.Horizontal() + b.Border.Horizontal()
	}
	if !st.MaxWidth.IsAuto() {
		w = math.Min(w, st.MaxWidth.Resolve(cbW, 0)-pb)
	}
	if !st.MinWidth.IsAuto() {
		w = math.Max(w, st.MinWidth.Resolve(cbW, 0)-pb)
	}
	return math.Max(0, w)
}

// placeHorizontally resolves auto margins and sets b.X.
func placeHorizontally(b *Box, x, cbW float64, center bool) {
	st := b.Style
	used := b.W + b.Padding.Horizontal() + b.Border.Horizontal() + b.Margin.Horizontal()
	remaining := cbW - used
	ml := b.Margin.Left
	switch {
	case st.Margin[1].IsAuto() && st.Margin[3].IsAuto():
		if remaining > 0 {
			ml += remaining / 2
		}
	case st.Margin[3].IsAuto():
		if remaining > 0 {
			ml += remaining
		}
	case st.Margin[1].IsAuto():
	case center && remaining > 0:
		ml += remaining / 2
	}
	b.Margin.Left = ml
	b.X = x + ml + b.Border.Left + b.Padding.Left
}

// layoutBlockContents lays out the children (or inline content) of a block
// container whose X, Y and W are known, and sets its height.
func layoutBlockContents(b *Box) {
	var h float64
	if len(b.Children) == 0 {
		h = layoutInline(b)
	} else {
		center := b.Style.TextAlign == "-webkit-center"
		cursor := b.Y
		prevMB := 0.0
		for i, c := range b.Children {
			resolveEdges(c, b.W)
			y := cursor
			if i > 0 {
				y = cursor + math.Max(prevMB, c.Margin.Top) - c.Margin.Top
			}
			layoutBlockLevel(c, b.X, y, b.W, center)
			_, by, _, bh := c.BorderBox()
			cursor = by + bh
			prevMB = c.Margin.Bottom
		}
		h = cursor + prevMB - b.Y
	}
	b.H = usedHeight(b, h)
}

// usedHeight applies height, min-height to a content height.
func usedHeight(b *Box, h float64) float64 {
	st := b.Style
	pb := 0.0
	if st.BoxSizingBorder {
		pb = b.Padding.Vertical() + b.Border.Vertical()
	}
	if st.Height.Unit == style.Px {
		h = st.Height.Value - pb
	}
	if st.MinHeight.Unit == style.Px {
		h = math.Max(h, st.MinHeight.Value-pb)
	}
	return math.Max(0, h)
}

// replacedSize computes the content size of a replaced element.
func replacedSize(b *Box, cbW float64) (w, h float64) {
	st := b.Style
	var nw, nh float64
	if b.Image != nil {
		r := b.Image.Bounds()
		nw, nh = float64(r.Dx()), float64(r.Dy())
	}
	hasW := !st.Width.IsAuto() && !(st.Width.Unit == style.Percent && cbW <= 0)
	hasH := st.Height.Unit == style.Px
	if hasW {
		w = st.Width.Resolve(cbW, 0)
	}
	if hasH {
		h = st.Height.Value
	}
	switch {
	case hasW && hasH:
	case hasW:
		if nw > 0 {
			h = w * nh / nw
		}
	case hasH:
		if nh > 0 {
			w = h * nw / nh
		}
	default:
		w, h = nw, nh
	}
	if st.BoxSizingBorder {
		w = math.Max(0, w-b.Padding.Horizontal()-b.Border.Horizontal())
		h = math.Max(0, h-b.Padding.Vertical()-b.Border.Vertical())
	}
	return clampWidth(b, w, cbW), h
}

// shrinkToFit lays out an atomic inline-level box (inline-block, image or
// inline table) with its top-left margin edge at (0, 0), given the
// available width.
func layoutAtomic(b *Box, avail float64) {
	resolveEdges(b, avail)
	switch b.Type {
	case ReplacedBox:
		b.W, b.H = replacedSize(b, avail)
		b.X = b.Margin.Left + b.Border.Left + b.Padding.Left
		b.Y = b.Margin.Top + b.Border.Top + b.Padding.Top
		return
	case TableBox:
		b.W = tableWidth(b, avail)
		b.X = b.Margin.Left + b.Border.Left + b.Padding.Left
		b.Y = b.Margin.Top + b.Border.Top + b.Padding.Top
		layoutTable(b)
		return
	}
	st := b.Style
	pb := b.Padding.Horizontal() + b.Border.Horizontal()
	var w float64
	if st.Width.IsAuto() {
		minW, maxW := intrinsic(b)
		mh := b.Margin.Horizontal()
		w = math.Min(math.Max(minW-mh, avail-mh), maxW-mh) - pb
	} else {
		w = st.Width.Resolve(avail, 0)
		if st.BoxSizingBorder {
			w -= pb
		}
	}
	b.W = clampWidth(b, w, avail)
	b.X = b.Margin.Left + b.Border.Left + b.Padding.Left
	b.Y = b.Margin.Top + b.Border.Top + b.Padding.Top
	layoutBlockContents(b)
}

// intrinsic returns the min-content and max-content widths of b's margin
// box.
func intrinsic(b *Box) (minW, maxW float64) {
	if b.intrinsic != nil {
		return b.intrinsic[0], b.intrinsic[1]
	}
	resolveEdges(b, 0)
	st := b.Style
	pb := b.Padding.Horizontal() + b.Border.Horizontal()
	switch b.Type {
	case ReplacedBox:
		w, _ := replacedSize(b, 0)
		minW, maxW = w+pb, w+pb
		if st.Width.Unit == style.Percent {
			minW = pb
		}
	case TableBox:
		minW, maxW = tableIntrinsic(b)
	default:
		// Cells fold in their specified width during table layout.
		if st.Width.Unit == style.Px && b.Type != CellBox {
			w := st.Width.Value
			if !st.BoxSizingBorder {
				w += pb
			}
			minW, maxW = w, w
		} else {
			if len(b.Children) == 0 {
				minW, maxW = inlineIntrinsic(b)
			} else {
				for _, c := range b.Children {
					cmin, cmax := intrinsic(c)
					minW = math.Max(minW, cmin)
					maxW = math.Max(maxW, cmax)
				}
			}
			minW += pb
			maxW += pb
		}
	}
	if st.MinWidth.Unit == style.Px {
		mw := st.MinWidth.Value
		if !st.BoxSizingBorder {
			mw += pb
		}
		minW = math.Max(minW, mw)
		maxW = math.Max(maxW, mw)
	}
	maxW = math.Max(minW, maxW)
	mh := 0.0
	if st.Margin[1].Unit == style.Px {
		mh += st.Margin[1].Value
	}
	if st.Margin[3].Unit == style.Px {
		mh += st.Margin[3].Value
	}
	if b.Type != CellBox && b.Type != RowBox && b.Type != RowGroupBox {
		minW += mh
		maxW += mh
	}
	b.intrinsic = &[2]float64{minW, maxW}
	return minW, maxW
}

// translate moves b and all of its descendants by (dx, dy).
func translate(b *Box, dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	b.X += dx
	b.Y += dy
	for _, c := range b.Children {
		translate(c, dx, dy)
	}
	for _, l := range b.Lines {
		l.Y += dy
		l.Baseline += dy
		for _, f := range l.Frags {
			f.X += dx
			f.Baseline += dy
			if f.Box != nil {
				translate(f.Box, dx, dy)
			}
		}
	}
}
