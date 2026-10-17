package fetch

import (
	"bufio"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadResponseContentLength(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nX-Test: a\r\n\r\nhelloEXTRA"
	resp, err := ReadResponse(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != "hello" || resp.HeaderValue("X-TEST") != "a" {
		t.Fatalf("unexpected response: %+v body=%q", resp, resp.Body)
	}
}

func TestReadResponseChunked(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki\r\n5;ext=1\r\npedia\r\n0\r\nTrailer: x\r\n\r\n"
	resp, err := ReadResponse(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "Wikipedia" {
		t.Fatalf("body = %q", resp.Body)
	}
}

func TestReadResponseMalformed(t *testing.T) {
	for _, raw := range []string{"", "garbage\r\n\r\n", "HTTP/1.1 abc OK\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\n"} {
		if _, err := ReadResponse(bufio.NewReader(strings.NewReader(raw))); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

func TestGetHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/gzip?x=1", http.StatusFound)
		case "/gzip":
			if r.Header.Get("User-Agent") != UserAgent {
				t.Errorf("user agent = %q", r.Header.Get("User-Agent"))
			}
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			zw.Write([]byte("compressed " + r.URL.RawQuery))
			zw.Close()
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	resp, err := Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "compressed x=1" {
		t.Fatalf("body = %q", resp.Body)
	}
	if resp.URL.Path != "/gzip" {
		t.Fatalf("final URL = %s", resp.URL)
	}

	if _, err := Get(srv.URL + "/loop"); err == nil {
		t.Fatal("expected redirect loop error")
	}

	resp, err = Get(srv.URL + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 404 {
		t.Fatalf("status = %d", resp.Status)
	}
}

func TestGetFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.html")
	if err := os.WriteFile(p, []byte("<p>hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{p, "file://" + p} {
		resp, err := Get(u)
		if err != nil {
			t.Fatal(err)
		}
		if string(resp.Body) != "<p>hi" {
			t.Fatalf("body = %q", resp.Body)
		}
	}
}
