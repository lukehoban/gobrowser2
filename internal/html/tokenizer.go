// Package html implements an HTML tokenizer and a simplified tree builder
// that produces a dom.Node tree.
package html

import (
	"strings"

	"github.com/lukehoban/gobrowser2/internal/dom"
)

// TokenType identifies the kind of a Token.
type TokenType int

const (
	TextToken TokenType = iota
	StartTagToken
	EndTagToken
	CommentToken
	DoctypeToken
)

// Token is a single lexical HTML token.
type Token struct {
	Type        TokenType
	Data        string // tag name (lower-case), text, or comment
	Attrs       []dom.Attr
	SelfClosing bool
}

// rawTextElements contain text that is not parsed as markup.
var rawTextElements = map[string]bool{"script": true, "style": true, "xmp": true, "iframe": true, "noembed": true, "noframes": true}

// rcdataElements contain text in which only character references are decoded.
var rcdataElements = map[string]bool{"title": true, "textarea": true}

// Tokenize splits src into tokens.
func Tokenize(src string) []Token {
	t := &tokenizer{src: src}
	return t.run()
}

type tokenizer struct {
	src    string
	pos    int
	tokens []Token
	text   strings.Builder
}

func (t *tokenizer) flushText() {
	if t.text.Len() > 0 {
		t.tokens = append(t.tokens, Token{Type: TextToken, Data: t.text.String()})
		t.text.Reset()
	}
}

func (t *tokenizer) run() []Token {
	for t.pos < len(t.src) {
		lt := strings.IndexByte(t.src[t.pos:], '<')
		if lt < 0 {
			t.text.WriteString(DecodeEntities(t.src[t.pos:], false))
			t.pos = len(t.src)
			break
		}
		t.text.WriteString(DecodeEntities(t.src[t.pos:t.pos+lt], false))
		t.pos += lt
		if !t.markup() {
			t.text.WriteByte('<')
			t.pos++
		}
	}
	t.flushText()
	return t.tokens
}

// markup attempts to consume markup starting at '<'. It returns false if the
// '<' should be treated as text.
func (t *tokenizer) markup() bool {
	rest := t.src[t.pos:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		end := strings.Index(rest[4:], "-->")
		t.flushText()
		if end < 0 {
			t.tokens = append(t.tokens, Token{Type: CommentToken, Data: rest[4:]})
			t.pos = len(t.src)
		} else {
			t.tokens = append(t.tokens, Token{Type: CommentToken, Data: rest[4 : 4+end]})
			t.pos += 4 + end + 3
		}
		return true
	case strings.HasPrefix(rest, "<!") || strings.HasPrefix(rest, "<?"):
		end := strings.IndexByte(rest, '>')
		t.flushText()
		data := rest[2:]
		if end < 0 {
			t.pos = len(t.src)
		} else {
			data = rest[2:end]
			t.pos += end + 1
		}
		if strings.HasPrefix(rest, "<!") && len(data) >= 7 && strings.EqualFold(data[:7], "doctype") {
			t.tokens = append(t.tokens, Token{Type: DoctypeToken, Data: strings.TrimSpace(data[7:])})
		} else {
			t.tokens = append(t.tokens, Token{Type: CommentToken, Data: data})
		}
		return true
	case strings.HasPrefix(rest, "</"):
		if len(rest) < 3 || !isASCIILetter(rest[2]) {
			if len(rest) >= 3 && rest[2] == '>' {
				t.pos += 3 // "</>" is ignored
				return true
			}
			return false
		}
		t.flushText()
		t.pos += 2
		name := t.tagName()
		// Skip attributes on end tags.
		t.attributes()
		t.tokens = append(t.tokens, Token{Type: EndTagToken, Data: name})
		return true
	case len(rest) >= 2 && isASCIILetter(rest[1]):
		t.flushText()
		t.pos++
		tok := Token{Type: StartTagToken, Data: t.tagName()}
		tok.Attrs, tok.SelfClosing = t.attributes()
		t.tokens = append(t.tokens, tok)
		if rawTextElements[tok.Data] || rcdataElements[tok.Data] {
			t.rawText(tok.Data)
		}
		return true
	}
	return false
}

func (t *tokenizer) tagName() string {
	start := t.pos
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if isSpace(c) || c == '/' || c == '>' {
			break
		}
		t.pos++
	}
	return strings.ToLower(t.src[start:t.pos])
}

// attributes parses attributes up to and including the closing '>'.
func (t *tokenizer) attributes() (attrs []dom.Attr, selfClosing bool) {
	seen := map[string]bool{}
	for {
		t.skipSpace()
		if t.pos >= len(t.src) {
			return
		}
		c := t.src[t.pos]
		if c == '>' {
			t.pos++
			return
		}
		if c == '/' {
			t.pos++
			if t.pos < len(t.src) && t.src[t.pos] == '>' {
				t.pos++
				selfClosing = true
				return
			}
			continue
		}
		// Attribute name.
		start := t.pos
		t.pos++ // the first character may be '=' per spec
		for t.pos < len(t.src) {
			c := t.src[t.pos]
			if isSpace(c) || c == '/' || c == '>' || c == '=' {
				break
			}
			t.pos++
		}
		name := strings.ToLower(t.src[start:t.pos])
		value := ""
		t.skipSpace()
		if t.pos < len(t.src) && t.src[t.pos] == '=' {
			t.pos++
			t.skipSpace()
			if t.pos < len(t.src) {
				switch q := t.src[t.pos]; q {
				case '"', '\'':
					end := strings.IndexByte(t.src[t.pos+1:], q)
					if end < 0 {
						value = t.src[t.pos+1:]
						t.pos = len(t.src)
					} else {
						value = t.src[t.pos+1 : t.pos+1+end]
						t.pos += end + 2
					}
				default:
					vs := t.pos
					for t.pos < len(t.src) && !isSpace(t.src[t.pos]) && t.src[t.pos] != '>' {
						t.pos++
					}
					value = t.src[vs:t.pos]
				}
			}
			value = DecodeEntities(value, true)
		}
		if !seen[name] {
			seen[name] = true
			attrs = append(attrs, dom.Attr{Name: name, Value: value})
		}
	}
}

// rawText consumes text up to the matching end tag of a raw text or RCDATA
// element.
func (t *tokenizer) rawText(tag string) {
	rest := t.src[t.pos:]
	lower := strings.ToLower(rest)
	end := -1
	for off := 0; ; {
		i := strings.Index(lower[off:], "</"+tag)
		if i < 0 {
			break
		}
		j := off + i + 2 + len(tag)
		if j >= len(lower) || isSpace(lower[j]) || lower[j] == '>' || lower[j] == '/' {
			end = off + i
			break
		}
		off = j
	}
	if end < 0 {
		end = len(rest)
	}
	text := rest[:end]
	if rcdataElements[tag] {
		text = DecodeEntities(text, false)
	}
	if text != "" {
		t.tokens = append(t.tokens, Token{Type: TextToken, Data: text})
	}
	t.pos += end
}

func (t *tokenizer) skipSpace() {
	for t.pos < len(t.src) && isSpace(t.src[t.pos]) {
		t.pos++
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
