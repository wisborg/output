package diagram

import (
	"strings"
	"testing"
)

// costPage is a router for a page on which edge 0 runs from a to b, either
// side of a tall box between them: a page that is the same above the box
// and below it, row for row, so that a path round the top costs what one
// round the bottom does until something is put on one side. Edge 1 is
// another's that edge 0 may not share a line with, edge 2 one it may.
func costPage() (r *router, a, b Rect) {
	a, b = Rect{2, 7, 3, 3}, Rect{24, 7, 3, 3}
	r = routerFor(map[string]Rect{"a": a, "b": b, "o": {9, 3, 11, 11}})
	if r.h != 17 {
		panic("the page is no longer the same above the box and below")
	}
	return r, a, b
}

// routerFor is a router for a page of the boxes, with costPage's edges.
func routerFor(boxes map[string]Rect) *router {
	l := &layouter{
		d:    &Diagram{Edges: []Edge{{From: "a", To: "b", Head: true}, {From: "x", To: "y", Head: true}, {From: "a", To: "z", Head: true}}},
		out:  &Layout{},
		rect: boxes,
	}
	for id, rc := range boxes {
		l.out.Nodes = append(l.out.Nodes, PlacedNode{Node: &Node{ID: id}, Rect: rc})
	}
	return newRouter(l)
}

// roundTop is whether pts goes round the top of the box between a and b.
func roundTop(pts []Point) bool {
	for _, p := range pts {
		if p.Y < 3 {
			return true
		}
	}
	return false
}

// Each of the costs a path is weighed by, seen deciding which way round
// it goes: a line to cross, or a straight line a picture draws at an angle
// across its way, sends it round the other side; one it may share draws it
// to its own side; another's line a cell off, running the same way, sends
// it round the other side. Each is put above the box and then below, so a
// cost that did not count would send the path the same way both times.
func TestRouteCostsChooseTheWay(t *testing.T) {
	for _, c := range []struct {
		name string
		put  func(r *router, row func(int) int)
		away bool // the path goes round the other side from what was put
	}{
		{"a line to cross", func(r *router, row func(int) int) {
			r.line(Point{14, row(0)}, Point{14, row(2)}, 1)
		}, true},
		{"a picture's slanting line", func(r *router, row func(int) int) {
			for y := 0; y <= 2; y++ {
				r.slant[row(y)][14] = true
			}
		}, true},
		{"another's line beside it", func(r *router, row func(int) int) {
			r.line(Point{5, row(0)}, Point{23, row(0)}, 1)
		}, true},
		{"a line it may share", func(r *router, row func(int) int) {
			r.line(Point{5, row(1)}, Point{23, row(1)}, 2)
		}, false},
	} {
		for _, above := range []bool{true, false} {
			r, a, b := costPage()
			row := func(y int) int { return y }
			if !above {
				row = func(y int) int { return r.h - 1 - y }
			}
			c.put(r, row)
			for _, strict := range []bool{true, false} {
				pts := r.find(a, b, false, 0, strict)
				if pts == nil {
					t.Fatalf("%s, above %v, strict %v: no path", c.name, above, strict)
				}
				if roundTop(pts) != (above != c.away) {
					t.Errorf("%s, above %v, strict %v: %v goes round the wrong side", c.name, above, strict, pts)
				}
			}
		}
	}
}

// Where the rules allow no path, the relaxed search takes one they forbid,
// but only where nothing they allow will do, at joinCost a cell -- more
// than any detour. So it goes round to a side of b where nobody ends
// rather than end where another's arrowhead is; makes two turns rather
// than turn where a picture's straight line crosses; and, with every way
// into b passing an arrowhead, passes one, where it once found no path at
// all and the edge was drawn through the boxes.
func TestRelaxedRouteJoinsOnlyWhereItMust(t *testing.T) {
	a, b := Rect{2, 2, 3, 3}, Rect{14, 2, 3, 3}
	r := routerFor(map[string]Rect{"a": a, "b": b})
	r.heads[3][14] = []int{1} // on b's side facing a, its one cell
	pts := r.find(a, b, false, 0, false)
	if pts == nil || pts[len(pts)-1] == (Point{14, 3}) {
		t.Errorf("%v ends at another's arrowhead", pts)
	}

	a, b = Rect{2, 2, 3, 3}, Rect{14, 10, 3, 3}
	r = routerFor(map[string]Rect{"a": a, "b": b})
	r.slant[3][15] = true      // where the one turn right then down would be
	r.heads[11][14] = []int{1} // and down then right ends at an arrowhead
	pts = r.find(a, b, false, 0, false)
	for _, p := range pts[1 : len(pts)-1] {
		if r.slant[p.Y][p.X] {
			t.Errorf("%v turns on a picture's straight line", pts)
		}
	}

	a, b = Rect{2, 2, 3, 3}, Rect{14, 2, 3, 3}
	r = routerFor(map[string]Rect{"a": a, "b": b})
	for y := b.Y - 1; y <= b.Y+b.H; y++ {
		for x := b.X - 1; x <= b.X+b.W; x++ {
			if !inside(b, x, y) {
				r.heads[y][x] = []int{1}
			}
		}
	}
	if pts := r.find(a, b, false, 0, true); pts != nil {
		t.Errorf("a strict path %v past another's arrowheads", pts)
	}
	if pts := r.find(a, b, false, 0, false); pts == nil {
		t.Errorf("no relaxed path where every way in passes an arrowhead")
	}
}

