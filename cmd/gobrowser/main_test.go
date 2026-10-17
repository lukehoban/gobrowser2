package main

import (
	"strings"
	"testing"
)

func TestRunDumpDOM(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"-dump=dom", "../../html/testdata/hn_like.html"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#document (quirks)", `<table id="hnmain"`, `"Example News"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRunErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"https://news.ycombinator.com/"},
		{"-dump=bogus", "../../html/testdata/hn_like.html"},
		{"does-not-exist.html"},
	} {
		if err := run(args, &strings.Builder{}); err == nil {
			t.Errorf("run(%q) succeeded, want error", args)
		}
	}
}
