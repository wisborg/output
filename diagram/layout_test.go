package diagram

import (
	"strings"
	"testing"
)

func node(id string) Item { return Item{Node: &Node{ID: id}} }

func rects(t *testing.T, d *Diagram) map[string]Rect {
	t.Helper()
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Rect{}
	for _, n := range lay.Nodes {
		out[n.Node.ID] = n.Rect
	}
	return out
}

func mid(r Rect) (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

// apart fails the test where two boxes overlap or touch: whatever a layout
// does, every box is clear of every other.
func apart(t *testing.T, r map[string]Rect) {
	t.Helper()
	for a, ra := range r {
		for b, rb := range r {
			if a < b && ra.X <= rb.X+rb.W && rb.X <= ra.X+ra.W && ra.Y <= rb.Y+rb.H && rb.Y <= ra.Y+ra.H {
				t.Errorf("%s at %v and %s at %v overlap", a, ra, b, rb)
			}
		}
	}
}

// A grid fills its cells in the order its items are written, row by row,
// and a pinned item takes its own cell, the rest flowing round it.
func TestGridPlacesInOrder(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 2, Items: []Item{node("a"), node("b"), node("c"), {Node: &Node{ID: "p", Row: 1, Col: 1}}}}}
	r := rects(t, d)
	apart(t, r)
	// p has row 1, column 1; a, b and c flow into the cells left: row 1
	// column 2, then row 2.
	px, py := mid(r["p"])
	ax, ay := mid(r["a"])
	bx, by := mid(r["b"])
	cx, cy := mid(r["c"])
	if !(px < ax && py == ay) {
		t.Errorf("a at (%d,%d) is not right of p at (%d,%d)", ax, ay, px, py)
	}
	if !(by > py && bx == px) || !(cy == by && cx == ax) {
		t.Errorf("b at (%d,%d) and c at (%d,%d) are not the second row", bx, by, cx, cy)
	}
}

// Two items pinned to one cell are refused, naming both.
func TestGridRefusesAClash(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 2, Items: []Item{{Node: &Node{ID: "a", Col: 2}}, {Node: &Node{ID: "b", Row: 1, Col: 2}}}}}
	_, err := d.Layout()
	if err == nil || !strings.Contains(err.Error(), `"a" and "b"`) {
		t.Errorf("%v, want an error naming a and b", err)
	}
}

// The point of a grid: every box is where the diagram puts it, whatever
// the edges. A replica that stops replicating moves no box.
func TestGridIgnoresEdges(t *testing.T) {
	build := func(edges []Edge) *Diagram {
		return &Diagram{
			Root: Group{Rows: 2, Items: []Item{
				{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 3, Items: []Item{node("db1"), node("db2"), node("db3")}}},
				{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 3, Items: []Item{node("dr1")}}},
			}},
			Edges: edges,
		}
	}
	all := rects(t, build([]Edge{{From: "db1", To: "db2"}, {From: "db1", To: "db3"}, {From: "db1", To: "dr1"}}))
	broken := rects(t, build([]Edge{{From: "db1", To: "db2"}, {From: "db3", To: "dr1"}}))
	none := rects(t, build(nil))
	for id, r := range all {
		if broken[id] != r || none[id] != r {
			t.Errorf("%s at %v with every link, %v with one broken, %v with none", id, r, broken[id], none[id])
		}
	}
}

// Grids stacked in a grid share their columns: what is in the first column
// of one region is above what is in the first column of the next, though
// the boxes differ in width.
func TestStackedGridsShareColumns(t *testing.T) {
	d := &Diagram{Root: Group{Rows: 2, Items: []Item{
		{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 2, Items: []Item{
			{Node: &Node{ID: "dbA", Label: "dbA primary, wide"}}, node("dbB")}}},
		{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 2, Items: []Item{
			node("dbC"), {Node: &Node{ID: "dbD", Col: 2}}}}},
	}}}
	r := rects(t, d)
	apart(t, r)
	ax, _ := mid(r["dbA"])
	bx, _ := mid(r["dbB"])
	cx, _ := mid(r["dbC"])
	dx, _ := mid(r["dbD"])
	if cx != ax || dx != bx {
		t.Errorf("dbC's middle at %d under dbA's at %d, dbD's at %d under dbB's at %d: the columns are not shared", cx, ax, dx, bx)
	}
}

