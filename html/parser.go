package html

import "strings"

// Parse parses an HTML document into a DOM tree.
//
// The tree builder is a simplified version of the WHATWG algorithm: it
// handles implied html/head/body elements, void elements, optional end
// tags, implied table sections and rows, table-scoped recovery, and foster
// parenting of stray content in tables. It does not implement the adoption
// agency algorithm or reconstruction of active formatting elements.
func Parse(src string) *Node {
	p := &parser{doc: &Node{Type: DocumentNode}}
	z := NewTokenizer(src)
	for {
		tok := z.Next()
		if tok.Type == EOFToken {
			break
		}
		p.process(tok)
	}
	p.doc.Quirks = p.quirks()
	return p.doc
}

var voidElements = set("area", "base", "br", "col", "embed", "hr", "img",
	"input", "keygen", "link", "meta", "param", "source", "track", "wbr")

var headElements = set("base", "basefont", "bgsound", "link", "meta",
	"noscript", "script", "style", "template", "title")

// closesP lists start tags that implicitly close an open <p>.
var closesP = set("address", "article", "aside", "blockquote", "center",
	"details", "dialog", "dir", "div", "dl", "dd", "dt", "fieldset",
	"figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5",
	"h6", "header", "hgroup", "hr", "li", "listing", "main", "menu", "nav",
	"ol", "p", "pre", "search", "section", "summary", "ul", "xmp")

// blockEndTags close the matching element if it is in scope, closing any
// elements opened inside it.
var blockEndTags = set("address", "applet", "article", "aside",
	"blockquote", "button", "center", "details", "dialog", "dir", "div", "dl",
	"fieldset", "figcaption", "figure", "footer", "form", "header", "hgroup",
	"listing", "main", "marquee", "menu", "nav", "object", "ol", "pre",
	"search", "section", "summary", "ul")

var headings = set("h1", "h2", "h3", "h4", "h5", "h6")

var formattingElements = set("a", "b", "big", "code", "em", "font", "i",
	"nobr", "s", "small", "strike", "strong", "tt", "u")

// special elements block the generic end-tag algorithm from closing
// elements further up the stack.
var special = set("address", "applet", "area", "article", "aside", "base",
	"basefont", "bgsound", "blockquote", "body", "br", "button", "caption",
	"center", "col", "colgroup", "dd", "details", "dir", "div", "dl", "dt",
	"embed", "fieldset", "figcaption", "figure", "footer", "form", "frame",
	"frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header",
	"hgroup", "hr", "html", "iframe", "img", "input", "keygen", "li", "link",
	"listing", "main", "marquee", "menu", "meta", "nav", "noembed",
	"noframes", "noscript", "object", "ol", "p", "param", "plaintext", "pre",
	"script", "search", "section", "select", "source", "style", "summary",
	"table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead",
	"title", "tr", "track", "ul", "wbr", "xmp")

// Scope boundaries.
var (
	defaultScope  = set("applet", "caption", "html", "table", "td", "th", "marquee", "object", "template")
	buttonScope   = union(defaultScope, set("button"))
	listItemScope = union(defaultScope, set("ol", "ul"))
	tableScope    = set("html", "table", "template")
)

var tableSections = set("tbody", "thead", "tfoot")

// fosterTargets are elements whose children must be table structure;
// other content inserted into them is foster-parented before the table.
var fosterTargets = set("table", "tbody", "thead", "tfoot", "tr")

// tableContent is allowed directly inside fosterTargets.
var tableContent = set("caption", "colgroup", "col", "tbody", "thead",
	"tfoot", "tr", "td", "th", "script", "style", "template", "form")

type parser struct {
	doc         *Node
	html        *Node
	head        *Node
	body        *Node
	stack       []*Node
	doctype     *Node
	skipNewline bool
}

func (p *parser) process(tok Token) {
	skipNL := p.skipNewline
	p.skipNewline = false
	switch tok.Type {
	case DoctypeToken:
		if p.html == nil && p.doctype == nil {
			p.doctype = &Node{Type: DoctypeNode, Data: tok.Data, Attrs: tok.Attrs}
			p.doc.AppendChild(p.doctype)
		}
	case CommentToken:
		c := &Node{Type: CommentNode, Data: tok.Data}
		if p.html == nil {
			p.doc.AppendChild(c)
		} else {
			p.current().AppendChild(c)
		}
	case TextToken:
		text := tok.Data
		if skipNL {
			text = strings.TrimPrefix(text, "\n")
		}
		p.text(text)
	case StartTagToken:
		p.startTag(tok)
	case EndTagToken:
		p.endTag(tok.Data)
	}
}

func (p *parser) current() *Node {
	return p.stack[len(p.stack)-1]
}

func (p *parser) ensureHTML() {
	if p.html != nil {
		return
	}
	p.html = &Node{Type: ElementNode, Data: "html"}
	p.doc.AppendChild(p.html)
	p.stack = []*Node{p.html}
}

