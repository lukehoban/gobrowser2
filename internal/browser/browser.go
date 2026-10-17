// Package browser wires the pipeline together: fetch → parse → style →
// layout → paint.
package browser

import (
	"bytes"
	"context"
	"image"
	"image/color"
	_ "image/gif"  // register decoder
	_ "image/jpeg" // register decoder
	_ "image/png"  // register decoder
	"io"
	"log"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/net/html/charset"

	"github.com/lukehoban/gobrowser2/internal/css"
	"github.com/lukehoban/gobrowser2/internal/dom"
	"github.com/lukehoban/gobrowser2/internal/fetch"
	"github.com/lukehoban/gobrowser2/internal/layout"
	"github.com/lukehoban/gobrowser2/internal/render"
	"github.com/lukehoban/gobrowser2/internal/style"
	"github.com/lukehoban/gobrowser2/internal/svg"
)

// Page is a loaded document with its subresources.
type Page struct {
	URL    *url.URL
	Doc    *dom.Node
	Sheets []*css.Stylesheet
	Styles map[*dom.Node]*style.Style
	Images map[string]layout.Image // by absolute URL

	fetcher *fetch.Fetcher
	logf    func(format string, args ...any)
	// local reports whether the document itself was loaded from a file.
	// It is fixed at load time so that <base> cannot change it.
	local bool
}

// Load fetches and parses the document at u along with its stylesheets and
// images. Subresource failures are logged and otherwise ignored.
func Load(ctx context.Context, f *fetch.Fetcher, u *url.URL) (*Page, error) {
	res, err := f.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return LoadBytes(ctx, f, res.URL, res.Body, res.ContentType)
}

// LoadBytes is like Load for an already-fetched document.
func LoadBytes(ctx context.Context, f *fetch.Fetcher, u *url.URL, body []byte, contentType string) (*Page, error) {
	var r io.Reader = bytes.NewReader(body)
	// Without a declared charset, prefer UTF-8 when the bytes are valid
	// UTF-8 rather than the windows-1252 fallback of the sniffing algorithm.
	if strings.Contains(strings.ToLower(contentType), "charset") || !utf8.Valid(body) {
		if cr, err := charset.NewReader(bytes.NewReader(body), contentType); err == nil {
			r = cr
		}
	}
	doc, err := dom.Parse(r)
	if err != nil {
		return nil, err
	}
	p := &Page{URL: u, Doc: doc, Images: map[string]layout.Image{}, fetcher: f, logf: log.Printf, local: u.Scheme == "file"}
	if b := doc.Find("base"); b != nil {
		if h, ok := b.Attr("href"); ok {
			if bu := fetch.Resolve(u, h); bu != nil && p.allowed(bu) {
				p.URL = bu
			}
		}
	}
	p.loadStylesheets(ctx)
	p.Styles = style.Compute(doc, p.Sheets, p.URL)
	p.loadImages(ctx)
	return p, nil
}

// allowed reports whether a subresource may be loaded by this page. Remote
// pages may not read local files.
func (p *Page) allowed(u *url.URL) bool {
	switch u.Scheme {
	case "http", "https", "data":
		return true
	case "file":
		return p.local
	}
	return false
}

func (p *Page) get(ctx context.Context, u *url.URL) (*fetch.Resource, error) {
	return p.fetcher.Get(ctx, u)
}

func (p *Page) loadStylesheets(ctx context.Context) {
	type slot struct {
		inline string
		u      *url.URL
		sheet  *css.Stylesheet
	}
	var slots []*slot
	p.Doc.Walk(func(n *dom.Node) {
		if n.Type != dom.ElementNode {
			return
		}
		switch n.Tag {
		case "style":
			if m := n.AttrOr("media", "all"); !mediaApplies(m) {
				return
			}
			slots = append(slots, &slot{inline: n.TextContent()})
		case "link":
			rel := strings.Fields(strings.ToLower(n.AttrOr("rel", "")))
			isSheet := false
			for _, r := range rel {
				if r == "stylesheet" {
					isSheet = true
				}
				if r == "alternate" {
					return
				}
			}
			if !isSheet || !mediaApplies(n.AttrOr("media", "all")) {
				return
			}
			if u := fetch.Resolve(p.URL, n.AttrOr("href", "")); u != nil && p.allowed(u) {
				slots = append(slots, &slot{u: u})
			}
		}
	})
	var wg sync.WaitGroup
	for _, s := range slots {
		if s.u == nil {
			s.sheet = css.Parse(s.inline, p.URL)
			continue
		}
		wg.Add(1)
		go func(s *slot) {
			defer wg.Done()
			res, err := p.get(ctx, s.u)
			if err != nil {
				p.logf("stylesheet %s: %v", s.u, err)
				return
			}
			s.sheet = css.Parse(string(res.Body), res.URL)
		}(s)
	}
	wg.Wait()
	for _, s := range slots {
		if s.sheet != nil {
			p.Sheets = append(p.Sheets, s.sheet)
		}
	}
}

