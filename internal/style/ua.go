package style

import (
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
)

// uaCSS is the user-agent stylesheet, loosely following the HTML spec's
// rendering section.
const uaCSS = `
html, address, blockquote, body, center, dd, details, dialog, dir, div, dl, dt, fieldset,
figcaption, figure, footer, form, h1, h2, h3, h4, h5, h6, header, hgroup, hr, legend, main,
menu, nav, ol, p, pre, section, summary, ul, article, aside, listing, plaintext, xmp, optgroup {
  display: block;
}
head, script, style, title, meta, link, base, template, noscript, datalist, param, area, rp,
[hidden], input[type=hidden], colgroup, col, option, iframe, video, audio, object, embed, canvas {
  display: none;
}
html { font-family: serif; }
body { margin: 8px; }
p, blockquote, dl, figure { margin-top: 1em; margin-bottom: 1em; }
blockquote, figure { margin-left: 40px; margin-right: 40px; }
dd { margin-left: 40px; }
h1 { font-size: 2em; margin-top: 0.67em; margin-bottom: 0.67em; font-weight: bold; }
h2 { font-size: 1.5em; margin-top: 0.83em; margin-bottom: 0.83em; font-weight: bold; }
h3 { font-size: 1.17em; margin-top: 1em; margin-bottom: 1em; font-weight: bold; }
h4 { margin-top: 1.33em; margin-bottom: 1.33em; font-weight: bold; }
h5 { font-size: 0.83em; margin-top: 1.67em; margin-bottom: 1.67em; font-weight: bold; }
h6 { font-size: 0.67em; margin-top: 2.33em; margin-bottom: 2.33em; font-weight: bold; }
ul, ol, menu, dir { margin-top: 1em; margin-bottom: 1em; padding-left: 40px; }
ol { list-style-type: decimal; }
ul ul, ol ul, ul ol, ol ol { margin-top: 0; margin-bottom: 0; }
ul ul, ol ul { list-style-type: circle; }
li { display: list-item; }
table { display: table; border-spacing: 2px; box-sizing: border-box; }
caption { display: table-caption; text-align: center; }
thead, tbody, tfoot { display: table-row-group; vertical-align: middle; }
tr { display: table-row; vertical-align: inherit; }
td, th { display: table-cell; vertical-align: inherit; padding: 1px; }
th { font-weight: bold; text-align: -internal-center; }
b, strong { font-weight: bold; }
i, em, cite, var, dfn, address { font-style: italic; }
u, ins { text-decoration: underline; }
s, strike, del { text-decoration: line-through; }
pre, xmp, listing, plaintext { white-space: pre; margin-top: 1em; margin-bottom: 1em; }
pre, xmp, listing, plaintext, code, kbd, samp, tt, textarea { font-family: monospace; }
small { font-size: smaller; }
big { font-size: larger; }
sub, sup { font-size: smaller; }
sub { vertical-align: sub; }
sup { vertical-align: super; }
a:link { color: #0000ee; text-decoration: underline; }
center { text-align: -webkit-center; }
hr { border-width: 1px; border-style: solid; border-color: #c0c0c0; margin-top: 0.5em; margin-bottom: 0.5em; }
input, select, button, textarea { display: inline-block; font-size: 13.333px; font-family: sans-serif; font-weight: normal; font-style: normal; text-align: left; color: black; }
input, textarea, select { border: 1px solid #767676; padding: 1px 2px; background-color: white; }
button, input[type=submit], input[type=button], input[type=reset] { border: 1px solid #767676; padding: 1px 6px; background-color: #efefef; text-align: center; }
nobr { white-space: nowrap; }
`

var uaSheet = css.Parse(uaCSS)

