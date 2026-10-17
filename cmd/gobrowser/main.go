// Command gobrowser renders a web page to a PNG image.
//
// Usage:
//
//	gobrowser [flags] URL|FILE
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"

	"github.com/lukehoban/gobrowser2/internal/browser"
)

func main() {
	out := flag.String("o", "out.png", "output PNG file")
	width := flag.Int("width", 1024, "viewport width in px")
	height := flag.Int("height", 0, "output height in px (0 renders the full page)")
	dumpDOM := flag.Bool("dump-dom", false, "print the parsed DOM tree to stdout")
	verbose := flag.Bool("v", false, "print warnings (failed subresources etc.)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: gobrowser [flags] URL|FILE\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	page, err := browser.Load(flag.Arg(0), browser.Options{Width: *width, Height: *height})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
	if *verbose {
		for _, w := range page.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
	}
	if *dumpDOM {
		fmt.Print(page.Doc.String())
	}
	img := page.Render()
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
	b := img.Bounds()
	fmt.Fprintf(os.Stderr, "wrote %s (%dx%d)\n", *out, b.Dx(), b.Dy())
}