// Without a grid, an item goes a layer below those its edges come from,
// two branches of a step sit either side of it, and the step they meet at
// sits under the middle of them.
func TestLayersFollowEdges(t *testing.T) {
	d := &Diagram{
		Root: Group{Items: []Item{node("start"), node("check"), node("yes"), node("no"), node("end")}},
		Edges: []Edge{
			{From: "start", To: "check"}, {From: "check", To: "yes"}, {From: "check", To: "no"},
			{From: "yes", To: "end"}, {From: "no", To: "end"},
		},
	}
	r := rects(t, d)
	apart(t, r)
	sx, sy := mid(r["start"])
	cx, cy := mid(r["check"])
	yx, yy := mid(r["yes"])
	nx, ny := mid(r["no"])
	ex, ey := mid(r["end"])
	if !(sy < cy && cy < yy && yy == ny && ny < ey) {
		t.Errorf("layers at rows %d, %d, %d/%d, %d: not one below the other", sy, cy, yy, ny, ey)
	}
	if yx >= nx || abs(cx-(yx+nx)/2) > 1 || abs(ex-cx) > 1 || abs(sx-cx) > 1 {
		t.Errorf("start at %d, check at %d, the branches at %d and %d, end at %d: the flow is not centred on its step", sx, cx, yx, nx, ex)
	}
}

// A cycle still lays out: its first item, in the order written, is at the
// top.
func TestLayersBreakACycle(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a"), node("b"), node("c")}},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}}}
	r := rects(t, d)
	apart(t, r)
	if !(r["a"].Y < r["b"].Y && r["b"].Y < r["c"].Y) {
		t.Errorf("a at row %d, b %d, c %d: want a, then b, then c", r["a"].Y, r["b"].Y, r["c"].Y)
	}
}

// A line from the first box of a row to the third goes round the second
// rather than through it, where it would read as a line from the second.
func TestEdgeGoesRoundABox(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 3, Items: []Item{node("a"), node("b"), node("c")}},
		Edges: []Edge{{From: "a", To: "c", Head: true}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := lay.Node("b")
	pts := lay.Edges[0].Points
	for i := 1; i < len(pts); i++ {
		p, q := pts[i-1], pts[i]
		if min(p.X, q.X) <= b.Rect.X+b.Rect.W-1 && max(p.X, q.X) >= b.Rect.X &&
			min(p.Y, q.Y) <= b.Rect.Y+b.Rect.H-1 && max(p.Y, q.Y) >= b.Rect.Y {
			t.Fatalf("the edge from a to c, %v, passes through b at %v", pts, b.Rect)
		}
	}
}

// Every segment of every edge runs along a row or a column, from an
// outline cell of its tail to one of its head.
func TestEdgesRunAlongRowsAndColumns(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a"), node("b"), node("c"), node("d")}},
		Edges: []Edge{{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "c", To: "d"}, {From: "b", To: "d"}, {From: "d", To: "d"}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	onOutline := func(p Point, r Rect) bool {
		in := p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
		edge := p.X == r.X || p.X == r.X+r.W-1 || p.Y == r.Y || p.Y == r.Y+r.H-1
		return in && edge
	}
	for _, e := range lay.Edges {
		pts := e.Points
		for i := 1; i < len(pts); i++ {
			if pts[i].X != pts[i-1].X && pts[i].Y != pts[i-1].Y {
				t.Errorf("%s to %s: a diagonal segment %v to %v", e.Edge.From, e.Edge.To, pts[i-1], pts[i])
			}
		}
		from, _ := lay.Node(e.Edge.From)
		to, _ := lay.Node(e.Edge.To)
		if !onOutline(pts[0], from.Rect) || !onOutline(pts[len(pts)-1], to.Rect) {
			t.Errorf("%s to %s: %v does not run from outline to outline", e.Edge.From, e.Edge.To, pts)
		}
	}
}

func TestLayoutRefuses(t *testing.T) {
	for _, c := range []struct {
		d    *Diagram
		want string
	}{
		{&Diagram{Root: Group{Items: []Item{node("a")}}, Edges: []Edge{{From: "a", To: "x"}}}, `"x", which is no node`},
		{&Diagram{Root: Group{Items: []Item{node("a"), {Group: &Group{ID: "g", Items: []Item{node("a")}}}}}}, `"a" is in the diagram twice`},
	} {
		if _, err := c.d.Layout(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v, want an error saying %s", err, c.want)
		}
	}
}

// A label is not written over a line: the label of an edge crossing a
// lane's line, which would be in the middle of its edge, is moved beside
// it, clear of the line.
func TestLabelsKeepOffLines(t *testing.T) {
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Label: "east", Columns: 1, Items: []Item{node("a")}}},
			{Group: &Group{ID: "west", Style: Lane, Label: "west", Columns: 1, Items: []Item{node("b")}}},
		}},
		Edges: []Edge{{From: "a", To: "b", Head: true, Label: "async"}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := lay.Text()
	at := lay.Edges[0].LabelAt
	if got := string([]rune(text[at.Y])[at.X : at.X+5]); got != "async" {
		t.Errorf("the label is not at %v, where Layout put it:\n%s", at, strings.Join(text, "\n"))
	}
	for _, g := range lay.Groups {
		for _, div := range g.Dividers {
			if div[0].Y == at.Y {
				t.Errorf("the label is on the lane's line, row %d:\n%s", at.Y, strings.Join(text, "\n"))
			}
		}
	}
}
