package css

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

func TestParseStylesheet(t *testing.T) {
	ss := Parse(`
/* comment */
body  { font-family:Verdana, Geneva, sans-serif; font-size:10pt; color:#828282; }
a:link, .x > b { color:#000000 !important; text-decoration:none; }
@import url("foo.css");
@font-face { font-family: X; src: url(x.woff) }
@media only screen and (min-width : 300px) and (max-width : 750px) {
  #hnmain { width: 100%; min-width: 0; }
}
.bad::before { content: "}" }
.votearrow { background: url("triangle.svg"), linear-gradient(transparent, transparent) no-repeat; }
`)
	if len(ss.Rules) != 4 {
		for _, r := range ss.Rules {
			t.Logf("rule %v", r.Selectors[0].Text)
		}
		t.Fatalf("got %d rules", len(ss.Rules))
	}
	body := ss.Rules[0]
	if body.Selectors[0].Text != "body" || len(body.Decls) != 3 || body.Decls[1] != (Declaration{"font-size", "10pt", false}) {
		t.Errorf("body rule = %+v", body)
	}
	if len(ss.Rules[1].Selectors) != 2 || !ss.Rules[1].Decls[0].Important || ss.Rules[1].Decls[1].Property != "text-decoration-line" {
		t.Errorf("rule 1 = %+v", ss.Rules[1])
	}
	if ss.Rules[2].Media == "" || ss.Rules[2].Selectors[0].Compounds[0].ID != "hnmain" {
		t.Errorf("media rule = %+v", ss.Rules[2])
	}
	bg := ss.Rules[3].Decls
	if len(bg) != 2 || bg[0].Value != "transparent" || bg[1].Value != `url("triangle.svg")` {
		t.Errorf("background = %+v", bg)
	}
}

func TestExpand(t *testing.T) {
	got := ParseDeclarations("margin: 3px 2px 6px; border:1px white solid; padding: 0 10px; font: bold 12px/1.5 Arial, sans-serif")
	want := map[string]string{
		"margin-top": "3px", "margin-right": "2px", "margin-bottom": "6px", "margin-left": "2px",
		"border-top-width": "1px", "border-left-style": "solid", "border-bottom-color": "white",
		"padding-top": "0", "padding-left": "10px",
		"font-weight": "bold", "font-size": "12px", "line-height": "1.5", "font-family": "Arial, sans-serif",
	}
	m := map[string]string{}
	for _, d := range got {
		m[d.Property] = d.Value
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]color.NRGBA{
		"#ff6600":             {0xff, 0x66, 0x00, 0xff},
		"#fff":                {0xff, 0xff, 0xff, 0xff},
		"WHITE":               {0xff, 0xff, 0xff, 0xff},
		"rgb(1, 2, 3)":        {1, 2, 3, 255},
		"rgba(0,0,0,0.5)":     {0, 0, 0, 128},
		"rgb(100% 0% 0% / 1)": {255, 0, 0, 255},
		"transparent":         {},
	}
	for in, want := range cases {
		got, ok := ParseColor(in)
		if !ok || got != want {
			t.Errorf("ParseColor(%q) = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"#ggg", "rgb(1,2)", "nope", "#12345"} {
		if _, ok := ParseColor(bad); ok {
			t.Errorf("ParseColor(%q) succeeded", bad)
		}
	}
}

func TestMediaMatches(t *testing.T) {
	cases := []struct {
		q    string
		w    float64
		want bool
	}{
		{"", 1000, true},
		{"screen", 1000, true},
		{"print", 1000, false},
		{"only screen and (min-width : 300px) and (max-width : 750px)", 1000, false},
		{"only screen and (min-width : 300px) and (max-width : 750px)", 500, true},
		{"only screen and (-webkit-min-device-pixel-ratio: 2), only screen and (min-device-pixel-ratio: 2)", 1000, false},
		{"not print", 1000, true},
		{"print, screen and (max-width: 40em)", 600, true},
	}
	for _, c := range cases {
		if got := MediaMatches(c.q, c.w); got != c.want {
			t.Errorf("MediaMatches(%q, %v) = %v", c.q, c.w, got)
		}
	}
}

func TestSelectors(t *testing.T) {
	// <div id=main class="a b"><p class=x>1</p><span>2</span><a href=#>3</a></div>
	div := dom.NewElement("div", dom.Attr{Name: "id", Value: "main"}, dom.Attr{Name: "class", Value: "a b"})
	p := dom.NewElement("p", dom.Attr{Name: "class", Value: "x"})
	span := dom.NewElement("span")
	a := dom.NewElement("a", dom.Attr{Name: "href", Value: "#"}, dom.Attr{Name: "type", Value: "submit"})
	div.AppendChild(p)
	div.AppendChild(span)
	div.AppendChild(a)
	root := dom.NewElement("body")
	root.AppendChild(div)

	cases := []struct {
		sel  string
		n    *dom.Node
		want bool
	}{
		{"div", div, true},
		{"*", span, true},
		{"#main", div, true},
		{"div.a.b", div, true},
		{".a.c", div, false},
		{"body span", span, true},
		{"body > span", span, false},
		{"div > span", span, true},
		{"p + span", span, true},
		{"p ~ a", a, true},
		{"span + p", p, false},
		{"a:link", a, true},
		{"a:visited", a, false},
		{"a:hover", a, false},
		{"p:first-child", p, true},
		{"a:last-child", a, true},
		{"[type='submit']", a, true},
		{"a[type^=sub]", a, true},
		{"[href]", span, false},
		{".a .x", p, true},
	}
	for _, c := range cases {
		s := ParseSelector(c.sel)
		if s == nil {
			t.Errorf("ParseSelector(%q) failed", c.sel)
			continue
		}
		if got := s.Matches(c.n); got != c.want {
			t.Errorf("%q matches <%s> = %v, want %v", c.sel, c.n.Tag, got, c.want)
		}
	}
	for _, bad := range []string{"", "a >", "> a", "p::before", "li:nth-child(2)", "a:unknown"} {
		if ParseSelector(bad) != nil {
			t.Errorf("ParseSelector(%q) should fail", bad)
		}
	}
}

func TestSpecificity(t *testing.T) {
	cases := map[string]Specificity{
		"a":                {0, 0, 1},
		".subtext a:link":  {0, 2, 1},
		"#hnmain td.title": {1, 1, 1},
		"*":                {0, 0, 0},
	}
	for sel, want := range cases {
		if got := ParseSelector(sel).Specificity(); !reflect.DeepEqual(got, want) {
			t.Errorf("Specificity(%q) = %v, want %v", sel, got, want)
		}
	}
}
