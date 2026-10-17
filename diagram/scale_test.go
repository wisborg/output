package diagram

import (
	"math"
	"testing"
)

// A straight line ends on each node's outline: the box's edge, or an
// ellipse's curve.
func TestStraightEndsOnOutlines(t *testing.T) {
	d := &Diagram{
		Root:     Group{Columns: 2, Items: []Item{node("a"), {Node: &Node{ID: "e", Shape: Ellipse}}, node("x"), node("b")}},
		Edges:    []Edge{{From: "a", To: "b"}, {From: "e", To: "x"}},
		Straight: true,
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	s := ScaleFor(16)
	onBox := func(id string, x, y float64) bool {
		n, _ := lay.Node(id)
		x0, y0, x1, y1 := s.Box(n.Rect)
		near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
		in := x >= x0-1e-6 && x <= x1+1e-6 && y >= y0-1e-6 && y <= y1+1e-6
		return in && (near(x, x0) || near(x, x1) || near(y, y0) || near(y, y1))
	}
	onEllipse := func(id string, x, y float64) bool {
		n, _ := lay.Node(id)
		x0, y0, x1, y1 := s.Box(n.Rect)
		cx, cy, rx, ry := (x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2
		return math.Abs(sq((x-cx)/rx)+sq((y-cy)/ry)-1) < 1e-6
	}
	for _, e := range lay.Edges {
		if !e.Straight {
			t.Fatalf("%s to %s is not straight", e.Edge.From, e.Edge.To)
		}
		xs, ys := s.Straight(lay, e)
		if xs[0] == xs[1] || ys[0] == ys[1] {
			t.Errorf("%s to %s runs along a row or column, %v %v: want a diagonal", e.Edge.From, e.Edge.To, xs, ys)
		}
		from := onBox(e.Edge.From, xs[0], ys[0])
		if e.Edge.From == "e" {
			from = onEllipse("e", xs[0], ys[0])
		}
		if !from || !onBox(e.Edge.To, xs[1], ys[1]) {
			t.Errorf("%s to %s from (%.2f, %.2f) to (%.2f, %.2f): not on the outlines", e.Edge.From, e.Edge.To, xs[0], ys[0], xs[1], ys[1])
		}
	}
}
