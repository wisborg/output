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

// outlineByEnds is what outline will be once it reads the nodes' ends: the
// edge's choices on the page, on the sides and beside the field asked for
// unless anySide, and clear of others' lines and ends unless not strict,
// then priced for joints over those kept.
func outlineByEnds(rt *router, n PlacedNode, i int, strict bool, sides Side, field Rect) (cells []Point, out, costs []int) {
	var kept []endChoice
	for _, c := range n.choices(sides, field) {
		if !rt.in(c.at.X, c.at.Y) {
			continue
		}
		if !c.asked && !rt.anySide {
			continue
		}
		if !rt.mayEnd(i, rt.heads[c.at.Y][c.at.X], rt.tails[c.at.Y][c.at.X]) || rt.foreign(i, c.at.X, c.at.Y) != 0 {
			if strict {
				continue
			}
			c.cost += joinCost
		}
		kept = append(kept, c)
	}
	priceJoints(kept)
	for _, c := range kept {
		cells, out, costs = append(cells, c.at), append(out, c.way), append(costs, c.cost)
	}
	return cells, out, costs
}

// The ends a node keeps, chosen from as an edge asks and priced for its
// joints, are the cells outline offers the search today: each cell, each
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
		for _, n := range lay.Nodes {
			l.rect[n.Node.ID] = n.Rect
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
						wc, wo, wk := rt.outline(n.Rect, i, strict, end.sides, end.field)
						gc, gout, gk := outlineByEnds(rt, n, i, strict, end.sides, end.field)
						if !reflect.DeepEqual(wc, gc) || !reflect.DeepEqual(wo, gout) || !reflect.DeepEqual(wk, gk) {
							t.Fatalf("seed %d, edge %d at %s, strict %v, any side %v:\noutline %v %v %v\nby ends %v %v %v",
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
