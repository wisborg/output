package diagram

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// parallelGrid is a grid of nine nodes, three by three, with labels of
// different sizes, and edges of which several run between the same two
// nodes, as the picture sets apart: the case both promises below once
// failed in, and which randomDiagram seldom makes.
func parallelGrid(rnd *rand.Rand) *Diagram {
	labels := []string{"a", "ab", "abc", "wide label", "x\ny", "a\nb\nc"}
	shapes := []Shape{Box, Rounded, Ellipse, Cylinder}
	d := &Diagram{Straight: true, Root: Group{Columns: 3}}
	for i := range 9 {
		d.Root.Items = append(d.Root.Items, Item{Node: &Node{ID: fmt.Sprint("n", i), Label: labels[rnd.Intn(len(labels))], Shape: shapes[rnd.Intn(len(shapes))]}})
	}
	for range 1 + rnd.Intn(3) {
		from, to := rnd.Intn(9), rnd.Intn(9)
		if from == to {
			continue
		}
		for range 2 + rnd.Intn(6) {
			d.Edges = append(d.Edges, Edge{From: fmt.Sprint("n", from), To: fmt.Sprint("n", to)})
		}
	}
	for range rnd.Intn(6) {
		d.Edges = append(d.Edges, Edge{From: fmt.Sprint("n", rnd.Intn(9)), To: fmt.Sprint("n", rnd.Intn(9))})
	}
	return d
}

// A straight line set apart from the others between its nodes is held to
// what any straight line is: clear of every box but its own two, as the
// picture draws it -- checked between the middles alone, a line drawn up to
// three rows aside crossed a third box -- and no path along rows and
// columns runs along it: kept clear of the line between the middles, a
// path ran along one drawn half a row or more beside it, and the picture
// drew the two as one -- 154 times in 3,000 such grids, where a line
// between the middles was run along 4 times, each where no other way was.
// It begins and ends inside its boxes, off their outlines, as the picture
// draws it from where it leaves a box, worked out from a point inside:
// set apart further, it was drawn from a box's corner or from beside it.
// And the picture draws it from its shape's outline, as drawn: an
// ellipse's curve, a cylinder's sides, lid and base. Held inside the
// rectangle alone, a line set apart from an ellipse's middle began in
// the rectangle's corner, beside the curve.
//
// The line is judged as drawn, not as the router records it: between its
// boxes, on the row it runs along, or on both rows where it runs half way
// between two.
func TestSetApartStraightLines(t *testing.T) {
	shifted, curved := 0, 0
	s := ScaleFor(16)
	for seed := range int64(400) {
		d := parallelGrid(rand.New(rand.NewSource(seed)))
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		// The cells of the lines along a row, and of those along a column.
		on := [2]map[Point]bool{{}, {}}
		for _, e := range lay.Edges {
			if !e.Straight {
				continue
			}
			a, _ := lay.Node(e.Edge.From)
			b, _ := lay.Node(e.Edge.To)
			x0, y0, x1, y1 := lay.ends(e)
			if lay.lineCrossesBox(a, b, x0, y0, x1, y1) {
				t.Fatalf("seed %d: %s -> %s, set %v aside, crosses a box", seed, e.Edge.From, e.Edge.To, e.Shift)
			}
			if e.Shift != 0 {
				shifted++
			}
			xs, ys := s.Straight(lay, e)
			for k, n := range []PlacedNode{a, b} {
				x, y := xs[k*(len(xs)-1)], ys[k*(len(ys)-1)]
				if !onDrawnOutline(s, n, x, y) {
					t.Fatalf("seed %d: %s -> %s, set %v aside, is drawn from (%.1f, %.1f), off the outline of %s, %v drawn from %v", seed, e.Edge.From, e.Edge.To, e.Shift, x, y, n.Node.ID, n.Node.Shape, n.Rect)
				}
				if e.Shift != 0 && (n.Node.Shape == Ellipse || n.Node.Shape == Cylinder) {
					curved++
				}
			}
			for _, c := range []struct {
				r    Rect
				x, y float64
			}{{a.Rect, x0, y0}, {b.Rect, x1, y1}} {
				if e.Shift != 0 && (c.x <= float64(c.r.X) || c.x >= float64(c.r.X+c.r.W-1) || c.y <= float64(c.r.Y) || c.y >= float64(c.r.Y+c.r.H-1)) {
					t.Fatalf("seed %d: %s -> %s, set %v aside, ends at (%v, %v), not inside its box %v", seed, e.Edge.From, e.Edge.To, e.Shift, c.x, c.y, c.r)
				}
			}
			switch {
			case y0 == y1:
				lo, hi := a.Rect, b.Rect
				if hi.X < lo.X {
					lo, hi = hi, lo
				}
				for _, y := range rowsOf(y0) {
					for x := lo.X + lo.W; x < hi.X; x++ {
						on[0][Point{x, y}] = true
					}
				}
			case x0 == x1:
				lo, hi := a.Rect, b.Rect
				if hi.Y < lo.Y {
					lo, hi = hi, lo
				}
				for _, x := range rowsOf(x0) {
					for y := lo.Y + lo.H; y < hi.Y; y++ {
						on[1][Point{x, y}] = true
					}
				}
			}
		}
		for _, e := range lay.Edges {
			// Not a path found only where joining another's line was
			// allowed, at joinCost, as no other way was free: a crowded
			// grid of these has such paths, and they may join any line.
			if e.Straight || e.Edge.Invisible || e.joined || e.plain || len(e.Points) == 0 {
				continue
			}
			prev := e.Points[0]
			for k := 1; k < len(e.Points); k++ {
				for p := range cellsAfter(e.Points[k-1], e.Points[k]) {
					// A step along a row on a line along one, or down a
					// column on one along that: crossing lines side by
					// side, a step from one to the next, is no fault.
					along := on[0]
					if p.X == prev.X {
						along = on[1]
					}
					if along[p] && along[prev] {
						t.Fatalf("seed %d: %s -> %s runs along a straight line at %v", seed, e.Edge.From, e.Edge.To, p)
					}
					prev = p
				}
			}
		}
	}
	if shifted == 0 || curved == 0 {
		t.Fatalf("%d straight lines set apart, %d ends of them at an ellipse or a cylinder: the test tests too little", shifted, curved)
	}
}

