// Package svg is a small SVG renderer: it parses a subset of SVG (paths and
// basic shapes with solid fills and transforms) and rasterizes it with
// golang.org/x/image/vector.
package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/lukehoban/gobrowser2/internal/style"
	"golang.org/x/image/vector"
)

// Image is a parsed SVG document.
type Image struct {
	W, H    float64    // intrinsic size in CSS pixels
	ViewBox [4]float64 // min-x, min-y, width, height
	Shapes  []Shape
}

// Shape is a filled path in viewBox coordinates.
type Shape struct {
	Path []Seg
	Fill color.RGBA // premultiplied
}

// Seg is a path segment. Op is one of 'M', 'L', 'Q', 'C', 'Z'; P holds the
// control points and end point in absolute viewBox coordinates.
type Seg struct {
	Op byte
	P  [3][2]float64
}

// matrix is an affine transform [a b c d e f].
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) apply(p [2]float64) [2]float64 {
	return [2]float64{m[0]*p[0] + m[2]*p[1] + m[4], m[1]*p[0] + m[3]*p[1] + m[5]}
}

type state struct {
	fill    string
	opacity float64
	m       matrix
}

// Parse parses an SVG document.
func Parse(data []byte) (*Image, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	im := &Image{}
	stack := []state{{fill: "black", opacity: 1, m: identity}}
	root := true
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			attrs := map[string]string{}
			for _, a := range t.Attr {
				attrs[a.Name.Local] = a.Value
			}
			if st, ok := attrs["style"]; ok {
				for _, decl := range strings.Split(st, ";") {
					if k := strings.IndexByte(decl, ':'); k > 0 {
						attrs[strings.TrimSpace(decl[:k])] = strings.TrimSpace(decl[k+1:])
					}
				}
			}
			cur := stack[len(stack)-1]
			if v, ok := attrs["fill"]; ok {
				cur.fill = v
			}
			for _, k := range []string{"opacity", "fill-opacity"} {
				if v, ok := attrs[k]; ok {
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						cur.opacity *= math.Max(0, math.Min(1, f))
					}
				}
			}
			if v, ok := attrs["transform"]; ok {
				cur.m = cur.m.mul(parseTransform(v))
			}
			stack = append(stack, cur)
			if root {
				if t.Name.Local != "svg" {
					return nil, fmt.Errorf("svg: root element is <%s>", t.Name.Local)
				}
				im.setRoot(attrs)
				root = false
				continue
			}
			im.addShape(t.Name.Local, attrs, cur)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root {
		return nil, fmt.Errorf("svg: no root element")
	}
	return im, nil
}

func (im *Image) setRoot(a map[string]string) {
	vb := numbers(a["viewBox"])
	w, wok := parseLen(a["width"])
	h, hok := parseLen(a["height"])
	if len(vb) == 4 && vb[2] > 0 && vb[3] > 0 {
		copy(im.ViewBox[:], vb)
		switch {
		case wok && !hok:
			h = w * vb[3] / vb[2]
		case hok && !wok:
			w = h * vb[2] / vb[3]
		case !wok && !hok:
			w, h = vb[2], vb[3]
		}
	} else {
		if !wok {
			w = 300
		}
		if !hok {
			h = 150
		}
		im.ViewBox = [4]float64{0, 0, w, h}
	}
	im.W, im.H = w, h
}

func parseLen(s string) (float64, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil && f > 0
}

func (im *Image) addShape(name string, a map[string]string, st state) {
	if st.fill == "none" || st.fill == "transparent" {
		return
	}
	c, ok := style.ParseColor(st.fill)
	if !ok {
		c = color.RGBA{0, 0, 0, 255}
	}
	if st.opacity < 1 {
		f := st.opacity
		c = color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), uint8(float64(c.A) * f)}
	}
	num := func(k string) float64 {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(a[k]), "px"), 64)
		return f
	}
	var p []Seg
	switch name {
	case "path":
		p = ParsePath(a["d"])
	case "rect":
		x, y, w, h := num("x"), num("y"), num("width"), num("height")
		if w <= 0 || h <= 0 {
			return
		}
		p = []Seg{
			{Op: 'M', P: [3][2]float64{{x, y}}},
			{Op: 'L', P: [3][2]float64{{x + w, y}}},
			{Op: 'L', P: [3][2]float64{{x + w, y + h}}},
			{Op: 'L', P: [3][2]float64{{x, y + h}}},
			{Op: 'Z'},
		}
	case "circle":
		r := num("r")
		p = ellipse(num("cx"), num("cy"), r, r)
	case "ellipse":
		p = ellipse(num("cx"), num("cy"), num("rx"), num("ry"))
	case "polygon", "polyline":
		pts := numbers(a["points"])
		for i := 0; i+1 < len(pts); i += 2 {
			op := byte('L')
			if i == 0 {
				op = 'M'
			}
			p = append(p, Seg{Op: op, P: [3][2]float64{{pts[i], pts[i+1]}}})
		}
		p = append(p, Seg{Op: 'Z'})
	default:
		return
	}
	if len(p) == 0 {
		return
	}
	if st.m != identity {
		for i := range p {
			for j := range p[i].P {
				p[i].P[j] = st.m.apply(p[i].P[j])
			}
		}
	}
	im.Shapes = append(im.Shapes, Shape{Path: p, Fill: c})
}

