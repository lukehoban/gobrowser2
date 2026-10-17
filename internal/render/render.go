// Package render paints a laid-out box tree into an image.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/lukehoban/gobrowser2/internal/layout"
)

// Paint renders root into a new width x height RGBA image.
func Paint(root *layout.Box, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg, propagated := canvasBackground(root)
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	if root != nil {
		p := &painter{img: img, propagated: propagated}
		p.box(root)
	}
	return img
}

// canvasBackground implements background propagation from the root
// element (or else the body) to the canvas. It returns the canvas color
// and the box whose background it came from.
func canvasBackground(root *layout.Box) (color.NRGBA, *layout.Box) {
	white := color.NRGBA{255, 255, 255, 255}
	if root == nil {
		return white, nil
	}
	if root.Style.BackgroundColor.A > 0 {
		return over(white, root.Style.BackgroundColor), root
	}
	for _, c := range root.Children {
		if c.Node != nil && c.Node.Tag == "body" && c.Style.BackgroundColor.A > 0 {
			return over(white, c.Style.BackgroundColor), c
		}
	}
	return white, nil
}

func over(dst, src color.NRGBA) color.NRGBA {
	a := float64(src.A) / 255
	mix := func(d, s uint8) uint8 { return uint8(math.Round(float64(s)*a + float64(d)*(1-a))) }
	return color.NRGBA{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), 255}
}

type painter struct {
	img        *image.RGBA
	propagated *layout.Box // box whose background is painted on the canvas
}

func rect(x, y, w, h float64) image.Rectangle {
	return image.Rect(int(math.Round(x)), int(math.Round(y)), int(math.Round(x+w)), int(math.Round(y+h)))
}

func (p *painter) fill(r image.Rectangle, c color.NRGBA) {
	if c.A == 0 || r.Empty() {
		return
	}
	draw.Draw(p.img, r.Intersect(p.img.Bounds()), image.NewUniform(c), image.Point{}, draw.Over)
}

func (p *painter) box(b *layout.Box) {
	st := b.Style
	if !st.Hidden {
		if b != p.propagated {
			x, y, w, h := b.BorderBox()
			p.fill(rect(x, y, w, h), st.BackgroundColor)
		}
		p.borders(b)
		if b.Type == layout.ReplacedBox && b.Image != nil {
			dst := rect(b.X, b.Y, b.W, b.H)
			if !dst.Empty() {
				xdraw.ApproxBiLinear.Scale(p.img, dst, b.Image, b.Image.Bounds(), draw.Over, nil)
			}
		}
		if b.Marker != "" {
			p.marker(b)
		}
	}
	for _, c := range b.Children {
		p.box(c)
	}
	for _, l := range b.Lines {
		for _, f := range l.Frags {
			if f.Box != nil {
				p.box(f.Box)
				continue
			}
			if !f.Style.Hidden {
				p.text(f)
			}
		}
	}
}

func (p *painter) borders(b *layout.Box) {
	x, y, w, h := b.BorderBox()
	st := b.Style
	bw := b.Border
	if bw.Top > 0 {
		p.fill(rect(x, y, w, bw.Top), st.BorderColor[0])
	}
	if bw.Bottom > 0 {
		p.fill(rect(x, y+h-bw.Bottom, w, bw.Bottom), st.BorderColor[2])
	}
	if bw.Left > 0 {
		p.fill(rect(x, y+bw.Top, bw.Left, h-bw.Top-bw.Bottom), st.BorderColor[3])
	}
	if bw.Right > 0 {
		p.fill(rect(x+w-bw.Right, y+bw.Top, bw.Right, h-bw.Top-bw.Bottom), st.BorderColor[1])
	}
}

func (p *painter) text(f *layout.Frag) {
	st := f.Style
	if f.Face == nil {
		return
	}
	if f.Text != "" {
		d := font.Drawer{
			Dst:  p.img,
			Src:  image.NewUniform(st.Color),
			Face: f.Face,
			Dot:  fixed.Point26_6{X: fixed.Int26_6(math.Round(f.X * 64)), Y: fixed.Int26_6(math.Round(f.Baseline * 64))},
		}
		d.DrawString(f.Text)
	}
	thickness := math.Max(1, math.Round(st.FontSize/14))
	if st.Underline {
		p.fill(rect(f.X, f.Baseline+math.Max(1, f.Face.Descent/2), f.W, thickness), st.Color)
	}
	if st.LineThrough {
		p.fill(rect(f.X, f.Baseline-f.Face.Ascent*0.3, f.W, thickness), st.Color)
	}
}

// marker paints a list item marker to the left of its first line.
func (p *painter) marker(b *layout.Box) {
	baseline, ok := firstBaseline(b)
	if !ok {
		return
	}
	face := layout.FaceFor(b.Style)
	w := face.Measure(b.Marker)
	d := font.Drawer{
		Dst:  p.img,
		Src:  image.NewUniform(b.Style.Color),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(math.Round((b.X - w) * 64)), Y: fixed.Int26_6(math.Round(baseline * 64))},
	}
	d.DrawString(b.Marker)
}

func firstBaseline(b *layout.Box) (float64, bool) {
	if len(b.Lines) > 0 {
		return b.Lines[0].Baseline, true
	}
	for _, c := range b.Children {
		if y, ok := firstBaseline(c); ok {
			return y, true
		}
	}
	return 0, false
}