// onDrawnOutline says (x, y), in the picture's units, is on n's outline
// as the picture draws it: an ellipse's curve; a cylinder's sides between
// its lid's middle and its base's, or the outer half of either; a box's
// four sides, its corners included, as a box's are drawn round, if at
// all, by less than a cell. Worked out from the picture's own
// measures, Box and Lid, and not from the layout's test of a point
// inside a shape, so that it judges that test too.
func onDrawnOutline(s Scale, n PlacedNode, x, y float64) bool {
	const eps = 1e-6
	x0, y0, x1, y1 := s.Box(n.Rect)
	cx, rx := (x0+x1)/2, (x1-x0)/2
	near := func(a, b float64) bool { return math.Abs(a-b) < eps*max(1, math.Abs(b)) }
	onEllipse := func(cy, ry float64) bool { return near(sq((x-cx)/rx)+sq((y-cy)/ry), 1) }
	between := func(v, lo, hi float64) bool { return v > lo-eps && v < hi+eps }
	switch n.Node.Outline() {
	case Ellipse:
		return onEllipse((y0+y1)/2, (y1-y0)/2)
	case Cylinder:
		lid := s.Lid()
		side := (near(x, x0) || near(x, x1)) && between(y, y0+lid, y1-lid)
		return side || onEllipse(y0+lid, lid) && y <= y0+lid+eps || onEllipse(y1-lid, lid) && y >= y1-lid-eps
	}
	return (near(x, x0) || near(x, x1)) && between(y, y0, y1) || (near(y, y0) || near(y, y1)) && between(x, x0, x1)
}

// rowsOf is the rows, or the columns, a line along one at v is drawn on:
// the nearest to v, or both where v is half way between two. A column is
// narrower than a row is tall, so a line set apart half a row is set
// apart a fraction of a column other than a half, and is drawn in the
// nearer column.
func rowsOf(v float64) []int {
	if f := math.Floor(v); v-f == 0.5 {
		return []int{int(f), int(f) + 1}
	}
	return []int{int(math.Floor(v + 0.5))}
}

// cellsAfter yields the cells from a to b along a row or a column, b
// included and a not, so that a path's corner is met once.
func cellsAfter(a, b Point) func(func(Point) bool) {
	return func(yield func(Point) bool) {
		for p := a; p != b; {
			p = step(p, b)
			if !yield(p) {
				return
			}
		}
	}
}

// within is the shape as the picture draws it: from any point it says is
// inside a node, the picture's line, whichever way it runs, leaves from
// the outline as drawn. Of every shape and of sizes from three cells to
// a dozen, at points all over each box, as random layouts seldom set a
// line's end in a cylinder's corner -- held to the rectangle there, 20,000
// random grids made one -- and never yet in one by its base.
func TestWithinIsTheShapeDrawn(t *testing.T) {
	s := ScaleFor(16)
	for _, shape := range []Shape{Box, Rounded, Ellipse, Cylinder, Plain} {
		inside, outside := 0, 0
		for w := 3; w <= 12; w++ {
			for h := 3; h <= 6; h++ {
				n := PlacedNode{Node: &Node{ID: "n", Shape: shape}, Rect: Rect{X: 4, Y: 3, W: w, H: h}}
				for x := float64(n.Rect.X); x <= float64(n.Rect.X+w-1); x += 0.05 {
					for y := float64(n.Rect.Y); y <= float64(n.Rect.Y+h-1); y += 0.05 {
						if !within(n, x, y) {
							// Counted only off the box's outline: inside
							// the box, outside the curve.
							if x > float64(n.Rect.X)+1e-9 && x < float64(n.Rect.X+w-1)-1e-9 && y > float64(n.Rect.Y)+1e-9 && y < float64(n.Rect.Y+h-1)-1e-9 {
								outside++
							}
							continue
						}
						inside++
						// In the picture's units, as Scale.Straight asks.
						px, py := s.Margin+(x+0.5)*s.Cell, s.Margin+(y+0.5)*s.Line
						for k := range 24 {
							a := float64(k) * math.Pi / 12
							ex, ey := s.leave(n, px, py, px+1000*math.Cos(a), py+1000*math.Sin(a))
							if !onDrawnOutline(s, n, ex, ey) {
								t.Fatalf("%v %dx%d: a line from (%.2f, %.2f), within it, at %d degrees leaves from (%.1f, %.1f), off its outline", shape, w, h, x, y, k*15, ex, ey)
							}
						}
					}
				}
			}
		}
		if inside == 0 || (shape == Ellipse || shape == Cylinder) && outside == 0 {
			t.Fatalf("%v: %d points within, %d inside the box but not within: the test tests too little", shape, inside, outside)
		}
	}
}
