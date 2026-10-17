// Package style computes CSS styles for DOM elements: cascade, inheritance
// and value computation, including HTML presentational attributes.
package style

import (
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
)

// Display is the computed display type.
type Display int

const (
	DisplayInline Display = iota
	DisplayBlock
	DisplayInlineBlock
	DisplayListItem
	DisplayTable
	DisplayInlineTable
	DisplayTableRowGroup
	DisplayTableRow
	DisplayTableCell
	DisplayTableCaption
	DisplayNone
)

// Unit is the unit of a Length.
type Unit int

const (
	Auto Unit = iota
	Px
	Percent
)

// Length is a computed length: auto, an absolute px value, or a percentage.
type Length struct {
	Value float64
	Unit  Unit
}

// Resolve returns the length in px given the reference length for
// percentages. Auto resolves to def.
func (l Length) Resolve(ref, def float64) float64 {
	switch l.Unit {
	case Px:
		return l.Value
	case Percent:
		return l.Value / 100 * ref
	}
	return def
}

// IsAuto reports whether l is auto.
func (l Length) IsAuto() bool { return l.Unit == Auto }

// Style is the computed style of an element.
type Style struct {
	Display Display

	Color           color.NRGBA
	BackgroundColor color.NRGBA
	BackgroundImage string // URL, or ""

	FontFamily string
	FontSize   float64 // px
	Bold       bool
	Italic     bool
	// LineHeight is the used line height in px, or 0 for "normal".
	LineHeight       float64
	lineHeightFactor float64 // non-zero when specified as a number

	TextAlign     string // left, right, center, justify, -webkit-center
	VerticalAlign string // baseline, top, middle, bottom, ...
	WhiteSpace    string // normal, nowrap, pre, pre-wrap, pre-line
	Underline     bool
	LineThrough   bool
	Hidden        bool // visibility: hidden
	TextTransform string

	Margin      [4]Length // top, right, bottom, left
	Padding     [4]Length
	BorderWidth [4]float64
	BorderStyle [4]string
	BorderColor [4]color.NRGBA

	Width, Height      Length
	MinWidth, MaxWidth Length
	MinHeight          Length
	BoxSizingBorder    bool
	BorderSpacing      float64
	ListStyleType      string
	OverflowHidden     bool
}

// Styles maps elements to their computed style.
type Styles map[*dom.Node]*Style

// Of returns the style for n, or for its parent element if n is a text node.
func (s Styles) Of(n *dom.Node) *Style {
	for n != nil {
		if st, ok := s[n]; ok {
			return st
		}
		n = n.Parent
	}
	return nil
}

// DefaultFontSize is the initial font size in px.
const DefaultFontSize = 16

// matched is a declaration together with its cascade sort key.
type matched struct {
	decl  css.Declaration
	tier  int
	spec  css.Specificity
	order int
}

