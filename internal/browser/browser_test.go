package browser

import (
	"context"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lukehoban/gobrowser2/internal/fetch"
)

func load(t *testing.T, raw string) *Page {
	t.Helper()
	u, err := fetch.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load(context.Background(), fetch.New(), u)
	if err != nil {
		t.Fatal(err)
	}
	p.logf = t.Logf
	return p
}

func isOrange(c color.RGBA) bool { return c == color.RGBA{0xff, 0x66, 0x00, 0xff} }

// TestRenderHNFixture renders the synthetic HN-shaped page and checks the
// main landmarks of the layout.
func TestRenderHNFixture(t *testing.T) {
	p := load(t, filepath.Join("..", "..", "testdata", "hn", "index.html"))
	if len(p.Sheets) == 0 {
		t.Fatal("stylesheet not loaded")
	}
	img, _ := p.Render(1280)
	b := img.Bounds()
	if b.Dx() != 1280 || b.Dy() < 300 {
		t.Fatalf("image size %v", b)
	}

	// The orange header bar spans the 85%-wide centered table.
	minX, maxX, minY, maxY := b.Max.X, -1, b.Max.Y, -1
	for y := 0; y < 60; y++ {
		for x := 0; x < b.Dx(); x++ {
			if isOrange(img.RGBAAt(x, y)) {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if minX < 100 || minX > 106 || maxX < 1174 || maxX > 1180 {
		t.Errorf("header x range [%d, %d], want ~[103, 1177]", minX, maxX)
	}
	if minY < 7 || minY > 9 || maxY < 28 || maxY > 33 {
		t.Errorf("header y range [%d, %d], want ~[8, 31]", minY, maxY)
	}

	// The body below the header has the beige background and rows of text.
	if got := img.RGBAAt(640, 200); got != (color.RGBA{0xf6, 0xf6, 0xef, 0xff}) && !isDark(got) {
		t.Errorf("story area pixel = %v, want beige", got)
	}
	if n := textBands(img, image.Rect(150, 35, 900, 300)); n < 8 {
		t.Errorf("found %d text bands in the story list, want at least 8", n)
	}
}

func isDark(c color.RGBA) bool { return int(c.R)+int(c.G)+int(c.B) < 3*160 }

// textBands counts runs of rows containing dark pixels within r.
func textBands(img *image.RGBA, r image.Rectangle) int {
	n, in := 0, false
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dark := false
		for x := r.Min.X; x < r.Max.X && !dark; x++ {
			dark = isDark(img.RGBAAt(x, y))
		}
		if dark && !in {
			n++
		}
		in = dark
	}
	return n
}

// TestRemotePageCannotReadFiles checks that neither relative URLs nor <base>
// let a remote page load file: subresources.
func TestRemotePageCannotReadFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.css"), []byte("body { background: red }"), 0o644)
	fileURL := "file://" + filepath.ToSlash(dir) + "/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<base href="` + fileURL + `"><link rel="stylesheet" href="x.css"><link rel="stylesheet" href="` + fileURL + `x.css"><body>hi`))
	}))
	defer srv.Close()
	p := load(t, srv.URL)
	if len(p.Sheets) != 0 {
		t.Errorf("remote page loaded %d local stylesheets", len(p.Sheets))
	}
	if p.URL.Scheme != "http" {
		t.Errorf("<base> changed the document URL to %v", p.URL)
	}
}
