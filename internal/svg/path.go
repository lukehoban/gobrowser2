package svg

import (
	"math"
	"strconv"
)

// numbers parses a list of numbers separated by whitespace and/or commas.
func numbers(s string) []float64 {
	sc := scanner{s: s}
	var out []float64
	for {
		f, ok := sc.number()
		if !ok {
			return out
		}
		out = append(out, f)
	}
}

type scanner struct {
	s string
	i int
}

func (sc *scanner) skipSep() {
	for sc.i < len(sc.s) {
		switch sc.s[sc.i] {
		case ' ', '\t', '\n', '\r', ',':
			sc.i++
		default:
			return
		}
	}
}

// number scans a number such as "-1.5e3", ".5" or the "1.5" and ".5" in "1.5.5".
func (sc *scanner) number() (float64, bool) {
	sc.skipSep()
	start := sc.i
	i := sc.i
	if i < len(sc.s) && (sc.s[i] == '+' || sc.s[i] == '-') {
		i++
	}
	digits, dot := false, false
	for i < len(sc.s) {
		c := sc.s[i]
		switch {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' && !dot:
			dot = true
		default:
			goto exp
		}
		i++
	}
exp:
	if !digits {
		return 0, false
	}
	if i < len(sc.s) && (sc.s[i] == 'e' || sc.s[i] == 'E') {
		j := i + 1
		if j < len(sc.s) && (sc.s[j] == '+' || sc.s[j] == '-') {
			j++
		}
		if j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
			for j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
				j++
			}
			i = j
		}
	}
	f, err := strconv.ParseFloat(sc.s[start:i], 64)
	if err != nil {
		return 0, false
	}
	sc.i = i
	return f, true
}

// flag scans an arc flag, which may be written without separators.
func (sc *scanner) flag() (bool, bool) {
	sc.skipSep()
	if sc.i < len(sc.s) && (sc.s[sc.i] == '0' || sc.s[sc.i] == '1') {
		sc.i++
		return sc.s[sc.i-1] == '1', true
	}
	return false, false
}

func isCommand(c byte) bool {
	switch c | 0x20 {
	case 'm', 'l', 'h', 'v', 'c', 's', 'q', 't', 'a', 'z':
		return true
	}
	return false
}

// ParsePath parses SVG path data into absolute segments. Smooth curves are
// expanded and elliptical arcs are converted to cubic Béziers.
func ParsePath(d string) []Seg {
	sc := scanner{s: d}
	var out []Seg
	var cur, start, lastCtrl [2]float64
	var prevOp byte
	var cmd byte
	for {
		sc.skipSep()
		if sc.i >= len(sc.s) {
			return out
		}
		if c := sc.s[sc.i]; isCommand(c) {
			cmd = c
			sc.i++
		} else if cmd == 0 {
			return out
		}
		rel := cmd >= 'a'
		abs := func(x, y float64) [2]float64 {
			if rel {
				return [2]float64{cur[0] + x, cur[1] + y}
			}
			return [2]float64{x, y}
		}
		nums := func(n int) ([]float64, bool) {
			v := make([]float64, n)
			for i := range v {
				f, ok := sc.number()
				if !ok {
					return nil, false
				}
				v[i] = f
			}
			return v, true
		}
		upper := cmd &^ 0x20
		switch upper {
		case 'Z':
			out = append(out, Seg{Op: 'Z'})
			cur = start
			prevOp = 'Z'
			continue
		case 'M', 'L', 'T':
			v, ok := nums(2)
			if !ok {
				return out
			}
			p := abs(v[0], v[1])
			switch upper {
			case 'M':
				out = append(out, Seg{Op: 'M', P: [3][2]float64{p}})
				start = p
				// Subsequent pairs are implicit lineto commands.
				if rel {
					cmd = 'l'
				} else {
					cmd = 'L'
				}
			case 'L':
				out = append(out, Seg{Op: 'L', P: [3][2]float64{p}})
			case 'T':
				c := cur
				if prevOp == 'Q' {
					c = reflect(lastCtrl, cur)
				}
				out = append(out, Seg{Op: 'Q', P: [3][2]float64{c, p}})
				lastCtrl = c
			}
			cur = p
		case 'H', 'V':
			v, ok := nums(1)
			if !ok {
				return out
			}
			p := cur
			i := 0
			if upper == 'V' {
				i = 1
			}
			if rel {
				p[i] += v[0]
			} else {
				p[i] = v[0]
			}
			out = append(out, Seg{Op: 'L', P: [3][2]float64{p}})
			cur = p
		case 'C':
			v, ok := nums(6)
			if !ok {
				return out
			}
			c1, c2, p := abs(v[0], v[1]), abs(v[2], v[3]), abs(v[4], v[5])
			out = append(out, Seg{Op: 'C', P: [3][2]float64{c1, c2, p}})
			lastCtrl, cur = c2, p
		case 'S':
			v, ok := nums(4)
			if !ok {
				return out
			}
			c1 := cur
			if prevOp == 'C' {
				c1 = reflect(lastCtrl, cur)
			}
			c2, p := abs(v[0], v[1]), abs(v[2], v[3])
			out = append(out, Seg{Op: 'C', P: [3][2]float64{c1, c2, p}})
			lastCtrl, cur = c2, p
			upper = 'C'
		case 'Q':
			v, ok := nums(4)
			if !ok {
				return out
			}
			c, p := abs(v[0], v[1]), abs(v[2], v[3])
			out = append(out, Seg{Op: 'Q', P: [3][2]float64{c, p}})
			lastCtrl, cur = c, p
		case 'A':
			v, ok := nums(3)
			if !ok {
				return out
			}
			large, ok1 := sc.flag()
			sweep, ok2 := sc.flag()
			e, ok3 := nums(2)
			if !ok1 || !ok2 || !ok3 {
				return out
			}
			p := abs(e[0], e[1])
			out = append(out, arcToCubics(cur, p, v[0], v[1], v[2], large, sweep)...)
			cur = p
		}
		if upper == 'T' {
			upper = 'Q'
		}
		prevOp = upper
	}
}