// Compute computes styles for every element in doc. Author sheets are
// applied in order after the user-agent sheet. viewportWidth is used to
// evaluate media queries.
func Compute(doc *dom.Node, author []*css.Stylesheet, viewportWidth float64) Styles {
	type indexedRule struct {
		rule  *css.Rule
		ua    bool
		order int
	}
	var rules []indexedRule
	order := 0
	ua := []*css.Stylesheet{uaSheet}
	if doc.Quirks {
		ua = append(ua, quirksSheet)
	}
	for _, ss := range ua {
		for _, r := range ss.Rules {
			rules = append(rules, indexedRule{r, true, order})
			order++
		}
	}
	for _, ss := range author {
		for _, r := range ss.Rules {
			if !css.MediaMatches(r.Media, viewportWidth) {
				continue
			}
			rules = append(rules, indexedRule{r, false, order})
			order++
		}
	}

	styles := Styles{}
	var root *Style
	var visit func(n *dom.Node, parent *Style)
	visit = func(n *dom.Node, parent *Style) {
		if n.Type != dom.ElementNode {
			for _, c := range n.Children {
				visit(c, parent)
			}
			return
		}
		var ms []matched
		for _, ir := range rules {
			for _, sel := range ir.rule.Selectors {
				if !sel.Matches(n) {
					continue
				}
				for _, d := range ir.rule.Decls {
					tier := 2
					switch {
					case ir.ua:
						tier = 0
					case d.Important:
						tier = 4
					}
					ms = append(ms, matched{d, tier, sel.Specificity(), ir.order})
				}
			}
		}
		for _, d := range presentationalHints(n) {
			ms = append(ms, matched{d, 1, css.Specificity{}, 0})
		}
		if v, ok := n.Attr("style"); ok {
			for _, d := range css.ParseDeclarations(v) {
				tier := 3
				if d.Important {
					tier = 5
				}
				ms = append(ms, matched{d, tier, css.Specificity{}, 0})
			}
		}
		sort.SliceStable(ms, func(i, j int) bool {
			a, b := ms[i], ms[j]
			if a.tier != b.tier {
				return a.tier < b.tier
			}
			if a.spec != b.spec {
				return a.spec.Less(b.spec)
			}
			return a.order < b.order
		})
		specified := map[string]string{}
		for _, m := range ms {
			specified[m.decl.Property] = m.decl.Value
		}
		st := computeStyle(specified, parent, root)
		if root == nil {
			root = st
		}
		styles[n] = st
		for _, c := range n.Children {
			visit(c, st)
		}
	}
	visit(doc, nil)
	return styles
}

func initialStyle() *Style {
	st := &Style{
		Display:       DisplayInline,
		Color:         color.NRGBA{0, 0, 0, 255},
		FontFamily:    "serif",
		FontSize:      DefaultFontSize,
		TextAlign:     "left",
		VerticalAlign: "baseline",
		WhiteSpace:    "normal",
		ListStyleType: "disc",
		MaxWidth:      Length{Unit: Auto},
	}
	for i := range st.BorderStyle {
		st.BorderStyle[i] = "none"
	}
	return st
}

