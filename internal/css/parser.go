// Package css parses CSS stylesheets and implements selector matching.
package css

import (
	"strings"
)

// Stylesheet is a parsed list of style rules.
type Stylesheet struct {
	Rules []*Rule
}

// Rule is a style rule: a selector list and its declarations. Media holds
// the enclosing @media prelude, or "" if the rule is unconditional.
type Rule struct {
	Selectors []*Selector
	Decls     []Declaration
	Media     string
}

// Declaration is a single "property: value" pair.
type Declaration struct {
	Property  string
	Value     string
	Important bool
}

// Parse parses a stylesheet. Invalid constructs are skipped.
func Parse(src string) *Stylesheet {
	p := &parser{src: stripComments(src)}
	ss := &Stylesheet{}
	p.rules(ss, "")
	return ss
}

// ParseDeclarations parses the contents of a declaration block (for
// example a style attribute), expanding shorthand properties.
func ParseDeclarations(src string) []Declaration {
	var out []Declaration
	for _, part := range splitTopLevel(stripComments(src), ';') {
		name, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		important := false
		if i := strings.LastIndex(value, "!"); i >= 0 && strings.EqualFold(strings.TrimSpace(value[i+1:]), "important") {
			important = true
			value = strings.TrimSpace(value[:i])
		}
		out = append(out, Expand(Declaration{Property: name, Value: value, Important: important})...)
	}
	return out
}

type parser struct {
	src string
	pos int
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && isSpace(p.src[p.pos]) {
		p.pos++
	}
}

// rules parses rules until the end of input or an unmatched '}'.
func (p *parser) rules(ss *Stylesheet, media string) {
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return
		}
		switch p.src[p.pos] {
		case '}':
			p.pos++
			return
		case '@':
			p.atRule(ss, media)
			continue
		case '<', '-':
			// Skip HTML comment markers "<!--" and "-->".
			if strings.HasPrefix(p.src[p.pos:], "<!--") {
				p.pos += 4
				continue
			}
			if strings.HasPrefix(p.src[p.pos:], "-->") {
				p.pos += 3
				continue
			}
		}
		prelude, ok := p.readUntil('{')
		if !ok {
			return
		}
		body := p.readBlock()
		sels := ParseSelectorList(prelude)
		if len(sels) == 0 {
			continue
		}
		ss.Rules = append(ss.Rules, &Rule{Selectors: sels, Decls: ParseDeclarations(body), Media: media})
	}
}

func (p *parser) atRule(ss *Stylesheet, media string) {
	start := p.pos
	p.pos++
	for p.pos < len(p.src) && (isIdentChar(p.src[p.pos])) {
		p.pos++
	}
	name := strings.ToLower(p.src[start+1 : p.pos])
	// Read the prelude up to ';' or '{'.
	i := p.pos
	depth := 0
	for i < len(p.src) {
		c := p.src[i]
		if c == '"' || c == '\'' {
			i = skipString(p.src, i)
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
		} else if depth <= 0 && (c == ';' || c == '{' || c == '}') {
			break
		}
		i++
	}
	prelude := strings.TrimSpace(p.src[p.pos:i])
	p.pos = i
	if p.pos >= len(p.src) {
		return
	}
	switch p.src[p.pos] {
	case ';':
		p.pos++
		return
	case '}':
		return
	}
	// Block.
	p.pos++
	switch name {
	case "media":
		m := prelude
		if media != "" {
			m = media + " and " + prelude
		}
		p.rules(ss, m)
	case "supports":
		p.rules(ss, media)
	default:
		p.pos--
		p.readBlock()
	}
}

// readUntil reads up to (and consumes) the delimiter, respecting strings.
func (p *parser) readUntil(delim byte) (string, bool) {
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '"' || c == '\'' {
			p.pos = skipString(p.src, p.pos)
			continue
		}
		if c == delim {
			s := p.src[start:p.pos]
			p.pos++
			return s, true
		}
		p.pos++
	}
	return p.src[start:], false
}

// readBlock reads a {}-delimited block body. The opening '{' has already
// been consumed.
func (p *parser) readBlock() string {
	if p.pos < len(p.src) && p.src[p.pos] == '{' {
		p.pos++
	}
	start := p.pos
	depth := 1
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case '"', '\'':
			p.pos = skipString(p.src, p.pos)
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				s := p.src[start:p.pos]
				p.pos++
				return s
			}
		}
		p.pos++
	}
	return p.src[start:]
}

// skipString returns the index just past the quoted string starting at i.
func skipString(s string, i int) int {
	q := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == q {
			return i + 1
		}
		if s[i] == '\n' {
			return i
		}
		i++
	}
	return i
}

func stripComments(s string) string {
	if !strings.Contains(s, "/*") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c == '"' || c == '\'' {
			j := skipString(s, i)
			sb.WriteString(s[i:j])
			i = j
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 4
			sb.WriteByte(' ')
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

// splitTopLevel splits s on sep, ignoring separators inside strings and
// parentheses.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			i = skipString(s, i)
			continue
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == sep && depth <= 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
		i++
	}
	return append(parts, s[start:])
}

// SplitValues splits a property value into space-separated components,
// keeping function calls such as rgb(1, 2, 3) intact.
func SplitValues(v string) []string {
	var out []string
	depth := 0
	start := -1
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '(':
			depth++
		case c == ')':
			depth--
		case (c == '"' || c == '\'') && depth == 0:
			if start < 0 {
				start = i
			}
			i = skipString(v, i) - 1
			continue
		case isSpace(c) && depth <= 0:
			if start >= 0 {
				out = append(out, v[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, v[start:])
	}
	return out
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isIdentChar(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c >= 0x80
}
