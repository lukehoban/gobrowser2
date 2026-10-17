package style

import (
	"image/color"
	"math"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/html"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestCompute(t *testing.T) {
	doc := html.Parse(`<body><center><table id="hnmain" width="85%" bgcolor="#f6f6ef" cellpadding="3">
<tr><td bgcolor="ff6600" align="right" valign="top" class="title"><span class="pagetop"><b>HN</b> <a href="news">new</a></span></td></tr>
<tr class="spacer" style="height:5px; color: red !important"><td class="subtext"><a href="x">x</a></td></tr>
</table></center><p style="font-size: 2em; margin: 1em auto">para</p>`)
	sheet := css.Parse(`
body { font-family: Verdana; font-size: 10pt; color: #828282; }
td { font-size: 10pt; }
.pagetop { color: #222222; line-height: 12px; }
a:link { color: #000000; text-decoration: none; }
.subtext a:link { color: #828282; }
#hnmain { min-width: 796px; }
tr.spacer { color: blue; }
@media print { body { color: red; } }
`)
	st := Compute(doc, []*css.Stylesheet{sheet}, 1024)

	body := st[doc.Find("body")]
	if body.Display != DisplayBlock || body.Margin[0].Value != 8 || !near(body.FontSize, 13.333) {
		t.Errorf("body style = %+v", body)
	}
	if body.Color != (color.NRGBA{0x82, 0x82, 0x82, 255}) {
		t.Errorf("body color = %v (print media should not apply)", body.Color)
	}
	if c := st[doc.Find("center")]; c.TextAlign != "-webkit-center" {
		t.Errorf("center text-align = %q", c.TextAlign)
	}
	table := st[doc.Find("table")]
	if table.Display != DisplayTable || table.Width != (Length{85, Percent}) || table.MinWidth != (Length{796, Px}) {
		t.Errorf("table style = %+v", table)
	}
	if table.BackgroundColor != (color.NRGBA{0xf6, 0xf6, 0xef, 255}) {
		t.Errorf("table bg = %v", table.BackgroundColor)
	}
	tds := doc.FindAll("td")
	td := st[tds[0]]
	if td.BackgroundColor != (color.NRGBA{0xff, 0x66, 0x00, 255}) || td.TextAlign != "-webkit-center" && td.TextAlign != "right" {
		t.Errorf("td style = %+v", td)
	}
	if td.TextAlign != "right" || td.VerticalAlign != "top" || td.Padding[0].Value != 3 {
		t.Errorf("td align/valign/padding = %q %q %v", td.TextAlign, td.VerticalAlign, td.Padding)
	}
	if td2 := st[tds[1]]; td2.VerticalAlign != "middle" {
		t.Errorf("td2 vertical-align = %q, want inherited middle", td2.VerticalAlign)
	}
	pagetop := st[doc.Find("span")]
	if pagetop.Color != (color.NRGBA{0x22, 0x22, 0x22, 255}) || pagetop.LineHeight != 12 {
		t.Errorf("pagetop = %+v", pagetop)
	}
	if b := st[doc.Find("b")]; !b.Bold {
		t.Errorf("b not bold")
	}
	as := doc.FindAll("a")
	if a := st[as[0]]; a.Color != (color.NRGBA{0, 0, 0, 255}) || a.Underline {
		t.Errorf("a:link = color %v underline %v", a.Color, a.Underline)
	}
	if a := st[as[1]]; a.Color != (color.NRGBA{0x82, 0x82, 0x82, 255}) {
		t.Errorf(".subtext a:link color = %v", a.Color)
	}
	spacer := st[doc.FindAll("tr")[1]]
	if spacer.Height != (Length{5, Px}) || spacer.Color != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("spacer = height %v color %v", spacer.Height, spacer.Color)
	}
	p := st[doc.Find("p")]
	if !near(p.FontSize, 80.0/3) || !near(p.Margin[0].Value, 80.0/3) || !p.Margin[1].IsAuto() {
		t.Errorf("p = size %v margin %v", p.FontSize, p.Margin)
	}
	if head := st[doc.Find("head")]; head.Display != DisplayNone {
		t.Errorf("head display = %v", head.Display)
	}
}

func TestParseLength(t *testing.T) {
	cases := map[string]Length{
		"10px": {10, Px}, "10pt": {40.0 / 3, Px}, "2em": {20, Px}, "50%": {50, Percent},
		"auto": {0, Auto}, "0": {0, Px}, "1.5rem": {24, Px},
	}
	for in, want := range cases {
		got, ok := parseLength(in, 10, nil)
		if !ok || !near(got.Value, want.Value) || got.Unit != want.Unit {
			t.Errorf("parseLength(%q) = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"10", "px", "abc", "1e999px"} {
		if _, ok := parseLength(bad, 10, nil); ok {
			t.Errorf("parseLength(%q) succeeded", bad)
		}
	}
}

func TestLegacyAttributes(t *testing.T) {
	if v, ok := legacyDimension("85%"); !ok || v != "85%" {
		t.Errorf("legacyDimension(85%%) = %q", v)
	}
	if v, ok := legacyDimension("18"); !ok || v != "18px" {
		t.Errorf("legacyDimension(18) = %q", v)
	}
	if v, ok := legacyColor("ff6600"); !ok || v != "#ff6600" {
		t.Errorf("legacyColor = %q", v)
	}
	if v, ok := legacyFontSize("+1"); !ok || v != "large" {
		t.Errorf("legacyFontSize(+1) = %q", v)
	}
}
