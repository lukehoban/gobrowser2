// Package style computes the cascaded, inherited and computed style of every
// element in a document.
package style

import (
	"image/color"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
)

// Sides indexes per-side properties.
const (
	Top = iota
	Right
	Bottom
	Left
)

// Style is the computed style of an element.
type Style struct {
	Display string

	Color              color.RGBA
	BackgroundColor    color.RGBA
	BackgroundImage    string // absolute URL, or ""
	BackgroundSize     Length // width; auto means intrinsic
	BackgroundNoRepeat bool

	FontSize   float64
	Bold       bool
	Italic     bool
	Mono       bool
	LineHeight float64 // pixels; 0 means normal

	TextAlign     string // left, right, center, -webkit-left/right/center
	VerticalAlign string
	WhiteSpace    string
	Underline     bool // includes decorations propagated from ancestors
	LineThrough   bool
	Hidden        bool // visibility: hidden

	Margin      [4]Length
	Padding     [4]Length
	BorderWidth [4]float64
	BorderStyle [4]string
	BorderColor [4]color.RGBA

	Width, Height      Length
	MinWidth, MaxWidth Length
	MinHeight          Length
	BorderSpacing      float64
	BorderBox          bool // box-sizing: border-box
}

// HasBorder reports whether side i has a visible border.
func (s *Style) HasBorder(i int) bool {
	return s.BorderWidth[i] > 0 && s.BorderStyle[i] != "" && s.BorderStyle[i] != "none" && s.BorderStyle[i] != "hidden"
}

// Border returns the used border width of side i.
func (s *Style) Border(i int) float64 {
	if s.HasBorder(i) {
		return s.BorderWidth[i]
	}
	return 0
}

// IsTableish reports whether the display value is an internal table type.
func (s *Style) IsTableish() bool {
	return strings.HasPrefix(s.Display, "table-")
}

func initial() *Style {
	s := &Style{
		Display:       "inline",
		Color:         color.RGBA{0, 0, 0, 255},
		FontSize:      16,
		TextAlign:     "left",
		VerticalAlign: "baseline",
		WhiteSpace:    "normal",
	}
	for i := range s.Margin {
		s.Margin[i] = PxLen(0)
		s.Padding[i] = PxLen(0)
	}
	return s
}

// inherit returns a style with the inherited properties of parent and the
// initial values of all others.
func inherit(p *Style) *Style {
	s := initial()
	s.Color = p.Color
	s.FontSize = p.FontSize
	s.Bold = p.Bold
	s.Italic = p.Italic
	s.Mono = p.Mono
	s.LineHeight = p.LineHeight
	s.TextAlign = p.TextAlign
	s.WhiteSpace = p.WhiteSpace
	s.Underline = p.Underline
	s.LineThrough = p.LineThrough
	s.Hidden = p.Hidden
	s.BorderSpacing = p.BorderSpacing
	return s
}

// Sheet is an author stylesheet.
type Sheet = css.Stylesheet

type matched struct {
	decl      css.Decl
	spec      css.Specificity
	order     int
	important bool
	level     int // 0 UA, 1 presentational hints, 2 author, 3 inline
}

var uaSheet = css.Parse(uaCSS, nil)
var quirksSheet = css.Parse(quirksCSS, nil)

// Compute computes styles for every element under doc. Author sheets are in
// document order; base is the document URL, used for url() in style
// attributes.
func Compute(doc *dom.Node, sheets []*css.Stylesheet, base *url.URL) map[*dom.Node]*Style {
	out := map[*dom.Node]*Style{}
	root := initial()
	root.Display = "block"
	ua := []*css.Stylesheet{uaSheet}
	if doc.Quirks {
		ua = append(ua, quirksSheet)
	}
	var walk func(n *dom.Node, parent *Style)
	walk = func(n *dom.Node, parent *Style) {
		for _, c := range n.Children {
			if c.Type != dom.ElementNode {
				continue
			}
			s := computeOne(c, parent, ua, sheets, base)
			out[c] = s
			walk(c, s)
		}
	}
	walk(doc, root)
	return out
}

