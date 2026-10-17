package style

import (
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
)

var sideNames = [4]string{"top", "right", "bottom", "left"}

var borderStyles = map[string]bool{
	"none": true, "hidden": true, "dotted": true, "dashed": true, "solid": true,
	"double": true, "groove": true, "ridge": true, "inset": true, "outset": true,
}

// expand writes the longhands for a declaration into sp, expanding
// shorthands. Unsupported properties are stored as-is and ignored later.
func expand(prop, val string, sp map[string]string) {
	if !strings.Contains(strings.ToLower(val), "url(") {
		val = strings.ToLower(val)
	}
	parts := css.SplitValue(val)
	switch prop {
	case "margin", "padding":
		for i, v := range boxValues(parts) {
			sp[prop+"-"+sideNames[i]] = v
		}
	case "border-width", "border-style", "border-color":
		kind := strings.TrimPrefix(prop, "border-")
		for i, v := range boxValues(parts) {
			sp["border-"+sideNames[i]+"-"+kind] = v
		}
	case "border", "border-top", "border-right", "border-bottom", "border-left":
		sides := sideNames[:]
		if prop != "border" {
			sides = []string{strings.TrimPrefix(prop, "border-")}
		}
		w, st, c := "medium", "none", "currentcolor"
		for _, p := range parts {
			switch {
			case borderStyles[p]:
				st = p
			case isLengthLike(p) || p == "thin" || p == "medium" || p == "thick":
				w = p
			default:
				c = p
			}
		}
		for _, side := range sides {
			sp["border-"+side+"-width"] = w
			sp["border-"+side+"-style"] = st
			sp["border-"+side+"-color"] = c
		}
	case "background":
		sp["background-color"] = "transparent"
		sp["background-image"] = "none"
		sp["background-repeat"] = "repeat"
		// Only the last layer may carry a color; images come from any layer.
		for _, p := range parts {
			switch {
			case strings.HasPrefix(strings.ToLower(p), "url("):
				if sp["background-image"] == "none" {
					sp["background-image"] = p
				}
			case p == "no-repeat" || p == "repeat" || p == "repeat-x" || p == "repeat-y":
				sp["background-repeat"] = p
			case p == ",":
			default:
				if _, ok := ParseColor(p); ok {
					sp["background-color"] = p
				}
			}
		}
	case "font":
		expandFont(parts, sp)
	case "text-decoration":
		sp["text-decoration-line"] = val
	default:
		sp[prop] = val
	}
}

func isLengthLike(s string) bool {
	return s != "" && (s[0] >= '0' && s[0] <= '9' || s[0] == '.' || s[0] == '-' && len(s) > 1)
}

// boxValues expands 1-4 values to top, right, bottom, left.
func boxValues(p []string) []string {
	switch len(p) {
	case 1:
		return []string{p[0], p[0], p[0], p[0]}
	case 2:
		return []string{p[0], p[1], p[0], p[1]}
	case 3:
		return []string{p[0], p[1], p[2], p[1]}
	case 4:
		return p
	}
	return nil
}

func expandFont(parts []string, sp map[string]string) {
	sp["font-style"] = "normal"
	sp["font-weight"] = "normal"
	sp["line-height"] = "normal"
	for i, p := range parts {
		switch {
		case p == "italic" || p == "oblique":
			sp["font-style"] = p
		case p == "bold" || p == "bolder" || p == "lighter" || len(p) == 3 && p[1:] == "00":
			sp["font-weight"] = p
		case p == "normal" || p == "small-caps":
		case isLengthLike(p) || fontSizeKeywords[p] > 0:
			size := p
			if k := strings.IndexByte(p, '/'); k >= 0 {
				size = p[:k]
				sp["line-height"] = p[k+1:]
			}
			sp["font-size"] = size
			sp["font-family"] = strings.Join(parts[i+1:], " ")
			return
		}
	}
}

