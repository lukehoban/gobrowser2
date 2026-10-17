package html

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// outline serializes the children of n with every element explicitly
// closed, attributes omitted, and text quoted, so tests can assert on tree
// shape compactly.
func outline(n *Node) string {
	var sb strings.Builder
	for _, c := range n.Children {
		writeOutline(&sb, c)
	}
	return sb.String()
}

func writeOutline(sb *strings.Builder, n *Node) {
	switch n.Type {
	case ElementNode:
		sb.WriteString("<" + n.Data + ">")
		for _, c := range n.Children {
			writeOutline(sb, c)
		}
		if !voidElements[n.Data] {
			sb.WriteString("</" + n.Data + ">")
		}
	case TextNode:
		sb.WriteString(strconv.Quote(n.Data))
	case CommentNode:
		sb.WriteString("<!--" + n.Data + "-->")
	case DoctypeNode:
		sb.WriteString("<!DOCTYPE " + n.Data + ">")
	}
}

// body returns the outline of the <body> element's children.
func body(doc *Node) string {
	var find func(*Node) *Node
	find = func(n *Node) *Node {
		if n.Type == ElementNode && n.Data == "body" {
			return n
		}
		for _, c := range n.Children {
			if b := find(c); b != nil {
				return b
			}
		}
		return nil
	}
	b := find(doc)
	if b == nil {
		return "<no body>"
	}
	return outline(b)
}

func TestParseDocumentStructure(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"empty", "", ""},
		{"implied html head body", "hello", `<html><head></head><body>"hello"</body></html>`},
		{"explicit", "<html><head><title>T</title></head><body>x</body></html>",
			`<html><head><title>"T"</title></head><body>"x"</body></html>`},
		{"head elements without head", "<meta charset=utf-8><title>T</title><p>x",
			`<html><head><meta><title>"T"</title></head><body><p>"x"</p></body></html>`},
		{"doctype and comments", "<!DOCTYPE html><!-- c --><html><body></body></html>",
			`<!DOCTYPE html><!-- c --><html><head></head><body></body></html>`},
		{"whitespace before body dropped", "<html>\n <head>\n</head>\n<body>x",
			`<html><head></head><body>"x"</body></html>`},
		{"content after body stays in body", "<body>a</body><script>s</script></html>",
			`<html><head></head><body>"a"<script>"s"</script></body></html>`},
		{"head content after head end", "<head></head><link rel=x><p>a",
			`<html><head><link></head><body><p>"a"</p></body></html>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outline(Parse(tt.src)); got != tt.want {
				t.Errorf("Parse(%q)\n got: %s\nwant: %s", tt.src, got, tt.want)
			}
		})
	}
}

func TestParseBody(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"void elements", "a<br>b<img src=x>c<hr>", `"a"<br>"b"<img>"c"<hr>`},
		{"self-closing non-void ignored", "<div/>a</div>b", `<div>"a"</div>"b"`},
		{"p closed by block", "<p>a<div>b</div>", `<p>"a"</p><div>"b"</div>`},
		{"p closed by p", "<p>a<p>b", `<p>"a"</p><p>"b"</p>`},
		{"stray end p", "a</p>b", `"a"<p></p>"b"`},
		{"end br", "a</br>b", `"a"<br>"b"`},
		{"li implied end", "<ul><li>a<li>b</ul>", `<ul><li>"a"</li><li>"b"</li></ul>`},
		{"nested list li", "<ul><li>a<ul><li>b</ul><li>c</ul>",
			`<ul><li>"a"<ul><li>"b"</li></ul></li><li>"c"</li></ul>`},
		{"dd dt", "<dl><dt>a<dd>b<dt>c</dl>", `<dl><dt>"a"</dt><dd>"b"</dd><dt>"c"</dt></dl>`},
		{"option", "<select><option>a<option>b</select>",
			`<select><option>"a"</option><option>"b"</option></select>`},
		{"headings", "<h1>a<h2>b</h1>c", `<h1>"a"</h1><h2>"b"</h2>"c"`},
		{"nested a", "<a>1<a>2</a>3", `<a>"1"</a><a>"2"</a>"3"`},
		{"unmatched end tag ignored", "<div>a</span>b</div>", `<div>"ab"</div>`},
		{"end tag blocked by special", "<span><div>a</span>b</div>", `<span><div>"ab"</div></span>`},
		{"formatting end tag", "<b>a<i>b</b>c", `<b>"a"<i>"b"</i></b>"c"`},
		{"pre leading newline", "<pre>\nx\n</pre>", `<pre>"x\n"</pre>`},
		{"textarea leading newline", "<textarea>\n&lt;x</textarea>", `<textarea>"<x"</textarea>`},
		{"block in inline", "<center><a href=x><div>v</div></a></center>",
			`<center><a><div>"v"</div></a></center>`},
		{"unclosed at eof", "<div><span>a", `<div><span>"a"</span></div>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := body(Parse(tt.src)); got != tt.want {
				t.Errorf("Parse(%q)\n got: %s\nwant: %s", tt.src, got, tt.want)
			}
		})
	}
}