func computeOne(n *dom.Node, parent *Style, ua, author []*css.Stylesheet, base *url.URL) *Style {
	var ms []matched
	order := 0
	collect := func(sheets []*css.Stylesheet, level int) {
		for _, sh := range sheets {
			for _, r := range sh.Rules {
				var best *css.Selector
				for _, sel := range r.Selectors {
					if sel.Matches(n) && (best == nil || best.Spec.Less(sel.Spec)) {
						best = sel
					}
				}
				order++
				if best == nil {
					continue
				}
				for _, d := range r.Decls {
					ms = append(ms, matched{decl: d, spec: best.Spec, order: order, important: d.Important, level: level})
				}
			}
		}
	}
	collect(ua, 0)
	for _, d := range presentationalHints(n) {
		ms = append(ms, matched{decl: d, level: 1})
	}
	collect(author, 2)
	if st, ok := n.Attr("style"); ok {
		for _, d := range css.ParseDeclarations(st, base) {
			ms = append(ms, matched{decl: d, level: 3, important: d.Important})
		}
	}
	rank := func(m matched) int {
		if m.important {
			return 10 + m.level
		}
		return m.level
	}
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.spec != b.spec {
			return a.spec.Less(b.spec)
		}
		return a.order < b.order
	})
	specified := map[string]string{}
	for _, m := range ms {
		expand(m.decl.Prop, m.decl.Value, specified)
	}
	return computeStyle(specified, parent)
}

var fontSizeKeywords = map[string]float64{
	"xx-small": 9, "x-small": 10, "small": 13, "medium": 16, "large": 18, "x-large": 24, "xx-large": 32,
}

