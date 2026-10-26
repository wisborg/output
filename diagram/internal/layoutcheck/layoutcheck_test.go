package layoutcheck

import (
	"strings"
	"testing"
)

// Two boxes, a above b, and a record r of two fields alike between them
// to the right, its divider meeting its top at (24, 0) and its bottom at
// (24, 4).
func page() []Box {
	return []Box{
		{ID: "a", Rect: Rect{0, 0, 7, 3}},
		{ID: "b", Rect: Rect{0, 10, 7, 3}},
		{ID: "r", Rect: Rect{20, 0, 9, 5}, Dividers: [][2]Point{{{24, 0}, {24, 4}}}},
		{ID: "t", Rect: Rect{20, 10, 9, 3}},
		{ID: "m", Rect: Rect{0, 5, 7, 3}},
	}
}

// Each kind of broken promise is said, naming it, and a layout keeping
// them all says nothing: a checker that passes everything is no check.
func TestCheck(t *testing.T) {
	for _, c := range []struct {
		name string
		e    Edge
		o    Options
		want string // in what is said; empty for nothing said
	}{
		{"a line round the box between", Edge{From: "a", To: "b", Points: []Point{{6, 1}, {9, 1}, {9, 11}, {6, 11}}}, Options{}, ""},
		{"a line through the box between", Edge{From: "a", To: "b", Points: []Point{{3, 2}, {3, 10}}}, Options{}, "runs through m"},
		{"through it, drawn plain", Edge{From: "a", To: "b", Points: []Point{{3, 2}, {3, 10}}}, Options{Plain: true}, ""},
		{"a slanting stretch", Edge{From: "a", To: "b", Points: []Point{{6, 1}, {9, 2}, {9, 11}, {6, 11}}}, Options{}, "neither a row nor a column"},
		{"an end off the outline", Edge{From: "a", To: "b", Points: []Point{{7, 1}, {9, 1}, {9, 11}, {6, 11}}}, Options{}, "off a's outline"},
		{"an end at a corner", Edge{From: "a", To: "b", Points: []Point{{6, 0}, {9, 0}, {9, 11}, {6, 11}}}, Options{}, "a corner of a"},
		{"an end running back into its box", Edge{From: "a", To: "b", Points: []Point{{6, 1}, {5, 1}, {5, 20}}}, Options{}, "runs on into a"},
		{"an end where a divider meets the outline", Edge{From: "r", To: "t", Points: []Point{{24, 4}, {24, 10}}}, Options{}, "where a divider of r meets"},
		{"an end beside it", Edge{From: "r", To: "t", Points: []Point{{23, 4}, {23, 10}}}, Options{}, ""},
		// Drawn plain, an end may stay at a joint only where every move
		// off it would run through a box: here there is room.
		{"at a joint, drawn plain, with room", Edge{From: "r", To: "t", Points: []Point{{24, 4}, {24, 10}}}, Options{Plain: true}, "where a divider of r meets"},
		{"a side kept, and said dropped", Edge{From: "a", To: "b", FromSide: East, SidesDropped: true, Points: []Point{{6, 1}, {9, 1}, {9, 11}, {6, 11}}}, Options{}, "SidesDropped true, but the path keeps"},
		{"a side missed, and not said", Edge{From: "a", To: "b", FromSide: West, Points: []Point{{6, 1}, {9, 1}, {9, 11}, {6, 11}}}, Options{}, "SidesDropped false, but the path does not keep"},
		{"a side missed, and said", Edge{From: "a", To: "b", FromSide: West, SidesDropped: true, Points: []Point{{6, 1}, {9, 1}, {9, 11}, {6, 11}}}, Options{}, ""},
		{"a field kept", Edge{From: "r", To: "t", ToField: Rect{21, 11, 7, 1}, FromField: Rect{21, 1, 3, 3}, Points: []Point{{22, 4}, {22, 10}}}, Options{}, ""},
		// Across from the field, but on a side it does not reach: it
		// meets the top and the left, and the line leaves by the bottom.
		{"a field's side not reached", Edge{From: "r", To: "t", FromField: Rect{21, 1, 3, 1}, Points: []Point{{22, 4}, {22, 10}}}, Options{}, "does not keep"},
		{"a field missed, and not said", Edge{From: "r", To: "t", FromField: Rect{25, 1, 3, 3}, Points: []Point{{22, 4}, {22, 10}}}, Options{}, "does not keep"},
		{"an edge drawn nowhere", Edge{From: "a", To: "b"}, Options{}, ""},
		// Out of r beside its bottom joint, along, and down from straight
		// under the joint: the stretch along runs on from the divider.
		{"a turn straight out from a joint", Edge{From: "r", To: "t", Points: []Point{{23, 4}, {23, 5}, {24, 5}, {24, 10}}}, Options{}, "turns at {24 5}, straight out from a joint of r"},
		{"the same, a point repeated", Edge{From: "r", To: "t", Points: []Point{{23, 4}, {23, 5}, {24, 5}, {24, 5}, {24, 10}}}, Options{}, "turns at {24 5}"},
		{"the same, drawn plain", Edge{From: "r", To: "t", Points: []Point{{23, 4}, {23, 5}, {24, 5}, {24, 10}}}, Options{Plain: true}, ""},
		{"the same, this path alone drawn plain", Edge{From: "r", To: "t", Plain: true, Points: []Point{{23, 4}, {23, 5}, {24, 5}, {24, 10}}}, Options{}, ""},
		{"through a box, this path alone drawn plain", Edge{From: "a", To: "b", Plain: true, Points: []Point{{3, 2}, {3, 10}}}, Options{}, ""},
		{"a line running past it along the side", Edge{From: "a", To: "t", Points: []Point{{6, 1}, {9, 1}, {9, 5}, {31, 5}, {31, 11}, {28, 11}}}, Options{}, ""},
		{"the same, a point repeated straight out from it", Edge{From: "a", To: "t", Points: []Point{{6, 1}, {9, 1}, {9, 5}, {24, 5}, {24, 5}, {31, 5}, {31, 11}, {28, 11}}}, Options{}, ""},
	} {
		got := strings.Join(Check(Layout{Boxes: page(), Edges: []Edge{c.e}}, c.o), "\n")
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: said %q, want %q", c.name, got, c.want)
		}
	}
}