// presentationalHints maps legacy HTML attributes to CSS declarations.
func presentationalHints(n *dom.Node) []css.Decl {
	var out []css.Decl
	add := func(prop, val string) {
		expandInto := map[string]string{}
		expand(prop, val, expandInto)
		for k, v := range expandInto {
			out = append(out, css.Decl{Prop: k, Value: v})
		}
	}
	attr := func(name string) (string, bool) { return n.Attr(name) }

	switch n.Tag {
	case "body", "table", "tr", "td", "th", "thead", "tbody", "tfoot":
		if v, ok := attr("bgcolor"); ok {
			if _, ok := ParseColor(v); ok {
				add("background-color", v)
			} else if _, ok := ParseColor("#" + v); ok {
				add("background-color", "#"+v)
			}
		}
	}
	switch n.Tag {
	case "table", "td", "th", "img", "hr", "col", "input", "iframe", "canvas", "video":
		if v, ok := attr("width"); ok {
			if l, ok := parseHTMLLength(v); ok {
				add("width", l)
			}
		}
	}
	switch n.Tag {
	case "table", "td", "th", "tr", "img", "iframe", "canvas", "video":
		if v, ok := attr("height"); ok {
			if l, ok := parseHTMLLength(v); ok {
				add("height", l)
			}
		}
	}
	switch n.Tag {
	case "td", "th", "tr", "thead", "tbody", "tfoot", "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "caption":
		if v, ok := attr("align"); ok {
			switch strings.ToLower(v) {
			case "center", "middle":
				add("text-align", "-webkit-center")
			case "right":
				add("text-align", "-webkit-right")
			case "left":
				add("text-align", "-webkit-left")
			case "justify":
				add("text-align", "justify")
			}
		}
		if v, ok := attr("valign"); ok {
			add("vertical-align", strings.ToLower(v))
		}
		if _, ok := attr("nowrap"); ok && (n.Tag == "td" || n.Tag == "th") {
			add("white-space", "nowrap")
		}
	case "table":
		if v, ok := attr("align"); ok && strings.EqualFold(v, "center") {
			add("margin-left", "auto")
			add("margin-right", "auto")
		}
		if v, ok := attr("cellspacing"); ok {
			if l, ok := parseHTMLLength(v); ok {
				add("border-spacing", l)
			}
		}
		if v, ok := attr("border"); ok {
			w, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil && v == "" {
				w = 1
			}
			if w > 0 {
				add("border", strconv.Itoa(w)+"px outset gray")
			}
		}
	case "img":
		if v, ok := attr("border"); ok {
			if w, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && w > 0 {
				add("border", strconv.Itoa(w)+"px solid")
			}
		}
	case "font":
		if v, ok := attr("color"); ok {
			add("color", v)
		}
		if v, ok := attr("face"); ok {
			add("font-family", v)
		}
		if v, ok := attr("size"); ok {
			if px, ok := fontSizeAttr(v); ok {
				add("font-size", px)
			}
		}
	case "body":
		if v, ok := attr("text"); ok {
			add("color", v)
		}
	}
	// Cells inherit cellpadding and the border attribute from their table.
	if n.Tag == "td" || n.Tag == "th" {
		if t := enclosingTable(n); t != nil {
			if v, ok := t.Attr("cellpadding"); ok {
				if l, ok := parseHTMLLength(v); ok {
					add("padding", l)
				}
			}
			if v, ok := t.Attr("border"); ok {
				if w, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && w > 0 {
					add("border", "1px inset gray")
				}
			}
		}
	}
	return out
}

func enclosingTable(n *dom.Node) *dom.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Tag == "table" {
			return p
		}
	}
	return nil
}

func fontSizeAttr(v string) (string, bool) {
	v = strings.TrimSpace(v)
	rel := 0
	if strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		rel = 3
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return "", false
	}
	n += rel
	sizes := []string{"x-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "48px"}
	if n < 1 {
		n = 1
	}
	if n > 7 {
		n = 7
	}
	return sizes[n], true
}