// A label slides along its stretch to find a free place, but stays beside
// it: never above or below a column's ends, nor wholly past a row's. One
// three rows beyond a stretch two cells long is beside nothing of its
// edge's, and as near another's line as its own.
func TestLabelTriesStayBesideTheirStretch(t *testing.T) {
	for _, try := range segmentTries([]Point{{5, 0}, {5, 3}}, nil, 4, 1) {
		if try.X == 5+2 && (try.Y < 0 || try.Y > 3) {
			t.Errorf("down a column from row 0 to 3, tried at row %d", try.Y)
		}
	}
	const width = 6
	for _, try := range segmentTries([]Point{{0, 5}, {3, 5}}, nil, width, 1) {
		if try.Y == 5-1 && (try.X > 3 || try.X+width-1 < 0) {
			t.Errorf("along a row from column 0 to 3, tried at columns %d to %d", try.X, try.X+width-1)
		}
	}
}

// A frame's label is set in its top edge, near its left corner, but not
// over a line crossing that edge: it moves along, so a line going in is
// seen going in.
func TestFrameLabelKeepsOffCrossings(t *testing.T) {
	d := &Diagram{
		Root: Group{Columns: 1, Items: []Item{
			node("out"),
			{Group: &Group{Style: Frame, Label: "label", Columns: 3, Items: []Item{node("in"), node("other"), node("third")}}},
		}},
		Edges: []Edge{{From: "out", To: "in", Head: true}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	g := lay.Groups[0]
	n := Width(g.Group.Label) + 2
	crossed := false
	for x := range cellsOf(lay.Edges[0]) {
		if x.Y != g.Rect.Y {
			continue
		}
		crossed = true
		if x.X >= g.LabelAt.X && x.X < g.LabelAt.X+n {
			t.Errorf("the label at %v is over the crossing at %v:\n%s", g.LabelAt, x, strings.Join(lay.Text(), "\n"))
		}
	}
	if !crossed || g.LabelAt.Y != g.Rect.Y || g.LabelAt.X+n > g.Rect.X+g.Rect.W-2 {
		t.Errorf("crossed %v; the label at %v, in a frame at %+v", crossed, g.LabelAt, g.Rect)
	}
}

// labelOf is a label of lines of n "w"s each.
func labelOf(n ...int) string {
	lines := make([]string, len(n))
	for k, w := range n {
		lines[k] = strings.Repeat("w", w)
	}
	return strings.Join(lines, "\n")
}

// A label with no free place left is written over what is there rather
// than lost, but over a line or another label before a box, which hides
// what the box says. These two have no free place for their labels.
func TestCrowdedLabelsKeepOffBoxes(t *testing.T) {
	a, b := "n0", "n1"+strings.Repeat("x", 19)
	d := &Diagram{
		Title: "title",
		Root:  Group{Columns: 2, Items: nodes(a, b)},
		Edges: []Edge{
			{From: b, To: a, Head: true, Label: labelOf(3, 30)},
			{From: a, To: b, Head: true, Label: labelOf(21, 27, 10, 13, 12, 8)},
		},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lay.Edges {
		for i, l := range strings.Split(e.Edge.Label, "\n") {
			for k := range Width(l) {
				for _, n := range lay.Nodes {
					if inside(n.Rect, e.LabelAt.X+k, e.LabelAt.Y+i) {
						t.Fatalf("a label at %v is written over %s:\n%s", e.LabelAt, n.Node.ID, strings.Join(lay.Text(), "\n"))
					}
				}
			}
		}
	}
}
