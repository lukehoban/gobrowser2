// Command gobrowser loads an HTML document and runs it through the browser
// pipeline, printing debug dumps of intermediate stages.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lukehoban/gobrowser2/html"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("gobrowser", flag.ContinueOnError)
	dump := fs.String("dump", "dom", "debug dump to print: dom")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: gobrowser [flags] <file>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one input")
	}
	input := fs.Arg(0)
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return fmt.Errorf("fetching URLs is not implemented yet (see https://github.com/lukehoban/gobrowser2/issues/2)")
	}
	src, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	doc := html.Parse(string(src))
	switch *dump {
	case "dom":
		return doc.Dump(stdout)
	default:
		return fmt.Errorf("unknown -dump value %q", *dump)
	}
}
