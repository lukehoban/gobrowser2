package css

import (
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

// Selector is a complex selector: compound selectors joined by
// combinators. Compounds[0] is the leftmost; Combinators[i] joins
// Compounds[i] and Compounds[i+1] and is one of ' ', '>', '+', '~'.
type Selector struct {
	Compounds   []Compound
	Combinators []byte
	Text        string
}

// Compound is a sequence of simple selectors that all apply to one element.
type Compound struct {
	Tag     string // "" or "*" for any
	ID      string
	Classes []string
	Attrs   []AttrSelector
	Pseudos []string // lower-cased pseudo-class names, e.g. "link"
}

// AttrSelector is an attribute selector such as [type='submit'].
type AttrSelector struct {
	Name, Op, Value string // Op is "", "=", "~=", "|=", "^=", "$=", "*="
}

// Specificity is the (ids, classes, types) specificity triple.
type Specificity [3]int

// Less reports whether s has lower specificity than o.
func (s Specificity) Less(o Specificity) bool {
	for i := range s {
		if s[i] != o[i] {
			return s[i] < o[i]
		}
	}
	return false
}

// Specificity computes the selector's specificity.
func (s *Selector) Specificity() Specificity {
	var sp Specificity
	for _, c := range s.Compounds {
		if c.ID != "" {
			sp[0]++
		}
		sp[1] += len(c.Classes) + len(c.Attrs) + len(c.Pseudos)
		if c.Tag != "" && c.Tag != "*" {
			sp[2]++
		}
	}
	return sp
}

// ParseSelectorList parses a comma-separated selector list. Selectors that
// fail to parse (or use unsupported features) are dropped.
func ParseSelectorList(src string) []*Selector {
	var out []*Selector
	for _, part := range splitTopLevel(src, ',') {
		if s := ParseSelector(part); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// ParseSelector parses a single complex selector, returning nil if it is
// invalid or unsupported.
func ParseSelector(src string) *Selector {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil
	}
	sel := &Selector{Text: src}
	i := 0
	pendingComb := byte(0)
	for i < len(src) {
		// Whitespace and combinators.
		sawSpace := false
		for i < len(src) && isSpace(src[i]) {
			i++
			sawSpace = true
		}
		if i >= len(src) {
			break
		}
		if c := src[i]; c == '>' || c == '+' || c == '~' {
			if len(sel.Compounds) == 0 || pendingComb != 0 {
				return nil
			}
			pendingComb = c
			i++
			continue
		}
		if len(sel.Compounds) > 0 {
			if pendingComb == 0 {
				if !sawSpace {
					return nil
				}
				pendingComb = ' '
			}
			sel.Combinators = append(sel.Combinators, pendingComb)
		}
		pendingComb = 0
		comp, n, ok := parseCompound(src[i:])
		if !ok || n == 0 {
			return nil
		}
		sel.Compounds = append(sel.Compounds, comp)
		i += n
	}
	if len(sel.Compounds) == 0 || pendingComb != 0 {
		return nil
	}
	return sel
}

// supportedPseudos lists pseudo-classes understood by Matches. Dynamic
// states (hover etc.) are supported but never match.
var supportedPseudos = map[string]bool{
	"link": true, "visited": true, "any-link": true, "hover": true, "active": true, "focus": true,
	"focus-within": true, "focus-visible": true, "first-child": true, "last-child": true,
	"only-child": true, "root": true, "empty": true, "checked": true, "disabled": true, "enabled": true,
}

func parseCompound(s string) (Compound, int, bool) {
	var c Compound
	i := 0
	if i < len(s) && s[i] == '*' {
		c.Tag = "*"
		i++
	} else if i < len(s) && isIdentChar(s[i]) {
		n := identLen(s[i:])
		c.Tag = strings.ToLower(s[i : i+n])
		i += n
	}
	for i < len(s) {
		switch s[i] {
		case '#':
			n := identLen(s[i+1:])
			if n == 0 {
				return c, 0, false
			}
			c.ID = s[i+1 : i+1+n]
			i += 1 + n
		case '.':
			n := identLen(s[i+1:])
			if n == 0 {
				return c, 0, false
			}
			c.Classes = append(c.Classes, s[i+1:i+1+n])
			i += 1 + n
		case '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return c, 0, false
			}
			a, ok := parseAttrSelector(s[i+1 : i+end])
			if !ok {
				return c, 0, false
			}
			c.Attrs = append(c.Attrs, a)
			i += end + 1
		case ':':
			if i+1 < len(s) && s[i+1] == ':' {
				return c, 0, false // pseudo-elements are not supported
			}
			n := identLen(s[i+1:])
			name := strings.ToLower(s[i+1 : i+1+n])
			if !supportedPseudos[name] || (i+1+n < len(s) && s[i+1+n] == '(') {
				return c, 0, false
			}
			c.Pseudos = append(c.Pseudos, name)
			i += 1 + n
		default:
			return c, i, true
		}
	}
	return c, i, true
}

func parseAttrSelector(s string) (AttrSelector, bool) {
	s = strings.TrimSpace(s)
	n := identLen(s)
	if n == 0 {
		return AttrSelector{}, false
	}
	a := AttrSelector{Name: strings.ToLower(s[:n])}
	rest := strings.TrimSpace(s[n:])
	if rest == "" {
		return a, true
	}
	for _, op := range []string{"~=", "|=", "^=", "$=", "*=", "="} {
		if strings.HasPrefix(rest, op) {
			a.Op = op
			v := strings.TrimSpace(rest[len(op):])
			// Drop a trailing case-sensitivity flag.
			if len(v) > 2 && (strings.HasSuffix(v, " i") || strings.HasSuffix(v, " s")) {
				v = strings.TrimSpace(v[:len(v)-2])
			}
			if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
				v = v[1 : len(v)-1]
			}
			a.Value = v
			return a, true
		}
	}
	return AttrSelector{}, false
}

