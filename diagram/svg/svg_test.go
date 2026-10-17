package svg

import (
	"bytes"
	"encoding/xml"
	"image/color"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/wisborg/output/diagram"
)

// The document is well-formed XML, whatever the labels hold, with a text
// element for each line of each label -- the reason to write SVG at all is
// text a reader can select -- and an element for each shape.
func TestWriteIsWellFormed(t *testing.T) {
	d := &diagram.Diagram{
		Title: `a & "b"`,
		Root: diagram.Group{Items: []diagram.Item{
			{Group: &diagram.Group{ID: "c", Label: "<cluster>", Columns: 2, Items: []diagram.Item{
				{Node: &diagram.Node{ID: "a", Label: "one\ntwo", Shape: diagram.Cylinder}},
				{Node: &diagram.Node{ID: "b", Shape: diagram.Ellipse, Line: diagram.Dashed}},
			}}},
		}},
		Edges: []diagram.Edge{{From: "a", To: "b", Head: true, Tail: true, Label: "x < y"}},
		Legend: &diagram.Legend{Label: "Key & more", Entries: []diagram.LegendEntry{
			{Label: "ok > fine", Fill: color.RGBA{0, 0xFF, 0, 0xFF}},
			{Label: "async", Sample: diagram.Stroke, Line: diagram.Dashed},
		}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, lay, Options{}); err != nil {
		t.Fatal(err)
	}
	var texts []string
	elements := map[string]int{}
	dec := xml.NewDecoder(&buf)
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			elements[tok.Name.Local]++
			inText = tok.Name.Local == "text"
		case xml.CharData:
			if inText {
				texts = append(texts, string(tok))
			}
		case xml.EndElement:
			inText = false
		}
	}
	got := strings.Join(texts, "|")
	for _, want := range []string{`a & "b"`, "<cluster>", "one", "two", "b", "x < y", "Key & more", "ok > fine", "async"} {
		if !strings.Contains(got, want) {
			t.Errorf("no text %q in %q", want, got)
		}
	}
	if elements["ellipse"] < 2 || elements["svg"] != 1 {
		t.Errorf("elements %v: want the ellipse, the cylinder's lid, one svg", elements)
	}
}

// A label is text someone else may have written, and an SVG is opened in a
// browser, which runs a script in one as it would in a page. Whatever a
// label holds -- markup, an attribute's quotes, a control character, a byte
// that is not UTF-8, a character XML forbids -- the document is still
// well-formed, holds only the elements this package writes and no
// attribute that runs or fetches anything, and the label is still there as
// text.
func TestWriteHoldsHostileLabelsAsText(t *testing.T) {
	hostile := []string{
		`</text><script>alert(1)</script>`,
		`" onload="alert(1)`,
		`' onclick='x`,
		`<a href="javascript:alert(1)">x</a>`,
		"]]><foreignObject/>",
		"form\ffeed \x01 bell\a",
		"bad \xff byte",
		"￾ not a character",
	}
	var items []diagram.Item
	var edges []diagram.Edge
	for i, l := range hostile {
		id := string(rune('a' + i))
		items = append(items, diagram.Item{Node: &diagram.Node{ID: id, Label: l}})
		if i > 0 {
			edges = append(edges, diagram.Edge{From: string(rune('a' + i - 1)), To: id, Head: true, Label: l})
		}
	}
	d := &diagram.Diagram{Title: hostile[0], Root: diagram.Group{Columns: 3, Items: items}, Edges: edges}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, lay, Options{}); err != nil {
		t.Fatal(err)
	}
	written := map[string]bool{"svg": true, "rect": true, "path": true, "ellipse": true, "text": true}
	var text strings.Builder
	dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if !written[tok.Name.Local] {
				t.Errorf("a <%s> element", tok.Name.Local)
			}
			for _, a := range tok.Attr {
				name := strings.ToLower(a.Name.Local)
				if strings.HasPrefix(name, "on") || strings.Contains(name, "href") || strings.Contains(strings.ToLower(a.Value), "javascript:") {
					t.Errorf("<%s %s=%q>", tok.Name.Local, a.Name.Local, a.Value)
				}
			}
		case xml.CharData:
			text.Write(tok)
		}
	}
	for _, l := range hostile[:5] {
		if !strings.Contains(text.String(), l) {
			t.Errorf("the label %q is not in the text", l)
		}
	}
}

// A text size that is not a number is an error, not width="NaN"; no layout
// is an error, not a nil dereference.
func TestWriteRefuses(t *testing.T) {
	lay, err := (&diagram.Diagram{Root: diagram.Group{Items: []diagram.Item{{Node: &diagram.Node{ID: "a"}}}}}).Layout()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	for _, px := range []float64{math.NaN(), math.Inf(1)} {
		if err := Write(&buf, lay, Options{TextPx: px}); err == nil {
			t.Errorf("TextPx %v wrote %q", px, buf.String())
		}
	}
	if err := Write(&buf, nil, Options{}); err == nil {
		t.Error("a nil layout was written")
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %d bytes on refusing", buf.Len())
	}
}
