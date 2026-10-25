package diagram

import (
	"fmt"
	"math/rand"
	"reflect"
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
			if pn := lay.nodeAt[n.Node.ID]; !reflect.DeepEqual(pn.ends(), n.ends()) {
				t.Fatalf("seed %d: %s's ends differ between Nodes and nodeAt", seed, n.Node.ID)
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
			rt := newRouter(l)
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