func identLen(s string) int {
	i := 0
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	return i
}

// Matches reports whether the element n matches the selector.
func (s *Selector) Matches(n *dom.Node) bool {
	return s.matchFrom(len(s.Compounds)-1, n)
}

func (s *Selector) matchFrom(i int, n *dom.Node) bool {
	if !s.Compounds[i].matches(n) {
		return false
	}
	if i == 0 {
		return true
	}
	switch s.Combinators[i-1] {
	case ' ':
		for p := parentElement(n); p != nil; p = parentElement(p) {
			if s.matchFrom(i-1, p) {
				return true
			}
		}
	case '>':
		if p := parentElement(n); p != nil {
			return s.matchFrom(i-1, p)
		}
	case '+':
		if p := prevElement(n); p != nil {
			return s.matchFrom(i-1, p)
		}
	case '~':
		for p := prevElement(n); p != nil; p = prevElement(p) {
			if s.matchFrom(i-1, p) {
				return true
			}
		}
	}
	return false
}

func parentElement(n *dom.Node) *dom.Node {
	if n.Parent != nil && n.Parent.Type == dom.ElementNode {
		return n.Parent
	}
	return nil
}

func prevElement(n *dom.Node) *dom.Node {
	if n.Parent == nil {
		return nil
	}
	var prev *dom.Node
	for _, c := range n.Parent.Children {
		if c == n {
			return prev
		}
		if c.Type == dom.ElementNode {
			prev = c
		}
	}
	return nil
}

func (c *Compound) matches(n *dom.Node) bool {
	if n.Type != dom.ElementNode {
		return false
	}
	if c.Tag != "" && c.Tag != "*" && c.Tag != n.Tag {
		return false
	}
	if c.ID != "" {
		if id, _ := n.Attr("id"); id != c.ID {
			return false
		}
	}
	for _, cls := range c.Classes {
		if !n.HasClass(cls) {
			return false
		}
	}
	for _, a := range c.Attrs {
		if !a.matches(n) {
			return false
		}
	}
	for _, p := range c.Pseudos {
		if !matchPseudo(p, n) {
			return false
		}
	}
	return true
}

func (a *AttrSelector) matches(n *dom.Node) bool {
	v, ok := n.Attr(a.Name)
	if !ok {
		return false
	}
	switch a.Op {
	case "":
		return true
	case "=":
		return v == a.Value
	case "~=":
		for _, f := range strings.Fields(v) {
			if f == a.Value {
				return true
			}
		}
		return false
	case "|=":
		return v == a.Value || strings.HasPrefix(v, a.Value+"-")
	case "^=":
		return a.Value != "" && strings.HasPrefix(v, a.Value)
	case "$=":
		return a.Value != "" && strings.HasSuffix(v, a.Value)
	case "*=":
		return a.Value != "" && strings.Contains(v, a.Value)
	}
	return false
}

func matchPseudo(p string, n *dom.Node) bool {
	switch p {
	case "link", "any-link":
		_, ok := n.Attr("href")
		return ok && (n.Tag == "a" || n.Tag == "area")
	case "first-child":
		return n.Parent != nil && prevElement(n) == nil && n.Parent.Type == dom.ElementNode
	case "last-child":
		if n.Parent == nil || n.Parent.Type != dom.ElementNode {
			return false
		}
		kids := n.Parent.ElementChildren()
		return kids[len(kids)-1] == n
	case "only-child":
		return n.Parent != nil && n.Parent.Type == dom.ElementNode && len(n.Parent.ElementChildren()) == 1
	case "root":
		return n.Parent != nil && n.Parent.Type == dom.DocumentNode
	case "empty":
		for _, c := range n.Children {
			if c.Type == dom.ElementNode || (c.Type == dom.TextNode && c.Text != "") {
				return false
			}
		}
		return true
	case "checked":
		_, ok := n.Attr("checked")
		return ok
	case "disabled":
		_, ok := n.Attr("disabled")
		return ok
	case "enabled":
		_, ok := n.Attr("disabled")
		return !ok && (n.Tag == "input" || n.Tag == "button" || n.Tag == "select" || n.Tag == "textarea")
	}
	// Dynamic states (:visited, :hover, :focus, ...) never match in a
	// static render.
	return false
}
