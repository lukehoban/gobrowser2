package layout

import (
	"math"
	"strings"
	"unicode"

	"github.com/lukehoban/gobrowser2/internal/fonts"
	"github.com/lukehoban/gobrowser2/internal/style"
)

type pieceKind int

const (
	wordPiece pieceKind = iota
	spacePiece
	atomicPiece
	spacingPiece // inline element start/end margin+border+padding
	breakPiece
)

type piece struct {
	kind        pieceKind
	text        string
	w           float64
	style       *style.Style
	face        *fonts.Face
	item        *InlineItem
	collapsible bool // whitespace that disappears at line edges
	breakAfter  bool // soft wrap opportunity after this piece
}

// FaceFor returns the font face for a style.
func FaceFor(st *style.Style) *fonts.Face {
	return fonts.Get(st.FontFamily, st.FontSize, st.Bold, st.Italic)
}

// lineMetrics returns the ascent+half-leading above the baseline and the
// remainder below it for text in the given style.
func lineMetrics(st *style.Style) (above, below float64) {
	f := FaceFor(st)
	lh := st.LineHeight
	if lh <= 0 {
		lh = math.Round(st.FontSize * 1.2)
	}
	half := (lh - (f.Ascent + f.Descent)) / 2
	above = f.Ascent + half
	return above, lh - above
}

func transformText(s, tt string) string {
	switch tt {
	case "uppercase":
		return strings.ToUpper(s)
	case "lowercase":
		return strings.ToLower(s)
	case "capitalize":
		prev := ' '
		return strings.Map(func(r rune) rune {
			out := r
			if unicode.IsSpace(prev) {
				out = unicode.ToTitle(r)
			}
			prev = r
			return out
		}, s)
	}
	return s
}

func isCollapsibleSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// pieces converts inline items into measured pieces, applying whitespace
// processing. atomicWidth supplies the margin-box width of atomic items.
func pieces(items []*InlineItem, atomicWidth func(*Box) float64) []*piece {
	var out []*piece
	lastSpace := true
	markBreak := func() {
		if len(out) > 0 {
			out[len(out)-1].breakAfter = true
		}
	}
	for _, it := range items {
		st := it.Style
		switch it.Kind {
		case BreakItem:
			out = append(out, &piece{kind: breakPiece, style: st, item: it})
			lastSpace = true
		case OpenItem, CloseItem:
			var w float64
			if it.Kind == OpenItem {
				w = st.Margin[3].Resolve(0, 0) + st.BorderWidth[3] + st.Padding[3].Resolve(0, 0)
			} else {
				w = st.Margin[1].Resolve(0, 0) + st.BorderWidth[1] + st.Padding[1].Resolve(0, 0)
			}
			out = append(out, &piece{kind: spacingPiece, w: w, style: st, item: it})
		case AtomicItem:
			wrap := st.WhiteSpace != "nowrap" && st.WhiteSpace != "pre"
			if wrap {
				markBreak()
			}
			out = append(out, &piece{kind: atomicPiece, w: atomicWidth(it.Box), style: st, item: it, breakAfter: wrap})
			lastSpace = false
		case TextItem:
			face := FaceFor(st)
			text := transformText(it.Text, st.TextTransform)
			ws := st.WhiteSpace
			collapse := ws != "pre" && ws != "pre-wrap"
			wrap := ws != "nowrap" && ws != "pre"
			keepNewlines := ws == "pre" || ws == "pre-wrap" || ws == "pre-line"
			var word strings.Builder
			flushWord := func() {
				if word.Len() == 0 {
					return
				}
				s := word.String()
				out = append(out, &piece{kind: wordPiece, text: s, w: face.Measure(s), style: st, face: face, item: it})
				word.Reset()
			}
			for _, r := range text {
				switch {
				case r == '\n' && keepNewlines:
					flushWord()
					out = append(out, &piece{kind: breakPiece, style: st, item: it})
					lastSpace = true
				case isCollapsibleSpace(r) && collapse:
					flushWord()
					if !lastSpace {
						out = append(out, &piece{kind: spacePiece, text: " ", w: face.Measure(" "), style: st, face: face, item: it, collapsible: true, breakAfter: wrap})
					}
					lastSpace = true
				case (r == ' ' || r == '\t') && !collapse:
					flushWord()
					s := " "
					if r == '\t' {
						s = "        "
					}
					out = append(out, &piece{kind: spacePiece, text: s, w: face.Measure(s), style: st, face: face, item: it, breakAfter: wrap})
					lastSpace = false
				case r == '\r' || r == '\u200b':
					// Ignored; zero-width space is a break opportunity.
					if r == '\u200b' && wrap {
						flushWord()
						markBreak()
					}
				default:
					if r == '\u00a0' {
						r = ' '
					}
					if r == '\u00ad' { // soft hyphen: invisible
						continue
					}
					word.WriteRune(r)
					lastSpace = false
				}
			}
			flushWord()
		}
	}
	return out
}

