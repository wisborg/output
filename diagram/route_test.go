package diagram

import (
	"strings"
	"testing"
)

// cellsOf is each cell of e's path, with the directions the path leaves
// it in.
func cellsOf(e PlacedEdge) map[Point]uint8 {
	cells := map[Point]uint8{}
	for k := 1; k < len(e.Points); k++ {
		a, b := e.Points[k-1], e.Points[k]
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
			if p != a {
				cells[p] |= dir(-dx, -dy)
			}
			if p != b {
				cells[p] |= dir(dx, dy)
			}
			if p == b {
				break
			}
		}
	}
	return cells
}

// Two edges that share neither their head nor their tail never meet but
// where one crosses the other at a right angle: no line taken over half
// way, no two drawn as one with a head at each end, no turn where another
// runs, each of which reads as a connection that is not there. A service
// mesh, every box with several edges, is where they used to.
func TestEdgesNeverJoin(t *testing.T) {
	ellipse := func(id string) Item { return Item{Node: &Node{ID: id, Shape: Ellipse}} }
	d := &Diagram{
		Root: Group{Columns: 3, Items: []Item{
			ellipse("api"), {Node: &Node{ID: "cache", Shape: Cylinder}}, {Node: &Node{ID: "db", Shape: Cylinder}},
			ellipse("worker"), node("queue"), ellipse("mail"),
		}},
		Edges: []Edge{
			{From: "api", To: "cache", Head: true}, {From: "cache", To: "api", Head: true},
			{From: "api", To: "db", Head: true}, {From: "api", To: "queue", Head: true},
			{From: "queue", To: "worker", Head: true}, {From: "worker", To: "db", Head: true},
			{From: "worker", To: "mail", Head: true}, {From: "db", To: "cache", Head: true, Line: Dashed},
		},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for i, a := range lay.Edges {
		for _, b := range lay.Edges[i+1:] {
			if (a.Edge.From == b.Edge.From) != (a.Edge.To == b.Edge.To) && a.Edge.Line == b.Edge.Line {
				continue // these may share a trunk or a stem
			}
			cb := cellsOf(b)
			for p, bitsA := range cellsOf(a) {
				bitsB, both := cb[p]
				if !both {
					continue
				}
				crossing := bitsA == up|down && bitsB == left|right || bitsA == left|right && bitsB == up|down
				if !crossing {
					t.Errorf("%s to %s and %s to %s meet at %v, not crossing:\n%s", a.Edge.From, a.Edge.To, b.Edge.From, b.Edge.To, p, text)
				}
			}
		}
	}
	// And none passes through a box but its own two.
	for _, e := range lay.Edges {
		for p := range cellsOf(e) {
			for _, n := range lay.Nodes {
				r := n.Rect
				inside := p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
				if inside && n.Node.ID != e.Edge.From && n.Node.ID != e.Edge.To {
					t.Errorf("%s to %s passes through %s at %v:\n%s", e.Edge.From, e.Edge.To, n.Node.ID, p, text)
				}
			}
		}
	}
}
