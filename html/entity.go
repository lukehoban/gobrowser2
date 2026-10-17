package html

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// namedEntities is a subset of the HTML named character references covering
// those commonly seen on real pages.
var namedEntities = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'",
	"nbsp": "\u00a0", "copy": "\u00a9", "reg": "\u00ae", "trade": "\u2122",
	"hellip": "\u2026", "mdash": "\u2014", "ndash": "\u2013",
	"lsquo": "\u2018", "rsquo": "\u2019", "sbquo": "\u201a",
	"ldquo": "\u201c", "rdquo": "\u201d", "bdquo": "\u201e",
	"bull": "\u2022", "middot": "\u00b7", "laquo": "\u00ab", "raquo": "\u00bb",
	"lsaquo": "\u2039", "rsaquo": "\u203a", "times": "\u00d7", "divide": "\u00f7",
	"deg": "\u00b0", "plusmn": "\u00b1", "micro": "\u00b5", "para": "\u00b6",
	"sect": "\u00a7", "cent": "\u00a2", "pound": "\u00a3", "yen": "\u00a5",
	"euro": "\u20ac", "iexcl": "\u00a1", "iquest": "\u00bf", "shy": "\u00ad",
	"ensp": "\u2002", "emsp": "\u2003", "thinsp": "\u2009",
	"zwnj": "\u200c", "zwj": "\u200d", "dagger": "\u2020", "Dagger": "\u2021",
	"larr": "\u2190", "uarr": "\u2191", "rarr": "\u2192", "darr": "\u2193",
	"harr": "\u2194", "hearts": "\u2665", "frac12": "\u00bd", "frac14": "\u00bc",
	"frac34": "\u00be", "sup2": "\u00b2", "sup3": "\u00b3",
	"Agrave": "\u00c0", "Aacute": "\u00c1", "Auml": "\u00c4", "Ccedil": "\u00c7",
	"Eacute": "\u00c9", "Ouml": "\u00d6", "Uuml": "\u00dc", "szlig": "\u00df",
	"agrave": "\u00e0", "aacute": "\u00e1", "acirc": "\u00e2", "auml": "\u00e4",
	"aring": "\u00e5", "ccedil": "\u00e7", "egrave": "\u00e8", "eacute": "\u00e9",
	"ecirc": "\u00ea", "euml": "\u00eb", "iacute": "\u00ed", "iuml": "\u00ef",
	"ntilde": "\u00f1", "oacute": "\u00f3", "ocirc": "\u00f4", "ouml": "\u00f6",
	"oslash": "\u00f8", "uacute": "\u00fa", "uuml": "\u00fc",
}

// legacyEntities may appear without a trailing semicolon.
var legacyEntities = []string{"nbsp", "amp", "copy", "quot", "reg", "lt", "gt"}

// windows1252 maps numeric references in 0x80-0x9F to the characters
// browsers actually render for them.
var windows1252 = map[int]rune{
	0x80: '\u20ac', 0x82: '\u201a', 0x83: '\u0192', 0x84: '\u201e', 0x85: '\u2026',
	0x86: '\u2020', 0x87: '\u2021', 0x88: '\u02c6', 0x89: '\u2030', 0x8a: '\u0160',
	0x8b: '\u2039', 0x8c: '\u0152', 0x8e: '\u017d', 0x91: '\u2018', 0x92: '\u2019',
	0x93: '\u201c', 0x94: '\u201d', 0x95: '\u2022', 0x96: '\u2013', 0x97: '\u2014',
	0x98: '\u02dc', 0x99: '\u2122', 0x9a: '\u0161', 0x9b: '\u203a', 0x9c: '\u0153',
	0x9e: '\u017e', 0x9f: '\u0178',
}

// decodeEntities replaces character references in s. inAttr selects the
// stricter attribute-value rules for references missing a semicolon.
func decodeEntities(s string, inAttr bool) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			j := strings.IndexByte(s[i:], '&')
			if j < 0 {
				sb.WriteString(s[i:])
				break
			}
			sb.WriteString(s[i : i+j])
			i += j
			continue
		}
		repl, n := decodeRef(s[i:], inAttr)
		if n == 0 {
			sb.WriteByte('&')
			i++
			continue
		}
		sb.WriteString(repl)
		i += n
	}
	return sb.String()
}

// decodeRef decodes the character reference at the start of s (which begins
// with '&'). It returns the replacement and the number of bytes consumed,
// or n == 0 if s does not start with a recognized reference.
func decodeRef(s string, inAttr bool) (string, int) {
	if len(s) > 1 && s[1] == '#' {
		return decodeNumericRef(s)
	}
	j := 1
	for j < len(s) && isAlnum(s[j]) {
		j++
	}
	name := s[1:j]
	if name == "" {
		return "", 0
	}
	if j < len(s) && s[j] == ';' {
		if v, ok := namedEntities[name]; ok {
			return v, j + 1
		}
	}
	for _, legacy := range legacyEntities {
		if !strings.HasPrefix(name, legacy) {
			continue
		}
		next := 1 + len(legacy)
		if inAttr && next < len(s) && (isAlnum(s[next]) || s[next] == '=') {
			return "", 0
		}
		return namedEntities[legacy], next
	}
	return "", 0
}

func decodeNumericRef(s string) (string, int) {
	i := 2
	base := 10
	if i < len(s) && (s[i] == 'x' || s[i] == 'X') {
		base = 16
		i++
	}
	start := i
	for i < len(s) && isDigit(s[i], base) {
		i++
	}
	if i == start {
		return "", 0
	}
	n, err := strconv.ParseInt(s[start:i], base, 32)
	if err != nil || n > utf8.MaxRune {
		n = utf8.RuneError
	}
	if i < len(s) && s[i] == ';' {
		i++
	}
	r := rune(n)
	if w, ok := windows1252[int(n)]; ok {
		r = w
	} else if n == 0 || (n >= 0xd800 && n <= 0xdfff) {
		r = utf8.RuneError
	}
	return string(r), i
}

func isDigit(c byte, base int) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	return base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')
}
