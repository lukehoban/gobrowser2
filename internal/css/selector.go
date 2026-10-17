package css

import (
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

// Selector is a complex selector: compound selectors joined by combinators.
// Compounds[0] is the leftmost; Combinators[i] joins Compounds[i] and
// Compounds[i+1] and is one of ' ', '>', '+', '~'.
type Selector struct {
	Compounds   []Compound
	Combinators []byte
	Spec        Specificity
}

// Specificity is (ids, classes/attributes/pseudo-classes, types).
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

// Compound is a sequence of simple selectors that all apply to one element.
type Compound struct {
	Tag     string // "" or "*" matches any element
	IDs     []string
	Classes []string
	Attrs   []AttrSel
	Pseudos []string
}

// AttrSel is an attribute selector such as [type='submit'].
type AttrSel struct {
	Name, Op, Value string // Op is "", "=", "~=", "|=", "^=", "$=", "*="
}

// supportedPseudos lists the pseudo-classes we can evaluate statically.
// Dynamic states such as :hover never match.
var supportedPseudos = map[string]bool{
	"link": true, "visited": true, "hover": true, "active": true, "focus": true,
	"first-child": true, "last-child": true, "root": true,
}

// ParseSelectorList parses a comma-separated selector list. Following CSS
// error handling, the whole list is invalid if any selector is.
func ParseSelectorList(s string) ([]*Selector, bool) {
	var out []*Selector
	for _, part := range splitTopLevel(s, ',') {
		sel, ok := parseSelector(strings.TrimSpace(part))
		if !ok {
			return nil, false
		}
		out = append(out, sel)
	}
	return out, len(out) > 0
}

func parseSelector(s string) (*Selector, bool) {
	if s == "" {
		return nil, false
	}
	sel := &Selector{}
	var cur Compound
	have := false
	pendingComb := byte(0)
	finish := func() bool {
		if !have {
			return false
		}
		if len(sel.Compounds) > 0 {
			if pendingComb == 0 {
				pendingComb = ' '
			}
			sel.Combinators = append(sel.Combinators, pendingComb)
		}
		sel.Compounds = append(sel.Compounds, cur)
		cur = Compound{}
		have = false
		pendingComb = 0
		return true
	}
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case isSpace(c):
			if have && !finish() {
				return nil, false
			}
			i++
		case c == '>' || c == '+' || c == '~':
			if have && !finish() {
				return nil, false
			}
			if len(sel.Compounds) == 0 || pendingComb != 0 {
				return nil, false
			}
			pendingComb = c
			i++
		case c == '*':
			cur.Tag = "*"
			have = true
			i++
		case c == '.' || c == '#':
			name, n := readIdent(s, i+1)
			if name == "" {
				return nil, false
			}
			if c == '.' {
				cur.Classes = append(cur.Classes, name)
			} else {
				cur.IDs = append(cur.IDs, name)
			}
			have = true
			i = n
		case c == '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return nil, false
			}
			a, ok := parseAttrSel(s[i+1 : i+end])
			if !ok {
				return nil, false
			}
			cur.Attrs = append(cur.Attrs, a)
			have = true
			i += end + 1
		case c == ':':
			if i+1 < len(s) && s[i+1] == ':' {
				return nil, false // pseudo-elements are not supported
			}
			name, n := readIdent(s, i+1)
			name = strings.ToLower(name)
			if !supportedPseudos[name] {
				return nil, false
			}
			cur.Pseudos = append(cur.Pseudos, name)
			have = true
			i = n
		default:
			name, n := readIdent(s, i)
			if name == "" {
				return nil, false
			}
			cur.Tag = strings.ToLower(name)
			have = true
			i = n
		}
	}
	if !finish() || pendingComb != 0 {
		return nil, false
	}
	for _, c := range sel.Compounds {
		sel.Spec[0] += len(c.IDs)
		sel.Spec[1] += len(c.Classes) + len(c.Attrs) + len(c.Pseudos)
		if c.Tag != "" && c.Tag != "*" {
			sel.Spec[2]++
		}
	}
	return sel, true
}

func readIdent(s string, i int) (string, int) {
	start := i
	for i < len(s) {
		c := s[i]
		if c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 {
			i++
			continue
		}
		if c == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		break
	}
	return strings.ReplaceAll(s[start:i], `\`, ""), i
}

func parseAttrSel(s string) (AttrSel, bool) {
	s = strings.TrimSpace(s)
	for _, op := range []string{"~=", "|=", "^=", "$=", "*=", "="} {
		if k := strings.Index(s, op); k >= 0 {
			name := strings.ToLower(strings.TrimSpace(s[:k]))
			val := strings.TrimSpace(s[k+len(op):])
			// Drop a trailing case-sensitivity flag ("i" / "s").
			if f := strings.Fields(val); len(f) == 2 && (f[1] == "i" || f[1] == "s") {
				val = f[0]
			}
			if name == "" {
				return AttrSel{}, false
			}
			return AttrSel{Name: name, Op: op, Value: Unquote(val)}, true
		}
	}
	if s == "" {
		return AttrSel{}, false
	}
	return AttrSel{Name: strings.ToLower(s)}, true
}

// Matches reports whether the element n matches the selector.
func (sel *Selector) Matches(n *dom.Node) bool {
	return sel.matchFrom(n, len(sel.Compounds)-1)
}

func (sel *Selector) matchFrom(n *dom.Node, idx int) bool {
	if !sel.Compounds[idx].matches(n) {
		return false
	}
	if idx == 0 {
		return true
	}
	switch sel.Combinators[idx-1] {
	case '>':
		p := parentElement(n)
		return p != nil && sel.matchFrom(p, idx-1)
	case ' ':
		for p := parentElement(n); p != nil; p = parentElement(p) {
			if sel.matchFrom(p, idx-1) {
				return true
			}
		}
		return false
	case '+':
		p := prevElement(n)
		return p != nil && sel.matchFrom(p, idx-1)
	case '~':
		for p := prevElement(n); p != nil; p = prevElement(p) {
			if sel.matchFrom(p, idx-1) {
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

func siblingElements(n *dom.Node) []*dom.Node {
	if n.Parent == nil {
		return nil
	}
	var out []*dom.Node
	for _, c := range n.Parent.Children {
		if c.Type == dom.ElementNode {
			out = append(out, c)
		}
	}
	return out
}

func prevElement(n *dom.Node) *dom.Node {
	var prev *dom.Node
	for _, c := range siblingElements(n) {
		if c == n {
			return prev
		}
		prev = c
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
	for _, id := range c.IDs {
		if n.AttrOr("id", "") != id {
			return false
		}
	}
	for _, cl := range c.Classes {
		if !n.HasClass(cl) {
			return false
		}
	}
	for _, a := range c.Attrs {
		if !a.matches(n) {
			return false
		}
	}
	for _, p := range c.Pseudos {
		switch p {
		case "link":
			if _, ok := n.Attr("href"); !ok || (n.Tag != "a" && n.Tag != "area") {
				return false
			}
		case "first-child":
			sibs := siblingElements(n)
			if len(sibs) == 0 || sibs[0] != n {
				return false
			}
		case "last-child":
			sibs := siblingElements(n)
			if len(sibs) == 0 || sibs[len(sibs)-1] != n {
				return false
			}
		case "root":
			if n.Tag != "html" {
				return false
			}
		default: // :visited, :hover, :active, :focus never match in a static render
			return false
		}
	}
	return true
}

func (a *AttrSel) matches(n *dom.Node) bool {
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
