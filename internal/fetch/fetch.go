// Package fetch loads documents and subresources over HTTP(S) or from the
// local filesystem.
package fetch

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxBodySize bounds how much of any single resource is read.
const MaxBodySize = 32 << 20

// Resource is a fetched document or subresource.
type Resource struct {
	URL         *url.URL // final URL after redirects
	ContentType string
	Body        []byte
}

// Fetcher retrieves resources. The zero value is not usable; use New.
type Fetcher struct {
	Client    *http.Client
	UserAgent string
}

// New returns a Fetcher with sensible timeouts.
func New() *Fetcher {
	return &Fetcher{
		Client:    &http.Client{Timeout: 30 * time.Second},
		UserAgent: "gobrowser2/0.1 (+https://github.com/lukehoban/gobrowser2)",
	}
}

// ParseURL turns a command-line argument into a URL. Arguments without a
// scheme are treated as local file paths.
func ParseURL(s string) (*url.URL, error) {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "file://") {
		return url.Parse(s)
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		return nil, err
	}
	return &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}, nil
}

// Resolve resolves ref against base. It returns nil if ref is malformed.
func Resolve(base *url.URL, ref string) *url.URL {
	ref = strings.TrimSpace(ref)
	r, err := url.Parse(ref)
	if err != nil {
		return nil
	}
	if base == nil {
		return r
	}
	return base.ResolveReference(r)
}

// Get fetches u.
func (f *Fetcher) Get(ctx context.Context, u *url.URL) (*Resource, error) {
	switch u.Scheme {
	case "data":
		return decodeDataURL(u)
	case "file":
		p := filepath.FromSlash(u.Path)
		fh, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		body, err := io.ReadAll(io.LimitReader(fh, MaxBodySize))
		if err != nil {
			return nil, err
		}
		return &Resource{URL: u, ContentType: contentTypeForPath(p), Body: body}, nil
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", f.UserAgent)
		resp, err := f.Client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize))
		if err != nil {
			return nil, err
		}
		return &Resource{URL: resp.Request.URL, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func contentTypeForPath(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	return ""
}

// decodeDataURL decodes a data: URL (RFC 2397).
func decodeDataURL(u *url.URL) (*Resource, error) {
	raw := strings.TrimPrefix(u.String(), "data:")
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil, fmt.Errorf("malformed data URL")
	}
	meta, data := raw[:comma], raw[comma+1:]
	var body []byte
	if strings.HasSuffix(meta, ";base64") {
		meta = strings.TrimSuffix(meta, ";base64")
		unesc, err := url.PathUnescape(data)
		if err != nil {
			unesc = data
		}
		body, err = base64.StdEncoding.DecodeString(unesc)
		if err != nil {
			return nil, err
		}
	} else {
		s, err := url.PathUnescape(data)
		if err != nil {
			return nil, err
		}
		body = []byte(s)
	}
	if len(body) > MaxBodySize {
		return nil, fmt.Errorf("data URL too large")
	}
	return &Resource{URL: u, ContentType: meta, Body: body}, nil
}
