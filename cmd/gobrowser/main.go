// Command gobrowser renders a web page to a PNG.
//
// Usage:
//
//	gobrowser [-o out.png] [-width 1280] [-dump dom|layout] <url-or-file>
package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/lukehoban/gobrowser2/internal/browser"
	"github.com/lukehoban/gobrowser2/internal/fetch"
	"github.com/lukehoban/gobrowser2/internal/layout"
)

func main() {
	out := flag.String("o", "", "write a PNG rendering to this file")
	width := flag.Int("width", 1280, "viewport width in CSS pixels")
	dump := flag.String("dump", "", "print a debug dump to stdout: dom or layout")
	timeout := flag.Duration("timeout", 60*time.Second, "overall timeout")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: gobrowser [flags] <url-or-file>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 || (*out == "" && *dump == "") || *width < 1 || *width > 10000 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *out, *width, *dump, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
}

func run(arg, out string, width int, dump string, timeout time.Duration) error {
	u, err := fetch.ParseURL(arg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	page, err := browser.Load(ctx, fetch.New(), u)
	if err != nil {
		return err
	}
	switch dump {
	case "":
	case "dom":
		page.Doc.Dump(os.Stdout)
	case "layout":
		layout.Dump(os.Stdout, page.Layout(width).Root)
	default:
		return fmt.Errorf("unknown -dump value %q", dump)
	}
	if out == "" {
		return nil
	}
	img, _ := page.Render(width)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
