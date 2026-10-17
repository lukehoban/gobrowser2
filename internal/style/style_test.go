package style

import (
	"image/color"
	"strings"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
)

func compute(t *testing.T, html, sheet string) (*dom.Node, map[*dom.Node]*Style) {
	t.Helper()
	doc, err := dom.Parse(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc, Compute(doc, []*css.Stylesheet{css.Parse(sheet, nil)}, nil)
}

func TestCascade(t *testing.T) {
	doc, st := compute(t, `<div id="x" class="c" style="color: blue">t</div><p class="c">p</p>`,
		`div { color: red } .c { color: green !important } #x { color: yellow } p { margin: 1em 2px }`)
	if got := st[doc.Find("div")].Color; got != (color.RGBA{0, 128, 0, 255}) {
		t.Errorf("!important author rule should beat inline style, got %v", got)
	}
	p := st[doc.Find("p")]
	if p.Margin[Top] != PxLen(16) || p.Margin[Right] != PxLen(2) {
		t.Errorf("margin = %v", p.Margin)
	}
}

func TestInheritanceAndUnits(t *testing.T) {
	doc, st := compute(t, `<!DOCTYPE html><body><span class="s"><b>x</b></span></body>`,
		`body { font-size: 10pt; color: #828282 } .s { font-size: 2em; text-decoration: underline }`)
	b := st[doc.Find("b")]
	if b.FontSize < 26.6 || b.FontSize > 26.7 {
		t.Errorf("font-size = %v, want 26.67", b.FontSize)
	}
	if !b.Bold || !b.Underline {
		t.Errorf("bold=%v underline=%v", b.Bold, b.Underline)
	}
	if b.Color != (color.RGBA{0x82, 0x82, 0x82, 255}) {
		t.Errorf("color = %v", b.Color)
	}
}

func TestPresentationalHints(t *testing.T) {
	doc, st := compute(t, `<table width="85%" bgcolor="#f6f6ef" cellpadding="3" cellspacing="0"><tr><td align="right" valign="top" width="18">x</td></tr></table>`, ``)
	tbl := st[doc.Find("table")]
	if tbl.Width != (Length{85, Percent}) || tbl.BackgroundColor != (color.RGBA{0xf6, 0xf6, 0xef, 255}) || tbl.BorderSpacing != 0 {
		t.Errorf("table style: width=%v bg=%v spacing=%v", tbl.Width, tbl.BackgroundColor, tbl.BorderSpacing)
	}
	td := st[doc.Find("td")]
	if td.Padding[Left] != PxLen(3) || td.TextAlign != "-webkit-right" || td.VerticalAlign != "top" || td.Width != PxLen(18) {
		t.Errorf("td style: padding=%v align=%v valign=%v width=%v", td.Padding, td.TextAlign, td.VerticalAlign, td.Width)
	}
}

func TestQuirksTableReset(t *testing.T) {
	html := `<center><table><tr><td>x</td></tr></table></center>`
	doc, st := compute(t, html, `center { font-size: 20px }`)
	if got := st[doc.Find("td")].TextAlign; got != "left" {
		t.Errorf("quirks mode: td text-align = %q, want left", got)
	}
	if got := st[doc.Find("td")].FontSize; got != 16 {
		t.Errorf("quirks mode: td font-size = %v, want 16 (tables don't inherit font-size)", got)
	}
	doc, st = compute(t, "<!DOCTYPE html>"+html, ``)
	if got := st[doc.Find("td")].TextAlign; got != "-webkit-center" {
		t.Errorf("standards mode: td text-align = %q, want -webkit-center", got)
	}
}

func TestShorthands(t *testing.T) {
	doc, st := compute(t, `<div>x</div>`,
		`div { border: 1px white solid; margin: 3px 2px 6px; background: url("a.svg"), linear-gradient(red, red) no-repeat; font: bold 12px/14px monospace }`)
	d := st[doc.Find("div")]
	if d.BorderWidth[Top] != 1 || d.BorderStyle[Left] != "solid" || d.BorderColor[Bottom] != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("border: %v %v %v", d.BorderWidth, d.BorderStyle, d.BorderColor)
	}
	if d.Margin[Top] != PxLen(3) || d.Margin[Left] != PxLen(2) || d.Margin[Bottom] != PxLen(6) {
		t.Errorf("margin: %v", d.Margin)
	}
	if d.BackgroundImage != "a.svg" || !d.BackgroundNoRepeat {
		t.Errorf("background: %q norepeat=%v", d.BackgroundImage, d.BackgroundNoRepeat)
	}
	if !d.Bold || !d.Mono || d.FontSize != 12 || d.LineHeight != 14 {
		t.Errorf("font: bold=%v mono=%v size=%v lh=%v", d.Bold, d.Mono, d.FontSize, d.LineHeight)
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]color.RGBA{
		"#f60":              {255, 102, 0, 255},
		"#ff6600":           {255, 102, 0, 255},
		"ff6600":            {255, 102, 0, 255},
		"rgb(1, 2, 3)":      {1, 2, 3, 255},
		"rgba(255,0,0,0.5)": {128, 0, 0, 128},
		"White":             {255, 255, 255, 255},
	}
	for in, want := range cases {
		if got, ok := ParseColor(in); !ok || got != want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseColor("linear-gradient(red, red)"); ok {
		t.Error("gradient should not parse as a color")
	}
}
