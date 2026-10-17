// Package render paints a laid-out box tree into an RGBA image.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/lukehoban/gobrowser2/internal/layout"
	"github.com/lukehoban/gobrowser2/internal/style"
)

// MaxHeight bounds the height of the output image.
const MaxHeight = 32000

// Options configures painting.
type Options struct {
	Width      int
	Background color.RGBA
	// BackgroundImage resolves CSS background-image URLs; may be nil.
	BackgroundImage func(url string) layout.Image
}

// Paint renders the layout result into a new image whose height is the
// document height.
func Paint(res *layout.Result, opt Options) *image.RGBA {
	h := int(math.Ceil(res.Height))
	if h < 1 {
		h = 1
	}
	if h > MaxHeight {
		h = MaxHeight
	}
	img := image.NewRGBA(image.Rect(0, 0, opt.Width, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(opt.Background), image.Point{}, draw.Src)
	p := &painter{dst: img, opt: opt}
	p.box(res.Root)
	return img
}

type painter struct {
	dst *image.RGBA
	opt Options
}

func rect(x, y, w, h float64) image.Rectangle {
	return image.Rect(int(math.Round(x)), int(math.Round(y)), int(math.Round(x+w)), int(math.Round(y+h)))
}

func (p *painter) fill(r image.Rectangle, c color.RGBA) {
	if c.A == 0 {
		return
	}
	draw.Draw(p.dst, r, image.NewUniform(c), image.Point{}, draw.Over)
}

func (p *painter) box(b *layout.Box) {
	s := b.Style
	visible := !s.Hidden
	switch b.Kind {
	case layout.KText, layout.KInline, layout.KBreak:
		// Painted via the line boxes of their block container.
	default:
		if visible {
			p.background(b)
			p.borders(b)
			if b.Kind == layout.KReplaced && b.Image != nil {
				b.Image.Draw(p.dst, rect(b.X, b.Y, b.W, b.H))
			}
		}
	}
	for _, l := range b.Lines {
		for _, f := range l.Frags {
			if f.Box == nil && !f.Style.Hidden {
				p.text(f)
			}
		}
	}
	for _, c := range b.Children {
		p.box(c)
	}
}

func (p *painter) background(b *layout.Box) {
	x, y, w, h := b.BorderBox()
	r := rect(x, y, w, h)
	p.fill(r, b.Style.BackgroundColor)
	if b.Style.BackgroundImage == "" || p.opt.BackgroundImage == nil {
		return
	}
	im := p.opt.BackgroundImage(b.Style.BackgroundImage)
	if im == nil {
		return
	}
	iw, ih := im.Size()
	if bs := b.Style.BackgroundSize; !bs.IsAuto() && iw > 0 {
		nw := bs.Resolve(w, iw)
		ih, iw = ih*nw/iw, nw
	}
	if iw < 1 || ih < 1 {
		return
	}
	// Paint the image at the top-left of the padding box, clipped to the
	// border box. Repetition is not implemented yet.
	px := x + b.Border[style.Left]
	py := y + b.Border[style.Top]
	tile := rect(px, py, iw, ih)
	sub := p.dst.SubImage(r.Intersect(p.dst.Bounds())).(*image.RGBA)
	if !tile.Overlaps(sub.Bounds()) {
		return
	}
	im.Draw(sub, tile)
}

func (p *painter) borders(b *layout.Box) {
	x, y, w, h := b.BorderBox()
	s := b.Style
	bt, br, bb, bl := b.Border[style.Top], b.Border[style.Right], b.Border[style.Bottom], b.Border[style.Left]
	if bt > 0 {
		p.fill(rect(x, y, w, bt), borderColor(s, style.Top))
	}
	if bb > 0 {
		p.fill(rect(x, y+h-bb, w, bb), borderColor(s, style.Bottom))
	}
	if bl > 0 {
		p.fill(rect(x, y+bt, bl, h-bt-bb), borderColor(s, style.Left))
	}
	if br > 0 {
		p.fill(rect(x+w-br, y+bt, br, h-bt-bb), borderColor(s, style.Right))
	}
}

// borderColor approximates inset/outset shading on the given side.
func borderColor(s *style.Style, side int) color.RGBA {
	c := s.BorderColor[side]
	dark := func(c color.RGBA) color.RGBA {
		return color.RGBA{uint8(float64(c.R) * 0.6), uint8(float64(c.G) * 0.6), uint8(float64(c.B) * 0.6), c.A}
	}
	switch s.BorderStyle[side] {
	case "inset", "groove":
		if side == style.Top || side == style.Left {
			return dark(c)
		}
	case "outset", "ridge":
		if side == style.Bottom || side == style.Right {
			return dark(c)
		}
	}
	return c
}

func (p *painter) text(f *layout.Frag) {
	s := f.Style
	face := layout.FaceFor(s)
	d := &xfont.Drawer{
		Dst:  p.dst,
		Src:  image.NewUniform(s.Color),
		Face: face.Face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(math.Round(f.X * 64)), Y: fixed.Int26_6(math.Round(f.Y * 64))},
	}
	d.DrawString(f.Text)
	if s.Underline {
		thick := math.Max(1, math.Round(s.FontSize/14))
		p.fill(rect(f.X, f.Y+math.Max(1, s.FontSize*0.1), f.W, thick), s.Color)
	}
	if s.LineThrough {
		thick := math.Max(1, math.Round(s.FontSize/14))
		p.fill(rect(f.X, f.Y-s.FontSize*0.3, f.W, thick), s.Color)
	}
}