// enterHead makes head the current node, creating it if needed. It must
// only be called before the body has been created.
func (p *parser) enterHead() {
	p.ensureHTML()
	if p.head == nil {
		p.head = &Node{Type: ElementNode, Data: "head"}
		p.html.AppendChild(p.head)
	}
	if !p.onStack(p.head) {
		p.stack = append(p.stack, p.head)
	}
}

// enterBody creates the body element (closing the head) if it does not
// already exist.
func (p *parser) enterBody(attrs []Attr) {
	p.ensureHTML()
	if p.body != nil {
		mergeAttrs(p.body, attrs)
		return
	}
	if p.head == nil {
		p.head = &Node{Type: ElementNode, Data: "head"}
		p.html.AppendChild(p.head)
	}
	p.stack = p.stack[:1]
	p.body = &Node{Type: ElementNode, Data: "body", Attrs: attrs}
	p.html.AppendChild(p.body)
	p.stack = append(p.stack, p.body)
}

func (p *parser) text(text string) {
	if text == "" {
		return
	}
	if p.body == nil {
		cur := (*Node)(nil)
		if p.html != nil {
			cur = p.current()
		}
		if cur == nil || cur == p.html || cur == p.head {
			// Whitespace before the body is dropped; other text starts it.
			trimmed := strings.TrimLeft(text, " \t\n\r\f")
			if trimmed == "" {
				return
			}
			text = trimmed
			p.enterBody(nil)
		}
	}
	n := &Node{Type: TextNode, Data: text}
	parent, before := p.insertionPoint(n)
	if before == nil && len(parent.Children) > 0 {
		if last := parent.Children[len(parent.Children)-1]; last.Type == TextNode {
			last.Data += text
			return
		}
	}
	if before != nil {
		if i := indexOf(parent.Children, before); i > 0 && parent.Children[i-1].Type == TextNode {
			parent.Children[i-1].Data += text
			return
		}
	}
	parent.InsertBefore(n, before)
}

// insertionPoint returns where n should be inserted: as a child of parent,
// before the node before (or appended if before is nil).
func (p *parser) insertionPoint(n *Node) (parent, before *Node) {
	cur := p.current()
	if cur.Type != ElementNode || !fosterTargets[cur.Data] {
		return cur, nil
	}
	if n.Type == TextNode && strings.Trim(n.Data, " \t\n\r\f") == "" {
		return cur, nil
	}
	if n.Type == ElementNode && tableContent[n.Data] {
		return cur, nil
	}
	if n.Type == ElementNode && n.Data == "input" {
		if t, _ := n.GetAttr("type"); strings.EqualFold(t, "hidden") {
			return cur, nil
		}
	}
	// Foster parent: insert before the innermost open table.
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Data == "table" {
			table := p.stack[i]
			if table.Parent != nil {
				return table.Parent, table
			}
			return p.stack[i-1], nil
		}
	}
	return cur, nil
}

func (p *parser) insert(tok Token) *Node {
	n := &Node{Type: ElementNode, Data: tok.Data, Attrs: tok.Attrs}
	parent, before := p.insertionPoint(n)
	parent.InsertBefore(n, before)
	if !voidElements[tok.Data] {
		p.stack = append(p.stack, n)
	}
	return n
}

func (p *parser) startTag(tok Token) {
	name := tok.Data
	p.ensureHTML()
	switch name {
	case "html":
		mergeAttrs(p.html, tok.Attrs)
		return
	case "head":
		if p.body == nil && p.head == nil {
			p.enterHead()
			mergeAttrs(p.head, tok.Attrs)
		}
		return
	case "body":
		p.enterBody(tok.Attrs)
		return
	}
	if p.body == nil {
		if headElements[name] {
			// Allow nesting inside e.g. <noscript> in head.
			if cur := p.current(); cur == p.html {
				p.enterHead()
			}
			p.insert(tok)
			return
		}
		p.enterBody(nil)
	}

	switch {
	case name == "li":
		p.closeListItem(set("li"))
	case name == "dd" || name == "dt":
		p.closeListItem(set("dd", "dt"))
	case name == "option":
		p.popIfCurrent("option")
	case name == "optgroup":
		p.popIfCurrent("option")
		p.popIfCurrent("optgroup")
	case name == "a":
		// Simplified: nested anchors close the outer one.
		if p.inScope("a", defaultScope) {
			p.popUntil("a")
		}
	case name == "nobr":
		if p.inScope("nobr", defaultScope) {
			p.popUntil("nobr")
		}
	}

	if closesP[name] || (name == "table" && !p.quirks()) {
		p.closeP()
	}
	if headings[name] && headings[p.current().Data] {
		p.pop()
	}

	switch name {
	case "caption", "colgroup", "tbody", "thead", "tfoot":
		if !p.inScope("table", tableScope) {
			return
		}
		p.clearTo(set("table"))
	case "col":
		if !p.inScope("table", tableScope) {
			return
		}
		if p.current().Data != "colgroup" {
			p.clearTo(set("table"))
			p.insert(Token{Data: "colgroup"})
		}
	case "tr":
		if !p.inScope("table", tableScope) {
			return
		}
		p.clearTo(union(set("table"), tableSections))
		if p.current().Data == "table" {
			p.insert(Token{Data: "tbody"})
		}
	case "td", "th":
		if !p.inScope("table", tableScope) {
			return
		}
		p.clearTo(union(set("table", "tr"), tableSections))
		if p.current().Data == "table" {
			p.insert(Token{Data: "tbody"})
		}
		if tableSections[p.current().Data] {
			p.insert(Token{Data: "tr"})
		}
	case "pre", "listing", "textarea":
		p.skipNewline = true
	}
	p.insert(tok)
}

