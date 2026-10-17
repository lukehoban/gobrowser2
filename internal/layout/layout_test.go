package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
	"github.com/lukehoban/gobrowser2/internal/style"
)

func lay(t *testing.T, html, sheet string, width float64) (*dom.Node, map[*dom.Node]*Box, *Result) {
	t.Helper()
	doc, err := dom.Parse(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	st := style.Compute(doc, []*css.Stylesheet{css.Parse(sheet, nil)}, nil)
	res := Layout(doc, st, Options{Width: width, Quirks: doc.Quirks})
	boxes := map[*dom.Node]*Box{}
	var walk func(b *Box)
	walk = func(b *Box) {
		if b.Node != nil {
			boxes[b.Node] = b
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(res.Root)
	return doc, boxes, res
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestBlockLayout(t *testing.T) {
	doc, boxes, _ := lay(t, `<!DOCTYPE html><body><div id="a"></div><div id="b"></div><div id="c"></div></body>`,
		`body { margin: 8px } #a { height: 10px; margin-bottom: 20px } #b { width: 100px; height: 5px; margin: 10px auto } #c { width: 50%; padding: 2px; border: 1px solid; height: 4px }`, 400)
	byID := func(id string) *Box {
		var n *dom.Node
		doc.Walk(func(x *dom.Node) {
			if x.AttrOr("id", "") == id {
				n = x
			}
		})
		return boxes[n]
	}
	a, b, c := byID("a"), byID("b"), byID("c")
	if !near(a.X, 8) || !near(a.W, 384) || !near(a.Y, 8) {
		t.Errorf("a: %+v", [4]float64{a.X, a.Y, a.W, a.H})
	}
	if !near(b.Y, 8+10+20) {
		t.Errorf("b.Y = %v, want 38 (margins collapse to 20)", b.Y)
	}
	if !near(b.X, 8+(384-100)/2) {
		t.Errorf("b.X = %v, want centered", b.X)
	}
	if !near(c.W, 192) || !near(c.X, 8+3) || !near(c.Y, 38+5+10+3) {
		t.Errorf("c: x=%v y=%v w=%v", c.X, c.Y, c.W)
	}
}

func TestLineBreaking(t *testing.T) {
	doc, boxes, _ := lay(t, `<!DOCTYPE html><div>aaa bbb ccc ddd eee fff</div><p style="white-space:nowrap">aaa bbb ccc ddd eee fff</p>`,
		`body { margin: 0 } div, p { width: 60px; font-size: 10px; margin: 0 }`, 400)
	div := boxes[doc.Find("div")]
	if len(div.Lines) < 2 {
		t.Fatalf("got %d lines, want wrapping", len(div.Lines))
	}
	for _, l := range div.Lines {
		w := 0.0
		for _, f := range l.Frags {
			w = math.Max(w, f.X+f.W-div.X)
		}
		if w > 60.01 {
			t.Errorf("line overflows: %v", w)
		}
		if strings.HasPrefix(l.Frags[0].Text, " ") {
			t.Errorf("line starts with a space: %q", l.Frags[0].Text)
		}
	}
	if p := boxes[doc.Find("p")]; len(p.Lines) != 1 {
		t.Errorf("nowrap: got %d lines, want 1", len(p.Lines))
	}
}

func TestTextAlign(t *testing.T) {
	doc, boxes, _ := lay(t, `<!DOCTYPE html><div style="text-align:right">x</div><center><div style="width:100px;height:1px"></div></center>`,
		`body { margin: 0 }`, 400)
	div := boxes[doc.Find("div")]
	f := div.Lines[0].Frags[0]
	if !near(f.X+f.W, 400) {
		t.Errorf("right-aligned text ends at %v", f.X+f.W)
	}
	inner := boxes[doc.Find("center").Children[0]]
	if !near(inner.X, 150) {
		t.Errorf("<center> child x = %v, want 150", inner.X)
	}
}

func TestTableLayout(t *testing.T) {
	html := `<table id="t" width="400" cellspacing="0" cellpadding="0"><tr><td style="width:18px">x</td><td>a b c d e f g</td><td>z</td></tr><tr><td colspan="3">span</td></tr></table>`
	doc, boxes, _ := lay(t, html, `body { margin: 0 }`, 800)
	tbl := boxes[doc.Find("table")]
	if !near(tbl.W, 400) {
		t.Errorf("table width = %v", tbl.W)
	}
	tds := doc.FindAll("td")
	c0, c1, c2, span := boxes[tds[0]], boxes[tds[1]], boxes[tds[2]], boxes[tds[3]]
	if !near(c0.W, 18) {
		t.Errorf("fixed column = %v, want 18", c0.W)
	}
	if !near(c0.W+c1.W+c2.W, 400) {
		t.Errorf("columns sum to %v", c0.W+c1.W+c2.W)
	}
	if c1.W <= c2.W {
		t.Errorf("wider content should get more space: %v vs %v", c1.W, c2.W)
	}
	if !near(span.W, 400) || span.Y <= c0.Y {
		t.Errorf("spanning cell: w=%v y=%v", span.W, span.Y)
	}
}

func TestTableShrinkToFitAndVerticalAlign(t *testing.T) {
	html := `<table cellspacing="2" cellpadding="1"><tr><td valign="top">a</td><td><div style="height:50px"></div></td><td>m</td></tr><tr style="height:10px"></tr></table>`
	doc, boxes, _ := lay(t, html, `body { margin: 0 } td { font-size: 10px }`, 800)
	tbl := boxes[doc.Find("table")]
	if tbl.W > 100 {
		t.Errorf("auto table should shrink to fit, w=%v", tbl.W)
	}
	tds := doc.FindAll("td")
	top, mid := boxes[tds[0]], boxes[tds[2]]
	if len(top.Lines) == 0 || len(mid.Lines) == 0 {
		t.Fatal("cells have no lines")
	}
	if !(top.Lines[0].Y < mid.Lines[0].Y) {
		t.Errorf("valign=top line at %v should be above middle-aligned line at %v", top.Lines[0].Y, mid.Lines[0].Y)
	}
	rows := doc.FindAll("tr")
	if r := boxes[rows[1]]; !near(r.H, 10) {
		t.Errorf("empty row height = %v, want 10", r.H)
	}
}