func TestParseTables(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"implied tbody", "<table><tr><td>a</td></tr></table>",
			`<table><tbody><tr><td>"a"</td></tr></tbody></table>`},
		{"implied tbody and tr", "<table><td>a</table>",
			`<table><tbody><tr><td>"a"</td></tr></tbody></table>`},
		{"unclosed cells and rows", "<table><tr><td>a<td>b<tr><td>c</table>",
			`<table><tbody><tr><td>"a"</td><td>"b"</td></tr><tr><td>"c"</td></tr></tbody></table>`},
		{"self-closing tr (HN)", "<table><tr><td>a</td></tr><tr style='height:10px'/><tr><td>b</td></tr></table>",
			`<table><tbody><tr><td>"a"</td></tr><tr></tr><tr><td>"b"</td></tr></tbody></table>`},
		{"explicit sections", "<table><thead><tr><th>h<tbody><tr><td>d</table>",
			`<table><thead><tr><th>"h"</th></tr></thead><tbody><tr><td>"d"</td></tr></tbody></table>`},
		{"nested tables", "<table><tr><td><table><tr><td>in</table>out</table>",
			`<table><tbody><tr><td><table><tbody><tr><td>"in"</td></tr></tbody></table>"out"</td></tr></tbody></table>`},
		{"cell closes open inline", "<table><tr><td><span>a<td>b</table>",
			`<table><tbody><tr><td><span>"a"</span></td><td>"b"</td></tr></tbody></table>`},
		{"stray div end in cell ignored", "<div><table><tr><td>a</div>b</table></div>",
			`<div><table><tbody><tr><td>"ab"</td></tr></tbody></table></div>`},
		{"stray cell outside table ignored", "<td>a</td><tr>b",
			`"ab"`},
		{"foster parented text", "<table>x<tr><td>a</table>",
			`"x"<table><tbody><tr><td>"a"</td></tr></tbody></table>`},
		{"foster parented element", "<table><tr><div>x</div><td>a</table>",
			`<div>"x"</div><table><tbody><tr><td>"a"</td></tr></tbody></table>`},
		{"whitespace kept in table", "<table> <tr> <td>a</table>",
			`<table>" "<tbody><tr>" "<td>"a"</td></tr></tbody></table>`},
		{"col implies colgroup", "<table><col><tr><td>a</table>",
			`<table><colgroup><col></colgroup><tbody><tr><td>"a"</td></tr></tbody></table>`},
		{"quirks table does not close p", "<p><table><tr><td>a</table>",
			`<p><table><tbody><tr><td>"a"</td></tr></tbody></table></p>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := body(Parse(tt.src)); got != tt.want {
				t.Errorf("Parse(%q)\n got: %s\nwant: %s", tt.src, got, tt.want)
			}
		})
	}
}

func TestStandardsTableClosesP(t *testing.T) {
	doc := Parse("<!DOCTYPE html><p><table></table>")
	if doc.Quirks {
		t.Fatal("expected no-quirks mode")
	}
	if got, want := body(doc), `<p></p><table></table>`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestQuirksMode(t *testing.T) {
	tests := []struct {
		src    string
		quirks bool
	}{
		{"<p>x", true},
		{"<!DOCTYPE html>", false},
		{"<!doctype HTML>", false},
		{`<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`, false},
		{`<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">`, true},
		{`<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">`, false},
		{`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "x">`, false},
		{`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">`, true},
	}
	for _, tt := range tests {
		if got := Parse(tt.src).Quirks; got != tt.quirks {
			t.Errorf("Parse(%q).Quirks = %v, want %v", tt.src, got, tt.quirks)
		}
	}
}

