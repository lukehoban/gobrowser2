package style

import (
	"image/color"
	"strconv"
	"strings"
)

// Unit is the unit of a computed Length.
type Unit int

const (
	Auto Unit = iota
	Px
	Percent
)

// Length is a computed length: auto, an absolute pixel value or a percentage.
type Length struct {
	V    float64
	Unit Unit
}

// PxLen returns a pixel length.
func PxLen(v float64) Length { return Length{V: v, Unit: Px} }

// IsAuto reports whether l is auto.
func (l Length) IsAuto() bool { return l.Unit == Auto }

// Resolve returns the length in pixels, resolving percentages against base.
// Auto resolves to def.
func (l Length) Resolve(base, def float64) float64 {
	switch l.Unit {
	case Px:
		return l.V
	case Percent:
		return l.V * base / 100
	}
	return def
}

// parseLength parses a CSS length. em and % (for font-size) are resolved
// against fontSize. It returns ok=false for unparseable values.
func parseLength(s string, fontSize float64) (Length, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "auto" {
		return Length{}, true
	}
	if s == "0" {
		return PxLen(0), true
	}
	num, unit := splitNumber(s)
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return Length{}, false
	}
	switch unit {
	case "px", "":
		return PxLen(v), unit == "px" || v == 0
	case "pt":
		return PxLen(v * 4 / 3), true
	case "pc":
		return PxLen(v * 16), true
	case "in":
		return PxLen(v * 96), true
	case "cm":
		return PxLen(v * 96 / 2.54), true
	case "mm":
		return PxLen(v * 96 / 25.4), true
	case "em":
		return PxLen(v * fontSize), true
	case "rem":
		return PxLen(v * 16), true
	case "ex", "ch":
		return PxLen(v * fontSize / 2), true
	case "%":
		return Length{V: v, Unit: Percent}, true
	}
	return Length{}, false
}

// parseHTMLLength parses a presentational attribute length such as "85%" or
// "18" (pixels). It returns ok=false for invalid or non-positive values.
func parseHTMLLength(s string) (string, bool) {
	s = strings.TrimSpace(s)
	num, unit := splitNumber(s)
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v < 0 {
		return "", false
	}
	if strings.HasPrefix(unit, "%") {
		return strconv.FormatFloat(v, 'f', -1, 64) + "%", true
	}
	return strconv.FormatFloat(v, 'f', -1, 64) + "px", true
}

func splitNumber(s string) (num, unit string) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return s[:i], s[i:]
}

// ParseColor parses a CSS color. HTML attribute colors without a leading '#'
// are accepted too.
func ParseColor(s string) (color.RGBA, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := namedColors[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		return parseRGBFunc(s)
	}
	h := strings.TrimPrefix(s, "#")
	for _, r := range h {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return color.RGBA{}, false
		}
	}
	hex := func(x string) uint8 {
		v, _ := strconv.ParseUint(x, 16, 8)
		return uint8(v)
	}
	switch len(h) {
	case 3, 4:
		c := color.RGBA{hex(h[0:1]) * 17, hex(h[1:2]) * 17, hex(h[2:3]) * 17, 255}
		if len(h) == 4 {
			c.A = hex(h[3:4]) * 17
		}
		return premul(c), true
	case 6, 8:
		c := color.RGBA{hex(h[0:2]), hex(h[2:4]), hex(h[4:6]), 255}
		if len(h) == 8 {
			c.A = hex(h[6:8])
		}
		return premul(c), true
	}
	return color.RGBA{}, false
}

func parseRGBFunc(s string) (color.RGBA, bool) {
	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return color.RGBA{}, false
	}
	parts := strings.FieldsFunc(s[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
	if len(parts) < 3 {
		return color.RGBA{}, false
	}
	var ch [4]float64
	ch[3] = 1
	for i := 0; i < len(parts) && i < 4; i++ {
		p := parts[i]
		pct := strings.HasSuffix(p, "%")
		v, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
		if err != nil {
			return color.RGBA{}, false
		}
		switch {
		case i < 3 && pct:
			v = v * 255 / 100
		case i == 3 && pct:
			v /= 100
		}
		ch[i] = v
	}
	clamp := func(v, max float64) float64 {
		if v < 0 {
			return 0
		}
		if v > max {
			return max
		}
		return v
	}
	c := color.RGBA{uint8(clamp(ch[0], 255) + 0.5), uint8(clamp(ch[1], 255) + 0.5), uint8(clamp(ch[2], 255) + 0.5), uint8(clamp(ch[3], 1)*255 + 0.5)}
	return premul(c), true
}

// premul converts a straight-alpha color to the premultiplied form that
// image/color.RGBA expects.
func premul(c color.RGBA) color.RGBA {
	if c.A == 255 {
		return c
	}
	a := uint32(c.A)
	return color.RGBA{uint8(uint32(c.R) * a / 255), uint8(uint32(c.G) * a / 255), uint8(uint32(c.B) * a / 255), c.A}
}

var namedColors = map[string]color.RGBA{
	"transparent": {0, 0, 0, 0},
	"black":       {0, 0, 0, 255},
	"silver":      {192, 192, 192, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"white":       {255, 255, 255, 255},
	"maroon":      {128, 0, 0, 255},
	"red":         {255, 0, 0, 255},
	"purple":      {128, 0, 128, 255},
	"fuchsia":     {255, 0, 255, 255},
	"magenta":     {255, 0, 255, 255},
	"green":       {0, 128, 0, 255},
	"lime":        {0, 255, 0, 255},
	"olive":       {128, 128, 0, 255},
	"yellow":      {255, 255, 0, 255},
	"navy":        {0, 0, 128, 255},
	"blue":        {0, 0, 255, 255},
	"teal":        {0, 128, 128, 255},
	"aqua":        {0, 255, 255, 255},
	"cyan":        {0, 255, 255, 255},
	"orange":      {255, 165, 0, 255},
	"darkgray":    {169, 169, 169, 255},
	"darkgrey":    {169, 169, 169, 255},
	"lightgray":   {211, 211, 211, 255},
	"lightgrey":   {211, 211, 211, 255},
	"dimgray":     {105, 105, 105, 255},
	"dimgrey":     {105, 105, 105, 255},
	"gainsboro":   {220, 220, 220, 255},
	"whitesmoke":  {245, 245, 245, 255},
	"darkred":     {139, 0, 0, 255},
	"darkblue":    {0, 0, 139, 255},
	"darkgreen":   {0, 100, 0, 255},
	"lightblue":   {173, 216, 230, 255},
	"beige":       {245, 245, 220, 255},
	"brown":       {165, 42, 42, 255},
	"gold":        {255, 215, 0, 255},
	"pink":        {255, 192, 203, 255},
	"ivory":       {255, 255, 240, 255},
}
