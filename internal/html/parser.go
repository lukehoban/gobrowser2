package html

import (
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

// voidElements never have children or end tags.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
	"keygen": true, "basefont": true, "frame": true,
}

// headElements are placed in <head> when they appear before the body starts.
var headElements = map[string]bool{
	"base": true, "basefont": true, "link": true, "meta": true, "noscript": true, "script": true,
	"style": true, "template": true, "title": true,
}

// closesP lists start tags that implicitly close an open <p>.
var closesP = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "center": true, "details": true,
	"dialog": true, "dir": true, "div": true, "dl": true, "fieldset": true, "figcaption": true, "figure": true,
	"footer": true, "form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hgroup": true, "hr": true, "main": true, "menu": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "summary": true, "table": true, "ul": true, "li": true, "dd": true, "dt": true,
	"listing": true, "plaintext": true, "xmp": true,
}

// scopeBoundaries stop the search for a matching open element.
var scopeBoundaries = map[string]bool{
	"html": true, "table": true, "td": true, "th": true, "caption": true, "marquee": true,
	"object": true, "applet": true, "template": true,
}

// tableContext elements cannot directly contain flow content; such content
// is "foster parented" in front of the table.
var tableContext = map[string]bool{"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true}

// tableChildren may legitimately appear directly inside table context.
var tableChildren = map[string]bool{
	"caption": true, "colgroup": true, "col": true, "tbody": true, "thead": true, "tfoot": true,
	"tr": true, "td": true, "th": true, "script": true, "style": true, "template": true, "form": true,
}

// Parse parses an HTML document into a DOM tree. The result always contains
// html, head and body elements.
func Parse(src string) *dom.Node {
	b := &builder{doc: &dom.Node{Type: dom.DocumentNode, Quirks: true}}
	for _, tok := range Tokenize(src) {
		b.token(tok)
	}
	b.ensureBody()
	return b.doc
}

type builder struct {
	doc        *dom.Node
	htmlEl     *dom.Node
	head, body *dom.Node
	stack      []*dom.Node
}

func (b *builder) current() *dom.Node { return b.stack[len(b.stack)-1] }

func (b *builder) ensureHTML(attrs []dom.Attr) {
	if b.htmlEl != nil {
		mergeAttrs(b.htmlEl, attrs)
		return
	}
	b.htmlEl = dom.NewElement("html", attrs...)
	b.doc.AppendChild(b.htmlEl)
	b.stack = []*dom.Node{b.htmlEl}
}

func (b *builder) ensureHead() {
	b.ensureHTML(nil)
	if b.head == nil {
		b.head = dom.NewElement("head")
		b.htmlEl.AppendChild(b.head)
	}
}

func (b *builder) ensureBody() {
	b.ensureHead()
	if b.body == nil {
		b.body = dom.NewElement("body")
		b.htmlEl.AppendChild(b.body)
		b.stack = []*dom.Node{b.htmlEl, b.body}
	}
}

func mergeAttrs(n *dom.Node, attrs []dom.Attr) {
	for _, a := range attrs {
		if _, ok := n.Attr(a.Name); !ok {
			n.Attrs = append(n.Attrs, a)
		}
	}
}

func (b *builder) token(tok Token) {
	switch tok.Type {
	case DoctypeToken:
		if b.htmlEl == nil {
			b.doc.Quirks = !isStandardsDoctype(tok.Data)
		}
		return
	case CommentToken:
		c := &dom.Node{Type: dom.CommentNode, Text: tok.Data}
		if b.htmlEl == nil {
			b.doc.AppendChild(c)
		} else {
			b.current().AppendChild(c)
		}
		return
	}

	if b.body == nil {
		if b.beforeBody(tok) {
			return
		}
		b.ensureBody()
	}

	switch tok.Type {
	case TextToken:
		b.text(tok.Data)
	case StartTagToken:
		b.startTag(tok)
	case EndTagToken:
		b.endTag(tok.Data)
	}
}

// beforeBody handles tokens before <body> has been created. It returns true
// if the token was fully handled.
// isStandardsDoctype reports whether a doctype (the text after
// "<!DOCTYPE") selects standards mode. This is a simplification of the
// HTML spec's quirks-mode detection.
func isStandardsDoctype(d string) bool {
	fields := strings.Fields(strings.ToLower(d))
	if len(fields) == 0 || fields[0] != "html" {
		return false
	}
	if len(fields) == 1 {
		return true
	}
	rest := strings.Join(fields[1:], " ")
	switch {
	case strings.Contains(rest, "xhtml 1.0 strict"), strings.Contains(rest, "xhtml 1.1"),
		strings.Contains(rest, "html 4.01//en\""), strings.Contains(rest, "html 4.0//en\""),
		strings.Contains(rest, "dtd/strict.dtd"), strings.Contains(rest, "about:legacy-compat"):
		return true
	case strings.Contains(rest, "transitional") || strings.Contains(rest, "frameset"):
		// Transitional doctypes with a system identifier are almost standards.
		return strings.Contains(rest, "http://www.w3.org/tr/")
	}
	return false
}

func (b *builder) beforeBody(tok Token) bool {
	switch tok.Type {
	case TextToken:
		if strings.TrimSpace(tok.Data) == "" {
			return true
		}
		// Text inside an open head element (e.g. <title>).
		if len(b.stack) > 0 && b.current() != b.htmlEl && b.current() != b.head {
			b.current().AppendChild(dom.NewText(tok.Data))
			return true
		}
		return false
	case StartTagToken:
		switch tok.Data {
		case "html":
			b.ensureHTML(tok.Attrs)
			return true
		case "head":
			b.ensureHead()
			return true
		case "body":
			b.ensureHead()
			b.body = dom.NewElement("body", tok.Attrs...)
			b.htmlEl.AppendChild(b.body)
			b.stack = []*dom.Node{b.htmlEl, b.body}
			return true
		}
		if headElements[tok.Data] {
			b.ensureHead()
			el := dom.NewElement(tok.Data, tok.Attrs...)
			b.head.AppendChild(el)
			b.stack = []*dom.Node{b.htmlEl}
			if !voidElements[tok.Data] {
				b.stack = append(b.stack, el)
			}
			return true
		}
		return false
	case EndTagToken:
		if len(b.stack) > 1 && b.current().Tag == tok.Data {
			b.stack = b.stack[:len(b.stack)-1]
			return true
		}
		switch tok.Data {
		case "head", "html":
			b.ensureHead()
			return true
		case "body", "br":
			return false
		}
		return true // stray end tag
	}
	return false
}

func (b *builder) text(data string) {
	cur := b.current()
	if tableContext[cur.Tag] {
		if strings.TrimSpace(data) == "" {
			return
		}
		parent, before := b.fosterLocation()
		insertBefore(parent, dom.NewText(data), before)
		return
	}
	if n := len(cur.Children); n > 0 && cur.Children[n-1].Type == dom.TextNode {
		cur.Children[n-1].Text += data
		return
	}
	cur.AppendChild(dom.NewText(data))
}

// fosterLocation returns where content misplaced inside a table should go:
// immediately before the innermost open table.
func (b *builder) fosterLocation() (parent, before *dom.Node) {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if b.stack[i].Tag == "table" {
			t := b.stack[i]
			if t.Parent != nil {
				return t.Parent, t
			}
			return b.stack[i-1], nil
		}
	}
	return b.current(), nil
}