func computeStyle(sp map[string]string, p *Style) *Style {
	s := inherit(p)
	// font-size first: em units elsewhere depend on it.
	if v, ok := sp["font-size"]; ok {
		switch {
		case v == "inherit":
		case v == "smaller":
			s.FontSize = p.FontSize / 1.2
		case v == "larger":
			s.FontSize = p.FontSize * 1.2
		case fontSizeKeywords[v] > 0:
			s.FontSize = fontSizeKeywords[v]
		default:
			if l, ok := parseLength(v, p.FontSize); ok {
				s.FontSize = l.Resolve(p.FontSize, p.FontSize)
			}
		}
	}
	fs := s.FontSize
	length := func(prop string, dst *Length, parentVal Length) {
		v, ok := sp[prop]
		if !ok {
			return
		}
		if v == "inherit" {
			*dst = parentVal
			return
		}
		if l, ok := parseLength(v, fs); ok {
			*dst = l
		}
	}
	colorProp := func(prop string, dst *color.RGBA, parentVal color.RGBA) {
		v, ok := sp[prop]
		if !ok {
			return
		}
		switch v {
		case "inherit":
			*dst = parentVal
		case "currentcolor":
			*dst = s.Color
		default:
			if c, ok := ParseColor(v); ok {
				*dst = c
			}
		}
	}
	colorProp("color", &s.Color, p.Color)
	// Border colors default to currentColor.
	for i := range s.BorderColor {
		s.BorderColor[i] = s.Color
	}
	colorProp("background-color", &s.BackgroundColor, p.BackgroundColor)

	if v, ok := sp["display"]; ok {
		if v == "inherit" {
			s.Display = p.Display
		} else {
			s.Display = v
		}
	}
	if v, ok := sp["background-image"]; ok {
		s.BackgroundImage = extractURL(v)
	}
	if v, ok := sp["background-size"]; ok {
		if f := css.SplitValue(v); len(f) > 0 {
			if l, ok := parseLength(f[0], fs); ok {
				s.BackgroundSize = l
			}
		}
	}
	if v, ok := sp["background-repeat"]; ok {
		s.BackgroundNoRepeat = strings.HasPrefix(v, "no-repeat")
	}
	if v, ok := sp["font-weight"]; ok {
		switch v {
		case "bold", "bolder":
			s.Bold = true
		case "normal", "lighter":
			s.Bold = false
		case "inherit":
		default:
			if n, err := strconv.Atoi(v); err == nil {
				s.Bold = n >= 600
			}
		}
	}
	if v, ok := sp["font-style"]; ok && v != "inherit" {
		s.Italic = v == "italic" || v == "oblique"
	}
	if v, ok := sp["font-family"]; ok && v != "inherit" {
		s.Mono = isMonospaceFamily(v)
	}
	if v, ok := sp["line-height"]; ok {
		switch v {
		case "inherit":
		case "normal":
			s.LineHeight = 0
		default:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				s.LineHeight = f * fs
			} else if l, ok := parseLength(v, fs); ok {
				s.LineHeight = l.Resolve(fs, 0)
			}
		}
	}
	if v, ok := sp["text-align"]; ok && v != "inherit" {
		switch v {
		case "start":
			v = "left"
		case "end":
			v = "right"
		case "-webkit-auto", "-internal-center":
			v = "-webkit-center"
		}
		s.TextAlign = v
	}
	if v, ok := sp["vertical-align"]; ok {
		if v == "inherit" {
			s.VerticalAlign = p.VerticalAlign
		} else {
			s.VerticalAlign = v
		}
	}
	if v, ok := sp["white-space"]; ok && v != "inherit" {
		s.WhiteSpace = v
	}
	if v, ok := sp["text-decoration-line"]; ok {
		for _, f := range strings.Fields(v) {
			switch f {
			case "underline":
				s.Underline = true
			case "line-through":
				s.LineThrough = true
			}
		}
	}
	if v, ok := sp["visibility"]; ok && v != "inherit" {
		s.Hidden = v == "hidden" || v == "collapse"
	}
	if v, ok := sp["box-sizing"]; ok {
		s.BorderBox = v == "border-box"
	}
	if v, ok := sp["border-spacing"]; ok && v != "inherit" {
		if f := strings.Fields(v); len(f) > 0 {
			if l, ok := parseLength(f[0], fs); ok {
				s.BorderSpacing = l.Resolve(0, 0)
			}
		}
	}
	for i, side := range sideNames {
		length("margin-"+side, &s.Margin[i], p.Margin[i])
		length("padding-"+side, &s.Padding[i], p.Padding[i])
		if v, ok := sp["border-"+side+"-style"]; ok {
			s.BorderStyle[i] = v
		}
		colorProp("border-"+side+"-color", &s.BorderColor[i], p.BorderColor[i])
		if v, ok := sp["border-"+side+"-width"]; ok {
			s.BorderWidth[i] = borderWidth(v, fs)
		} else if s.BorderStyle[i] != "" {
			s.BorderWidth[i] = 3 // medium
		}
	}
	length("width", &s.Width, p.Width)
	length("height", &s.Height, p.Height)
	length("min-width", &s.MinWidth, p.MinWidth)
	length("max-width", &s.MaxWidth, p.MaxWidth)
	length("min-height", &s.MinHeight, p.MinHeight)
	if s.MinWidth.IsAuto() {
		s.MinWidth = PxLen(0)
	}
	if s.MinHeight.IsAuto() {
		s.MinHeight = PxLen(0)
	}
	return s
}

func borderWidth(v string, fs float64) float64 {
	switch v {
	case "thin":
		return 1
	case "medium":
		return 3
	case "thick":
		return 5
	}
	if l, ok := parseLength(v, fs); ok {
		return l.Resolve(0, 0)
	}
	return 3
}

func isMonospaceFamily(v string) bool {
	for _, f := range strings.Split(v, ",") {
		f = strings.ToLower(css.Unquote(strings.TrimSpace(f)))
		switch f {
		case "monospace", "courier", "courier new", "consolas", "menlo", "monaco":
			return true
		case "sans-serif", "serif", "verdana", "arial", "helvetica", "geneva", "times", "times new roman", "georgia":
			return false
		}
	}
	return false
}

func extractURL(v string) string {
	low := strings.ToLower(v)
	k := strings.Index(low, "url(")
	if k < 0 {
		return ""
	}
	end := strings.IndexByte(v[k:], ')')
	if end < 0 {
		return ""
	}
	return css.Unquote(strings.TrimSpace(v[k+4 : k+end]))
}

// AnonymousBlock returns the style of an anonymous block box inside a box
// with style parent.
func AnonymousBlock(parent *Style) *Style {
	s := inherit(parent)
	s.Display = "block"
	return s
}
