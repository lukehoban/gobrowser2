package css

import (
	"net/url"
	"strings"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

func TestParse(t *testing.T) {
	base, _ := url.Parse("https://example.com/s/news.css")
	ss := Parse(`
/* comment */
body { color: #828282; font-size: 10pt }
@media only screen and (max-width: 750px) { body { color: red } }
@import "x.css";
a:link, .x > b { color: black !important; background: url("tri.svg") }
p::before { content: "x" }
.ok { margin: 0 }
`, base)
	if len(ss.Rules) != 3 {
		t.Fatalf("got %d rules, want 3 (media skipped, pseudo-element rule dropped)", len(ss.Rules))
	}
	r := ss.Rules[1]
	if len(r.Selectors) != 2 {
		t.Fatalf("got %d selectors", len(r.Selectors))
	}
	if !r.Decls[0].Important || r.Decls[0].Value != "black" {
		t.Errorf("decl = %+v", r.Decls[0])
	}
	if got := r.Decls[1].Value; got != `url("https://example.com/s/tri.svg")` {
		t.Errorf("url not resolved: %s", got)
	}
}

func TestSelectorMatching(t *testing.T) {
	doc, _ := dom.Parse(strings.NewReader(`<div id="m" class="a b"><span class="sub"><a href="x" class="u">l</a></span><input type="submit"><p>1</p><p>2</p></div>`))
	a := doc.Find("a")
	in := doc.Find("input")
	ps := doc.FindAll("p")
	cases := []struct {
		sel  string
		n    *dom.Node
		want bool
	}{
		{"a", a, true},
		{"a:link", a, true},
		{"a:visited", a, false},
		{"a:hover", a, false},
		{".sub a", a, true},
		{"div > a", a, false},
		{"span > a.u", a, true},
		{"#m .sub > a", a, true},
		{".a.b a", a, true},
		{".a.c a", a, false},
		{"input[type='submit']", in, true},
		{"input[type=text]", in, false},
		{"[type]", in, true},
		{"p + p", ps[1], true},
		{"p + p", ps[0], false},
		{"span ~ p", ps[1], true},
		{"p:last-child", ps[1], true},
		{"*", in, true},
	}
	for _, c := range cases {
		sels, ok := ParseSelectorList(c.sel)
		if !ok {
			t.Errorf("%q: parse failed", c.sel)
			continue
		}
		if got := sels[0].Matches(c.n); got != c.want {
			t.Errorf("%q matches <%s> = %v, want %v", c.sel, c.n.Tag, got, c.want)
		}
	}
}

func TestSpecificity(t *testing.T) {
	spec := func(s string) Specificity {
		sels, ok := ParseSelectorList(s)
		if !ok {
			t.Fatalf("parse %q", s)
		}
		return sels[0].Spec
	}
	if got := spec("#a .b c:link"); got != (Specificity{1, 2, 1}) {
		t.Errorf("got %v", got)
	}
	if !spec(".subtext a:link").Less(spec("#x a")) {
		t.Error("class+pseudo should be less specific than id")
	}
}

func TestSplitValue(t *testing.T) {
	got := SplitValue(`url("a b.svg"), linear-gradient(transparent, transparent) no-repeat`)
	want := []string{`url("a b.svg")`, ",", "linear-gradient(transparent, transparent)", "no-repeat"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
}
