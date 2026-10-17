package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/html"
	"github.com/lukehoban/gobrowser2/internal/layout"
	"github.com/lukehoban/gobrowser2/internal/style"
)

func paintHTML(src string, w, h int) *image.RGBA {
	doc := html.Parse(src)
	root := layout.Build(doc, style.Compute(doc, nil, float64(w)), nil)
	layout.Layout(root, float64(w))
	return Paint(root, w, h)
}

func rgba(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

func TestPaintBackgroundsAndBorders(t *testing.T) {
	img := paintHTML(`<!DOCTYPE html><body style="margin:0; background:#00f">
<div style="margin:10px; width:50px; height:20px; background:#ff6600; border:2px solid #000"></div>`, 100, 60)
	cases := []struct {
		x, y int
		want color.RGBA
	}{
		{5, 5, color.RGBA{0, 0, 255, 255}},     // canvas gets the body background
		{11, 11, color.RGBA{0, 0, 0, 255}},     // border
		{30, 20, color.RGBA{255, 102, 0, 255}}, // content background
		{80, 20, color.RGBA{0, 0, 255, 255}},   // outside the div
		{30, 50, color.RGBA{0, 0, 255, 255}},   // below the div
	}
	for _, c := range cases {
		if got := rgba(img, c.x, c.y); got != c.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestPaintText(t *testing.T) {
	img := paintHTML(`<!DOCTYPE html><body style="margin:0; color:#f00; font-size:20px">Hello</body>`, 100, 30)
	red := 0
	for y := 0; y < 30; y++ {
		for x := 0; x < 100; x++ {
			if c := rgba(img, x, y); c.R > 200 && c.G < 50 && c.B < 50 {
				red++
			}
		}
	}
	if red < 50 {
		t.Errorf("found %d red text pixels, want glyphs to be drawn", red)
	}
	for x := 70; x < 100; x++ {
		if c := rgba(img, x, 15); c != (color.RGBA{255, 255, 255, 255}) {
			t.Fatalf("pixel (%d,15) = %v, want white after the text", x, c)
		}
	}
}