func insertBefore(parent, child, before *dom.Node) {
	child.Parent = parent
	if before == nil {
		parent.Children = append(parent.Children, child)
		return
	}
	for i, c := range parent.Children {
		if c == before {
			parent.Children = append(parent.Children[:i], append([]*dom.Node{child}, parent.Children[i:]...)...)
			return
		}
	}
	parent.Children = append(parent.Children, child)
}

// inScope returns the stack index of the nearest open element named tag,
// or -1 if a scope boundary is reached first.
func (b *builder) inScope(tag string, boundaries map[string]bool) int {
	for i := len(b.stack) - 1; i >= 0; i-- {
		t := b.stack[i].Tag
		if t == tag {
			return i
		}
		if boundaries[t] {
			return -1
		}
	}
	return -1
}

var buttonScope = merge(scopeBoundaries, "button")
var listScope = merge(scopeBoundaries, "ol", "ul")
var tableScope = map[string]bool{"html": true, "table": true, "template": true}

func merge(m map[string]bool, extra ...string) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	for _, e := range extra {
		out[e] = true
	}
	return out
}

func (b *builder) popTo(i int) {
	if i < 2 { // never pop html/body
		i = 2
	}
	if i < len(b.stack) {
		b.stack = b.stack[:i]
	}
}

func (b *builder) closeP() {
	if i := b.inScope("p", buttonScope); i >= 0 {
		b.popTo(i)
	}
}