func reflect(ctrl, about [2]float64) [2]float64 {
	return [2]float64{2*about[0] - ctrl[0], 2*about[1] - ctrl[1]}
}

// arcToCubics converts an SVG elliptical arc to cubic Bézier segments
// (SVG 1.1 implementation notes, F.6.5).
func arcToCubics(p0, p1 [2]float64, rx, ry, phiDeg float64, large, sweep bool) []Seg {
	if p0 == p1 {
		return nil
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		return []Seg{{Op: 'L', P: [3][2]float64{p1}}}
	}
	phi := phiDeg * math.Pi / 180
	cos, sin := math.Cos(phi), math.Sin(phi)
	dx, dy := (p0[0]-p1[0])/2, (p0[1]-p1[1])/2
	x1 := cos*dx + sin*dy
	y1 := -sin*dx + cos*dy
	if l := x1*x1/(rx*rx) + y1*y1/(ry*ry); l > 1 {
		s := math.Sqrt(l)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*y1*y1 - ry*ry*x1*x1
	den := rx*rx*y1*y1 + ry*ry*x1*x1
	co := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		co = -co
	}
	cxp, cyp := co*rx*y1/ry, -co*ry*x1/rx
	cx := cos*cxp - sin*cyp + (p0[0]+p1[0])/2
	cy := sin*cxp + cos*cyp + (p0[1]+p1[1])/2
	angle := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	t1 := angle(1, 0, (x1-cxp)/rx, (y1-cyp)/ry)
	dt := angle((x1-cxp)/rx, (y1-cyp)/ry, (-x1-cxp)/rx, (-y1-cyp)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 2)))
	step := dt / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	pt := func(t float64) [2]float64 {
		x, y := rx*math.Cos(t), ry*math.Sin(t)
		return [2]float64{cos*x - sin*y + cx, sin*x + cos*y + cy}
	}
	deriv := func(t float64) [2]float64 {
		x, y := -rx*math.Sin(t), ry*math.Cos(t)
		return [2]float64{cos*x - sin*y, sin*x + cos*y}
	}
	var out []Seg
	t := t1
	for i := 0; i < n; i++ {
		a, b := pt(t), pt(t+step)
		da, db := deriv(t), deriv(t+step)
		c1 := [2]float64{a[0] + k*da[0], a[1] + k*da[1]}
		c2 := [2]float64{b[0] - k*db[0], b[1] - k*db[1]}
		if i == n-1 {
			b = p1
		}
		out = append(out, Seg{Op: 'C', P: [3][2]float64{c1, c2, b}})
		t += step
	}
	return out
}