// presentationalHints maps legacy HTML attributes to CSS declarations, per
// the HTML spec's rendering section.
func presentationalHints(n *dom.Node) []css.Declaration {
	var out []css.Declaration
	add := func(prop, value string) {
		out = append(out, css.Declaration{Property: prop, Value: value})
	}
	attr := func(name string) (string, bool) {
		v, ok := n.Attr(name)
		return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
	}
	if v, ok := attr("bgcolor"); ok {
		if c, ok := legacyColor(v); ok {
			add("background-color", c)
		}
	}
	switch n.Tag {
	case "body":
		if v, ok := attr("text"); ok {
			if c, ok := legacyColor(v); ok {
				add("color", c)
			}
		}
	case "table":
		dims(n, add, "width", "height")
		if v, ok := attr("cellspacing"); ok {
			if px, ok := legacyPx(v); ok {
				add("border-spacing", px)
			}
		}
		if v, ok := attr("border"); ok {
			if px, ok := legacyPx(v); ok && px != "0px" {
				for _, d := range css.Expand(css.Declaration{Property: "border", Value: px + " outset gray"}) {
					out = append(out, d)
				}
			}
		}
		if v, ok := attr("align"); ok {
			switch strings.ToLower(v) {
			case "center":
				add("margin-left", "auto")
				add("margin-right", "auto")
			}
		}
	case "td", "th":
		dims(n, add, "width", "height")
		alignHints(n, add)
		if _, ok := n.Attr("nowrap"); ok {
			add("white-space", "nowrap")
		}
		if table := enclosingTable(n); table != nil {
			if v, ok := table.Attr("cellpadding"); ok {
				if px, ok := legacyPx(strings.TrimSpace(v)); ok {
					for _, s := range []string{"top", "right", "bottom", "left"} {
						add("padding-"+s, px)
					}
				}
			}
			if v, ok := table.Attr("border"); ok {
				if px, ok := legacyPx(strings.TrimSpace(v)); ok && px != "0px" {
					for _, d := range css.Expand(css.Declaration{Property: "border", Value: "1px inset gray"}) {
						out = append(out, d)
					}
				}
			}
		}
	case "tr", "tbody", "thead", "tfoot":
		alignHints(n, add)
		if n.Tag == "tr" {
			dims(n, add, "height")
		}
	case "img":
		dims(n, add, "width", "height")
		if v, ok := attr("border"); ok {
			if px, ok := legacyPx(v); ok {
				for _, d := range css.Expand(css.Declaration{Property: "border", Value: px + " solid"}) {
					out = append(out, d)
				}
			}
		}
		if v, ok := attr("hspace"); ok {
			if px, ok := legacyPx(v); ok {
				add("margin-left", px)
				add("margin-right", px)
			}
		}
		if v, ok := attr("vspace"); ok {
			if px, ok := legacyPx(v); ok {
				add("margin-top", px)
				add("margin-bottom", px)
			}
		}
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "caption":
		if v, ok := attr("align"); ok {
			switch v = strings.ToLower(v); v {
			case "left", "right", "justify":
				add("text-align", v)
			case "center", "middle":
				add("text-align", "-webkit-center")
			}
		}
	case "font":
		if v, ok := attr("color"); ok {
			if c, ok := legacyColor(v); ok {
				add("color", c)
			}
		}
		if v, ok := attr("face"); ok {
			add("font-family", v)
		}
		if v, ok := attr("size"); ok {
			if s, ok := legacyFontSize(v); ok {
				add("font-size", s)
			}
		}
	case "hr":
		dims(n, add, "width")
		if v, ok := attr("size"); ok {
			if px, ok := legacyPx(v); ok {
				add("height", px)
			}
		}
		if v, ok := attr("color"); ok {
			if c, ok := legacyColor(v); ok {
				add("border-top-color", c)
				add("border-bottom-color", c)
				add("border-left-color", c)
				add("border-right-color", c)
				add("background-color", c)
			}
		}
	case "input":
		if v, ok := attr("size"); ok {
			if sz, err := strconv.Atoi(v); err == nil && sz > 0 {
				// Approximate the average character width in em.
				add("width", strconv.FormatFloat(float64(sz)*0.55, 'f', 2, 64)+"em")
			}
		}
	case "textarea":
		if v, ok := attr("cols"); ok {
			if c, err := strconv.Atoi(v); err == nil && c > 0 {
				add("width", strconv.FormatFloat(float64(c)*0.6, 'f', 2, 64)+"em")
			}
		}
		if v, ok := attr("rows"); ok {
			if r, err := strconv.Atoi(v); err == nil && r > 0 {
				add("height", strconv.FormatFloat(float64(r)*1.2, 'f', 2, 64)+"em")
			}
		}
	}
	return out
}

func alignHints(n *dom.Node, add func(string, string)) {
	if v, ok := n.Attr("align"); ok {
		switch v = strings.ToLower(strings.TrimSpace(v)); v {
		case "left", "right", "justify":
			add("text-align", v)
		case "center", "middle":
			add("text-align", "-webkit-center")
		}
	}
	if v, ok := n.Attr("valign"); ok {
		switch v = strings.ToLower(strings.TrimSpace(v)); v {
		case "top", "middle", "bottom", "baseline":
			add("vertical-align", v)
		}
	}
}

func dims(n *dom.Node, add func(string, string), names ...string) {
	for _, name := range names {
		if v, ok := n.Attr(name); ok {
			if l, ok := legacyDimension(strings.TrimSpace(v)); ok {
				add(name, l)
			}
		}
	}
}

func enclosingTable(n *dom.Node) *dom.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == dom.ElementNode && p.Tag == "table" {
			return p
		}
	}
	return nil
}

// legacyDimension parses an HTML dimension attribute ("85%", "18").
func legacyDimension(v string) (string, bool) {
	i := 0
	for i < len(v) && ((v[i] >= '0' && v[i] <= '9') || v[i] == '.') {
		i++
	}
	if i == 0 {
		return "", false
	}
	f, err := strconv.ParseFloat(v[:i], 64)
	if err != nil {
		return "", false
	}
	num := strconv.FormatFloat(f, 'f', -1, 64)
	if i < len(v) && v[i] == '%' {
		return num + "%", true
	}
	if f == 0 {
		return "", false
	}
	return num + "px", true
}

// legacyPx parses a non-negative integer attribute value as px.
func legacyPx(v string) (string, bool) {
	i := 0
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", false
	}
	return v[:i] + "px", true
}

// legacyColor parses an HTML color attribute, which may omit the '#'.
func legacyColor(v string) (string, bool) {
	if _, ok := css.ParseColor(v); ok {
		return v, true
	}
	if len(v) == 6 || len(v) == 3 {
		if _, ok := css.ParseColor("#" + v); ok {
			return "#" + v, true
		}
	}
	return "", false
}

func legacyFontSize(v string) (string, bool) {
	sizes := []string{"x-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large"}
	rel := 0
	if strings.HasPrefix(v, "+") {
		rel = 1
	} else if strings.HasPrefix(v, "-") {
		rel = -1
	}
	n, err := strconv.Atoi(strings.TrimLeft(v, "+-"))
	if err != nil {
		return "", false
	}
	if rel != 0 {
		n = 3 + rel*n
	}
	if n < 1 {
		n = 1
	}
	if n > 7 {
		n = 7
	}
	return sizes[n], true
}
