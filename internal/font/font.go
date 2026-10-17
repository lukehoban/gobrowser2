// Package font provides the embedded fonts used for text measurement and
// painting. It uses the Go fonts (BSD licensed) so output is deterministic
// and independent of system fonts.
package font

import (
	"sync"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Verdana's vertical metrics, as fractions of the em size.
const (
	verdanaAscent  = 2059.0 / 2048
	verdanaDescent = 430.0 / 2048
)

// Key identifies a face.
type Key struct {
	Mono   bool
	Bold   bool
	Italic bool
	Size   float64 // pixels
}

// Face is a sized font face with float metrics.
type Face struct {
	Face    xfont.Face
	Ascent  float64
	Descent float64
	mu      sync.Mutex
	advs    map[rune]float64
}

var (
	parseOnce sync.Once
	fonts     map[[3]bool]*opentype.Font

	mu    sync.Mutex
	faces = map[Key]*Face{}
)

func load() {
	fonts = map[[3]bool]*opentype.Font{}
	for k, data := range map[[3]bool][]byte{
		{false, false, false}: goregular.TTF,
		{false, true, false}:  gobold.TTF,
		{false, false, true}:  goitalic.TTF,
		{false, true, true}:   gobolditalic.TTF,
		{true, false, false}:  gomono.TTF,
		{true, true, false}:   gomonobold.TTF,
	} {
		f, err := opentype.Parse(data)
		if err != nil {
			panic(err)
		}
		fonts[k] = f
	}
}

// Get returns the face for k, creating it on first use.
func Get(k Key) *Face {
	parseOnce.Do(load)
	if k.Size <= 0 {
		k.Size = 16
	}
	mu.Lock()
	defer mu.Unlock()
	if f, ok := faces[k]; ok {
		return f
	}
	fk := [3]bool{k.Mono, k.Bold, k.Italic}
	if k.Mono {
		fk[2] = false
	}
	otf := fonts[fk]
	xf, err := opentype.NewFace(otf, &opentype.FaceOptions{Size: k.Size, DPI: 72, Hinting: xfont.HintingNone})
	if err != nil {
		panic(err)
	}
	m := xf.Metrics()
	f := &Face{Face: xf, Ascent: toF(m.Ascent), Descent: toF(m.Descent), advs: map[rune]float64{}}
	if !k.Mono {
		// Pages (Hacker News in particular) ask for Verdana, which we can't
		// embed. Use its vertical metrics so line heights match.
		f.Ascent, f.Descent = verdanaAscent*k.Size, verdanaDescent*k.Size
	}
	faces[k] = f
	return f
}

func toF(v fixed.Int26_6) float64 { return float64(v) / 64 }

// Measure returns the advance width of s in pixels.
func (f *Face) Measure(s string) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := 0.0
	for _, r := range s {
		a, ok := f.advs[r]
		if !ok {
			adv, ok2 := f.Face.GlyphAdvance(r)
			if !ok2 {
				adv, _ = f.Face.GlyphAdvance('?')
			}
			a = toF(adv)
			f.advs[r] = a
		}
		w += a
	}
	return w
}
