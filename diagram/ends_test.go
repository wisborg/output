package diagram

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// A node's ends and joints, kept relative to it when it was placed, are
// where they would be worked out now, after the layout has trimmed its
// margins and made room for a picture's labels, moving every node: kept
// in the page's cells, they would be a margin out.
func TestEndsFollowTheNode(t *testing.T) {
	for seed := range int64(300) {
		lay, err := randomDiagram(rand.New(rand.NewSource(seed))).Layout()
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range lay.Nodes {
			if !n.settled {
				t.Fatalf("seed %d: %s was never settled", seed, n.Node.ID)
			}
			if got, want := n.ends(), n.workEnds(); !reflect.DeepEqual(got, want) {
				t.Fatalf("seed %d: %s's ends %v, worked out now %v", seed, n.Node.ID, got, want)
			}
			if got, want := n.joints(), n.workJoints(); !reflect.DeepEqual(got, want) {
				t.Fatalf("seed %d: %s's joints %v, worked out now %v", seed, n.Node.ID, got, want)
			}
			fresh := n
			fresh.settled = false
			if got, want := n.outsideJoints(), fresh.outsideJoints(); !reflect.DeepEqual(got, want) {
				t.Fatalf("seed %d: %s's cells out from its joints %v, worked out now %v", seed, n.Node.ID, got, want)
			}
			if pn := lay.nodeAt[n.Node.ID]; !reflect.DeepEqual(pn.ends(), n.ends()) {
				t.Fatalf("seed %d: %s's ends differ between Nodes and nodeAt", seed, n.Node.ID)
			}
			// However often the layout moved it, nodeAt's copy is where
			// Nodes has it, its fields and dividers with it.
			if err := sameAndWhole(n, lay.nodeAt[n.Node.ID]); err != "" {
				t.Fatalf("seed %d: %s", seed, err)
			}
		}
	}
}

// outlineBefore is outline as it was before it asked the nodes' ends,
// kept as the rule they are held to: the router's own map of every
// node's joints, and of the plain-text nodes, whose corners a line may end
// at, asked cell by cell along r's outline.
func outlineBefore(rt *router, joints map[Point]bool, bare bool, r Rect, i int, strict bool, sides Side, field Rect) (cells []Point, out, costs []int) {
	var asked []bool
	add := func(p Point, way int) {
		if !rt.in(p.X, p.Y) || joints[p] {
			return
		}
		cost, ok := 0, true
		if sides != 0 && sides&wayside[way] == 0 || field != (Rect{}) && !besideField(r, field, p, way) {
			if !rt.anySide {
				return
			}
			cost, ok = missCost, false
		}
		if !rt.mayEnd(i, rt.heads[p.Y][p.X], rt.tails[p.Y][p.X]) || rt.foreign(i, p.X, p.Y) != 0 {
			if strict {
				return
			}
			cost += joinCost
		}
		for k, c := range cells {
			if c == p {
				if out[k] != way {
					cells, out, costs, asked = append(cells, p), append(out, way), append(costs, cost), append(asked, ok)
				}
				return
			}
		}
		cells, out, costs, asked = append(cells, p), append(out, way), append(costs, cost), append(asked, ok)
	}
	corner := 1
	if bare {
		corner = 0
	}
	for x := r.X + corner; x < r.X+r.W-corner; x++ {
		add(Point{x, r.Y}, 0)
		add(Point{x, r.Y + r.H - 1}, 1)
	}
	for y := r.Y + corner; y < r.Y+r.H-corner; y++ {
		add(Point{r.X, y}, 2)
		add(Point{r.X + r.W - 1, y}, 3)
	}
	if len(joints) == 0 {
		return cells, out, costs
	}
	group := func(k int) int {
		if asked[k] {
			return out[k] + 4
		}
		return out[k]
	}
	beside := make([]bool, len(cells))
	var clear [8]bool
	for k, p := range cells {
		beside[k] = joints[Point{p.X - 1, p.Y}] || joints[Point{p.X + 1, p.Y}] || joints[Point{p.X, p.Y - 1}] || joints[Point{p.X, p.Y + 1}]
		if !beside[k] {
			clear[group(k)] = true
		}
	}
	for k := range cells {
		if beside[k] && clear[group(k)] {
			costs[k] += jointCost
		}
	}
	return cells, out, costs
}

