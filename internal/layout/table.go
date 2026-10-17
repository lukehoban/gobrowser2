package layout

import (
	"math"

	"github.com/lukehoban/gobrowser2/internal/style"
)

// grid is the cell structure of a table.
type grid struct {
	rows  []*Box   // RowBoxes in order
	cells [][]*Box // cells per row
	start map[*Box]int
	ncols int
}

func buildGrid(t *Box) *grid {
	g := &grid{start: map[*Box]int{}}
	for _, group := range t.Children {
		for _, row := range group.Children {
			col := 0
			for _, cell := range row.Children {
				g.start[cell] = col
				col += cell.ColSpan
			}
			g.rows = append(g.rows, row)
			g.cells = append(g.cells, row.Children)
			g.ncols = max(g.ncols, col)
		}
	}
	return g
}

// columns holds per-column width constraints.
type columns struct {
	min, max, pct []float64
	fixed         []bool
}

// cellWidths returns a cell's min and max border-box widths, folding in a
// specified px width.
func cellWidths(c *Box) (minW, maxW float64, pct float64, fixed bool) {
	minW, maxW = intrinsic(c)
	st := c.Style
	switch st.Width.Unit {
	case style.Px:
		w := st.Width.Value
		if !st.BoxSizingBorder {
			w += c.Padding.Horizontal() + c.Border.Horizontal()
		}
		minW = math.Max(minW, w)
		maxW = math.Max(minW, w)
		fixed = true
	case style.Percent:
		pct = st.Width.Value
	}
	return
}