// computeStyle turns specified values into a computed style.
func computeStyle(spec map[string]string, parent, root *Style) *Style {
	st := initialStyle()
	if parent != nil {
		// Inherited properties.
		st.Color = parent.Color
		st.FontFamily = parent.FontFamily
		st.FontSize = parent.FontSize
		st.Bold = parent.Bold
		st.Italic = parent.Italic
		st.LineHeight = parent.LineHeight
		st.lineHeightFactor = parent.lineHeightFactor
		st.TextAlign = parent.TextAlign
		st.WhiteSpace = parent.WhiteSpace
		st.Hidden = parent.Hidden
		st.ListStyleType = parent.ListStyleType
		st.BorderSpacing = parent.BorderSpacing
		st.TextTransform = parent.TextTransform
		// text-decoration propagates to inline descendants.
		st.Underline = parent.Underline
		st.LineThrough = parent.LineThrough
	}
	inherit := func(prop string) bool { return spec[prop] == "inherit" && parent != nil }

	// Font size first; other lengths depend on it.
	if v, ok := spec["font-size"]; ok {
		parentSize := float64(DefaultFontSize)
		if parent != nil {
			parentSize = parent.FontSize
		}
		if px, ok := parseFontSize(v, parentSize, root); ok {
			st.FontSize = px
		}
	}
	em := st.FontSize
	length := func(v string) (Length, bool) { return parseLength(v, em, root) }

	if v, ok := spec["color"]; ok {
		if c, ok := css.ParseColor(v); ok {
			st.Color = c
		}
	}
	if v, ok := spec["font-family"]; ok && v != "inherit" {
		st.FontFamily = v
	}
	if v, ok := spec["font-weight"]; ok {
		switch v = strings.ToLower(v); v {
		case "bold", "bolder":
			st.Bold = true
		case "normal", "lighter":
			st.Bold = false
		default:
			if n, err := strconv.Atoi(v); err == nil {
				st.Bold = n >= 600
			}
		}
	}
	if v, ok := spec["font-style"]; ok {
		st.Italic = v == "italic" || v == "oblique"
	}
	if v, ok := spec["line-height"]; ok {
		v = strings.ToLower(v)
		switch {
		case v == "normal":
			st.LineHeight, st.lineHeightFactor = 0, 0
		case v == "inherit":
		default:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				st.lineHeightFactor = f
			} else if l, ok := length(v); ok {
				st.lineHeightFactor = 0
				st.LineHeight = l.Resolve(st.FontSize, 0)
			}
		}
	}
	if st.lineHeightFactor > 0 {
		st.LineHeight = st.lineHeightFactor * st.FontSize
	}

	if v, ok := spec["display"]; ok {
		if d, ok := parseDisplay(v); ok {
			st.Display = d
		}
	}
	if v, ok := spec["background-color"]; ok {
		if v == "currentcolor" {
			st.BackgroundColor = st.Color
		} else if c, ok := css.ParseColor(v); ok {
			st.BackgroundColor = c
		}
	}
	if v, ok := spec["background-image"]; ok {
		st.BackgroundImage = parseURL(v)
	}
	if v, ok := spec["text-align"]; ok && !inherit("text-align") {
		switch v = strings.ToLower(v); v {
		case "left", "right", "center", "justify", "-webkit-center":
			st.TextAlign = v
		case "start":
			st.TextAlign = "left"
		case "end":
			st.TextAlign = "right"
		case "-internal-center", "-webkit-auto":
			st.TextAlign = "center"
		}
	}
	if v, ok := spec["vertical-align"]; ok {
		if inherit("vertical-align") {
			st.VerticalAlign = parent.VerticalAlign
		} else {
			st.VerticalAlign = strings.ToLower(v)
		}
	}
	if v, ok := spec["white-space"]; ok && !inherit("white-space") {
		st.WhiteSpace = strings.ToLower(v)
	}
	if v, ok := spec["text-decoration-line"]; ok {
		switch v {
		case "underline":
			st.Underline = true
		case "line-through":
			st.LineThrough = true
		case "none":
			st.Underline, st.LineThrough = false, false
		}
	}
	if v, ok := spec["visibility"]; ok && !inherit("visibility") {
		st.Hidden = v == "hidden" || v == "collapse"
	}
	if v, ok := spec["text-transform"]; ok && !inherit("text-transform") {
		st.TextTransform = strings.ToLower(v)
	}
	if v, ok := spec["list-style-type"]; ok && !inherit("list-style-type") {
		st.ListStyleType = strings.ToLower(v)
	}
	if v, ok := spec["border-spacing"]; ok && !inherit("border-spacing") {
		if l, ok := length(strings.Fields(v + " 0")[0]); ok {
			st.BorderSpacing = l.Resolve(0, 0)
		}
	}
	if v, ok := spec["box-sizing"]; ok {
		st.BoxSizingBorder = v == "border-box"
	}
	if v, ok := spec["overflow"]; ok {
		st.OverflowHidden = v == "hidden" || v == "auto" || v == "scroll" || v == "clip"
	}

	for i, side := range [4]string{"top", "right", "bottom", "left"} {
		if v, ok := spec["margin-"+side]; ok {
			if l, ok := length(v); ok {
				st.Margin[i] = l
			} else if inherit("margin-" + side) {
				st.Margin[i] = parent.Margin[i]
			}
		} else {
			st.Margin[i] = Length{0, Px}
		}
		st.Padding[i] = Length{0, Px}
		if v, ok := spec["padding-"+side]; ok {
			if l, ok := length(v); ok && l.Unit != Auto {
				st.Padding[i] = l
			}
		}
		st.BorderColor[i] = st.Color
		if v, ok := spec["border-"+side+"-color"]; ok && v != "currentcolor" {
			if c, ok := css.ParseColor(v); ok {
				st.BorderColor[i] = c
			}
		}
		if v, ok := spec["border-"+side+"-style"]; ok {
			st.BorderStyle[i] = strings.ToLower(v)
		}
		width := 3.0 // medium
		if v, ok := spec["border-"+side+"-width"]; ok {
			switch v {
			case "thin":
				width = 1
			case "medium":
				width = 3
			case "thick":
				width = 5
			default:
				if l, ok := length(v); ok && l.Unit == Px {
					width = l.Value
					if width > 0 && width < 1 {
						width = 1
					}
				}
			}
		}
		if st.BorderStyle[i] == "none" || st.BorderStyle[i] == "hidden" {
			width = 0
		}
		st.BorderWidth[i] = width
	}

	sizes := []struct {
		prop string
		dst  *Length
	}{
		{"width", &st.Width}, {"height", &st.Height}, {"min-width", &st.MinWidth},
		{"max-width", &st.MaxWidth}, {"min-height", &st.MinHeight},
	}
	for _, s := range sizes {
		if v, ok := spec[s.prop]; ok {
			if l, ok := length(v); ok {
				*s.dst = l
			}
		}
	}
	return st
}

