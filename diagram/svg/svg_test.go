package svg

import (
	"bytes"
	"encoding/xml"
	"io"
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
	for _, want := range []string{`a & "b"`, "<cluster>", "one", "two", "b", "x < y"} {
		if !strings.Contains(got, want) {
			t.Errorf("no text %q in %q", want, got)
		}
	}
	if elements["ellipse"] < 2 || elements["svg"] != 1 {
		t.Errorf("elements %v: want the ellipse, the cylinder's lid, one svg", elements)
	}
}
