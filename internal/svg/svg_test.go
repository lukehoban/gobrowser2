package svg

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestParsePath(t *testing.T) {
	segs := ParsePath("M1.5.5l2-1H10v3z")
	want := []Seg{
		{Op: 'M', P: [3][2]float64{{1.5, 0.5}}},
		{Op: 'L', P: [3][2]float64{{3.5, -0.5}}},
		{Op: 'L', P: [3][2]float64{{10, -0.5}}},
		{Op: 'L', P: [3][2]float64{{10, 2.5}}},
		{Op: 'Z'},
	}
	if len(segs) != len(want) {
		t.Fatalf("got %d segments: %+v", len(segs), segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Errorf("seg %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
}

func TestParsePathArc(t *testing.T) {
	// A half circle from (0,0) to (20,0) with compact flags.
	segs := ParsePath("M0 0a10 10 0 1020 0")
	if len(segs) < 2 {
		t.Fatalf("got %+v", segs)
	}
	last := segs[len(segs)-1]
	if last.Op != 'C' || math.Abs(last.P[2][0]-20) > 1e-9 || math.Abs(last.P[2][1]) > 1e-9 {
		t.Errorf("arc should end at (20,0) with a cubic: %+v", last)
	}
	for _, s := range segs[1:] {
		for _, p := range s.P {
			if math.Hypot(p[0]-10, p[1]) > 10*1.2 {
				t.Errorf("arc control point %v strays from circle", p)
			}
		}
	}
}

const logo = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="4 4 188 188">
<path d="m4 4h188v188h-188z" fill="#f60"/>
<g transform="translate(96 96)"><rect x="-40" y="-40" width="80" height="80" fill="#fff"/></g>
</svg>`

func TestRender(t *testing.T) {
	im, err := Parse([]byte(logo))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := im.Size(); w != 18 || h != 18 {
		t.Errorf("size = %v x %v", w, h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 40, 40))
	im.Draw(dst, image.Rect(10, 10, 30, 30))
	orange := color.RGBA{255, 102, 0, 255}
	if got := dst.RGBAAt(12, 12); got != orange {
		t.Errorf("corner pixel = %v, want orange", got)
	}
	if got := dst.RGBAAt(20, 20); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("center pixel = %v, want white", got)
	}
	if got := dst.RGBAAt(5, 5); got.A != 0 {
		t.Errorf("pixel outside target rect was painted: %v", got)
	}
}

func TestMalformed(t *testing.T) {
	if _, err := Parse([]byte("<svg><path d=")); err == nil {
		t.Error("expected error for malformed XML")
	}
	if _, err := Parse([]byte("<html/>")); err == nil {
		t.Error("expected error for non-svg root")
	}
}
