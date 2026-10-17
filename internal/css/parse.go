// Package css parses stylesheets and matches selectors against the DOM.
//
// Only what the cascade needs is modelled: rules with selector lists and
// declarations. Declaration values are kept as (trimmed) strings and are
// interpreted by the style package. At-rules, including @media, are skipped:
// the browser renders the desktop layout only.
package css

import (
	"net/url"
	"strings"
)

// Stylesheet is a parsed list of style rules.
type Stylesheet struct {
	Rules []*Rule
}

// Rule is a style rule: a selector list and its declarations.
type Rule struct {
	Selectors []*Selector
	Decls     []Decl
}

// Decl is a single property declaration.
type Decl struct {
	Prop      string // lowercase property name
	Value     string
	Important bool
}

// Parse parses a stylesheet. Relative url() references are resolved against
// base, which may be nil.
func Parse(src string, base *url.URL) *Stylesheet {
	src = stripComments(src)
	ss := &Stylesheet{}
	i := 0
	for i < len(src) {
		i = skipSpace(src, i)
		if i >= len(src) {
			break
		}
		if src[i] == '@' {
			i = skipAtRule(src, i)
			continue
		}
		if src[i] == '}' { // stray close brace
			i++
			continue
		}
		open := indexOutsideStrings(src, i, '{')
		if open < 0 {
			break
		}
		prelude := src[i:open]
		end := matchingBrace(src, open)
		body := src[open+1 : end]
		i = end + 1
		sels, ok := ParseSelectorList(prelude)
		if !ok {
			continue
		}
		ss.Rules = append(ss.Rules, &Rule{Selectors: sels, Decls: ParseDeclarations(body, base)})
	}
	return ss
}

// ParseDeclarations parses the body of a rule or a style attribute.
func ParseDeclarations(src string, base *url.URL) []Decl {
	var out []Decl
	for _, part := range splitTopLevel(stripComments(src), ';') {
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(part[:colon]))
		val := strings.TrimSpace(part[colon+1:])
		if prop == "" || val == "" {
			continue
		}
		d := Decl{Prop: prop}
		if k := strings.LastIndexByte(val, '!'); k >= 0 && strings.EqualFold(strings.TrimSpace(val[k+1:]), "important") {
			d.Important = true
			val = strings.TrimSpace(val[:k])
		}
		d.Value = resolveURLs(val, base)
		out = append(out, d)
	}
	return out
}

// resolveURLs rewrites url(...) references in a value to absolute URLs.
func resolveURLs(val string, base *url.URL) string {
	if base == nil || !strings.Contains(strings.ToLower(val), "url(") {
		return val
	}
	var sb strings.Builder
	for {
		k := strings.Index(strings.ToLower(val), "url(")
		if k < 0 {
			sb.WriteString(val)
			return sb.String()
		}
		end := strings.IndexByte(val[k:], ')')
		if end < 0 {
			sb.WriteString(val)
			return sb.String()
		}
		end += k
		ref := Unquote(strings.TrimSpace(val[k+4 : end]))
		sb.WriteString(val[:k])
		if r, err := url.Parse(ref); err == nil {
			ref = base.ResolveReference(r).String()
		}
		sb.WriteString(`url("` + strings.ReplaceAll(ref, `"`, `%22`) + `")`)
		val = val[end+1:]
	}
}

// Unquote removes matching single or double quotes around s.
func Unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// SplitValue splits a declaration value on whitespace and commas outside
// parentheses and strings. Commas are returned as separate "," tokens.
func SplitValue(s string) []string {
	var out []string
	depth := 0
	var quote byte
	start := -1
	flush := func(i int) {
		if start >= 0 {
			out = append(out, s[start:i])
			start = -1
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			if start < 0 {
				start = i
			}
		case c == '(':
			depth++
			if start < 0 {
				start = i
			}
		case c == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'):
			flush(i)
			continue
		case depth == 0 && c == ',':
			flush(i)
			out = append(out, ",")
			continue
		default:
			if start < 0 {
				start = i
			}
		}
	}
	flush(len(s))
	return out
}

func stripComments(s string) string {
	if !strings.Contains(s, "/*") {
		return s
	}
	var sb strings.Builder
	for {
		k := strings.Index(s, "/*")
		if k < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:k])
		end := strings.Index(s[k+2:], "*/")
		if end < 0 {
			break
		}
		s = s[k+2+end+2:]
	}
	return sb.String()
}

func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// skipAtRule skips an at-rule starting at s[i] == '@', either up to and
// including the terminating ';' or its {} block.
func skipAtRule(s string, i int) int {
	for j := i; j < len(s); j++ {
		switch s[j] {
		case ';':
			return j + 1
		case '{':
			return matchingBrace(s, j) + 1
		case '"', '\'':
			j = skipString(s, j)
		}
	}
	return len(s)
}

// matchingBrace returns the index of the '}' matching the '{' at s[open], or
// len(s) if the block is unterminated.
func matchingBrace(s string, open int) int {
	depth := 0
	for j := open; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		case '"', '\'':
			j = skipString(s, j)
		}
	}
	return len(s)
}

// skipString returns the index of the closing quote of the string at s[i].
func skipString(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == q {
			return j
		}
	}
	return len(s) - 1
}

func indexOutsideStrings(s string, i int, c byte) int {
	for j := i; j < len(s); j++ {
		switch s[j] {
		case c:
			return j
		case '"', '\'':
			j = skipString(s, j)
		}
	}
	return -1
}

// splitTopLevel splits s on sep, ignoring separators inside strings and
// parentheses.
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for j := 0; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '"', '\'':
			j = skipString(s, j)
		case sep:
			if depth == 0 {
				out = append(out, s[start:j])
				start = j + 1
			}
		}
	}
	return append(out, s[start:])
}
