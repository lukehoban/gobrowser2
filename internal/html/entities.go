package html

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// namedEntities is a subset of the HTML named character references covering
// the most commonly used ones.
var namedEntities = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'", "nbsp": "\u00a0",
	"copy": "©", "reg": "®", "trade": "™", "deg": "°", "plusmn": "±", "para": "¶",
	"sect": "§", "middot": "·", "bull": "•", "hellip": "…", "prime": "′", "Prime": "″",
	"ndash": "–", "mdash": "—", "lsquo": "‘", "rsquo": "’", "sbquo": "‚",
	"ldquo": "“", "rdquo": "”", "bdquo": "„", "laquo": "«", "raquo": "»",
	"lsaquo": "‹", "rsaquo": "›", "times": "×", "divide": "÷", "minus": "−",
	"cent": "¢", "pound": "£", "yen": "¥", "euro": "€", "curren": "¤",
	"iexcl": "¡", "iquest": "¿", "shy": "\u00ad", "ensp": "\u2002", "emsp": "\u2003",
	"thinsp": "\u2009", "zwnj": "\u200c", "zwj": "\u200d", "lrm": "\u200e", "rlm": "\u200f",
	"larr": "←", "uarr": "↑", "rarr": "→", "darr": "↓", "harr": "↔",
	"hearts": "♥", "spades": "♠", "clubs": "♣", "diams": "♦", "loz": "◊",
	"frac12": "½", "frac14": "¼", "frac34": "¾", "sup1": "¹", "sup2": "²", "sup3": "³",
	"micro": "µ", "ordf": "ª", "ordm": "º", "not": "¬", "macr": "¯", "acute": "´",
	"cedil": "¸", "uml": "¨", "dagger": "†", "Dagger": "‡", "permil": "‰",
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "epsilon": "ε", "lambda": "λ",
	"mu": "μ", "pi": "π", "sigma": "σ", "tau": "τ", "phi": "φ", "omega": "ω",
	"Alpha": "Α", "Beta": "Β", "Gamma": "Γ", "Delta": "Δ", "Sigma": "Σ", "Omega": "Ω", "Pi": "Π",
	"infin": "∞", "ne": "≠", "le": "≤", "ge": "≥", "asymp": "≈", "equiv": "≡",
	"sum": "∑", "prod": "∏", "radic": "√", "part": "∂", "int": "∫", "forall": "∀", "exist": "∃",
	"isin": "∈", "and": "∧", "or": "∨", "cap": "∩", "cup": "∪", "empty": "∅", "nabla": "∇",
	"check": "✓", "star": "☆", "starf": "★",
	"Agrave": "À", "Aacute": "Á", "Acirc": "Â", "Atilde": "Ã", "Auml": "Ä", "Aring": "Å", "AElig": "Æ",
	"Ccedil": "Ç", "Egrave": "È", "Eacute": "É", "Ecirc": "Ê", "Euml": "Ë",
	"Igrave": "Ì", "Iacute": "Í", "Icirc": "Î", "Iuml": "Ï", "Ntilde": "Ñ",
	"Ograve": "Ò", "Oacute": "Ó", "Ocirc": "Ô", "Otilde": "Õ", "Ouml": "Ö", "Oslash": "Ø",
	"Ugrave": "Ù", "Uacute": "Ú", "Ucirc": "Û", "Uuml": "Ü", "Yacute": "Ý", "szlig": "ß",
	"agrave": "à", "aacute": "á", "acirc": "â", "atilde": "ã", "auml": "ä", "aring": "å", "aelig": "æ",
	"ccedil": "ç", "egrave": "è", "eacute": "é", "ecirc": "ê", "euml": "ë",
	"igrave": "ì", "iacute": "í", "icirc": "î", "iuml": "ï", "ntilde": "ñ",
	"ograve": "ò", "oacute": "ó", "ocirc": "ô", "otilde": "õ", "ouml": "ö", "oslash": "ø",
	"ugrave": "ù", "uacute": "ú", "ucirc": "û", "uuml": "ü", "yacute": "ý", "yuml": "ÿ",
}

// legacyEntities may appear without a trailing semicolon in text.
var legacyEntities = map[string]bool{"amp": true, "lt": true, "gt": true, "quot": true, "nbsp": true, "copy": true, "reg": true}

// windows1252 maps the C1 control range to the characters browsers use.
var windows1252 = map[int]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰',
	0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•',
	0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

// DecodeEntities replaces character references in s. inAttr selects the
// attribute-value rules for legacy references without semicolons.
func DecodeEntities(s string, inAttr bool) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		amp := strings.IndexByte(s[i:], '&')
		if amp < 0 {
			sb.WriteString(s[i:])
			break
		}
		sb.WriteString(s[i : i+amp])
		i += amp
		decoded, n := decodeOne(s[i:], inAttr)
		if n == 0 {
			sb.WriteByte('&')
			i++
			continue
		}
		sb.WriteString(decoded)
		i += n
	}
	return sb.String()
}

// decodeOne decodes a character reference at the start of s (which begins
// with '&'), returning the replacement and the number of bytes consumed.
func decodeOne(s string, inAttr bool) (string, int) {
	if len(s) < 2 {
		return "", 0
	}
	if s[1] == '#' {
		j := 2
		hex := false
		if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
			hex = true
			j++
		}
		start := j
		for j < len(s) && (isDigit(s[j]) || (hex && isHexLetter(s[j]))) {
			j++
		}
		if j == start {
			return "", 0
		}
		base := 10
		if hex {
			base = 16
		}
		v, err := strconv.ParseInt(s[start:j], base, 32)
		if j < len(s) && s[j] == ';' {
			j++
		}
		if err != nil || v > utf8.MaxRune || v == 0 || (v >= 0xD800 && v <= 0xDFFF) {
			return "\uFFFD", j
		}
		if r, ok := windows1252[int(v)]; ok {
			return string(r), j
		}
		return string(rune(v)), j
	}
	j := 1
	for j < len(s) && j < 40 && (isASCIILetter(s[j]) || isDigit(s[j])) {
		j++
	}
	name := s[1:j]
	if j < len(s) && s[j] == ';' {
		if v, ok := namedEntities[name]; ok {
			return v, j + 1
		}
		return "", 0
	}
	if legacyEntities[name] {
		// In attributes, "&amp=" style sequences are left alone.
		if inAttr && j < len(s) && (s[j] == '=' || isASCIILetter(s[j]) || isDigit(s[j])) {
			return "", 0
		}
		return namedEntities[name], j
	}
	return "", 0
}

func isDigit(c byte) bool     { return c >= '0' && c <= '9' }
func isHexLetter(c byte) bool { return (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
