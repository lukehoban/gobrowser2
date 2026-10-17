package html

import (
	"reflect"
	"testing"
)

func tokens(src string) []Token {
	z := NewTokenizer(src)
	var out []Token
	for {
		tok := z.Next()
		if tok.Type == EOFToken {
			return out
		}
		out = append(out, tok)
	}
}

func TestTokenizer(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Token
	}{
		{"text", "hello", []Token{{Type: TextToken, Data: "hello"}}},
		{"tags", "<P>x</P >", []Token{
			{Type: StartTagToken, Data: "p"},
			{Type: TextToken, Data: "x"},
			{Type: EndTagToken, Data: "p"},
		}},
		{"self-closing", "<br/><tr style='h:1'/>", []Token{
			{Type: StartTagToken, Data: "br", SelfClosing: true},
			{Type: StartTagToken, Data: "tr", Attrs: []Attr{{"style", "h:1"}}, SelfClosing: true},
		}},
		{"unquoted value with slash", "<a href=a/b>", []Token{
			{Type: StartTagToken, Data: "a", Attrs: []Attr{{"href", "a/b"}}},
		}},
		{"spaces around equals", `<a x = "1" y= 2>`, []Token{
			{Type: StartTagToken, Data: "a", Attrs: []Attr{{"x", "1"}, {"y", "2"}}},
		}},
		{"comment", "a<!-- b -- c -->d", []Token{
			{Type: TextToken, Data: "a"},
			{Type: CommentToken, Data: " b -- c "},
			{Type: TextToken, Data: "d"},
		}},
		{"empty comments", "<!---->< !--><!-->", []Token{
			{Type: CommentToken},
			{Type: TextToken, Data: "< !-->"},
			{Type: CommentToken},
		}},
		{"unterminated comment", "<!-- x", []Token{{Type: CommentToken, Data: " x"}}},
		{"bogus comments", "<?xml v?><!x>", []Token{
			{Type: CommentToken, Data: "xml v?"},
			{Type: CommentToken, Data: "x"},
		}},
		{"doctype", `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01//EN" 'sys'>`, []Token{
			{Type: DoctypeToken, Data: "html", Attrs: []Attr{{"public", "-//W3C//DTD HTML 4.01//EN"}, {"system", "sys"}}},
		}},
		{"literal less-than", "a < b <3", []Token{{Type: TextToken, Data: "a < b <3"}}},
		{"empty end tag", "a</>b", []Token{{Type: TextToken, Data: "a"}, {Type: TextToken, Data: "b"}}},
		{"script raw text", "<script>if (a<b) { x = '</div>' }</script>", []Token{
			{Type: StartTagToken, Data: "script"},
			{Type: TextToken, Data: "if (a<b) { x = '</div>' }"},
			{Type: EndTagToken, Data: "script"},
		}},
		{"style raw text, case-insensitive end", "<style>a>b{}</STYLE>", []Token{
			{Type: StartTagToken, Data: "style"},
			{Type: TextToken, Data: "a>b{}"},
			{Type: EndTagToken, Data: "style"},
		}},
		{"raw text prefix not end", "<script></scripts></script>", []Token{
			{Type: StartTagToken, Data: "script"},
			{Type: TextToken, Data: "</scripts>"},
			{Type: EndTagToken, Data: "script"},
		}},
		{"empty raw text", "<script></script>", []Token{
			{Type: StartTagToken, Data: "script"},
			{Type: EndTagToken, Data: "script"},
		}},
		{"rcdata decodes entities", "<title>a &amp; <b></title>", []Token{
			{Type: StartTagToken, Data: "title"},
			{Type: TextToken, Data: "a & <b>"},
			{Type: EndTagToken, Data: "title"},
		}},
		{"unterminated tag", "<div class=x", []Token{
			{Type: StartTagToken, Data: "div", Attrs: []Attr{{"class", "x"}}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokens(tt.src); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokens(%q)\n got: %+v\nwant: %+v", tt.src, got, tt.want)
			}
		})
	}
}

func TestDecodeEntities(t *testing.T) {
	tests := []struct {
		in     string
		inAttr bool
		want   string
	}{
		{"a &amp; b", false, "a & b"},
		{"&lt;&gt;&quot;&apos;", false, "<>\"'"},
		{"7&nbsp;comments", false, "7\u00a0comments"},
		{"tok&#x2F;s", false, "tok/s"},
		{"&#39;&#X41;&#65", false, "'AA"},
		{"&#150;", false, "\u2013"},
		{"&#0;&#xD800;&#x110000;", false, "\ufffd\ufffd\ufffd"},
		{"&unknown; & &;", false, "&unknown; & &;"},
		{"&amp &copy", false, "& \u00a9"},
		{"&notanentity", false, "&notanentity"},
		{"&ampx", false, "&x"},
		{"?a=1&ampx=2", true, "?a=1&ampx=2"},
		{"?a=1&copy=2", true, "?a=1&copy=2"},
		{"&amp;x", true, "&x"},
		{"\u00e9&eacute;", false, "\u00e9\u00e9"},
	}
	for _, tt := range tests {
		if got := decodeEntities(tt.in, tt.inAttr); got != tt.want {
			t.Errorf("decodeEntities(%q, %v) = %q, want %q", tt.in, tt.inAttr, got, tt.want)
		}
	}
}
