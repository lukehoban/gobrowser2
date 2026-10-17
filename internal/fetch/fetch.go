// Package fetch implements a minimal HTTP/1.1 client on top of raw TCP/TLS
// connections, plus support for file:// URLs and local paths.
package fetch

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// UserAgent is sent with every request.
const UserAgent = "gobrowser/0.1 (+https://github.com/lukehoban/gobrowser2)"

// MaxRedirects bounds the number of redirects followed by Get.
const MaxRedirects = 10

// Timeout bounds the duration of a single request.
var Timeout = 30 * time.Second

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
	for i := 0; ; i++ {
		resp, err := get(u)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case 301, 302, 303, 307, 308:
			if i >= MaxRedirects {
				return nil, fmt.Errorf("fetch %s: too many redirects", rawURL)
			}
			loc := resp.HeaderValue("location")
			if loc == "" {
				return resp, nil
			}
			next, err := u.Parse(loc)
			if err != nil {
				return nil, fmt.Errorf("fetch %s: bad redirect location %q: %w", u, loc, err)
			}
			u = next
			continue
		}
		return resp, nil
	}
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

func get(u *url.URL) (*Response, error) {
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

func httpGet(u *url.URL) (*Response, error) {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	addr := net.JoinHostPort(host, port)
	deadline := time.Now().Add(Timeout)
	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	var err error
	if u.Scheme == "https" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	var req bytes.Buffer
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&req, "Host: %s\r\n", u.Host)
	fmt.Fprintf(&req, "User-Agent: %s\r\n", UserAgent)
	req.WriteString("Accept: */*\r\n")
	req.WriteString("Accept-Encoding: gzip\r\n")
	req.WriteString("Connection: close\r\n\r\n")
	if _, err := conn.Write(req.Bytes()); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	resp, err := ReadResponse(bufio.NewReader(conn))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	resp.URL = u
	return resp, nil
}

// ReadResponse parses an HTTP/1.x response (status line, headers and body)
// from r, decoding chunked transfer encoding and gzip content encoding.
func ReadResponse(r *bufio.Reader) (*Response, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("reading status line: %w", err)
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		return nil, fmt.Errorf("malformed status line %q", line)
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed status code in %q", line)
	}
	resp := &Response{Status: status, Header: map[string][]string{}}
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, fmt.Errorf("reading headers: %w", err)
		}
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		resp.Header[name] = append(resp.Header[name], strings.TrimSpace(value))
	}

	var body []byte
	switch {
	case strings.EqualFold(resp.HeaderValue("transfer-encoding"), "chunked"):
		body, err = readChunked(r)
	case resp.HeaderValue("content-length") != "":
		n, perr := strconv.Atoi(resp.HeaderValue("content-length"))
		if perr != nil || n < 0 {
			return nil, fmt.Errorf("bad content-length %q", resp.HeaderValue("content-length"))
		}
		body = make([]byte, n)
		_, err = io.ReadFull(r, body)
	case status == 204 || status == 304 || (status >= 100 && status < 200):
	default:
		body, err = io.ReadAll(r)
	}
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if strings.EqualFold(resp.HeaderValue("content-encoding"), "gzip") && len(body) > 0 {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		body, err = io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
	}
	resp.Body = body
	return resp, nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && line != "" {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func readChunked(r *bufio.Reader) ([]byte, error) {
	var body []byte
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		sizeStr, _, _ := strings.Cut(line, ";")
		size, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 16, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("bad chunk size %q", line)
		}
		if size == 0 {
			// Skip trailers.
			for {
				line, err := readLine(r)
				if err != nil || line == "" {
					return body, nil
				}
			}
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, err
		}
		body = append(body, chunk...)
		if _, err := readLine(r); err != nil {
			return nil, err
		}
	}
}
