package layout

import (
	"math"
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/style"
)

type ctx struct {
	quirks bool
}

// resolveEdges computes padding, border and margins (auto margins are 0).
func resolveEdges(b *Box, cbW float64) {
	s := b.Style
	for i := 0; i < 4; i++ {
		b.Padding[i] = math.Max(0, s.Padding[i].Resolve(cbW, 0))
		b.Border[i] = s.Border(i)
		b.Margin[i] = s.Margin[i].Resolve(cbW, 0)
	}
	if b.Kind == KInline || b.Kind == KText || b.Kind == KBreak {
		b.Margin, b.Padding, b.Border = [4]float64{}, [4]float64{}, [4]float64{}
	}
	if b.Kind == KRow {
		b.Padding, b.Border, b.Margin = [4]float64{}, [4]float64{}, [4]float64{}
	}
	if b.Kind == KCell {
		b.Margin = [4]float64{}
	}
}

// specifiedWidth returns the content width from the width property, if set.
func specifiedWidth(b *Box, cbW float64) (float64, bool) {
	s := b.Style
	if s.Width.IsAuto() || s.Width.Unit == style.Percent && cbW <= 0 && b.Kind != KTable {
		return 0, false
	}
	w := s.Width.Resolve(cbW, 0)
	if s.BorderBox || b.Kind == KTable {
		w -= b.pbH()
	}
	return math.Max(0, w), true
}

func clampWidth(b *Box, w, cbW float64) float64 {
	s := b.Style
	if !s.MaxWidth.IsAuto() {
		if mw := s.MaxWidth.Resolve(cbW, 0); w > mw {
			w = mw
		}
	}
	if mw := s.MinWidth.Resolve(cbW, 0); s.MinWidth.Unit != style.Auto && w < mw {
		w = mw
		if s.BorderBox || b.Kind == KTable {
			w = math.Max(w-b.pbH(), 0)
		}
	}
	return w
}

// horizontalPlacement sets b.X and margins for a block-level box of content
// width w inside a containing block, honouring auto margins and the legacy
// -webkit-center/-webkit-right alignment used by <center> and align=.
func horizontalPlacement(b *Box, cbX, cbW, w float64, autoWidth bool, parentAlign string) {
	s := b.Style
	free := cbW - w - b.pbH() - b.Margin[style.Left] - b.Margin[style.Right]
	if !autoWidth && free > 0 {
		la, ra := s.Margin[style.Left].IsAuto(), s.Margin[style.Right].IsAuto()
		switch {
		case la && ra:
			b.Margin[style.Left] += free / 2
			b.Margin[style.Right] += free / 2
		case la:
			b.Margin[style.Left] += free
		case !ra && b.Margin[style.Left] == 0 && b.Margin[style.Right] == 0:
			switch parentAlign {
			case "-webkit-center":
				b.Margin[style.Left] += free / 2
			case "-webkit-right":
				b.Margin[style.Left] += free
			}
		}
	}
	b.X = cbX + b.Margin[style.Left] + b.Border[style.Left] + b.Padding[style.Left]
	b.W = w
}

// layoutBlock lays out a block container whose margin box top is at y.
func (c *ctx) layoutBlock(b *Box, cbX, cbW, y float64, parentAlign string) {
	resolveEdges(b, cbW)
	w, ok := specifiedWidth(b, cbW)
	if !ok {
		w = cbW - b.pbH() - b.Margin[style.Left] - b.Margin[style.Right]
	}
	w = math.Max(0, clampWidth(b, w, cbW))
	horizontalPlacement(b, cbX, cbW, w, !ok, parentAlign)
	b.Y = y + b.Margin[style.Top] + b.Border[style.Top] + b.Padding[style.Top]
	h := c.layoutContents(b)
	b.H = c.usedHeight(b, h)
}

func (c *ctx) usedHeight(b *Box, contentH float64) float64 {
	s := b.Style
	h := contentH
	if s.Height.Unit == style.Px {
		h = s.Height.V
		if s.BorderBox || b.Kind == KTable {
			h -= b.pbV()
		}
		if b.Kind == KCell || b.Kind == KTable {
			// Cells and tables grow to fit their content.
			h = math.Max(h, contentH)
		}
	}
	if s.MinHeight.Unit == style.Px {
		h = math.Max(h, s.MinHeight.V)
	}
	return math.Max(0, h)
}

// layoutContents lays out the children of a block container whose X, Y and
// W are set, returning the content height.
func (c *ctx) layoutContents(b *Box) float64 {
	for _, ch := range b.Children {
		if isBlockLevel(ch) {
			return c.layoutBlockChildren(b)
		}
	}
	return c.layoutInline(b)
}

func (c *ctx) layoutBlockChildren(b *Box) float64 {
	cursor := b.Y // bottom of the previous border box
	prevMB := 0.0 // bottom margin of the previous box
	for _, ch := range b.Children {
		resolveEdges(ch, b.W)
		top := collapse(prevMB, ch.Margin[style.Top])
		y := cursor + top - ch.Margin[style.Top] // margin box top
		switch {
		case ch.Kind == KTable:
			c.layoutTable(ch, b.X, b.W, y, b.Style.TextAlign)
		case ch.Kind == KReplaced:
			c.layoutReplaced(ch, b.W)
			horizontalPlacement(ch, b.X, b.W, ch.W, false, b.Style.TextAlign)
			ch.Y = y + ch.Margin[style.Top] + ch.Border[style.Top] + ch.Padding[style.Top]
		default:
			c.layoutBlock(ch, b.X, b.W, y, b.Style.TextAlign)
		}
		_, by, _, bh := ch.BorderBox()
		if bh == 0 && len(ch.Children) == 0 {
			// Empty boxes let margins collapse through them.
			prevMB = collapse(top, ch.Margin[style.Bottom])
			cursor = by
			continue
		}
		cursor = by + bh
		prevMB = ch.Margin[style.Bottom]
	}
	return cursor + prevMB - b.Y
}

// collapse returns the collapsed value of two adjoining margins.
func collapse(a, b float64) float64 {
	switch {
	case a >= 0 && b >= 0:
		return math.Max(a, b)
	case a < 0 && b < 0:
		return math.Min(a, b)
	}
	return a + b
}

// layoutReplaced sets the content size of a replaced element.
func (c *ctx) layoutReplaced(b *Box, cbW float64) {
	resolveEdges(b, cbW)
	s := b.Style
	iw, ih := 0.0, 0.0
	hasIntrinsic := false
	if b.Image != nil {
		iw, ih = b.Image.Size()
		hasIntrinsic = iw > 0 && ih > 0
	}
	switch b.Node.Tag {
	case "input", "select", "button", "textarea":
		size := 20.0
		if v, ok := b.Node.Attr("size"); ok {
			if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && n > 0 && n < 1000 {
				size = n
			}
		}
		iw = size * s.FontSize * 0.55
		ih = s.FontSize * 1.2
		hasIntrinsic = true
	}
	w, wok := 0.0, false
	if !s.Width.IsAuto() && !(s.Width.Unit == style.Percent && cbW <= 0) {
		w, wok = s.Width.Resolve(cbW, 0), true
	}
	h, hok := 0.0, false
	if s.Height.Unit == style.Px {
		h, hok = s.Height.V, true
	}
	switch {
	case wok && hok:
	case wok:
		h = 0
		if hasIntrinsic {
			h = w * ih / iw
		}
	case hok:
		w = 0
		if hasIntrinsic {
			w = h * iw / ih
		}
	default:
		w, h = iw, ih
	}
	b.W, b.H = math.Max(0, w), math.Max(0, h)
}
