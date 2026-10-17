package layout

import (
	"math"

	"github.com/lukehoban/gobrowser2/internal/style"
)

// cellPos is a cell placed in the table grid.
type cellPos struct {
	cell *Box
	col  int
}

// grid assigns cells to columns. Row spans are not supported.
func grid(t *Box) (rows [][]cellPos, ncols int) {
	for _, r := range t.Children {
		var cells []cellPos
		col := 0
		for _, c := range r.Children {
			cells = append(cells, cellPos{c, col})
			col += c.ColSpan
		}
		ncols = max(ncols, col)
		rows = append(rows, cells)
	}
	return rows, ncols
}

// columns computes per-column min/max-content widths following the CSS 2.1
// automatic table layout algorithm. fixed reports columns whose width came
// from a pixel width on a cell.
func (c *ctx) columns(t *Box) (mins, maxs []float64, fixed []bool) {
	rows, n := grid(t)
	mins, maxs, fixed = make([]float64, n), make([]float64, n), make([]bool, n)
	spacing := t.Style.BorderSpacing
	// Single-column cells first, then spanning cells.
	for _, r := range rows {
		for _, cp := range r {
			if cp.cell.ColSpan != 1 {
				continue
			}
			mn, mx := c.outerMinMax(cp.cell)
			mins[cp.col] = math.Max(mins[cp.col], mn)
			maxs[cp.col] = math.Max(maxs[cp.col], mx)
			if cp.cell.Style.Width.Unit == style.Px {
				fixed[cp.col] = true
			}
		}
	}
	for _, r := range rows {
		for _, cp := range r {
			span := cp.cell.ColSpan
			if span == 1 {
				continue
			}
			mn, mx := c.outerMinMax(cp.cell)
			spread(mins[cp.col:cp.col+span], maxs[cp.col:cp.col+span], mn-spacing*float64(span-1))
			spread(maxs[cp.col:cp.col+span], maxs[cp.col:cp.col+span], mx-spacing*float64(span-1))
		}
	}
	for i := range maxs {
		maxs[i] = math.Max(maxs[i], mins[i])
	}
	return mins, maxs, fixed
}

// spread grows cols so that they sum to at least want, distributing the
// excess in proportion to weights (or equally if they are all zero).
func spread(cols, weights []float64, want float64) {
	have, wsum := 0.0, 0.0
	for i := range cols {
		have += cols[i]
		wsum += weights[i]
	}
	extra := want - have
	if extra <= 0 {
		return
	}
	for i := range cols {
		if wsum > 0 {
			cols[i] += extra * weights[i] / wsum
		} else {
			cols[i] += extra / float64(len(cols))
		}
	}
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func spacingTotal(t *Box, ncols int) float64 {
	if ncols == 0 {
		return 0
	}
	return t.Style.BorderSpacing * float64(ncols+1)
}

// tableMinMax returns the min/max-content widths of a table's content box.
func (c *ctx) tableMinMax(t *Box) (float64, float64) {
	mins, maxs, _ := c.columns(t)
	st := spacingTotal(t, len(mins))
	return sum(mins) + st, sum(maxs) + st
}

// distribute assigns used column widths for the available width.
func distribute(mins, maxs []float64, fixed []bool, avail float64) []float64 {
	n := len(mins)
	w := make([]float64, n)
	sumMin, sumMax := sum(mins), sum(maxs)
	switch {
	case avail <= sumMin:
		copy(w, mins)
	case avail <= sumMax:
		f := (avail - sumMin) / (sumMax - sumMin)
		for i := range w {
			w[i] = mins[i] + (maxs[i]-mins[i])*f
		}
	default:
		copy(w, maxs)
		extra := avail - sumMax
		// Prefer growing auto columns; fall back to all columns.
		grow := make([]bool, n)
		any := false
		for i := range grow {
			grow[i] = !fixed[i]
			any = any || grow[i]
		}
		if !any {
			for i := range grow {
				grow[i] = true
			}
		}
		total, count := 0.0, 0
		for i := range w {
			if grow[i] {
				total += maxs[i]
				count++
			}
		}
		for i := range w {
			if !grow[i] {
				continue
			}
			if total > 0 {
				w[i] += extra * maxs[i] / total
			} else {
				w[i] += extra / float64(count)
			}
		}
	}
	return w
}

// layoutTable lays out a table whose margin box top is at y.
func (c *ctx) layoutTable(t *Box, cbX, cbW, y float64, parentAlign string) {
	resolveEdges(t, cbW)
	s := t.Style
	mins, maxs, fixed := c.columns(t)
	n := len(mins)
	spacing := s.BorderSpacing
	st := spacingTotal(t, n)
	pb := t.pbH()
	minOuter := sum(mins) + st + pb

	var W float64 // border-box width
	if !s.Width.IsAuto() {
		W = math.Max(s.Width.Resolve(cbW, 0), minOuter)
	} else {
		avail := cbW - t.Margin[style.Left] - t.Margin[style.Right]
		W = math.Max(minOuter, math.Min(avail, sum(maxs)+st+pb))
	}
	if !s.MinWidth.IsAuto() {
		W = math.Max(W, s.MinWidth.Resolve(cbW, 0))
	}
	widths := distribute(mins, maxs, fixed, W-pb-st)

	horizontalPlacement(t, cbX, cbW, W-pb, false, parentAlign)
	t.Y = y + t.Margin[style.Top] + t.Border[style.Top] + t.Padding[style.Top]

	colX := make([]float64, n+1)
	colX[0] = t.X + spacing
	for i := 0; i < n; i++ {
		colX[i+1] = colX[i] + widths[i] + spacing
	}

	rowY := t.Y
	if len(t.Children) > 0 {
		rowY += spacing
	}
	for _, r := range t.Children {
		resolveEdges(r, t.W)
		r.X, r.Y, r.W = t.X+spacing, rowY, math.Max(0, t.W-2*spacing)
		rowH := 0.0
		if r.Style.Height.Unit == style.Px {
			rowH = r.Style.Height.V
		}
		col := 0
		for _, cell := range r.Children {
			span := cell.ColSpan
			end := min(col+span, n)
			cw := colX[end] - colX[col] - spacing
			resolveEdges(cell, t.W)
			cell.X = colX[col] + cell.Border[style.Left] + cell.Padding[style.Left]
			cell.W = math.Max(0, cw-cell.pbH())
			cell.Y = rowY + cell.Border[style.Top] + cell.Padding[style.Top]
			cell.H = c.usedHeight(cell, c.layoutContents(cell))
			rowH = math.Max(rowH, cell.H+cell.pbV())
			col += span
		}
		for _, cell := range r.Children {
			contentH := cell.H
			extra := rowH - (cell.H + cell.pbV())
			dy := 0.0
			switch cell.Style.VerticalAlign {
			case "middle":
				dy = extra / 2
			case "bottom":
				dy = extra
			}
			translate(cell, 0, dy)
			cell.Y -= dy
			cell.H = contentH + extra
		}
		r.H = rowH
		rowY += rowH + spacing
	}
	t.H = c.usedHeight(t, rowY-t.Y)
}