// mediaApplies evaluates a media attribute for a desktop screen.
func mediaApplies(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	if m == "" || m == "all" || m == "screen" {
		return true
	}
	for _, part := range strings.Split(m, ",") {
		part = strings.TrimSpace(part)
		if part == "all" || part == "screen" {
			return true
		}
	}
	return false
}

func (p *Page) loadImages(ctx context.Context) {
	urls := map[string]*url.URL{}
	p.Doc.Walk(func(n *dom.Node) {
		s := p.Styles[n]
		if n.Type != dom.ElementNode || s == nil || s.Display == "none" {
			return
		}
		if n.Tag == "img" {
			if u := fetch.Resolve(p.URL, n.AttrOr("src", "")); u != nil && n.AttrOr("src", "") != "" {
				urls[u.String()] = u
			}
		}
		if s.BackgroundImage != "" {
			if u := fetch.Resolve(nil, s.BackgroundImage); u != nil {
				urls[u.String()] = u
			}
		}
	})
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for key, u := range urls {
		if !p.allowed(u) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := p.get(ctx, u)
			if err != nil {
				p.logf("image %s: %v", u, err)
				return
			}
			im, err := DecodeImage(res.Body, res.ContentType)
			if err != nil {
				p.logf("image %s: %v", u, err)
				return
			}
			mu.Lock()
			p.Images[key] = im
			mu.Unlock()
		}()
	}
	wg.Wait()
}

// DecodeImage decodes a raster (PNG, GIF, JPEG) or SVG image.
func DecodeImage(data []byte, contentType string) (layout.Image, error) {
	head := strings.TrimSpace(string(data[:min(len(data), 256)]))
	if strings.Contains(contentType, "svg") || strings.HasPrefix(head, "<svg") || strings.HasPrefix(head, "<?xml") {
		return svg.Parse(data)
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return rasterImage{im}, nil
}

type rasterImage struct{ im image.Image }

func (r rasterImage) Size() (float64, float64) {
	b := r.im.Bounds()
	return float64(b.Dx()), float64(b.Dy())
}

func (r rasterImage) Draw(dst *image.RGBA, rect image.Rectangle) {
	if rect.Empty() {
		return
	}
	xdraw.ApproxBiLinear.Scale(dst, rect, r.im, r.im.Bounds(), xdraw.Over, nil)
}

// Render lays out and paints the page at the given viewport width.
func (p *Page) Render(width int) (*image.RGBA, *layout.Result) {
	res := p.Layout(width)
	img := render.Paint(res, render.Options{
		Width:      width,
		Background: p.canvasBackground(),
		BackgroundImage: func(u string) layout.Image {
			return p.Images[u]
		},
	})
	return img, res
}

// Layout lays out the page at the given viewport width.
func (p *Page) Layout(width int) *layout.Result {
	return layout.Layout(p.Doc, p.Styles, layout.Options{
		Width:  float64(width),
		Quirks: p.Doc.Quirks,
		Image: func(n *dom.Node) layout.Image {
			src := n.AttrOr("src", "")
			if src == "" {
				return nil
			}
			if u := fetch.Resolve(p.URL, src); u != nil {
				if im, ok := p.Images[u.String()]; ok {
					return im
				}
			}
			return nil
		},
	})
}

// canvasBackground propagates the root or body background to the canvas.
func (p *Page) canvasBackground() color.RGBA {
	for _, tag := range []string{"html", "body"} {
		if n := p.Doc.Find(tag); n != nil {
			if s := p.Styles[n]; s != nil && s.BackgroundColor.A > 0 {
				return s.BackgroundColor
			}
		}
	}
	return color.RGBA{255, 255, 255, 255}
}