// breakLines splits pieces into lines that fit within width.
func breakLines(ps []*piece, width float64) [][]*piece {
	var lines [][]*piece
	var cur []*piece
	x := 0.0
	lastBreak := -1
	hasContent := func(line []*piece) bool {
		for _, p := range line {
			if p.kind == wordPiece || p.kind == atomicPiece || (p.kind == spacePiece && !p.collapsible) {
				return true
			}
		}
		return false
	}
	finish := func(forced bool) {
		if hasContent(cur) || forced {
			lines = append(lines, trimTrailing(cur))
		} else if len(cur) > 0 && len(lines) > 0 {
			// Keep empty inline boxes attached to the previous line.
			prev := lines[len(lines)-1]
			lines[len(lines)-1] = append(prev, cur...)
		} else if len(cur) > 0 {
			lines = append(lines, cur)
		}
		cur = nil
		x = 0
		lastBreak = -1
	}
	for _, p := range ps {
		if p.kind == breakPiece {
			cur = append(cur, p)
			finish(true)
			continue
		}
		if p.kind == spacePiece && p.collapsible && !hasContent(cur) {
			continue // leading collapsible whitespace
		}
		if (p.kind == wordPiece || p.kind == atomicPiece) && x+p.w > width+0.01 && lastBreak >= 0 && hasContent(cur[:lastBreak+1]) {
			carry := append([]*piece(nil), cur[lastBreak+1:]...)
			cur = cur[:lastBreak+1]
			finish(false)
			for len(carry) > 0 && carry[0].kind == spacePiece && carry[0].collapsible {
				carry = carry[1:]
			}
			cur = carry
			for _, c := range carry {
				x += c.w
			}
		}
		cur = append(cur, p)
		x += p.w
		if p.breakAfter {
			lastBreak = len(cur) - 1
		}
	}
	if len(cur) > 0 {
		finish(false)
	}
	return lines
}

// trimTrailing removes collapsible spaces at the end of a line (looking
// through inline start/end spacing).
func trimTrailing(line []*piece) []*piece {
	out := append([]*piece(nil), line...)
	for i := len(out) - 1; i >= 0; i-- {
		switch {
		case out[i].kind == spacePiece && out[i].collapsible:
			out = append(out[:i], out[i+1:]...)
		case out[i].kind == spacingPiece || out[i].kind == breakPiece:
			continue
		default:
			return out
		}
	}
	return out
}

func lineWidth(line []*piece) float64 {
	w := 0.0
	for _, p := range line {
		w += p.w
	}
	return w
}

// inlineIntrinsic returns the min-content and max-content widths of the
// inline content of b.
func inlineIntrinsic(b *Box) (minW, maxW float64) {
	minPs := pieces(b.Inline, func(a *Box) float64 { m, _ := intrinsic(a); return m })
	for _, l := range breakLines(minPs, 0) {
		minW = math.Max(minW, lineWidth(l))
	}
	maxPs := pieces(b.Inline, func(a *Box) float64 { _, m := intrinsic(a); return m })
	for _, l := range breakLines(maxPs, math.Inf(1)) {
		maxW = math.Max(maxW, lineWidth(l))
	}
	return minW, maxW
}

// layoutInline lays out b's inline content into line boxes within b's
// content box and returns the total height.
func layoutInline(b *Box) float64 {
	b.Lines = nil
	if len(b.Inline) == 0 {
		return 0
	}
	ps := pieces(b.Inline, func(a *Box) float64 {
		layoutAtomic(a, b.W)
		_, _, w, _ := a.BorderBox()
		return w + a.Margin.Horizontal()
	})
	strutAbove, strutBelow := lineMetrics(b.Style)
	y := b.Y
	for _, line := range breakLines(ps, b.W) {
		content := false
		forced := false
		for _, p := range line {
			switch p.kind {
			case wordPiece, atomicPiece:
				content = true
			case spacePiece:
				content = content || !p.collapsible
			case breakPiece:
				forced = true
			}
		}
		above, below := 0.0, 0.0
		if content || forced {
			above, below = strutAbove, strutBelow
			for _, p := range line {
				switch p.kind {
				case wordPiece, spacePiece, spacingPiece:
					a, d := lineMetrics(p.style)
					above = math.Max(above, a)
					below = math.Max(below, d)
				case atomicPiece:
					above = math.Max(above, p.item.Box.MarginBoxHeight())
				}
			}
		}
		l := &Line{Y: y, Height: above + below, Baseline: y + above}
		lw := lineWidth(line)
		x := b.X
		switch b.Style.TextAlign {
		case "right":
			x += b.W - lw
		case "center", "-webkit-center":
			x += (b.W - lw) / 2
		}
		for _, p := range line {
			f := &Frag{X: x, W: p.w, Baseline: l.Baseline, Style: p.style, Face: p.face, Node: p.item.Node}
			switch p.kind {
			case wordPiece, spacePiece:
				f.Text = p.text
				f.space = p.kind == spacePiece
				l.Frags = append(l.Frags, f)
			case atomicPiece:
				a := p.item.Box
				f.Box = a
				_, by, _, _ := a.BorderBox()
				top := by - a.Margin.Top
				left := a.X - a.Padding.Left - a.Border.Left - a.Margin.Left
				translate(a, x-left, l.Baseline-a.MarginBoxHeight()-top)
				l.Frags = append(l.Frags, f)
			case spacingPiece:
				f.spacing = true
			}
			x += p.w
		}
		b.Lines = append(b.Lines, l)
		y += l.Height
	}
	return y - b.Y
}