func ellipse(cx, cy, rx, ry float64) []Seg {
	if rx <= 0 || ry <= 0 {
		return nil
	}
	const k = 0.5522847498 // cubic Bézier circle approximation
	return []Seg{
		{Op: 'M', P: [3][2]float64{{cx + rx, cy}}},
		{Op: 'C', P: [3][2]float64{{cx + rx, cy + k*ry}, {cx + k*rx, cy + ry}, {cx, cy + ry}}},
		{Op: 'C', P: [3][2]float64{{cx - k*rx, cy + ry}, {cx - rx, cy + k*ry}, {cx - rx, cy}}},
		{Op: 'C', P: [3][2]float64{{cx - rx, cy - k*ry}, {cx - k*rx, cy - ry}, {cx, cy - ry}}},
		{Op: 'C', P: [3][2]float64{{cx + k*rx, cy - ry}, {cx + rx, cy - k*ry}, {cx + rx, cy}}},
		{Op: 'Z'},
	}
}

func parseTransform(s string) matrix {
	m := identity
	for {
		open := strings.IndexByte(s, '(')
		end := strings.IndexByte(s, ')')
		if open < 0 || end < open {
			return m
		}
		name := strings.TrimSpace(strings.Trim(s[:open], ", \t\n"))
		v := numbers(s[open+1 : end])
		s = s[end+1:]
		arg := func(i int, def float64) float64 {
			if i < len(v) {
				return v[i]
			}
			return def
		}
		var t matrix
		switch name {
		case "matrix":
			if len(v) != 6 {
				continue
			}
			copy(t[:], v)
		case "translate":
			t = matrix{1, 0, 0, 1, arg(0, 0), arg(1, 0)}
		case "scale":
			sx := arg(0, 1)
			t = matrix{sx, 0, 0, arg(1, sx), 0, 0}
		case "rotate":
			a := arg(0, 0) * math.Pi / 180
			cx, cy := arg(1, 0), arg(2, 0)
			t = matrix{1, 0, 0, 1, cx, cy}.
				mul(matrix{math.Cos(a), math.Sin(a), -math.Sin(a), math.Cos(a), 0, 0}).
				mul(matrix{1, 0, 0, 1, -cx, -cy})
		default:
			continue
		}
		m = m.mul(t)
	}
}

// Size implements layout.Image.
func (im *Image) Size() (float64, float64) { return im.W, im.H }

// Draw renders the image scaled into r (preserveAspectRatio xMidYMid meet).
func (im *Image) Draw(dst *image.RGBA, r image.Rectangle) {
	if r.Empty() || im.ViewBox[2] <= 0 || im.ViewBox[3] <= 0 || r.Dx()*r.Dy() > maxPixels {
		return
	}
	// Rasterize into a scratch image: vector.Rasterizer doesn't clip to dst.
	tmp := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	sx := float64(r.Dx()) / im.ViewBox[2]
	sy := float64(r.Dy()) / im.ViewBox[3]
	s := math.Min(sx, sy)
	ox := (float64(r.Dx()) - im.ViewBox[2]*s) / 2
	oy := (float64(r.Dy()) - im.ViewBox[3]*s) / 2
	tr := func(p [2]float64) (float32, float32) {
		return float32((p[0]-im.ViewBox[0])*s + ox), float32((p[1]-im.ViewBox[1])*s + oy)
	}
	ras := vector.NewRasterizer(r.Dx(), r.Dy())
	for _, sh := range im.Shapes {
		ras.Reset(r.Dx(), r.Dy())
		ras.DrawOp = draw.Over
		open := false
		for _, sg := range sh.Path {
			switch sg.Op {
			case 'M':
				if open {
					ras.ClosePath()
				}
				ras.MoveTo(tr(sg.P[0]))
				open = true
			case 'L':
				ras.LineTo(tr(sg.P[0]))
			case 'Q':
				ax, ay := tr(sg.P[0])
				bx, by := tr(sg.P[1])
				ras.QuadTo(ax, ay, bx, by)
			case 'C':
				ax, ay := tr(sg.P[0])
				bx, by := tr(sg.P[1])
				cx, cy := tr(sg.P[2])
				ras.CubeTo(ax, ay, bx, by, cx, cy)
			case 'Z':
				if open {
					ras.ClosePath()
					open = false
				}
			}
		}
		if open {
			ras.ClosePath()
		}
		ras.Draw(tmp, tmp.Bounds(), image.NewUniform(sh.Fill), image.Point{})
	}
	draw.Draw(dst, r, tmp, image.Point{}, draw.Over)
}

// maxPixels bounds the size of a single rendered SVG.
const maxPixels = 16 << 20
