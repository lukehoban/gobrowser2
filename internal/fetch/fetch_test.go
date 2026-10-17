package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/page", http.StatusFound)
		case "/page":
			if r.UserAgent() == "" {
				t.Error("missing User-Agent")
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<p>hi"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := New()
	u, _ := ParseURL(srv.URL + "/redirect")
	res, err := f.Get(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if res.URL.Path != "/page" || string(res.Body) != "<p>hi" || res.ContentType != "text/html; charset=utf-8" {
		t.Errorf("got %+v", res)
	}
	if _, err := f.Get(context.Background(), Resolve(u, "/missing")); err == nil {
		t.Error("expected error for 404")
	}
}

func TestFileAndData(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.css")
	os.WriteFile(p, []byte("p{}"), 0o644)
	u, err := ParseURL(p)
	if err != nil || u.Scheme != "file" {
		t.Fatalf("ParseURL: %v %v", u, err)
	}
	res, err := New().Get(context.Background(), u)
	if err != nil || string(res.Body) != "p{}" || res.ContentType != "text/css" {
		t.Errorf("file: %+v %v", res, err)
	}
	for in, want := range map[string]string{
		"data:text/plain,a%20b":       "a b",
		"data:;base64,aGk=":           "hi",
		"data:image/svg+xml;utf8,<x>": "<x>",
	} {
		res, err := New().Get(context.Background(), Resolve(nil, in))
		if err != nil || string(res.Body) != want {
			t.Errorf("%s: %+v %v", in, res, err)
		}
	}
	if _, err := New().Get(context.Background(), Resolve(nil, "ftp://x/y")); err == nil {
		t.Error("expected error for unsupported scheme")
	}
}