// outline, reading the ends a node keeps, offers the search the cells it
// offered before it did, by the rule outlineBefore keeps: each cell, each
// way out, each cost, and in the same order, which decides between two
// paths that cost the same. Asked on a page where every other edge's line
// is drawn, so that cells taken by other lines are left out, or charged,
// as outline does it, before the joints are priced over what is left.
func TestEndsMatchOutline(t *testing.T) {
	compared := 0
	for seed := range int64(400) {
		d := randomDiagram(rand.New(rand.NewSource(seed)))
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		l := &layouter{d: d, out: lay, rect: map[string]Rect{}}
		joints := map[Point]bool{}
		for _, n := range lay.Nodes {
			l.rect[n.Node.ID] = n.Rect
			for _, j := range n.joints() {
				joints[j] = true
			}
		}
		for i, e := range d.Edges {
			if e.Invisible {
				continue
			}
			rt := newRouter(l, edgeFields(l))
			for k, o := range lay.Edges {
				if k != i && len(o.Points) > 1 {
					rt.draw(o.Points, k)
				}
			}
			for _, end := range []struct {
				node  string
				sides Side
				field Rect
			}{{e.From, e.FromSide, rt.fields[i][0]}, {e.To, e.ToSide, rt.fields[i][1]}} {
				n := lay.nodeAt[end.node]
				for _, strict := range []bool{true, false} {
					for _, any := range []bool{false, true} {
						rt.anySide = any
						wc, wo, wk := outlineBefore(rt, joints, n.Node.Outline() == Plain, n.Rect, i, strict, end.sides, end.field)
						gc, gout, gk := rt.outline(n, i, strict, end.sides, end.field)
						if !reflect.DeepEqual(wc, gc) || !reflect.DeepEqual(wo, gout) || !reflect.DeepEqual(wk, gk) {
							t.Fatalf("seed %d, edge %d at %s, strict %v, any side %v:\nbefore %v %v %v\noutline %v %v %v",
								seed, i, end.node, strict, any, wc, wo, wk, gc, gout, gk)
						}
						compared++
					}
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("nothing compared")
	}
	t.Log(fmt.Sprint(compared, " outlines compared"))
}

// A braille stub chooses among the node's ends, as a path does: off a
// joint, and off the cells beside one where its side has a cell clear of
// both -- above a record of two fields alike, the line reaching it over
// the divider, the nearest cells are the joint and its neighbours -- and,
// into plain text one row tall, by its side, where it once had no cell
// off the corners and drew a small arrow at an angle.
func TestStubsChooseAmongTheEnds(t *testing.T) {
	r := &PlacedNode{Node: &Node{ID: "r", Record: []Field{{Label: "ab"}, {Label: "cd"}}}, Rect: Rect{10, 10, 11, 3}}
	placeFields(r, r.Node.Record, Rect{11, 11, 9, 1}, true)
	if js := r.joints(); len(js) != 2 || js[0] != (Point{15, 10}) {
		t.Fatalf("joints %v: the test no longer tests anything", js)
	}
	free := func(Point) bool { return true }
	at, arrow, ok := stub(*r, Point{15, 5}, 0, 1, free)
	if !ok || arrow != '▼' || at.Y != 9 || abs(at.X-15) <= 1 {
		t.Errorf("a stub into r from above its divider: %v %c %v, want one off the joint and its neighbours", at, arrow, ok)
	}
	// Only the joint's neighbours free: the stub takes one, as a line
	// into a field two cells wide does.
	at, _, ok = stub(*r, Point{15, 5}, 0, 1, func(p Point) bool { return abs(p.X-15) == 1 })
	if !ok || abs(at.X-15) != 1 {
		t.Errorf("only beside the joint free: %v %v, want a stub there", at, ok)
	}
	// A steep line from above and to the left ends by the top, the side it
	// runs more towards, even where the top's nearest cells are taken and a
	// cell on the left would be nearer.
	box := PlacedNode{Node: &Node{ID: "b"}, Rect: Rect{10, 10, 11, 5}}
	at, arrow, ok = stub(box, Point{8, 9}, 0.2, 1, func(p Point) bool { return p.Y != 9 || p.X >= 15 })
	if !ok || arrow != '▼' || at != (Point{15, 9}) {
		t.Errorf("a steep line from above and to the left: %v %c %v, want the top's first free cell, 15,9", at, arrow, ok)
	}
	label := PlacedNode{Node: &Node{ID: "a", Shape: Plain, Label: "a"}, Rect: Rect{30, 10, 1, 1}}
	for _, c := range []struct {
		near   Point
		dx, dy float64
		want   Point
		arrow  rune
	}{
		{Point{25, 10}, 1, 0.2, Point{29, 10}, '▶'},
		{Point{35, 10}, -1, 0.2, Point{31, 10}, '◀'},
		{Point{30, 5}, 0.1, 1, Point{30, 9}, '▼'},
	} {
		if at, arrow, ok := stub(label, c.near, c.dx, c.dy, free); !ok || at != c.want || arrow != c.arrow {
			t.Errorf("into a one-letter label from %v: %v %c %v, want %v %c", c.near, at, arrow, ok, c.want, c.arrow)
		}
	}
}

// Drawn in braille, straight lines into labels of one letter end in a stub
// and an arrowhead, as into any box, in every direction: no small arrow at
// an angle, hard to read, is left.
func TestBrailleStubsIntoOneLetterLabels(t *testing.T) {
	for _, dir := range []Direction{Down, Right, Up, Left} {
		d := &Diagram{Straight: true, Direction: dir, Root: Group{Items: []Item{{Node: &Node{ID: "source", Shape: Plain}}}}}
		for _, id := range []string{"a", "b", "c"} {
			d.Root.Items = append(d.Root.Items, Item{Node: &Node{ID: id, Shape: Plain}})
			d.Edges = append(d.Edges, Edge{From: "source", To: id, Head: true})
		}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(lay.Text(TextOptions{Braille: BrailleThin}), "\n")
		if strings.ContainsAny(text, "↗↘↙↖") || strings.Count(text, "▲")+strings.Count(text, "▼")+strings.Count(text, "◀")+strings.Count(text, "▶") != 3 {
			t.Errorf("%v: a small arrow at an angle, or not an arrowhead each:\n%s", dir, text)
		}
	}
}

// sameAndWhole says what is wrong, if anything, with copy, nodeAt's copy
// of the node n in Nodes: where it is not where n is, or its fields and
// dividers are not inside its own outline. The two share their fields
// and dividers, so it is the copy's Rect that a move left out of step
// shows, and against which its fields would lie a move off.
func sameAndWhole(n, copy PlacedNode) string {
	if copy.Rect != n.Rect {
		return fmt.Sprintf("%s is at %v in Nodes and %v in nodeAt", n.Node.ID, n.Rect, copy.Rect)
	}
	r := copy.Rect
	for _, f := range copy.Fields {
		if f := f.Rect; f.X <= r.X || f.Y <= r.Y || f.X+f.W >= r.X+r.W || f.Y+f.H >= r.Y+r.H {
			return fmt.Sprintf("%s's field at %v is not inside its outline %v", n.Node.ID, f, r)
		}
	}
	for _, d := range copy.Dividers {
		for _, p := range d {
			if p.X < r.X || p.Y < r.Y || p.X >= r.X+r.W || p.Y >= r.Y+r.H {
				return fmt.Sprintf("%s's divider %v is not on or inside its outline %v", n.Node.ID, d, r)
			}
		}
	}
	return ""
}

// A layout that grows at the top for a label -- one too wide for the gap
// between two neighbours in a grid goes above the top row -- moves every
// node a second time, after trimming its margins, and nodeAt's copy of
// each is still where Nodes has it, with its fields and dividers. That it
// grew is held by the same diagram without the label, whose boxes sit
// higher by the rows grown.
func TestNodeAtFollowsAShift(t *testing.T) {
	diagram := func(label string) *Diagram {
		r := &Node{ID: "r", Record: []Field{{Label: "x", Port: "x"}, {Label: "y"}}}
		d := &Diagram{Root: Group{Columns: 2, Items: []Item{{Node: &Node{ID: "a"}}, {Node: r}}}}
		d.Edges = []Edge{{From: "a", To: "r", ToPort: "x", Head: true, Label: label}}
		return d
	}
	lay, err := diagram("a label far too wide for the gap").Layout()
	if err != nil {
		t.Fatal(err)
	}
	bare, err := diagram("").Layout()
	if err != nil {
		t.Fatal(err)
	}
	if lay.Nodes[0].Rect.Y <= bare.Nodes[0].Rect.Y {
		t.Fatalf("the boxes are at row %d with the label and %d without: the layout did not grow at the top", lay.Nodes[0].Rect.Y, bare.Nodes[0].Rect.Y)
	}
	for _, n := range lay.Nodes {
		if err := sameAndWhole(n, lay.nodeAt[n.Node.ID]); err != "" {
			t.Error(err)
		}
	}
	if len(lay.Nodes[1].Fields) == 0 || len(lay.Nodes[1].Dividers) == 0 {
		t.Errorf("r has no fields or dividers to hold: %+v", lay.Nodes[1])
	}
}
