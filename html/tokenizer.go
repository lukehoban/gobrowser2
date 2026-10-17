package html

import (
	"strings"
)

// TokenType identifies the kind of a Token.
type TokenType int

const (
	EOFToken TokenType = iota
	TextToken
	StartTagToken
	EndTagToken
	CommentToken
	DoctypeToken
)

// Token is a single lexical HTML token.
//
// For tags, Data is the lowercased tag name. For text and comments, Data
// is the decoded content. For doctypes, Data is the lowercased doctype name
// and Attrs may hold "public" and "system" identifiers.
type Token struct {
	Type        TokenType
	Data        string
	Attrs       []Attr
	SelfClosing bool
}

// rawTextElements have content that is not parsed as markup.
var rawTextElements = map[string]bool{
	"script": true, "style": true, "xmp": true, "iframe": true,
	"noembed": true, "noframes": true,
}

// rcdataElements have content that is not parsed as markup but in which
// character references are decoded.
var rcdataElements = map[string]bool{"title": true, "textarea": true}

// Tokenizer splits an HTML document into tokens.
type Tokenizer struct {
	s   string
	pos int
	// rawTag is set after a raw text or RCDATA start tag; the following
	// content up to the matching end tag is returned as a single text token.
	rawTag string
}

// NewTokenizer returns a Tokenizer for src.
func NewTokenizer(src string) *Tokenizer {
	return &Tokenizer{s: src}
}

// Next returns the next token. At end of input it returns a token of type
// EOFToken, repeatedly.
func (z *Tokenizer) Next() Token {
	for {
		if z.rawTag != "" {
			tag := z.rawTag
			z.rawTag = ""
			end := z.findRawEnd(tag)
			text := z.s[z.pos:end]
			z.pos = end
			if text == "" {
				continue
			}
			if rcdataElements[tag] {
				text = decodeEntities(text, false)
			}
			return Token{Type: TextToken, Data: text}
		}
		if z.pos >= len(z.s) {
			return Token{Type: EOFToken}
		}
		if z.s[z.pos] != '<' {
			return z.text()
		}
		if tok, ok := z.markup(); ok {
			return tok
		}
	}
}

// findRawEnd returns the offset of the "</tag" that ends raw text content
// started at z.pos, or len(z.s) if there is none.
func (z *Tokenizer) findRawEnd(tag string) int {
	i := z.pos
	for {
		j := strings.Index(z.s[i:], "</")
		if j < 0 {
			return len(z.s)
		}
		i += j
		k := i + 2 + len(tag)
		if k <= len(z.s) && strings.EqualFold(z.s[i+2:k], tag) {
			if k == len(z.s) || isSpace(z.s[k]) || z.s[k] == '/' || z.s[k] == '>' {
				return i
			}
		}
		i += 2
	}
}

func (z *Tokenizer) text() Token {
	start := z.pos
	// A '<' that does not begin markup is literal text; include it so that
	// we always make progress.
	z.pos++
	for z.pos < len(z.s) && !z.startsMarkup(z.pos) {
		z.pos++
	}
	return Token{Type: TextToken, Data: decodeEntities(z.s[start:z.pos], false)}
}

// startsMarkup reports whether a tag, comment, doctype or similar begins
// at offset i.
func (z *Tokenizer) startsMarkup(i int) bool {
	if z.s[i] != '<' || i+1 >= len(z.s) {
		return false
	}
	c := z.s[i+1]
	return isAlpha(c) || c == '/' || c == '!' || c == '?'
}

// markup consumes markup at z.pos (which is '<'). It returns ok=false when
// the markup produces no token (for example "</>").
func (z *Tokenizer) markup() (Token, bool) {
	if !z.startsMarkup(z.pos) {
		return z.text(), true
	}
	rest := z.s[z.pos:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		return z.comment(), true
	case rest[1] == '!' && len(rest) >= 9 && strings.EqualFold(rest[2:9], "doctype"):
		return z.doctype(), true
	case rest[1] == '!' || rest[1] == '?':
		return z.bogusComment(z.pos + 2), true
	case rest[1] == '/':
		if len(rest) > 2 && rest[2] == '>' {
			z.pos += 3
			return Token{}, false
		}
		if len(rest) > 2 && isAlpha(rest[2]) {
			return z.tag(true), true
		}
		return z.bogusComment(z.pos + 2), true
	default:
		return z.tag(false), true
	}
}

func (z *Tokenizer) comment() Token {
	start := z.pos + 4
	// "<!-->" and "<!--->" are empty comments.
	for _, abrupt := range []string{">", "->"} {
		if strings.HasPrefix(z.s[start:], abrupt) {
			z.pos = start + len(abrupt)
			return Token{Type: CommentToken}
		}
	}
	end := strings.Index(z.s[start:], "-->")
	if end < 0 {
		z.pos = len(z.s)
		return Token{Type: CommentToken, Data: z.s[start:]}
	}
	z.pos = start + end + 3
	return Token{Type: CommentToken, Data: z.s[start : start+end]}
}