func parseDisplay(v string) (Display, bool) {
	switch strings.ToLower(v) {
	case "inline":
		return DisplayInline, true
	case "block", "flow-root", "flex", "grid":
		return DisplayBlock, true
	case "inline-block", "inline-flex", "inline-grid":
		return DisplayInlineBlock, true
	case "list-item":
		return DisplayListItem, true
	case "table":
		return DisplayTable, true
	case "inline-table":
		return DisplayInlineTable, true
	case "table-row-group", "table-header-group", "table-footer-group":
		return DisplayTableRowGroup, true
	case "table-row":
		return DisplayTableRow, true
	case "table-cell":
		return DisplayTableCell, true
	case "table-caption":
		return DisplayTableCaption, true
	case "none", "table-column", "table-column-group":
		return DisplayNone, true
	case "contents":
		return DisplayInline, true
	}
	return 0, false
}

var fontKeywords = map[string]float64{
	"xx-small": 9, "x-small": 10, "small": 13, "medium": 16, "large": 18, "x-large": 24,
	"xx-large": 32, "xxx-large": 48,
}

func parseFontSize(v string, parentSize float64, root *Style) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if px, ok := fontKeywords[v]; ok {
		return px, true
	}
	switch v {
	case "smaller":
		return parentSize / 1.2, true
	case "larger":
		return parentSize * 1.2, true
	case "inherit":
		return parentSize, true
	}
	l, ok := parseLength(v, parentSize, root)
	if !ok || l.Unit == Auto {
		return 0, false
	}
	return l.Resolve(parentSize, parentSize), true
}

// parseLength parses a CSS length, resolving relative units against em
// (the font size in px) and the root font size.
func parseLength(v string, em float64, root *Style) (Length, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "auto" {
		return Length{Unit: Auto}, true
	}
	if v == "none" {
		return Length{Unit: Auto}, true
	}
	num := v
	unit := ""
	for i := len(v) - 1; i >= 0; i-- {
		c := v[i]
		if (c >= '0' && c <= '9') || c == '.' {
			num, unit = v[:i+1], v[i+1:]
			break
		}
		if i == 0 {
			return Length{}, false
		}
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return Length{}, false
	}
	rootSize := float64(DefaultFontSize)
	if root != nil {
		rootSize = root.FontSize
	}
	switch unit {
	case "px":
		return Length{f, Px}, true
	case "":
		if f == 0 {
			return Length{0, Px}, true
		}
		return Length{}, false
	case "%":
		return Length{f, Percent}, true
	case "pt":
		return Length{f * 4 / 3, Px}, true
	case "pc":
		return Length{f * 16, Px}, true
	case "in":
		return Length{f * 96, Px}, true
	case "cm":
		return Length{f * 96 / 2.54, Px}, true
	case "mm":
		return Length{f * 96 / 25.4, Px}, true
	case "em":
		return Length{f * em, Px}, true
	case "rem":
		return Length{f * rootSize, Px}, true
	case "ex", "ch":
		return Length{f * em / 2, Px}, true
	}
	return Length{}, false
}

func parseURL(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(strings.ToLower(v), "url(") || !strings.HasSuffix(v, ")") {
		return ""
	}
	v = strings.TrimSpace(v[4 : len(v)-1])
	return strings.Trim(v, `"'`)
}