func (b *builder) inForeign() bool {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if t := b.stack[i].Tag; t == "svg" || t == "math" {
			return true
		}
	}
	return false
}

func (b *builder) insert(el *dom.Node, push bool) {
	cur := b.current()
	if tableContext[cur.Tag] && !tableChildren[el.Tag] {
		parent, before := b.fosterLocation()
		insertBefore(parent, el, before)
	} else {
		cur.AppendChild(el)
	}
	if push {
		b.stack = append(b.stack, el)
	}
}

func (b *builder) startTag(tok Token) {
	name := tok.Data
	switch name {
	case "html":
		mergeAttrs(b.htmlEl, tok.Attrs)
		return
	case "body":
		mergeAttrs(b.body, tok.Attrs)
		return
	case "head":
		return
	}

	if closesP[name] {
		b.closeP()
	}
	switch name {
	case "li":
		if i := b.inScope("li", listScope); i >= 0 {
			b.popTo(i)
		}
	case "dd", "dt":
		for i := len(b.stack) - 1; i >= 2; i-- {
			t := b.stack[i].Tag
			if t == "dd" || t == "dt" {
				b.popTo(i)
				break
			}
			if scopeBoundaries[t] || t == "dl" {
				break
			}
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		switch b.current().Tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			b.popTo(len(b.stack) - 1)
		}
	case "option":
		if b.current().Tag == "option" {
			b.popTo(len(b.stack) - 1)
		}
	case "a":
		if i := b.inScope("a", scopeBoundaries); i >= 0 {
			b.popTo(i)
		}
	case "caption", "colgroup", "tbody", "thead", "tfoot":
		i := b.inScope("table", tableScope)
		if i < 0 || b.stack[i].Tag != "table" {
			return
		}
		b.popTo(i + 1)
	case "col":
		if b.inScope("table", tableScope) < 0 {
			return
		}
	case "tr":
		if !b.clearToTableContext(rowStops) {
			return
		}
	case "td", "th":
		if !b.clearToTableContext(cellStops) {
			return
		}
		if b.current().Tag != "tr" {
			b.insert(dom.NewElement("tr"), true)
		}
	}

	el := dom.NewElement(name, tok.Attrs...)
	void := voidElements[name] || (tok.SelfClosing && b.inForeign())
	b.insert(el, !void)
}

// clearToTableContext pops the stack back to the nearest element in stops
// within the innermost open table, inserting an implicit <tbody> if the
// table itself is reached. It returns false if there is no open table.
func (b *builder) clearToTableContext(stops map[string]bool) bool {
	ti := b.inScope("table", tableScope)
	if ti < 0 || b.stack[ti].Tag != "table" {
		return false
	}
	for len(b.stack)-1 > ti && !stops[b.current().Tag] {
		b.stack = b.stack[:len(b.stack)-1]
	}
	if b.current().Tag == "table" {
		b.insert(dom.NewElement("tbody"), true)
	}
	return true
}

var rowStops = map[string]bool{"tbody": true, "thead": true, "tfoot": true, "table": true}
var cellStops = map[string]bool{"tr": true, "tbody": true, "thead": true, "tfoot": true, "table": true}

func (b *builder) endTag(name string) {
	switch name {
	case "html", "body", "head":
		return
	case "br":
		b.insert(dom.NewElement("br"), false)
		return
	case "p":
		if b.inScope("p", buttonScope) < 0 {
			b.insert(dom.NewElement("p"), false)
			return
		}
	}
	boundaries := scopeBoundaries
	switch name {
	case "table":
		boundaries = tableScope
	case "tr", "tbody", "thead", "tfoot", "td", "th", "caption":
		boundaries = tableScope
	case "li":
		boundaries = listScope
	}
	if i := b.inScope(name, boundaries); i >= 0 {
		b.popTo(i)
	}
}
