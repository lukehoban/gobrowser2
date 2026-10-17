package browser

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/fetch"
)

func TestLoadFixture(t *testing.T) {
	p, err := Load("../../testdata/hn-like.html", Options{Width: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", p.Warnings)
	}
	if p.Height < 300 || p.Height > 400 {
		t.Errorf("page height = %v, want ~350", p.Height)
	}
	img := p.Render()
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() < 300 {
		t.Fatalf("image bounds = %v", b)
	}
	orange := color.RGBA{0xff, 0x66, 0x00, 0xff}
	beige := color.RGBA{0xf6, 0xf6, 0xef, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	checks := []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"page margin", 40, 20, white},
		{"header bar", 500, 12, orange},
		{"main table", 500, 50, beige},
		{"logo", 92, 20, orange},
	}
	for _, c := range checks {
		if got := img.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("%s: pixel (%d,%d) = %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
	// The table is 85% wide and centered.
	if img.RGBAAt(83, 50) != white || img.RGBAAt(85, 50) != beige || img.RGBAAt(939, 50) != beige || img.RGBAAt(941, 50) != white {
		t.Errorf("main table is not centered at 85%% width")
	}
}

func TestLoadResolvesSubresources(t *testing.T) {
	files := map[string]string{
		"http://example.test/dir/page.html": `<!DOCTYPE html><link rel="stylesheet" href="../s.css"><link rel="stylesheet" href="print.css" media="print"><body><p>hi</p><img src="missing.png"><img src="file:///etc/hostname">`,
		"http://example.test/s.css":         `body { background: #123456 }`,
	}
	var fetched []string
	fake := func(raw string) (*fetch.Response, error) {
		fetched = append(fetched, raw)
		body, ok := files[raw]
		if !ok {
			return nil, fmt.Errorf("not found: %s", raw)
		}
		u, _ := url.Parse(raw)
		return &fetch.Response{URL: u, Status: 200, Body: []byte(body)}, nil
	}
	p, err := Load("http://example.test/dir/page.html", Options{Width: 200, Height: 50, Fetch: fake})
	if err != nil {
		t.Fatal(err)
	}
	if all := strings.Join(fetched, " "); strings.Contains(all, "print.css") || strings.Contains(all, "file:") {
		t.Errorf("print stylesheet and local files should not be fetched: %v", fetched)
	}
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "missing.png") {
		t.Errorf("warnings = %v, want one for missing.png", p.Warnings)
	}
	img := p.Render()
	if img.Bounds().Dy() != 50 {
		t.Errorf("height = %d, want 50", img.Bounds().Dy())
	}
	if got := img.RGBAAt(1, 1); got != (color.RGBA{0x12, 0x34, 0x56, 0xff}) {
		t.Errorf("canvas = %v, want body background", got)
	}
}
