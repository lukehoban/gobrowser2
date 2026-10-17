package fetch

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
