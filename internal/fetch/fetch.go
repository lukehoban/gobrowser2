// Package fetch loads http(s) resources with net/http, plus file:// URLs
// and local paths.
package fetch

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// UserAgent is sent with every request.
const UserAgent = "gobrowser/0.1 (+https://github.com/lukehoban/gobrowser2)"

// MaxRedirects bounds the number of redirects followed by Get.
const MaxRedirects = 10

// MaxBodySize bounds the size of a response body.
const MaxBodySize = 32 << 20

// Client is used for http(s) requests. It follows up to MaxRedirects
// redirects and transparently decompresses gzip responses.
var Client = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= MaxRedirects {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// Response is a fully-read HTTP (or file) response.
type Response struct {
	URL    *url.URL            // final URL after redirects
	Status int                 // HTTP status code (200 for files)
	Header map[string][]string // lower-cased header names
	Body   []byte
}

// HeaderValue returns the first value of the named header (case-insensitive).
func (r *Response) HeaderValue(name string) string {
	if v := r.Header[strings.ToLower(name)]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Get fetches the resource at rawURL, following redirects. rawURL may be an
// http(s) URL, a file:// URL or a local file path.
func Get(rawURL string) (*Response, error) {
	u, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "file":
		body, err := os.ReadFile(u.Path)
		if err != nil {
			return nil, err
		}
		return &Response{URL: u, Status: 200, Header: map[string][]string{}, Body: body}, nil
	case "http", "https":
		return httpGet(u)
	}
	return nil, fmt.Errorf("fetch %s: unsupported scheme %q", u, u.Scheme)
}

// ParseURL parses an http(s) or file URL, treating anything without a
// scheme as a local file path.
func ParseURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		abs := raw
		if !strings.HasPrefix(abs, "/") {
			wd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			abs = wd + "/" + raw
		}
		return &url.URL{Scheme: "file", Path: abs}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func httpGet(u *url.URL) (*Response, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,text/css,image/*,*/*;q=0.8")
	resp, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	if len(body) > MaxBodySize {
		return nil, fmt.Errorf("fetch %s: body exceeds %d bytes", u, MaxBodySize)
	}
	header := map[string][]string{}
	for k, v := range resp.Header {
		header[strings.ToLower(k)] = v
	}
	return &Response{URL: resp.Request.URL, Status: resp.StatusCode, Header: header, Body: body}, nil
}
