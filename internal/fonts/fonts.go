// Package fonts provides font faces for layout and painting, backed by the
// embedded Go font family so rendering is deterministic across machines.
package fonts

import (
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Face is a sized font face with cached metrics.
type Face struct {
	font.Face
	Size    float64 // em size in CSS px
	Ascent  float64
	Descent float64

	mu     sync.Mutex
	widths map[string]float64
}

type key struct {
	mono, bold, italic bool
	size               float64
}

var (
	mu     sync.Mutex
	parsed = map[key]*opentype.Font{}
	faces  = map[key]*Face{}
)

func source(k key) []byte {
	switch {
	case k.mono && k.bold && k.italic:
		return gomonobolditalic.TTF
	case k.mono && k.bold:
		return gomonobold.TTF
	case k.mono && k.italic:
		return gomonoitalic.TTF
	case k.mono:
		return gomono.TTF
	case k.bold && k.italic:
		return gobolditalic.TTF
	case k.bold:
		return gobold.TTF
	case k.italic:
		return goitalic.TTF
	}
	return goregular.TTF
}

// IsMonospace reports whether a CSS font-family list prefers a monospace
// font.
func IsMonospace(family string) bool {
	for _, f := range strings.Split(family, ",") {
		f = strings.ToLower(strings.Trim(strings.TrimSpace(f), `"'`))
		switch f {
		case "monospace", "courier", "courier new", "consolas", "menlo", "monaco", "lucida console":
			return true
		case "serif", "sans-serif", "verdana", "arial", "helvetica", "geneva", "times", "times new roman", "georgia":
			return false
		}
	}
	return false
}

// Get returns a face for the given family list, size in px and style.
func Get(family string, size float64, bold, italic bool) *Face {
	if size <= 0 {
		size = 1
	}
	// Quantize to quarter pixels to bound the cache.
	size = math.Round(size*4) / 4
	k := key{mono: IsMonospace(family), bold: bold, italic: italic, size: size}
	mu.Lock()
	defer mu.Unlock()
	if f, ok := faces[k]; ok {
		return f
	}
	pk := key{mono: k.mono, bold: bold, italic: italic}
	ot, ok := parsed[pk]
	if !ok {
		var err error
		ot, err = opentype.Parse(source(k))
		if err != nil {
			panic(err) // embedded fonts are always valid
		}
		parsed[pk] = ot
	}
	of, err := opentype.NewFace(ot, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic(err)
	}
	m := of.Metrics()
	f := &Face{
		Face:    of,
		Size:    size,
		Ascent:  fromFixed(m.Ascent),
		Descent: fromFixed(m.Descent),
		widths:  map[string]float64{},
	}
	faces[k] = f
	return f
}

// Measure returns the advance width of s in px.
func (f *Face) Measure(s string) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.widths[s]; ok {
		return w
	}
	w := fromFixed(font.MeasureString(f.Face, s))
	if len(f.widths) < 50000 {
		f.widths[s] = w
	}
	return w
}

func fromFixed(v fixed.Int26_6) float64 { return float64(v) / 64 }
