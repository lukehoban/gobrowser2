// Package browser wires the pipeline together: fetch, parse, style, layout
// and paint.
package browser

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoding
	_ "image/jpeg" // register JPEG decoding
	_ "image/png"  // register PNG decoding
	"math"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
	"github.com/lukehoban/gobrowser2/internal/fetch"
	"github.com/lukehoban/gobrowser2/internal/html"
	"github.com/lukehoban/gobrowser2/internal/layout"
	"github.com/lukehoban/gobrowser2/internal/render"
	"github.com/lukehoban/gobrowser2/internal/style"
)

// MaxHeight bounds the height of rendered images, in px.
const MaxHeight = 32768

// maxImagePixels bounds the decoded size of an image.
const maxImagePixels = 50_000_000

// Options configures page loading and rendering.
type Options struct {
	Width  int // viewport width in px
	Height int // output height in px; 0 renders the full page
	// Fetch retrieves a resource; defaults to fetch.Get.
	Fetch func(url string) (*fetch.Response, error)
}

// Page is a loaded and laid-out document.
type Page struct {
	URL      *url.URL
	Doc      *dom.Node
	Styles   style.Styles
	Root     *layout.Box
	Height   float64 // document height in px
	Warnings []string

	opts Options
}

// Load fetches and lays out the document at rawURL.
func Load(rawURL string, opts Options) (*Page, error) {
	if opts.Width <= 0 {
		opts.Width = 1024
	}
	if opts.Fetch == nil {
		opts.Fetch = fetch.Get
	}
	resp, err := opts.Fetch(rawURL)
	if err != nil {
		return nil, err
	}
	p := &Page{URL: resp.URL, opts: opts}
	if resp.Status >= 400 {
		p.warn("HTTP status %d for %s", resp.Status, resp.URL)
	}
	p.Doc = html.Parse(decodeText(resp.Body))
	if base := p.Doc.Find("base"); base != nil {
		if href, ok := base.Attr("href"); ok {
			if u, err := p.URL.Parse(href); err == nil {
				p.URL = u
			}
		}
	}
	sheets := p.loadStylesheets()
	p.Styles = style.Compute(p.Doc, sheets, float64(opts.Width))
	res := &layout.Resources{Images: p.loadImages()}
	p.Root = layout.Build(p.Doc, p.Styles, res)
	p.Height = layout.Layout(p.Root, float64(opts.Width))
	return p, nil
}

// Render paints the page.
func (p *Page) Render() *image.RGBA {
	h := p.opts.Height
	if h <= 0 {
		h = int(math.Ceil(p.Height))
	}
	h = min(max(h, 1), MaxHeight)
	return render.Paint(p.Root, p.opts.Width, h)
}

func (p *Page) warn(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

// decodeText converts a document body to a string, falling back to
// Latin-1 when the bytes are not valid UTF-8.
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

func (p *Page) resolve(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	u, err := p.URL.Parse(ref)
	if err != nil {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		return u.String(), true
	case "file":
		// Like browsers, only local pages may load local files.
		return u.String(), p.URL.Scheme == "file"
	}
	return "", false
}

// loadStylesheets gathers <style> and <link rel=stylesheet> sheets in
// document order.
func (p *Page) loadStylesheets() []*css.Stylesheet {
	var sheets []*css.Stylesheet
	p.Doc.Walk(func(n *dom.Node) bool {
		if n.Type != dom.ElementNode {
			return true
		}
		media, _ := n.Attr("media")
		switch n.Tag {
		case "style":
			sheets = append(sheets, withMedia(css.Parse(n.TextContent()), media))
			return false
		case "link":
			rel, _ := n.Attr("rel")
			if !hasToken(rel, "stylesheet") || hasToken(rel, "alternate") ||
				!css.MediaMatches(media, float64(p.opts.Width)) {
				return false
			}
			href, _ := n.Attr("href")
			u, ok := p.resolve(href)
			if !ok {
				return false
			}
			resp, err := p.opts.Fetch(u)
			if err != nil {
				p.warn("stylesheet %s: %v", u, err)
				return false
			}
			if resp.Status >= 400 {
				p.warn("stylesheet %s: HTTP %d", u, resp.Status)
				return false
			}
			sheets = append(sheets, withMedia(css.Parse(decodeText(resp.Body)), media))
			return false
		}
		return true
	})
	return sheets
}

func withMedia(ss *css.Stylesheet, media string) *css.Stylesheet {
	media = strings.TrimSpace(media)
	if media == "" {
		return ss
	}
	for _, r := range ss.Rules {
		if r.Media == "" {
			r.Media = media
		} else {
			r.Media = media + " and " + r.Media
		}
	}
	return ss
}

func hasToken(list, tok string) bool {
	for _, f := range strings.Fields(strings.ToLower(list)) {
		if f == tok {
			return true
		}
	}
	return false
}

// loadImages fetches and decodes <img> sources.
func (p *Page) loadImages() map[*dom.Node]image.Image {
	images := map[*dom.Node]image.Image{}
	cache := map[string]image.Image{}
	failed := map[string]bool{}
	for _, n := range p.Doc.FindAll("img") {
		if st := p.Styles[n]; st == nil || st.Display == style.DisplayNone {
			continue
		}
		src, _ := n.Attr("src")
		u, ok := p.resolve(src)
		if !ok || failed[u] {
			continue
		}
		if img, ok := cache[u]; ok {
			images[n] = img
			continue
		}
		resp, err := p.opts.Fetch(u)
		if err != nil || resp.Status >= 400 {
			p.warn("image %s: fetch failed", u)
			failed[u] = true
			continue
		}
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(resp.Body)); err == nil &&
			cfg.Width*cfg.Height > maxImagePixels {
			p.warn("image %s: too large (%dx%d)", u, cfg.Width, cfg.Height)
			failed[u] = true
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(resp.Body))
		if err != nil {
			p.warn("image %s: %v", u, err)
			failed[u] = true
			continue
		}
		cache[u] = img
		images[n] = img
	}
	return images
}
