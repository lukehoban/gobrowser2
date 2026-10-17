package html

import (
	"strings"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

func TestTokenize(t *testing.T) {
	toks := Tokenize(`<!DOCTYPE html><a href="x?a=1&amp;b=2" class='c' disabled>Tom &amp; Jerry&nbsp;&#x41;&#66;</a><!-- c --><br/>`)
	want := []struct {
		typ  TokenType
		data string
	}{
		{DoctypeToken, "html"},
		{StartTagToken, "a"},
		{TextToken, "Tom & Jerry\u00a0AB"},
		{EndTagToken, "a"},
		{CommentToken, " c "},
		{StartTagToken, "br"},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	for i, w := range want {
		if toks[i].Type != w.typ || toks[i].Data != w.data {
			t.Errorf("token %d = %+v, want %+v", i, toks[i], w)
		}
	}
	attrs := toks[1].Attrs
	if len(attrs) != 3 || attrs[0].Value != "x?a=1&b=2" || attrs[1].Value != "c" || attrs[2].Name != "disabled" {
		t.Errorf("attrs = %+v", attrs)
	}
	if !toks[5].SelfClosing {
		t.Errorf("br not self-closing")
	}
}

func TestTokenizeRawText(t *testing.T) {
	toks := Tokenize(`<script>if (a < b && c) { x = "</div>"; }</script><title>A &amp; B</title>`)
	if toks[1].Type != TextToken || toks[1].Data != `if (a < b && c) { x = "</div>"; }` {
		t.Errorf("script text = %+v", toks[1])
	}
	if toks[4].Data != "A & B" {
		t.Errorf("title text = %+v", toks[4])
	}
}

func TestTokenizeStrayLessThan(t *testing.T) {
	toks := Tokenize(`a < b <3`)
	if len(toks) != 1 || toks[0].Data != "a < b <3" {
		t.Errorf("tokens = %+v", toks)
	}
}

func TestDecodeEntities(t *testing.T) {
	cases := map[string]string{
		"&lt;&gt;&amp;&quot;":   `<>&"`,
		"&unknown; &":           "&unknown; &",
		"&#x2F; &#47; &#150;":   "/ / –",
		"&amp &copy 2024":       "& © 2024",
		"&mdash;&hellip;&euro;": "—…€",
	}
	for in, want := range cases {
		if got := DecodeEntities(in, false); got != want {
			t.Errorf("DecodeEntities(%q) = %q, want %q", in, got, want)
		}
	}
	if got := DecodeEntities("?a=1&amp=2", true); got != "?a=1&amp=2" {
		t.Errorf("attr legacy entity = %q", got)
	}
}

// structure renders a compact tag-only outline of the tree.
func structure(n *dom.Node) string {
	var sb strings.Builder
	var rec func(*dom.Node)
	rec = func(n *dom.Node) {
		switch n.Type {
		case dom.ElementNode:
			sb.WriteString("<" + n.Tag + ">")
			for _, c := range n.Children {
				rec(c)
			}
			sb.WriteString("</" + n.Tag + ">")
		case dom.TextNode:
			sb.WriteString(n.Text)
		case dom.DocumentNode:
			for _, c := range n.Children {
				rec(c)
			}
		}
	}
	rec(n)
	return sb.String()
}

func TestParse(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "<html><head></head><body></body></html>"},
		{"hello", "<html><head></head><body>hello</body></html>"},
		{"<title>T</title><p>a<p>b", "<html><head><title>T</title></head><body><p>a</p><p>b</p></body></html>"},
		{"<p>a<div>b</div>", "<html><head></head><body><p>a</p><div>b</div></body></html>"},
		{"<ul><li>a<li>b</ul>", "<html><head></head><body><ul><li>a</li><li>b</li></ul></body></html>"},
		{"<img src=x><br>t", "<html><head></head><body><img></img><br></br>t</body></html>"},
		{"<span>a</div>b</span>", "<html><head></head><body><span>ab</span></body></html>"},
		{"<a>1<a>2", "<html><head></head><body><a>1</a><a>2</a></body></html>"},
		{"x</p>y", "<html><head></head><body>x<p></p>y</body></html>"},
		// Tables: implicit tbody, implied end tags, whitespace dropped.
		{"<table> <tr><td>a<td>b <tr><td>c</table>",
			"<html><head></head><body><table><tbody><tr><td>a</td><td>b </td></tr><tr><td>c</td></tr></tbody></table></body></html>"},
		// Self-closing non-void tr is opened (HN uses <tr .../>).
		{"<table><tr><td>a</td></tr><tr/><tr><td>b</td></tr></table>",
			"<html><head></head><body><table><tbody><tr><td>a</td></tr><tr></tr><tr><td>b</td></tr></tbody></table></body></html>"},
		// Nested tables.
		{"<table><tr><td><table><tr><td>in</table>out</td></tr></table>",
			"<html><head></head><body><table><tbody><tr><td><table><tbody><tr><td>in</td></tr></tbody></table>out</td></tr></tbody></table></body></html>"},
		// Stray end tags inside cells don't escape the cell.
		{"<table><tr><td>a</span></b>b</td></tr></table>",
			"<html><head></head><body><table><tbody><tr><td>ab</td></tr></tbody></table></body></html>"},
		// Foster parenting.
		{"<table>oops<tr><td>a</td></tr></table>",
			"<html><head></head><body>oops<table><tbody><tr><td>a</td></tr></tbody></table></body></html>"},
		// Cell without row.
		{"<table><td>a</td></table>",
			"<html><head></head><body><table><tbody><tr><td>a</td></tr></tbody></table></body></html>"},
		// SVG self-closing children.
		{"<svg><path d=x /><rect/></svg>", "<html><head></head><body><svg><path></path><rect></rect></svg></body></html>"},
	}
	for _, c := range cases {
		if got := structure(Parse(c.in)); got != c.want {
			t.Errorf("Parse(%q)\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

func TestParseAttributes(t *testing.T) {
	doc := Parse(`<html lang="en"><body bgcolor="#fff"><table id="hnmain" width="85%"><tr><td>x</td></tr></table>`)
	html := doc.Find("html")
	if v, _ := html.Attr("lang"); v != "en" {
		t.Errorf("html lang = %q", v)
	}
	if v, _ := doc.Find("body").Attr("bgcolor"); v != "#fff" {
		t.Errorf("body bgcolor = %q", v)
	}
	if v, _ := doc.Find("table").Attr("width"); v != "85%" {
		t.Errorf("table width = %q", v)
	}
}

func TestQuirksMode(t *testing.T) {
	cases := []struct {
		in     string
		quirks bool
	}{
		{`<html><body>x`, true},
		{`<!DOCTYPE html><html><body>x`, false},
		{`<!doctype HTML>x`, false},
		{`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">x`, true},
		{`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">x`, false},
		{`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">x`, false},
		{`<!DOCTYPE foo>x`, true},
	}
	for _, c := range cases {
		if got := Parse(c.in).Quirks; got != c.quirks {
			t.Errorf("Parse(%q).Quirks = %v, want %v", c.in, got, c.quirks)
		}
	}
}