func TestAttributes(t *testing.T) {
	doc := Parse(`<a HREF="x?a=1&amp;b=2" id='q' data-x=bare checked class="a" class="dup" title="&copy=1" alt="&copy 2">`)
	a := doc.Children[0].Children[1].Children[0]
	want := []Attr{
		{"href", "x?a=1&b=2"}, {"id", "q"}, {"data-x", "bare"}, {"checked", ""},
		{"class", "a"}, {"title", "&copy=1"}, {"alt", "\u00a9 2"},
	}
	if len(a.Attrs) != len(want) {
		t.Fatalf("attrs = %v, want %v", a.Attrs, want)
	}
	for i := range want {
		if a.Attrs[i] != want[i] {
			t.Errorf("attr %d = %v, want %v", i, a.Attrs[i], want[i])
		}
	}
	if v, ok := a.GetAttr("id"); !ok || v != "q" {
		t.Errorf("GetAttr(id) = %q, %v", v, ok)
	}
	if _, ok := a.GetAttr("missing"); ok {
		t.Error("GetAttr(missing) reported present")
	}
}

func TestDump(t *testing.T) {
	var sb strings.Builder
	if err := Parse(`<p class="x">a&nbsp;b<!--c-->`).Dump(&sb); err != nil {
		t.Fatal(err)
	}
	want := `#document (quirks)
  <html>
    <head>
    <body>
      <p class="x">
        "a\u00a0b"
        <!-- c -->
`
	if sb.String() != want {
		t.Errorf("Dump:\n%s\nwant:\n%s", sb.String(), want)
	}
}

func TestParseHNLikeFixture(t *testing.T) {
	src, err := os.ReadFile("testdata/hn_like.html")
	if err != nil {
		t.Fatal(err)
	}
	doc := Parse(string(src))
	if !doc.Quirks {
		t.Error("HN has no doctype; expected quirks mode")
	}

	var rows, cells int
	var hnmain *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode {
			switch n.Data {
			case "tr":
				rows++
				if n.Parent.Data != "tbody" {
					t.Errorf("tr parent = %s, want tbody", n.Parent.Data)
				}
			case "td":
				cells++
				if n.Parent.Data != "tr" {
					t.Errorf("td parent = %s, want tr", n.Parent.Data)
				}
			case "table":
				if id, _ := n.GetAttr("id"); id == "hnmain" {
					hnmain = n
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	if rows != 9 || cells != 11 {
		t.Errorf("got %d rows and %d cells, want 9 and 11", rows, cells)
	}
	if hnmain == nil {
		t.Fatal("no #hnmain table")
	}
	if hnmain.Parent.Data != "center" {
		t.Errorf("#hnmain parent = %s, want center", hnmain.Parent.Data)
	}
	mainRows := hnmain.Children[0].Children
	if len(mainRows) != 4 {
		t.Errorf("#hnmain has %d rows, want 4 (header, spacer, bigbox, footer)", len(mainRows))
	}
}
