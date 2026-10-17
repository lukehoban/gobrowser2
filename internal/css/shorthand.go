package css

import (
	"strconv"
	"strings"
)

var sides = [4]string{"top", "right", "bottom", "left"}

var borderStyles = map[string]bool{
	"none": true, "hidden": true, "dotted": true, "dashed": true, "solid": true, "double": true,
	"groove": true, "ridge": true, "inset": true, "outset": true,
}

// Expand expands shorthand properties into their longhands. Non-shorthand
// declarations are returned unchanged.
func Expand(d Declaration) []Declaration {
	mk := func(prop, value string) Declaration {
		return Declaration{Property: prop, Value: value, Important: d.Important}
	}
	vals := SplitValues(d.Value)
	switch d.Property {
	case "margin", "padding":
		return boxSides(d, vals, d.Property+"-%s")
	case "border-width", "border-style", "border-color":
		kind := strings.TrimPrefix(d.Property, "border-")
		return boxSides(d, vals, "border-%s-"+kind)
	case "border", "border-top", "border-right", "border-bottom", "border-left":
		width, style, color := "medium", "none", "currentcolor"
		for _, v := range vals {
			lv := strings.ToLower(v)
			switch {
			case borderStyles[lv]:
				style = lv
			case isLength(lv) || lv == "thin" || lv == "medium" || lv == "thick":
				width = lv
			default:
				color = v
			}
		}
		targets := sides[:]
		if d.Property != "border" {
			targets = []string{strings.TrimPrefix(d.Property, "border-")}
		}
		var out []Declaration
		for _, s := range targets {
			out = append(out,
				mk("border-"+s+"-width", width),
				mk("border-"+s+"-style", style),
				mk("border-"+s+"-color", color))
		}
		return out
	case "background":
		color, image := "transparent", "none"
		for _, layer := range splitTopLevel(d.Value, ',') {
			for _, v := range SplitValues(strings.TrimSpace(layer)) {
				lv := strings.ToLower(v)
				switch {
				case strings.HasPrefix(lv, "url("):
					if image == "none" {
						image = v
					}
				case IsColor(lv):
					color = v
				}
			}
		}
		return []Declaration{mk("background-color", color), mk("background-image", image)}
	case "font":
		return expandFont(d, vals)
	case "list-style":
		for _, v := range vals {
			lv := strings.ToLower(v)
			if lv != "inside" && lv != "outside" && !strings.HasPrefix(lv, "url(") {
				return []Declaration{mk("list-style-type", lv)}
			}
		}
		return nil
	case "text-decoration":
		line := "none"
		for _, v := range vals {
			switch lv := strings.ToLower(v); lv {
			case "underline", "line-through", "overline", "none":
				line = lv
			}
		}
		return []Declaration{mk("text-decoration-line", line)}
	}
	return []Declaration{d}
}

func boxSides(d Declaration, vals []string, pattern string) []Declaration {
	var v [4]string
	switch len(vals) {
	case 1:
		v = [4]string{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		v = [4]string{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		v = [4]string{vals[0], vals[1], vals[2], vals[1]}
	case 4:
		v = [4]string{vals[0], vals[1], vals[2], vals[3]}
	default:
		return nil
	}
	out := make([]Declaration, 4)
	for i, s := range sides {
		out[i] = Declaration{Property: strings.Replace(pattern, "%s", s, 1), Value: v[i], Important: d.Important}
	}
	return out
}

func expandFont(d Declaration, vals []string) []Declaration {
	mk := func(prop, value string) Declaration {
		return Declaration{Property: prop, Value: value, Important: d.Important}
	}
	style, weight, size, lineHeight := "normal", "normal", "", "normal"
	i := 0
	for ; i < len(vals); i++ {
		lv := strings.ToLower(vals[i])
		switch {
		case lv == "italic" || lv == "oblique":
			style = lv
		case lv == "bold" || lv == "bolder" || lv == "lighter" || (len(lv) == 3 && lv[1:] == "00"):
			weight = lv
		case lv == "normal" || lv == "small-caps":
		default:
			goto size
		}
	}
size:
	if i >= len(vals) {
		return nil // system fonts etc. are not supported
	}
	size = vals[i]
	if s, lh, ok := strings.Cut(size, "/"); ok {
		size, lineHeight = s, lh
	}
	i++
	if i < len(vals) && strings.HasPrefix(vals[i], "/") {
		lineHeight = strings.TrimPrefix(vals[i], "/")
		if lineHeight == "" && i+1 < len(vals) {
			i++
			lineHeight = vals[i]
		}
		i++
	}
	family := strings.Join(vals[i:], " ")
	if family == "" {
		return nil
	}
	return []Declaration{
		mk("font-style", style), mk("font-weight", weight), mk("font-size", size),
		mk("line-height", lineHeight), mk("font-family", family),
	}
}

func isLength(v string) bool {
	if v == "0" {
		return true
	}
	for _, u := range []string{"px", "pt", "em", "rem", "ex", "%", "vh", "vw", "cm", "mm", "in", "pc", "ch"} {
		if strings.HasSuffix(v, u) {
			if _, err := strconv.ParseFloat(strings.TrimSuffix(v, u), 64); err == nil {
				return true
			}
		}
	}
	return false
}

// IsColor reports whether v looks like a CSS color value.
func IsColor(v string) bool {
	_, ok := ParseColor(v)
	return ok || v == "currentcolor"
}

// MediaMatches evaluates a media query list against a screen of the given
// width in CSS pixels. Unknown features evaluate to false.
func MediaMatches(query string, width float64) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	for _, q := range splitTopLevel(query, ',') {
		if mediaQueryMatches(strings.TrimSpace(q), width) {
			return true
		}
	}
	return false
}

func mediaQueryMatches(q string, width float64) bool {
	not := false
	parts := strings.Fields(q)
	if len(parts) > 0 && (parts[0] == "only" || parts[0] == "not") {
		not = parts[0] == "not"
		q = strings.TrimSpace(q[len(parts[0]):])
	}
	result := true
conds:
	for _, cond := range strings.Split(q, " and ") {
		cond = strings.TrimSpace(cond)
		switch cond {
		case "", "all", "screen":
			continue
		}
		if !strings.HasPrefix(cond, "(") || !strings.HasSuffix(cond, ")") {
			result = false // print, speech, etc.
			break conds
		}
		name, value, _ := strings.Cut(cond[1:len(cond)-1], ":")
		name = strings.TrimSpace(name)
		px, ok := parseMediaLength(strings.TrimSpace(value))
		switch {
		case name == "min-width" && ok:
			result = width >= px
		case name == "max-width" && ok:
			result = width <= px
		default:
			result = false
		}
		if !result {
			break conds
		}
	}
	if not {
		return !result
	}
	return result
}

func parseMediaLength(v string) (float64, bool) {
	scale := 1.0
	switch {
	case strings.HasSuffix(v, "px"):
		v = strings.TrimSuffix(v, "px")
	case strings.HasSuffix(v, "em"):
		v = strings.TrimSuffix(strings.TrimSuffix(v, "em"), "r")
		scale = 16
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, false
	}
	return f * scale, true
}
