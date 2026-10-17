package layout

import (
	"math"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/font"
	"github.com/lukehoban/gobrowser2/internal/style"
)

type itemKind int

const (
	itWord itemKind = iota
	itSpace
	itAtomic
	itBreak
	itGlue // unbreakable horizontal space: inline margins, borders, padding
)

type item struct {
	kind      itemKind
	text      string
	style     *style.Style
	box       *Box
	w         float64
	breakable bool // spaces only: a line may break here
}

// FaceFor returns the font face for a style.
func FaceFor(s *style.Style) *font.Face {
	return font.Get(font.Key{Mono: s.Mono, Bold: s.Bold, Italic: s.Italic, Size: s.FontSize})
}

// lineMetrics returns the space above and below the baseline that an inline
// box with style s occupies, including half-leading.
func lineMetrics(s *style.Style) (above, below float64) {
	f := FaceFor(s)
	lh := s.LineHeight
	if lh == 0 {
		lh = f.Ascent + f.Descent
	}
	half := (lh - (f.Ascent + f.Descent)) / 2
	return f.Ascent + half, f.Descent + half
}

func preservesSpaces(ws string) bool {
	return ws == "pre" || ws == "pre-wrap" || ws == "break-spaces"
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// itemCollector flattens the inline content of a block container.
type itemCollector struct {
	c         *ctx
	items     []item
	lastSpace bool // the previous item ended in collapsible white space
	cbW       float64
	measure   bool // computing intrinsic widths: don't lay out atomics
}

func (ic *itemCollector) collect(b *Box) {
	for _, ch := range b.Children {
		switch ch.Kind {
		case KText:
			ic.text(ch.Text, ch.Style)
		case KInline:
			ic.glue(ch, style.Left)
			ic.collect(ch)
			ic.glue(ch, style.Right)
		case KBreak:
			ic.items = append(ic.items, item{kind: itBreak, style: ch.Style})
			ic.lastSpace = true
		case KReplaced, KInlineBlock, KTable:
			it := item{kind: itAtomic, box: ch, style: ch.Style}
			if !ic.measure {
				ic.c.layoutAtomic(ch, ic.cbW)
				_, _, bw, _ := ch.BorderBox()
				it.w = bw + ch.Margin[style.Left] + ch.Margin[style.Right]
			}
			ic.items = append(ic.items, it)
			ic.lastSpace = false
		default:
			// Stray block-level boxes shouldn't occur here; treat as atomic.
			ic.items = append(ic.items, item{kind: itAtomic, box: ch, style: ch.Style})
			ic.lastSpace = false
		}
	}
}

// glue adds the margin, border and padding on one side of an inline box.
func (ic *itemCollector) glue(b *Box, side int) {
	s := b.Style
	w := s.Margin[side].Resolve(ic.cbW, 0) + s.Border(side) + math.Max(0, s.Padding[side].Resolve(ic.cbW, 0))
	if w != 0 {
		ic.items = append(ic.items, item{kind: itGlue, style: s, w: w})
	}
}

func (ic *itemCollector) text(t string, s *style.Style) {
	face := FaceFor(s)
	ws := s.WhiteSpace
	if preservesSpaces(ws) {
		for i, line := range strings.Split(t, "\n") {
			if i > 0 {
				ic.items = append(ic.items, item{kind: itBreak, style: s})
			}
			line = strings.ReplaceAll(line, "\t", "        ")
			for len(line) > 0 {
				k := strings.IndexByte(line, ' ')
				switch {
				case k < 0:
					ic.word(line, s, face)
					line = ""
				case k == 0:
					ic.items = append(ic.items, item{kind: itSpace, text: " ", style: s, w: face.Measure(" "), breakable: ws != "pre"})
					line = line[1:]
				default:
					ic.word(line[:k], s, face)
					line = line[k:]
				}
			}
		}
		ic.lastSpace = false
		return
	}
	breakable := ws != "nowrap"
	if ws == "pre-line" {
		parts := strings.Split(t, "\n")
		for i, p := range parts {
			if i > 0 {
				ic.items = append(ic.items, item{kind: itBreak, style: s})
				ic.lastSpace = true
			}
			ic.collapsed(p, s, face, breakable)
		}
		return
	}
	ic.collapsed(t, s, face, breakable)
}

func (ic *itemCollector) collapsed(t string, s *style.Style, face *font.Face, breakable bool) {
	i := 0
	for i < len(t) {
		if isSpaceByte(t[i]) {
			for i < len(t) && isSpaceByte(t[i]) {
				i++
			}
			if !ic.lastSpace {
				ic.items = append(ic.items, item{kind: itSpace, text: " ", style: s, w: face.Measure(" "), breakable: breakable})
				ic.lastSpace = true
			}
			continue
		}
		j := i
		for j < len(t) && !isSpaceByte(t[j]) {
			j++
		}
		ic.word(t[i:j], s, face)
		i = j
	}
}

func (ic *itemCollector) word(w string, s *style.Style, face *font.Face) {
	ic.items = append(ic.items, item{kind: itWord, text: w, style: s, w: face.Measure(w)})
	ic.lastSpace = false
}

// layoutAtomic lays out an atomic inline at the origin; it is translated
// into place once its line is positioned.
func (c *ctx) layoutAtomic(b *Box, cbW float64) {
	switch b.Kind {
	case KReplaced:
		c.layoutReplaced(b, cbW)
		b.X = b.Margin[style.Left] + b.Border[style.Left] + b.Padding[style.Left]
		b.Y = b.Margin[style.Top] + b.Border[style.Top] + b.Padding[style.Top]
	case KTable:
		c.layoutTable(b, 0, cbW, 0, "")
	default:
		resolveEdges(b, cbW)
		w, ok := specifiedWidth(b, cbW)
		if !ok {
			// Shrink-to-fit.
			mn, mx := c.contentMinMax(b)
			avail := cbW - b.pbH() - b.Margin[style.Left] - b.Margin[style.Right]
			w = math.Min(math.Max(mn, avail), mx)
		}
		w = math.Max(0, clampWidth(b, w, cbW))
		b.X = b.Margin[style.Left] + b.Border[style.Left] + b.Padding[style.Left]
		b.W = w
		b.Y = b.Margin[style.Top] + b.Border[style.Top] + b.Padding[style.Top]
		b.H = c.usedHeight(b, c.layoutContents(b))
	}
}

type placed struct {
	it item
	x  float64
}

// layoutInline lays out the inline content of b into line boxes and returns
// their total height.
func (c *ctx) layoutInline(b *Box) float64 {
	ic := &itemCollector{c: c, lastSpace: true, cbW: b.W}
	ic.collect(b)
	items := ic.items
	b.Lines = nil

	var line []placed
	x := 0.0
	var pending *item
	y := b.Y
	flush := func(forced bool) {
		if len(line) == 0 && !forced {
			return
		}
		y += c.finishLine(b, line, x, y)
		line, x, pending = nil, 0, nil
	}
	for i := 0; i < len(items); {
		it := items[i]
		switch {
		case it.kind == itBreak:
			flush(true)
			i++
			continue
		case it.kind == itSpace && it.breakable:
			if len(line) > 0 {
				p := it
				pending = &p
			}
			i++
			continue
		}
		// Gather an unbreakable segment.
		j, segW := i, 0.0
		for j < len(items) && items[j].kind != itBreak && !(items[j].kind == itSpace && items[j].breakable) {
			segW += items[j].w
			j++
		}
		spaceW := 0.0
		if pending != nil {
			spaceW = pending.w
		}
		if len(line) > 0 && x+spaceW+segW > b.W+0.01 {
			flush(false)
			spaceW = 0
		}
		if pending != nil && len(line) > 0 {
			line = append(line, placed{*pending, x})
			x += spaceW
		}
		pending = nil
		for k := i; k < j; k++ {
			line = append(line, placed{items[k], x})
			x += items[k].w
		}
		i = j
	}
	flush(false)
	return y - b.Y
}

// finishLine positions the items of one line and returns its height.
func (c *ctx) finishLine(b *Box, line []placed, width, y float64) float64 {
	hasText := false
	for _, p := range line {
		if p.it.kind == itWord || p.it.kind == itSpace {
			hasText = true
			break
		}
	}
	above, below := 0.0, 0.0
	// The strut. In quirks mode, lines holding only images don't get one
	// (an empty line from a <br> still does).
	if !c.quirks || hasText || len(line) == 0 {
		above, below = lineMetrics(b.Style)
	}
	pos := make([]float64, len(line))
	for i, p := range line {
		var a, d float64
		switch p.it.kind {
		case itWord, itSpace:
			a, d = lineMetrics(p.it.style)
		case itAtomic:
			bx := p.it.box
			_, _, _, bh := bx.BorderBox()
			h := bh + bx.Margin[style.Top] + bx.Margin[style.Bottom]
			switch bx.Style.VerticalAlign {
			case "middle":
				a = h/2 + b.Style.FontSize*0.25
				d = h - a
			default:
				a, d = h, 0
			}
		}
		pos[i] = a
		above = math.Max(above, a)
		below = math.Max(below, d)
	}
	baseline := y + above
	off := 0.0
	if free := b.W - width; free > 0 {
		switch b.Style.TextAlign {
		case "right", "-webkit-right":
			off = free
		case "center", "-webkit-center":
			off = free / 2
		}
	}
	ln := &Line{X: b.X, Y: y, W: b.W, H: above + below, Baseline: baseline}
	for i, p := range line {
		fx := b.X + off + p.x
		switch p.it.kind {
		case itGlue:
			continue
		case itAtomic:
			bx := p.it.box
			bxX, bxY, _, _ := bx.BorderBox()
			top := baseline - pos[i]
			translate(bx, fx+bx.Margin[style.Left]-bxX, top+bx.Margin[style.Top]-bxY)
			ln.Frags = append(ln.Frags, &Frag{X: fx, Y: top, W: p.it.w, Box: bx, Style: bx.Style})
		default:
			if n := len(ln.Frags); n > 0 {
				prev := ln.Frags[n-1]
				if prev.Box == nil && prev.Style == p.it.style && math.Abs(prev.X+prev.W-fx) < 0.01 {
					prev.Text += p.it.text
					prev.W += p.it.w
					continue
				}
			}
			ln.Frags = append(ln.Frags, &Frag{X: fx, Y: baseline, W: p.it.w, Text: p.it.text, Style: p.it.style})
		}
	}
	b.Lines = append(b.Lines, ln)
	return ln.H
}

// contentMinMax returns the min-content and max-content widths of b's
// content box.
func (c *ctx) contentMinMax(b *Box) (float64, float64) {
	if b.minMax != nil {
		return b.minMax[0], b.minMax[1]
	}
	var mn, mx float64
	switch {
	case b.Kind == KTable:
		mn, mx = c.tableMinMax(b)
	case hasBlockChild(b):
		for _, ch := range b.Children {
			cmn, cmx := c.outerMinMax(ch)
			mn = math.Max(mn, cmn)
			mx = math.Max(mx, cmx)
		}
	default:
		ic := &itemCollector{c: c, lastSpace: true, measure: true}
		ic.collect(b)
		seg, lineW := 0.0, 0.0
		for _, it := range ic.items {
			w := it.w
			if it.kind == itAtomic {
				amn, amx := c.outerMinMax(it.box)
				seg += amn
				lineW += amx
				continue
			}
			switch {
			case it.kind == itBreak:
				mn = math.Max(mn, seg)
				mx = math.Max(mx, lineW)
				seg, lineW = 0, 0
			case it.kind == itSpace && it.breakable:
				mn = math.Max(mn, seg)
				seg = 0
				lineW += w
			default:
				seg += w
				lineW += w
			}
		}
		mn = math.Max(mn, seg)
		mx = math.Max(mx, lineW)
	}
	b.minMax = &[2]float64{mn, mx}
	return mn, mx
}

func hasBlockChild(b *Box) bool {
	for _, ch := range b.Children {
		if isBlockLevel(ch) {
			return true
		}
	}
	return false
}

// outerMinMax returns the min/max-content contribution of b, including its
// margins, borders and padding.
func (c *ctx) outerMinMax(b *Box) (float64, float64) {
	switch b.Kind {
	case KText, KInline, KBreak:
		return 0, 0
	}
	resolveEdges(b, 0)
	pbm := b.pbH() + b.Margin[style.Left] + b.Margin[style.Right]
	if b.Kind == KReplaced {
		c.layoutReplaced(b, 0)
		return b.W + pbm, b.W + pbm
	}
	mn, mx := c.contentMinMax(b)
	s := b.Style
	if s.Width.Unit == style.Px {
		w := s.Width.V
		if s.BorderBox || b.Kind == KTable {
			w -= b.pbH()
		}
		mn = math.Max(mn, w)
		mx = math.Max(mn, w)
	}
	if s.MinWidth.Unit == style.Px {
		mw := s.MinWidth.V
		if s.BorderBox || b.Kind == KTable {
			mw -= b.pbH()
		}
		mn = math.Max(mn, mw)
		mx = math.Max(mx, mw)
	}
	return mn + pbm, mx + pbm
}