func (p *parser) endTag(name string) {
	if p.html == nil {
		p.ensureHTML()
	}
	switch name {
	case "html", "body":
		// Content after </body> stays in the body.
		return
	case "head":
		if p.body == nil && p.onStack(p.head) {
			p.popUntil("head")
		}
		return
	case "br":
		if p.body == nil {
			p.enterBody(nil)
		}
		p.insert(Token{Data: "br"})
		return
	case "p":
		if p.body == nil {
			p.enterBody(nil)
		}
		if !p.inScope("p", buttonScope) {
			p.insert(Token{Data: "p"})
		}
		p.closeP()
		return
	case "li":
		if p.inScope("li", listItemScope) {
			p.popUntil("li")
		}
		return
	case "dd", "dt":
		if p.inScope(name, defaultScope) {
			p.popUntil(name)
		}
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		for i := len(p.stack) - 1; i >= 0; i-- {
			n := p.stack[i].Data
			if headings[n] {
				p.stack = p.stack[:i]
				return
			}
			if defaultScope[n] {
				return
			}
		}
		return
	case "table", "tbody", "thead", "tfoot", "tr", "td", "th", "caption", "colgroup":
		if p.inScope(name, tableScope) {
			p.popUntil(name)
		}
		return
	}
	if formattingElements[name] || blockEndTags[name] {
		if p.inScope(name, defaultScope) {
			p.popUntil(name)
		}
		return
	}
	// Generic end tag: close the nearest matching element unless a special
	// element is in the way.
	for i := len(p.stack) - 1; i > 0; i-- {
		n := p.stack[i]
		if n.Data == name {
			p.stack = p.stack[:i]
			return
		}
		if special[n.Data] {
			return
		}
	}
}

func (p *parser) pop() {
	if len(p.stack) > 1 {
		p.stack = p.stack[:len(p.stack)-1]
	}
}

func (p *parser) popIfCurrent(name string) {
	if p.current().Data == name {
		p.pop()
	}
}

// popUntil pops elements up to and including the nearest one named name.
func (p *parser) popUntil(name string) {
	for i := len(p.stack) - 1; i > 0; i-- {
		if p.stack[i].Data == name {
			p.stack = p.stack[:i]
			return
		}
	}
}

// clearTo pops elements until the current node is one of names (or html).
func (p *parser) clearTo(names map[string]bool) {
	for len(p.stack) > 1 && !names[p.current().Data] {
		p.pop()
	}
}

// inScope reports whether an element named name is open, searching from
// the current node outward and stopping at any boundary element.
func (p *parser) inScope(name string, boundaries map[string]bool) bool {
	for i := len(p.stack) - 1; i >= 0; i-- {
		n := p.stack[i].Data
		if n == name {
			return true
		}
		if boundaries[n] {
			return false
		}
	}
	return false
}

func (p *parser) closeP() {
	if p.inScope("p", buttonScope) {
		p.popUntil("p")
	}
}

// closeListItem closes an open list item named in names, unless a special
// element (other than address, div and p) intervenes.
func (p *parser) closeListItem(names map[string]bool) {
	for i := len(p.stack) - 1; i > 0; i-- {
		n := p.stack[i].Data
		if names[n] {
			p.stack = p.stack[:i]
			return
		}
		if special[n] && n != "address" && n != "div" && n != "p" {
			return
		}
	}
}

func (p *parser) onStack(n *Node) bool {
	for _, s := range p.stack {
		if s == n {
			return true
		}
	}
	return false
}

// quirks reports whether the document should be rendered in quirks mode,
// using a simplified version of the WHATWG doctype rules.
func (p *parser) quirks() bool {
	if p.doctype == nil || p.doctype.Data != "html" {
		return true
	}
	public, hasPublic := p.doctype.GetAttr("public")
	_, hasSystem := p.doctype.GetAttr("system")
	if !hasPublic {
		return false
	}
	pub := strings.ToLower(public)
	if strings.HasPrefix(pub, "-//w3c//dtd xhtml 1.0") || strings.HasPrefix(pub, "-//w3c//dtd html 4.01//") {
		return false
	}
	if strings.HasPrefix(pub, "-//w3c//dtd html 4.01 ") {
		return !hasSystem
	}
	return true
}

func mergeAttrs(n *Node, attrs []Attr) {
	for _, a := range attrs {
		if !hasAttr(n.Attrs, a.Name) {
			n.Attrs = append(n.Attrs, a)
		}
	}
}

func indexOf(nodes []*Node, n *Node) int {
	for i, c := range nodes {
		if c == n {
			return i
		}
	}
	return -1
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func union(a, b map[string]bool) map[string]bool {
	m := make(map[string]bool, len(a)+len(b))
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	return m
}