func computeColumns(g *grid) *columns {
	n := g.ncols
	cols := &columns{min: make([]float64, n), max: make([]float64, n), pct: make([]float64, n), fixed: make([]bool, n)}
	type spanning struct {
		start, span int
		min, max    float64
	}
	var spans []spanning
	for _, row := range g.cells {
		for _, c := range row {
			minW, maxW, pct, fixed := cellWidths(c)
			s := g.start[c]
			if c.ColSpan == 1 {
				cols.min[s] = math.Max(cols.min[s], minW)
				cols.max[s] = math.Max(cols.max[s], maxW)
				cols.pct[s] = math.Max(cols.pct[s], pct)
				cols.fixed[s] = cols.fixed[s] || fixed
			} else {
				spans = append(spans, spanning{s, c.ColSpan, minW, maxW})
			}
		}
	}
	// Distribute spanning cells' excess widths over their columns.
	for _, sp := range spans {
		end := min(sp.start+sp.span, n)
		var curMin, curMax float64
		for i := sp.start; i < end; i++ {
			curMin += cols.min[i]
			curMax += cols.max[i]
		}
		k := float64(end - sp.start)
		if sp.min > curMin {
			for i := sp.start; i < end; i++ {
				if curMax > 0 {
					cols.min[i] += (sp.min - curMin) * cols.max[i] / curMax
				} else {
					cols.min[i] += (sp.min - curMin) / k
				}
			}
		}
		if sp.max > curMax {
			for i := sp.start; i < end; i++ {
				if curMax > 0 {
					cols.max[i] += (sp.max - curMax) * cols.max[i] / curMax
				} else {
					cols.max[i] += (sp.max - curMax) / k
				}
			}
		}
	}
	for i := range cols.max {
		cols.max[i] = math.Max(cols.max[i], cols.min[i])
	}
	return cols
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

// tableExtras returns the horizontal space used by spacing, borders and
// padding outside the columns.
func tableExtras(t *Box, ncols int) float64 {
	return t.Style.BorderSpacing*float64(ncols+1) + t.Padding.Horizontal() + t.Border.Horizontal()
}

// tableIntrinsic returns the min and max margin-box widths of a table.
func tableIntrinsic(t *Box) (minW, maxW float64) {
	g := buildGrid(t)
	cols := computeColumns(g)
	extra := tableExtras(t, g.ncols)
	minW = sum(cols.min) + extra
	maxW = sum(cols.max) + extra
	if t.Style.Width.Unit == style.Px {
		w := t.Style.Width.Value
		if !t.Style.BoxSizingBorder {
			w += t.Padding.Horizontal() + t.Border.Horizontal()
		}
		minW = math.Max(minW, w)
		maxW = minW
	}
	return minW, math.Max(minW, maxW)
}

// tableWidth computes the content width of a table in a containing block
// of width cbW (resolveEdges must have been called).
func tableWidth(t *Box, cbW float64) float64 {
	g := buildGrid(t)
	cols := computeColumns(g)
	pb := t.Padding.Horizontal() + t.Border.Horizontal()
	extra := tableExtras(t, g.ncols)
	minW := sum(cols.min) + extra
	maxW := sum(cols.max) + extra
	avail := cbW - t.Margin.Horizontal()
	st := t.Style
	var w float64 // border-box width
	if st.Width.IsAuto() {
		w = math.Min(avail, maxW)
	} else {
		w = st.Width.Resolve(cbW, 0)
		if !st.BoxSizingBorder {
			w += pb
		}
	}
	if !st.MinWidth.IsAuto() {
		mw := st.MinWidth.Resolve(cbW, 0)
		if !st.BoxSizingBorder {
			mw += pb
		}
		w = math.Max(w, mw)
	}
	if !st.MaxWidth.IsAuto() {
		mw := st.MaxWidth.Resolve(cbW, 0)
		if !st.BoxSizingBorder {
			mw += pb
		}
		w = math.Min(w, mw)
	}
	w = math.Max(w, minW)
	return math.Max(0, w-pb)
}

// distribute assigns column widths summing to avail.
func distribute(cols *columns, avail float64) []float64 {
	n := len(cols.min)
	widths := make([]float64, n)
	sumMin := sum(cols.min)
	if avail <= sumMin || n == 0 {
		copy(widths, cols.min)
		return widths
	}
	// Target widths: percentage columns get their share, others their max.
	target := make([]float64, n)
	for i := range target {
		target[i] = cols.max[i]
		if cols.pct[i] > 0 {
			target[i] = math.Max(cols.min[i], cols.pct[i]/100*avail)
		}
	}
	sumTarget := sum(target)
	if avail <= sumTarget {
		// Interpolate between min and target widths.
		f := 0.0
		if sumTarget > sumMin {
			f = (avail - sumMin) / (sumTarget - sumMin)
		}
		for i := range widths {
			widths[i] = cols.min[i] + f*(target[i]-cols.min[i])
		}
		return widths
	}
	copy(widths, target)
	extra := avail - sumTarget
	// Prefer growing auto columns, then fixed columns, then percentage ones.
	groups := []func(i int) bool{
		func(i int) bool { return !cols.fixed[i] && cols.pct[i] == 0 },
		func(i int) bool { return cols.fixed[i] && cols.pct[i] == 0 },
		func(i int) bool { return cols.pct[i] > 0 },
	}
	for _, in := range groups {
		var idx []int
		total := 0.0
		for i := 0; i < n; i++ {
			if in(i) {
				idx = append(idx, i)
				total += target[i]
			}
		}
		if len(idx) == 0 {
			continue
		}
		for _, i := range idx {
			if total > 0 {
				widths[i] += extra * target[i] / total
			} else {
				widths[i] += extra / float64(len(idx))
			}
		}
		break
	}
	return widths
}

// layoutTable lays out the rows and cells of a table whose X, Y and W are
// set, and sets its height.
func layoutTable(t *Box) {
	g := buildGrid(t)
	cols := computeColumns(g)
	sp := t.Style.BorderSpacing
	widths := distribute(cols, t.W-sp*float64(g.ncols+1))
	colX := make([]float64, g.ncols+1)
	x := t.X + sp
	for i := 0; i < g.ncols; i++ {
		colX[i] = x
		x += widths[i] + sp
	}
	colX[g.ncols] = x
	y := t.Y + sp
	rowIdx := 0
	for _, group := range t.Children {
		resolveEdges(group, t.W)
		group.X, group.Y, group.W = t.X+sp, y, t.W-2*sp
		for _, row := range group.Children {
			resolveEdges(row, t.W)
			row.X, row.Y, row.W = t.X+sp, y, t.W-2*sp
			rowH := 0.0
			if row.Style.Height.Unit == style.Px {
				rowH = row.Style.Height.Value
			}
			for _, cell := range g.cells[rowIdx] {
				s := g.start[cell]
				e := min(s+cell.ColSpan, g.ncols)
				cw := colX[e] - sp - colX[s]
				resolveEdges(cell, t.W)
				cell.W = math.Max(0, cw-cell.Padding.Horizontal()-cell.Border.Horizontal())
				cell.X = colX[s] + cell.Border.Left + cell.Padding.Left
				cell.Y = y + cell.Border.Top + cell.Padding.Top
				layoutBlockContents(cell)
				_, _, _, bh := cell.BorderBox()
				rowH = math.Max(rowH, bh)
			}
			for _, cell := range g.cells[rowIdx] {
				_, _, _, bh := cell.BorderBox()
				extra := rowH - bh
				off := 0.0
				switch cell.Style.VerticalAlign {
				case "middle":
					off = extra / 2
				case "bottom":
					off = extra
				}
				translate(cell, 0, off)
				cell.Y -= off
				cell.H += extra
			}
			row.H = rowH
			y += rowH + sp
			rowIdx++
		}
		group.H = math.Max(0, y-sp-group.Y)
	}
	t.H = usedHeight(t, y-t.Y)
}