func (z *Tokenizer) bogusComment(start int) Token {
	end := strings.IndexByte(z.s[start:], '>')
	if end < 0 {
		z.pos = len(z.s)
		return Token{Type: CommentToken, Data: z.s[start:]}
	}
	z.pos = start + end + 1
	return Token{Type: CommentToken, Data: z.s[start : start+end]}
}

func (z *Tokenizer) doctype() Token {
	start := z.pos + 9
	end := strings.IndexByte(z.s[start:], '>')
	var body string
	if end < 0 {
		body = z.s[start:]
		z.pos = len(z.s)
	} else {
		body = z.s[start : start+end]
		z.pos = start + end + 1
	}
	tok := Token{Type: DoctypeToken}
	fields := splitDoctype(body)
	if len(fields) > 0 {
		tok.Data = strings.ToLower(fields[0])
	}
	for i := 1; i < len(fields); i++ {
		switch strings.ToUpper(fields[i]) {
		case "PUBLIC":
			if i+1 < len(fields) {
				tok.Attrs = append(tok.Attrs, Attr{Name: "public", Value: fields[i+1]})
				i++
				if i+1 < len(fields) {
					tok.Attrs = append(tok.Attrs, Attr{Name: "system", Value: fields[i+1]})
					i++
				}
			}
		case "SYSTEM":
			if i+1 < len(fields) {
				tok.Attrs = append(tok.Attrs, Attr{Name: "system", Value: fields[i+1]})
				i++
			}
		}
	}
	return tok
}

// splitDoctype splits a doctype body on whitespace, treating quoted
// strings as single fields (without their quotes).
func splitDoctype(s string) []string {
	var fields []string
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		if q := s[i]; q == '"' || q == '\'' {
			end := strings.IndexByte(s[i+1:], q)
			if end < 0 {
				fields = append(fields, s[i+1:])
				break
			}
			fields = append(fields, s[i+1:i+1+end])
			i += end + 2
			continue
		}
		start := i
		for i < len(s) && !isSpace(s[i]) {
			i++
		}
		fields = append(fields, s[start:i])
	}
	return fields
}

func (z *Tokenizer) tag(end bool) Token {
	if end {
		z.pos += 2
	} else {
		z.pos++
	}
	start := z.pos
	for z.pos < len(z.s) && !isSpace(z.s[z.pos]) && z.s[z.pos] != '/' && z.s[z.pos] != '>' {
		z.pos++
	}
	tok := Token{Type: StartTagToken, Data: strings.ToLower(z.s[start:z.pos])}
	if end {
		tok.Type = EndTagToken
	}
	tok.Attrs, tok.SelfClosing = z.attrs()
	if end {
		// End tags never carry attributes.
		tok.Attrs = nil
		tok.SelfClosing = false
	} else if rawTextElements[tok.Data] || rcdataElements[tok.Data] {
		z.rawTag = tok.Data
	}
	return tok
}

// attrs parses attributes up to and including the closing '>' of a tag.
func (z *Tokenizer) attrs() (attrs []Attr, selfClosing bool) {
	for {
		for z.pos < len(z.s) && isSpace(z.s[z.pos]) {
			z.pos++
		}
		if z.pos >= len(z.s) {
			return attrs, false
		}
		switch z.s[z.pos] {
		case '>':
			z.pos++
			return attrs, false
		case '/':
			z.pos++
			if z.pos < len(z.s) && z.s[z.pos] == '>' {
				z.pos++
				return attrs, true
			}
			continue
		}
		start := z.pos
		// An '=' at the start of a name is part of the name.
		z.pos++
		for z.pos < len(z.s) && !isSpace(z.s[z.pos]) && z.s[z.pos] != '/' && z.s[z.pos] != '>' && z.s[z.pos] != '=' {
			z.pos++
		}
		name := strings.ToLower(z.s[start:z.pos])
		for z.pos < len(z.s) && isSpace(z.s[z.pos]) {
			z.pos++
		}
		value := ""
		if z.pos < len(z.s) && z.s[z.pos] == '=' {
			z.pos++
			for z.pos < len(z.s) && isSpace(z.s[z.pos]) {
				z.pos++
			}
			value = z.attrValue()
		}
		if !hasAttr(attrs, name) {
			attrs = append(attrs, Attr{Name: name, Value: value})
		}
	}
}

func (z *Tokenizer) attrValue() string {
	if z.pos >= len(z.s) {
		return ""
	}
	if q := z.s[z.pos]; q == '"' || q == '\'' {
		start := z.pos + 1
		end := strings.IndexByte(z.s[start:], q)
		if end < 0 {
			z.pos = len(z.s)
			return decodeEntities(z.s[start:], true)
		}
		z.pos = start + end + 1
		return decodeEntities(z.s[start:start+end], true)
	}
	start := z.pos
	for z.pos < len(z.s) && !isSpace(z.s[z.pos]) && z.s[z.pos] != '>' {
		z.pos++
	}
	return decodeEntities(z.s[start:z.pos], true)
}

func hasAttr(attrs []Attr, name string) bool {
	for _, a := range attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isAlnum(c byte) bool {
	return isAlpha(c) || c >= '0' && c <= '9'
}
