package html

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"", "<p>a", "<table><tr><td>a<td>b</table>", "<table>x<div>y</div></table>",
		"<!DOCTYPE html><title>&amp;</title>", "<script>x</script", "<a href='&#x2F;'>",
		"<ul><li>a<ul><li>b</ul></ul>", "</head><head><body></body></html>x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		doc := Parse(src)
		var check func(n *Node)
		check = func(n *Node) {
			for _, c := range n.Children {
				if c.Parent != n {
					t.Fatalf("child %q has wrong parent", c.Data)
				}
				check(c)
			}
		}
		check(doc)
		if err := doc.Dump(&strings.Builder{}); err != nil {
			t.Fatal(err)
		}
	})
}
