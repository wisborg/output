package diagram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The text drawing of two regions, a link within the first and a link from
// it to the second: the line crossing the lanes' line meets it in a cross,
// the edges leave their boxes through a tee in the outline and arrive at an
// arrowhead, and each lane's label is on its own row.
func TestTextDrawsLanes(t *testing.T) {
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 2, Items: []Item{node("dbA"), node("dbB")}}},
			{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 2, Items: []Item{node("dbC")}}},
		}},
		Edges: []Edge{{From: "dbA", To: "dbB", Head: true}, {From: "dbA", To: "dbC", Head: true}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	lines := lay.Text()
	text := strings.Join(lines, "\n")
	for _, want := range []string{"│ dbA ├─────▶│ dbB │", "┼", "▼", "us-east-1", "us-west-2", "└──┬──┘"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	for i, l := range lines {
		if n := utf8.RuneCountInString(l); n > lay.Width {
			t.Errorf("line %d is %d characters, wider than the layout's %d", i, n, lay.Width)
		}
	}
	if len(lines) != lay.Height {
		t.Errorf("%d lines, want the layout's %d", len(lines), lay.Height)
	}
}

// Dashed lines are drawn dashed, an invisible node keeps its cell and draws
// nothing, and a frame's label is set in its top edge.
func TestTextStyles(t *testing.T) {
	d := &Diagram{
		Title: "t",
		Root: Group{Items: []Item{{Group: &Group{ID: "f", Label: "frame", Columns: 3, Items: []Item{
			node("a"), {Node: &Node{ID: "gap", Invisible: true}}, {Node: &Node{ID: "b", Shape: Rounded}},
		}}}}},
		Edges: []Edge{{From: "a", To: "b", Line: Dashed}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for _, want := range []string{"t\n", "┌─ frame ─", "╭───╮", "╌"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "gap") {
		t.Errorf("the invisible node is drawn:\n%s", text)
	}
}