// Drawn plain, an end stays at a joint where every move along the side
// runs through a box: a box either side of the line, the worse fault.
func TestCheckPlainBoxedIn(t *testing.T) {
	boxes := append(page(), Box{ID: "left", Rect: Rect{21, 7, 3, 2}}, Box{ID: "right", Rect: Rect{25, 7, 3, 2}})
	e := Edge{From: "r", To: "t", Points: []Point{{24, 4}, {24, 10}}}
	if got := Check(Layout{Boxes: boxes, Edges: []Edge{e}}, Options{Plain: true}); len(got) != 0 {
		t.Errorf("boxed in, said %q", got)
	}
	// A box across the whole gap is crossed whether the end moves or not,
	// and keeps it at the joint for nothing.
	boxes = append(page(), Box{ID: "across", Rect: Rect{18, 7, 13, 2}})
	got := strings.Join(Check(Layout{Boxes: boxes, Edges: []Edge{e}}, Options{Plain: true}), "\n")
	if !strings.Contains(got, "where a divider of r meets") {
		t.Errorf("a box crossed moved or not, said %q", got)
	}
}

// A box with joints stands two cells or more from any other, so that a
// stub a cell out from its own box is never beside another's joint; boxes
// without joints may stand closer.
func TestCheckBoxesStandApart(t *testing.T) {
	for _, c := range []struct {
		near Rect
		want string
	}{
		{Rect{20, 6, 9, 3}, "r and near: 1 cells apart"},
		{Rect{30, 0, 3, 3}, "r and near: 1 cells apart"},
		{Rect{20, 7, 9, 2}, ""},
		{Rect{0, 4, 7, 1}, ""}, // beside a and m, neither with joints
	} {
		got := strings.Join(Check(Layout{Boxes: append(page(), Box{ID: "near", Rect: c.near})}, Options{}), "\n")
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("a box at %v: said %q, want %q", c.near, got, c.want)
		}
	}
}

// An invisible box is passed through, and has no joints; a plain one's
// corners may be ended at.
func TestCheckInvisibleAndPlain(t *testing.T) {
	boxes := page()
	boxes[4].Invisible = true
	boxes[2].Invisible = true
	boxes[0].Plain = true
	for _, e := range []Edge{
		{From: "a", To: "b", Points: []Point{{3, 2}, {3, 10}}},
		{From: "r", To: "t", Points: []Point{{24, 4}, {24, 10}}},
		{From: "a", To: "b", Points: []Point{{6, 0}, {9, 0}, {9, 11}, {6, 11}}},
	} {
		if got := Check(Layout{Boxes: boxes, Edges: []Edge{e}}, Options{}); len(got) != 0 {
			t.Errorf("%v: said %q", e.Points, got)
		}
	}
	if js := Joints(boxes[2]); len(js) != 0 {
		t.Errorf("an invisible record's joints: %v", js)
	}
}
