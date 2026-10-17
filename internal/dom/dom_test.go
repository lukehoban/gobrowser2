package dom

import (
	"strings"
	"testing"
)

func TestParseHNQuirks(t *testing.T) {
	src := `<html><body><table><tr style='height:10px'/><tr id="b"><td>a<td>b</table><script>x()</script></body></html>`
	doc, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Quirks {
		t.Error("document without doctype should be in quirks mode")
	}
	rows := doc.FindAll("tr")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (self-closing <tr/> is an open tag)", len(rows))
	}
	if got := len(rows[1].Children); got != 2 {
		t.Errorf("second row has %d cells, want 2 (unclosed <td> is closed implicitly)", got)
	}
	if doc.Find("tbody") == nil {
		t.Error("expected implied <tbody>")
	}
	if doc.Find("script") != nil {
		t.Error("<script> should be dropped")
	}
}

func TestStandardsMode(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<!DOCTYPE html><p>hi`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Quirks {
		t.Error("<!DOCTYPE html> should be standards mode")
	}
	if got := doc.Find("p").TextContent(); got != "hi" {
		t.Errorf("text = %q", got)
	}
}

func TestDump(t *testing.T) {
	doc, _ := Parse(strings.NewReader(`<p class="x">a  b</p>`))
	var sb strings.Builder
	doc.Dump(&sb)
	if !strings.Contains(sb.String(), `<p class="x">`) || !strings.Contains(sb.String(), `"a b"`) {
		t.Errorf("unexpected dump:\n%s", sb.String())
	}
}
